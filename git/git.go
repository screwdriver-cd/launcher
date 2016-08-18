package git

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

var execCommand = exec.Command

const (
	scmUrlIndex = 0
	branchIndex = 1
)

type repo struct {
	ScmURL string
	Path   string
	Branch string
}

// Repo is a git repo
type Repo interface {
	Checkout() error
	MergePR(prNumber, sha string) error
	GetPath() string
}

// New returns a new repo object
func New(scmURL, path string) (Repo, error) {
	parts := strings.Split(scmURL, "#")
	if len(parts) < 2 {
		return nil, fmt.Errorf("expected #branchname in SCM URL: %v", scmURL)
	}
	repo := repo{
		ScmURL: parts[scmUrlIndex],
		Path:   path,
		Branch: parts[branchIndex],
	}
	return Repo(repo), nil
}

func (r repo) GetPath() string {
	return r.Path
}

// MergePR fetches and merges the specified pull request to the specified branch
func (r repo) MergePR(prNumber string, sha string) error {
	if err := fetchPR(prNumber, r.Path); err != nil {
		return fmt.Errorf("fetching pr: %v", err)
	}

	if err := merge(sha, r.Path); err != nil {
		return fmt.Errorf("merging sha: %v", err)
	}

	return nil
}

// Checkout checks out the git repo at the specified branch and configures the setting
func (r repo) Checkout() error {
	log.Printf("Cloning %v, on branch %v", r.ScmURL, r.Branch)
	if err := clone(r.Branch, r.ScmURL, r.Path); err != nil {
		return fmt.Errorf("cloning repository: %v", err)
	}

	log.Printf("Setting user name and user email")
	if err := setConfig("user.name", "sd-buildbot", r.Path); err != nil {
		return fmt.Errorf("setting user name: %v", err)
	}

	if err := setConfig("user.email", "dev-null@screwdriver.cd", r.Path); err != nil {
		return fmt.Errorf("setting user email: %v", err)
	}

	return nil
}

// clone clones a git repo into a destination directory
func clone(branch, scmURL, destination string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %v", err)
	}
	return command(dir, "clone", "--quiet", "--progress", "--branch", branch, scmURL, destination)
}

// setConfig sets the specified git settting
func setConfig(setting, name, dir string) error {
	return command(dir, "config", setting, name)
}

// fetchPR fetches a pull request
func fetchPR(prNumber, dir string) error {
	return command(dir, "fetch", "origin", "pull/"+prNumber+"/head:pr")
}

// merge merges changes on the specified branch
func merge(branch, dir string) error {
	return command(dir, "merge", "--no-edit", branch)
}

// command executes the git command
func command(dir string, arguments ...string) error {
	cmd := execCommand("git", arguments...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = dir

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting git command: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("running git command: %v", err)
	}
	return nil
}
