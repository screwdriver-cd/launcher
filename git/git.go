package git

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

var execCommand = exec.Command

// Clone clones a git repo into a destination directory
func Clone(scmURL, destination string) error {
	log.Printf("Cloning %v", scmURL)
	parts := strings.Split(scmURL, "#")
	if len(parts) < 2 {
		return fmt.Errorf("expected #branchname in SCM URL: %v", scmURL)
	}

	repo := parts[0]
	branch := parts[1]
	cmd := execCommand("git", "clone", "--quiet", "--progress", "--branch", branch, repo, destination)

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

//SetConfig sets up git configuration
func SetConfig(setting, name string) error {
	cmd := execCommand("git", "config", setting, name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("starting git config command: %v", err)
	}

	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("setting git config: %v", err)
	}
	return nil
}
