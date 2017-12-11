package executor

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/kr/pty"
	"github.com/myesui/uuid"
	"github.com/screwdriver-cd/launcher/screwdriver"
)

const (
	// ExitLaunch is the exit code when a step fails to launch
	ExitLaunch = 255
	// ExitUnknown is the exit code when a step doesn't return an exit code (for some weird reason)
	ExitUnknown = 254
	// ExitOk is the exit code when a step runs successfully
	ExitOk = 0
)

// ErrStatus is an error that holds an exit status code
type ErrStatus struct {
	Status int
}

func (e ErrStatus) Error() string {
	return fmt.Sprintf("exit %d", e.Status)
}

// Create a sh file
func createShFile(path string, cmd screwdriver.CommandDef) error {
	defaultStart := "#!/bin/sh -e"
	return ioutil.WriteFile(path, []byte(defaultStart+"\n"+cmd.Cmd), 0755)
}

// Returns a single line (without the ending \n) from the input buffered reader
// Pulled from https://stackoverflow.com/a/12206365
func readln(r *bufio.Reader) (string, error) {
	var (
		isPrefix = true
		err      error
		line, ln []byte
	)

	for isPrefix && err == nil {
		line, isPrefix, err = r.ReadLine()
		ln = append(ln, line...)
	}

	return string(ln), err
}

// Copy lines until match string
func copyLinesUntil(r io.Reader, w io.Writer, match string) (int, error) {
	var (
		err    error
		t      string
		reader = bufio.NewReader(r)
		// Match the guid and exitCode
		reExit = regexp.MustCompile(fmt.Sprintf("(%s) ([0-9]+)", match))
		// Match the export SD_STEP_ID command
		reExport = regexp.MustCompile("export SD_STEP_ID=(" + match + ")")
	)
	t, err = readln(reader)
	for err == nil {
		parts := reExit.FindStringSubmatch(t)
		if len(parts) != 0 {
			exitCode, rerr := strconv.Atoi(parts[2])
			if rerr != nil {
				return ExitUnknown, fmt.Errorf("Error converting the exit code to int: %v", rerr)
			}
			if exitCode != 0 {
				return exitCode, fmt.Errorf("Launching command exit with code: %v", exitCode)
			}
			return ExitOk, nil
		}
		// Filter out the export command from the output
		exportCmd := reExport.FindStringSubmatch(t)
		if len(exportCmd) == 0 {
			_, werr := fmt.Fprintln(w, t)
			if werr != nil {
				return ExitUnknown, fmt.Errorf("Error piping logs to emitter: %v", werr)
			}
		}

		t, err = readln(reader)
	}
	if err != nil {
		return ExitUnknown, fmt.Errorf("Error with reader: %v", err)
	}
	return ExitOk, nil
}

func doRunCommand(guid, path string, emitter screwdriver.Emitter, f *os.File, fReader io.Reader) (int, error) {
	executionCommand := []string{
		"export SD_STEP_ID=" + guid,
		";. " + path,
		";echo",
		";echo " + guid + " $?\n",
	}
	shargs := strings.Join(executionCommand, " ")

	f.Write([]byte(shargs))

	return copyLinesUntil(fReader, emitter, guid)
}

// Executes teardown commands
func doRunTeardownCommand(cmd screwdriver.CommandDef, emitter screwdriver.Emitter, env []string, path string) (int, error) {
	shargs := []string{"-e", "-c"}
	shargs = append(shargs, cmd.Cmd)
	c := exec.Command("sh", shargs...)

	emitter.StartCmd(cmd)
	fmt.Fprintf(emitter, "$ %s\n", cmd.Cmd)
	c.Stdout = emitter
	c.Stderr = emitter

	c.Dir = path
	c.Env = append(env, c.Env...)

	if err := c.Start(); err != nil {
		return ExitLaunch, fmt.Errorf("Launching command %q: %v", cmd.Cmd, err)
	}

	if err := c.Wait(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus := exitError.Sys().(syscall.WaitStatus)

			return waitStatus.ExitStatus(), ErrStatus{waitStatus.ExitStatus()}
		}
		return ExitUnknown, fmt.Errorf("Running command %q: %v", cmd.Cmd, err)
	}

	return ExitOk, nil
}

// Run executes a slice of CommandDefs
func Run(path string, env []string, emitter screwdriver.Emitter, build screwdriver.Build, api screwdriver.API, buildID int) error {
	// Set up a single pseudo-terminal
	c := exec.Command("sh")
	c.Dir = path
	c.Env = append(env, c.Env...)

	f, err := pty.Start(c)
	if err != nil {
		return fmt.Errorf("Cannot start shell: %v", err)
	}

	// Run setup commands
	setupCommands := []string{
		"set -e",
		"PATH=$PATH:/opt/sd",
		"finish() { echo $SD_STEP_ID $?; }",
		"trap finish EXIT;\n",
	}
	shargs := strings.Join(setupCommands, " && ")

	f.Write([]byte(shargs))

	teardownFlag := false
	var firstError error
	var code int
	var cmdErr error

	cmds := build.Commands

	for _, cmd := range cmds {
		isTeardown, _ := regexp.MatchString("sd-teardown-*", cmd.Name)

		// Start set up & user steps if previous steps succeed
		if (!isTeardown) && (firstError == nil) {
			if err := api.UpdateStepStart(buildID, cmd.Name); err != nil {
				return fmt.Errorf("Updating step start %q: %v", cmd.Name, err)
			}

			// Create step script file
			stepFilePath := "/tmp/step.sh"
			if err := createShFile(stepFilePath, cmd); err != nil {
				return fmt.Errorf("Writing to step script file: %v", err)
			}

			// Generate guid for the step
			guid := uuid.NewV4().String()

			// Set current running step in emitter
			emitter.StartCmd(cmd)
			fmt.Fprintf(emitter, "$ %s\n", cmd.Cmd)

			fReader := bufio.NewReader(f)

			code, cmdErr = doRunCommand(guid, stepFilePath, emitter, f, fReader)
		} else if isTeardown {
			// Kill shell if first time switch to the teardown step
			if !teardownFlag {
				f.Write([]byte{4}) // EOT
				teardownFlag = true
			}

			if err := api.UpdateStepStart(buildID, cmd.Name); err != nil {
				return fmt.Errorf("Updating step start %q: %v", cmd.Name, err)
			}

			// Run teardown commands
			code, cmdErr = doRunTeardownCommand(cmd, emitter, env, path)
		}

		if err := api.UpdateStepStop(buildID, cmd.Name, code); err != nil {
			return fmt.Errorf("Updating step stop %q: %v", cmd.Name, err)
		}

		// Set first error flag
		if (firstError == nil) && (cmdErr != nil) {
			firstError = cmdErr
		}
	}

	// Return the first error occured
	if firstError != nil {
		return firstError
	}

	return nil
}
