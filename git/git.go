package git

import (
	"fmt"
	"os"
	"os/exec"
)

var execCommand = exec.Command

// Clone clones a git repo into a destination directory
func Clone(repo, destination string) error {
	cmd := execCommand("git", "clone", repo, destination)
	cmd.Stdout = os.Stdout
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("starting git clone command: %v", err)
	}
	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("cloning git repo: %v", err)
	}
	return nil
}
