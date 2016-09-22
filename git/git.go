package git

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var execCommand = exec.Command

const (
	scmURLIndex = 0
	branchIndex = 1
)

type repo struct {
	scmURL string
	path   string
	branch string
	logger io.Writer
}

// Repo is a git repo
type Repo interface {
	Checkout() error
	MergePR(prNumber, sha string) error
	Path() string
}

// New returns a new repo object
func New(scmURL, path string, logger io.Writer) (Repo, error) {
	parts := strings.Split(scmURL, "#")
	if len(parts) < 2 {
		return nil, fmt.Errorf("expected #branchname in SCM URL: %v", scmURL)
	}
	newrepo := repo{
		scmURL: parts[scmURLIndex],
		path:   path,
		branch: parts[branchIndex],
		logger: logger,
	}
	return Repo(newrepo), nil
}

func (r repo) Path() string {
	return r.path
}

// MergePR fetches and merges the specified pull request to the specified branch
func (r repo) MergePR(prNumber string, sha string) error {
	if err := fetchPR(prNumber, r.path, r.logger); err != nil {
		return fmt.Errorf("fetching pr: %v", err)
	}

	if err := merge(sha, r.path, r.logger); err != nil {
		return fmt.Errorf("merging sha: %v", err)
	}

	return nil
}

// Checkout checks out the git repo at the specified branch and configures the setting
func (r repo) Checkout() error {
	fmt.Fprintf(r.logger, "Cloning %v, on branch %v", r.scmURL, r.branch)
	if err := clone(r.branch, r.scmURL, r.path, r.logger); err != nil {
		return fmt.Errorf("cloning repository: %v", err)
	}

	fmt.Fprintf(r.logger, "Setting user name and user email")
	if err := setConfig("user.name", "sd-buildbot", r.path, r.logger); err != nil {
		return fmt.Errorf("setting user name: %v", err)
	}

	if err := setConfig("user.email", "dev-null@screwdriver.cd", r.path, r.logger); err != nil {
		return fmt.Errorf("setting user email: %v", err)
	}

	return nil
}

// clone clones a git repo into a destination directory
func clone(branch, scmURL, destination string, logger io.Writer) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %v", err)
	}
	return command(logger, dir, "clone", "--quiet", "--progress", "--branch", branch, scmURL, destination)
}

// setConfig sets the specified git settting
func setConfig(setting, name, dir string, logger io.Writer) error {
	return command(logger, dir, "config", setting, name)
}

// fetchPR fetches a pull request
func fetchPR(prNumber, dir string, logger io.Writer) error {
	return command(logger, dir, "fetch", "origin", "pull/"+prNumber+"/head:pr")
}

// merge merges changes on the specified branch
func merge(branch, dir string, logger io.Writer) error {
	return command(logger, dir, "merge", "--no-edit", branch)
}

// command executes the git command
func command(logger io.Writer, dir string, arguments ...string) error {
	cmd := execCommand("git", arguments...)

	cmd.Stdout = logger
	cmd.Stderr = logger
	cmd.Dir = dir

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting git command: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("running git command: %v", err)
	}
	return nil
}
