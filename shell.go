package main

import(
	"fmt"
	"os"
	"os/exec"
	"strings"
	"regexp"
)

// ShellCommand is a helper function that executes the specified shell command with its
// specified arguments. It prints the output of the shell command.
func ShellCommand(cmd string, args ...string) (out string, err error) {
	var output []byte

	output, err = exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(output), err
}

// Clone will clone the specified branch from the repository in path 
// and switch to its directory.
func Clone(path string, branch string) (err error) {
	_, err = ShellCommand("rm", "-rf", path)
	if err != nil{
		return err
	}

	fmt.Println("> Checking out source code from Github")
	_, err = ShellCommand("git", "clone", "--branch", branch, "--depth=50", "--quiet", "--progress", "https://" + path, path)
	if err != nil {
		return err
	}

	err = os.Chdir(path)
	if err != nil {
		return err
	}

	return err
}

// FetchAndMergePR will fetch and merge the specified pull request
// if the job name is a pull request
func FetchAndMergePR(branch string, job string) (PR_NUMBER string, err error) {
	reg := "^PR-[0-9]+$"
	matched, err := regexp.MatchString(reg, job)
	if err != nil {
		return "", err
	}

	if matched == true {
		PR_NUMBER = strings.Split(job, "-")[1]

		// ShellCommand("git", "config", "user.name", "sd-buildbot")
		// ShellCommand("git", "config", "user.email", "devnull@screwdriver.cd")

		ShellCommand("git", "config", "user.name", "Filbird")
		ShellCommand("git", "config", "user.email", "filidillidally@gmail.com")

		fmt.Println("> Fetching latest Pull Request", PR_NUMBER)
		_, err = ShellCommand("git", "fetch", "origin", "pull/" + PR_NUMBER + "/head:_pr")
		if err != nil {
			return "", err
		}

		fmt.Println("> Merging changes onto " + branch)
		_, err = ShellCommand("git", "merge", "_pr")
		if err != nil {
			return "", err
		}
	}

	return PR_NUMBER, err
}


// ArtifactsDirSetUp will create a directory to store artifacts.
// It exports the path to the directory as an environment variable, $ARTIFACTS_DIR
func ArtifactsDirSetUp() (err error) {
	out, err := os.Getwd()
	if err != nil {
		return err
	}

	artifacts := out + "/artifacts"
	_, err = ShellCommand("mkdir", "-p", artifacts)
	if err != nil {
		return err
	}

	err = os.Chdir(artifacts)
	if err != nil {
		return err
	}

	err = os.Setenv("ARTIFACTS_DIR", artifacts)
	if err != nil {
		return err
	}

	out = os.Getenv("ARTIFACTS_DIR")

	return nil
}