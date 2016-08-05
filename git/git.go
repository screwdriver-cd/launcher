package git

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

var execCommand = exec.Command
var clone = Clone
var setConfig = SetConfig
var mergePR = MergePR
var fetchPR = FetchPR
var merge = Merge

// command executes the git command
func command(arguments ...string) error {
	cmd := execCommand("git", arguments...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("starting git command: %v", err)
	}

	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("running git command: %v", err)
	}
	return nil
}

// Clone clones a git repo into a destination directory
func Clone(scmURL, destination string) error {
	log.Printf("Cloning %v", scmURL)
	parts := strings.Split(scmURL, "#")
	if len(parts) < 2 {
		return fmt.Errorf("expected #branchname in SCM URL: %v", scmURL)
	}

	repo := parts[0]
	branch := parts[1]
	return command("clone", "--quiet", "--progress", "--branch", branch, repo, destination)
}

// SetConfig sets up git configuration
func SetConfig(setting, name string) error {
	return command("config", setting, name)
}

// FetchPR fetches a pull request into a specified branch
func FetchPR(prNumber string, branch string) error {
	return command("fetch", "origin", "pull/"+prNumber+"/head:"+branch)
}

// Merge merges changes on the specified branch
func Merge(branch string) error {
	return command("merge", "--no-edit", branch)
}

// MergePR calls FetchPR and Merge
func MergePR(prNumber string, branch string) error {
	err := fetchPR(prNumber, branch)
	if err != nil {
		return fmt.Errorf("fetching pr: %v", err)
	}
	err = merge(branch)
	if err != nil {
		return fmt.Errorf("merging pr: %v", err)
	}
	return nil
}

// Setup clones a repository, sets the local config, and merges a PR if necessary
func Setup(scmURL, destination, pr string) error {
	err := clone(scmURL, destination)
	if err != nil {
		return fmt.Errorf("cloning repository: %v", err)
	}

	err = setConfig("user.name", "sd-buildbot")
	if err != nil {
		return fmt.Errorf("setting username: %v", err)
	}

	err = setConfig("user.email", "dev-null@screwdriver.cd")
	if err != nil {
		return fmt.Errorf("setting email: %v", err)
	}

	if pr != "" {
		err = mergePR(pr, "_pr")
		if err != nil {
			return fmt.Errorf("merging pr: %v", err)
		}
	}

	return nil
}
