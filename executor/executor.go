package executor

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/screwdriver-cd/launcher/screwdriver"
)

var execCommand = exec.Command

// ErrStatus is an error that holds an exit status code
type ErrStatus struct {
	Status int
}

func (e ErrStatus) Error() string {
	return fmt.Sprintf("exit %d", e.Status)
}

// Run executes a slice of CommandDefs
func Run(path string, emitter screwdriver.Emitter, job screwdriver.JobDef) error {
	cmds := job.Commands
	env := os.Environ()
	for k, v := range job.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	for _, cmd := range cmds {
		shargs := []string{"-e", "-c"}
		shargs = append(shargs, cmd.Cmd)
		c := execCommand("sh", shargs...)

		emitter.StartCmd(cmd)
		c.Stdout = emitter
		c.Stderr = emitter

		c.Dir = path
		c.Env = append(env, c.Env...)

		if err := c.Start(); err != nil {
			return fmt.Errorf("launching command %q: %v", cmd.Cmd, err)
		}

		if err := c.Wait(); err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				waitStatus := exitError.Sys().(syscall.WaitStatus)
				return ErrStatus{waitStatus.ExitStatus()}
			}
			return fmt.Errorf("running command %q: %v", cmd.Cmd, err)
		}
	}

	return nil
}
