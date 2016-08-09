package executor

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/screwdriver-cd/launcher/screwdriver"
)

var execCommand = exec.Command

// Run executes a slice of CommandDefs
func Run(cmds []screwdriver.CommandDef) error {
	for _, cmd := range cmds {
		shargs := []string{"-e", "-c"}
		shargs = append(shargs, cmd.Cmd)
		c := execCommand("sh", shargs...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Start(); err != nil {
			return fmt.Errorf("launching command %q: %v", cmd.Cmd, err)
		}

		if err := c.Wait(); err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				waitStatus := exitError.Sys().(syscall.WaitStatus)
				return fmt.Errorf("exit %d", waitStatus.ExitStatus())
			}
			return fmt.Errorf("running command %q: %v", cmd.Cmd, err)
		}
	}

	return nil
}
