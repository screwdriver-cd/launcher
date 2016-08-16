package executor

import (
	"fmt"
	"io"
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
func Run(output io.Writer, job screwdriver.JobDef) error {
	cmds := job.Commands
	env := job.Environment
	for _, cmd := range cmds {
		shargs := []string{"-e", "-c"}
		shargs = append(shargs, cmd.Cmd)
		c := execCommand("sh", shargs...)
		c.Stdout = output
		c.Stderr = output
		for k, v := range env {
			c.Env = append(c.Env, fmt.Sprintf("%s=%s", k, v))
		}

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
