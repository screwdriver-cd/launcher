package executor

import (
	"fmt"
	"os/exec"
	"syscall"

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

var execCommand = exec.Command

// ErrStatus is an error that holds an exit status code
type ErrStatus struct {
	Status int
}

func (e ErrStatus) Error() string {
	return fmt.Sprintf("exit %d", e.Status)
}

func doRun(cmd screwdriver.CommandDef, emitter screwdriver.Emitter, env []string, path string) (int, error) {
	shargs := []string{"-e", "-c"}
	shargs = append(shargs, cmd.Cmd)
	c := execCommand("sh", shargs...)

	emitter.StartCmd(cmd)
	fmt.Fprintf(emitter, "$ %s\n", cmd.Cmd)
	c.Stdout = emitter
	c.Stderr = emitter

	c.Dir = path
	c.Env = append(env, c.Env...)

	if err := c.Start(); err != nil {
		return ExitLaunch, fmt.Errorf("launching command %q: %v", cmd.Cmd, err)
	}

	if err := c.Wait(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus := exitError.Sys().(syscall.WaitStatus)
			return waitStatus.ExitStatus(), ErrStatus{waitStatus.ExitStatus()}
		}
		return ExitUnknown, fmt.Errorf("running command %q: %v", cmd.Cmd, err)
	}

	return ExitOk, nil
}

// Run executes a slice of CommandDefs
func Run(path string, env []string, emitter screwdriver.Emitter, build screwdriver.Build, api screwdriver.API, buildID string) error {
	cmds := build.Commands

	for k, v := range build.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	for _, cmd := range cmds {
		if err := api.UpdateStepStart(buildID, cmd.Name); err != nil {
			return fmt.Errorf("updating step start %q: %v", cmd.Name, err)
		}

		code, cmdErr := doRun(cmd, emitter, env, path)
		if err := api.UpdateStepStop(buildID, cmd.Name, code); err != nil {
			return fmt.Errorf("updating step stop %q: %v", cmd.Name, err)
		}

		if cmdErr != nil {
			return cmdErr
		}
	}

	return nil
}
