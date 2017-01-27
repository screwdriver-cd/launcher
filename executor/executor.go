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

	"github.com/kr/pty"
	"github.com/myesui/uuid"
	"github.com/screwdriver-cd/launcher/screwdriver"
)

const (
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

// Copy lines until match string
func copyLinesUntil(r io.Reader, w io.Writer, match string) (int, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		t := scanner.Text()
		// Match the guid and exitCode
		regex := fmt.Sprintf("(%s) ([0-9]+)", match)
		re := regexp.MustCompile(regex)
		parts := re.FindStringSubmatch(t)
		if len(parts) != 0 {
			exitCode, err := strconv.Atoi(parts[2])
			if err != nil {
				return ExitUnknown, fmt.Errorf("Error converting the exit code to int: %v", err)
			}
			if exitCode != 0 {
				return exitCode, fmt.Errorf("Launching command exit with code: %v", exitCode)
			}
			return ExitOk, nil
		}
		// Match the export SD_STEP_ID command and if not match print it to emitter
		reExport := regexp.MustCompile("export SD_STEP_ID=(" + match + ")")
		exportCmd := reExport.FindStringSubmatch(t)
		if len(exportCmd) == 0 {
			fmt.Fprintln(w, t)
		}
	}
	if err := scanner.Err(); err != nil {
		return ExitUnknown, fmt.Errorf("Error with scanner: %v", err)
	}
	return ExitOk, nil
}

// Source script file from the path
func doRunCommand(guid, path string, emitter screwdriver.Emitter, f *os.File, fReader io.Reader) (int, error) {
	executionCommand := []string{
		"export SD_STEP_ID=" + guid,
		";. " + path,
		";echo " + guid + " $?\n",
	}
	shargs := strings.Join(executionCommand, " ")

	f.Write([]byte(shargs))

	return copyLinesUntil(fReader, emitter, guid)
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
		"finish() { echo $SD_STEP_ID $?; }",
		"trap finish EXIT;\n",
	}
	shargs := strings.Join(setupCommands, " && ")

	f.Write([]byte(shargs))

	cmds := build.Commands

	for _, cmd := range cmds {
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

		// Execute command
		code, cmdErr := doRunCommand(guid, stepFilePath, emitter, f, fReader)
		if err := api.UpdateStepStop(buildID, cmd.Name, code); err != nil {
			return fmt.Errorf("Updating step stop %q: %v", cmd.Name, err)
		}

		if cmdErr != nil {
			return cmdErr
		}
	}

	f.Write([]byte{4}) // EOT

	return nil
}
