package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var execCommand = exec.Command

// Clone clones a git repo into a destination directory
func Clone(scmURL, destination string) error {
	parts := strings.Split(scmURL, "#")
	if len(parts) < 2 {
		return fmt.Errorf("expected #branchname in SCM URL: %v", scmURL)
	}

	repo := parts[0]
	cmd := execCommand("git", "clone", repo, destination)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

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
