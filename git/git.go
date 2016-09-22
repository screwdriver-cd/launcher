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
	sha    string
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
	if err := r.fetchPR(prNumber); err != nil {
		return fmt.Errorf("fetching pr: %v", err)
	}

	if err := r.merge(sha); err != nil {
		return fmt.Errorf("merging sha: %v", err)
	}

	return nil
}

// Checkout checks out the git repo at the specified branch and configures the setting
func (r repo) Checkout() error {
	fmt.Fprintf(r.logger, "Cloning %v, on branch %v\n", r.scmURL, r.branch)
	if err := r.clone(); err != nil {
		return fmt.Errorf("cloning repository: %v", err)
	}

	fmt.Fprintln(r.logger, "Setting user name and user email")
	if err := r.setConfig("user.name", "sd-buildbot"); err != nil {
		return fmt.Errorf("setting user name: %v", err)
	}

	if err := r.setConfig("user.email", "dev-null@screwdriver.cd"); err != nil {
		return fmt.Errorf("setting user email: %v", err)
	}

	return nil
}

// clone clones a git repo into its destination directory
func (r repo) clone() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	return command(r.logger, dir, "clone", "--quiet", "--progress", "--branch", r.branch, r.scmURL, r.path)
}

// setConfig applies a git config change to the repo
func (r repo) setConfig(setting, name string) error {
	return command(r.logger, r.path, "config", setting, name)
}

// fetchPR fetches a pull request
func (r repo) fetchPR(prNumber string) error {
	return command(r.logger, r.path, "fetch", "origin", "pull/"+prNumber+"/head:pr")
}

// merge merges changes on the specified branch
func (r repo) merge(branch string) error {
	return command(r.logger, r.path, "merge", "--no-edit", branch)
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
