package git

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

type execFunc func(command string, args ...string) *exec.Cmd

func getFakeExecCommand(validator func(string, ...string)) execFunc {
	return func(command string, args ...string) *exec.Cmd {
		validator(command, args...)
		return fakeExecCommand(command, args...)
	}
}

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestClone(t *testing.T) {
	wantRepo := "git@github.com:screwdriver-cd/launcher"
	scmURL := "git@github.com:screwdriver-cd/launcher#master"
	wantDest := "testdest"
	wantBranch := "master"
	execCommand = getFakeExecCommand(func(cmd string, args ...string) {
		want := []string{
			"clone", "--quiet", "--progress", "--branch", wantBranch, wantRepo, wantDest,
		}
		if len(args) != len(want) {
			t.Errorf("Incorrect args sent to git: %q, want %q", args, want)
		}
		for i, arg := range args {
			if arg != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, want[i])
			}
		}
	})

	err := Clone(scmURL, wantDest)
	if err != nil {
		t.Errorf("Unexpected error from git clone: %v", err)
	}
}

func TestCloneBadBranch(t *testing.T) {
	scmURL := "git@github.com:screwdriver-cd/launcher"
	wantDest := "testdest"

	err := Clone(scmURL, wantDest)
	if err == nil {
		t.Errorf("Missing error from git clone")
	}
	if err.Error() != "expected #branchname in SCM URL: "+scmURL {
		t.Errorf("Error did not match %v", err)
	}
}

func TestHelperProcess(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args[:]
	for i, val := range os.Args { // Should become something lke ["git", "clone"]
		args = os.Args[i:]
		if val == "--" {
			args = args[1:]
			break
		}
	}

	if len(args) > 1 {
		switch args[1] {
		case "clone":
			return
		case "config":
			return
		case "fetch":
			return
		case "merge":
			return
		}
	}
	os.Exit(255)
}

func TestSetConfig(t *testing.T) {
	testUserName := "sd-buildbot"
	testSetting := "user.name"
	execCommand = getFakeExecCommand(func(cmd string, args ...string) {
		want := []string{
			"config", "user.name", testUserName,
		}
		if len(args) != len(want) {
			t.Errorf("Incorrect args sent to git: %q, want %q", args, want)
		}
		for i, arg := range args {
			if arg != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, want[i])
			}
		}
	})

	err := SetConfig(testSetting, testUserName)
	if err != nil {
		t.Errorf("Unexpected error from git config: %v", err)
	}
}

func TestFetchPR(t *testing.T) {
	testPrNumber := "111"
	testBranch := "branch"
	execCommand = getFakeExecCommand(func(cmd string, args ...string) {
		want := []string{
			"fetch", "origin", "pull/" + testPrNumber + "/head:" + testBranch,
		}
		if len(args) != len(want) {
			t.Errorf("Incorrect args sent to git: %q, want %q", args, want)
		}
		for i, arg := range args {
			if arg != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, want[i])
			}
		}
	})

	err := FetchPR(testPrNumber, testBranch)
	if err != nil {
		t.Errorf("Unexpected error from git fetch %v", err)
	}
}

func TestMerge(t *testing.T) {
	testBranch := "branch"
	execCommand = getFakeExecCommand(func(cmd string, args ...string) {
		want := []string{
			"merge", "--no-edit", testBranch,
		}
		if len(args) != len(want) {
			t.Errorf("Incorrect args sent to git: %q, want %q", args, want)
		}
		for i, arg := range args {
			if arg != want[i] {
				t.Errorf("args[%d] = %q, want %q", i, arg, want[i])
			}
		}
	})

	err := Merge(testBranch)
	if err != nil {
		t.Errorf("Unexpected error from git merge %v", err)
	}
}

func TestMergePR(t *testing.T) {
	testPrNumber := "111"
	testBranch := "branch"

	fetchPR = func(prNumber, branch string) error {
		if prNumber != testPrNumber {
			t.Errorf("Fetch was called with prNumber %q, want %q", prNumber, testPrNumber)
		}
		if branch != testBranch {
			t.Errorf("Fetch was called with branch %q, want %q", branch, testBranch)
		}
		return nil
	}
	merge = func(branch string) error {
		if branch != testBranch {
			t.Errorf("Merge was called with branch %q, want %q", branch, testBranch)
		}
		return nil
	}

	err := MergePR(testPrNumber, testBranch)
	if err != nil {
		t.Errorf("Unexpected error from mergePR %v", err)
	}
}

func TestMergePRBadFetch(t *testing.T) {
	testPrNumber := "111"
	testBranch := "branch"

	fetchPR = func(prNumber, branch string) error {
		return fmt.Errorf("Spooky error")
	}

	err := MergePR(testPrNumber, testBranch)
	if err == nil {
		t.Errorf("Missing error from mergePR")
	}
	if err.Error() != "fetching pr: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestMergePRBadMerge(t *testing.T) {
	testPrNumber := "111"
	testBranch := "branch"

	fetchPR = func(prNumber, branch string) error {
		if prNumber != testPrNumber {
			t.Errorf("Fetch was called with prNumber %q, want %q", prNumber, testPrNumber)
		}
		if branch != testBranch {
			t.Errorf("Fetch was called with branch %q, want %q", branch, testBranch)
		}
		return nil
	}
	merge = func(branch string) error {
		return fmt.Errorf("Spooky error")
	}

	err := MergePR(testPrNumber, testBranch)
	if err == nil {
		t.Errorf("Missing error from mergePR")
	}
	if err.Error() != "merging pr: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestSetupNonPR(t *testing.T) {
	testPrNumber := ""
	testScmURL := "https://github.com/screwdriver-cd/launcher.git#master"
	testDestination := "/tmp"

	clone = func(scmUrl, destination string) error {
		if scmUrl != testScmURL {
			t.Errorf("Git clone was called with scmUrl %q, want %q", scmUrl, testScmURL)
		}
		if destination != testDestination {
			t.Errorf("Git clone was called with destination %q, want %q", destination, testDestination)
		}
		return nil
	}
	setConfig = func(setting, value string) error {
		if setting == "user.name" {
			if value != "sd-buildbot" {
				t.Errorf("Username was set with %q, want %q", value, "sd-buildbot")
			}
		} else if setting == "user.email" {
			if value != "dev-null@screwdriver.cd" {
				t.Errorf("Email was set with %q, want %q", value, "dev-null@screwdriver.cd")
			}
		} else {
			t.Errorf("Config was called with %q and %q", setting, value)
		}
		return nil
	}
	mergePR = func(pr, branch string) error {
		t.Errorf("Should not get here")
		return nil
	}
	err := Setup(testScmURL, testDestination, testPrNumber)

	if err != nil {
		t.Errorf("Unexpected error from setup %v", err)
	}
}

func TestSetupPR(t *testing.T) {
	testPrNumber := "111"
	testScmURL := "https://github.com/screwdriver-cd/launcher.git#master"
	testDestination := "/tmp"

	clone = func(scmUrl, destination string) error {
		if scmUrl != testScmURL {
			t.Errorf("Git clone was called with scmUrl %q, want %q", scmUrl, testScmURL)
		}
		if destination != testDestination {
			t.Errorf("Git clone was called with destination %q, want %q", destination, testDestination)
		}
		return nil
	}
	setConfig = func(setting, value string) error {
		if setting == "user.name" {
			if value != "sd-buildbot" {
				t.Errorf("Username was set with %q, want %q", value, "sd-buildbot")
			}
		} else if setting == "user.email" {
			if value != "dev-null@screwdriver.cd" {
				t.Errorf("Email was set with %q, want %q", value, "dev-null@screwdriver.cd")
			}
		} else {
			t.Errorf("Config was called with %q and %q", setting, value)
		}
		return nil
	}
	mergePR = func(pr, branch string) error {
		if pr != testPrNumber {
			t.Errorf("PR was sent with %q, want %q", pr, testPrNumber)
		}
		if branch != "_pr" {
			t.Errorf("Branch was sent with %q, want %q", branch, "_pr")
		}
		return nil
	}
	err := Setup(testScmURL, testDestination, testPrNumber)

	if err != nil {
		t.Errorf("Unexpected error from setup %v", err)
	}
}

func TestSetupBadClone(t *testing.T) {
	testPrNumber := "111"
	testScmURL := "https://github.com/screwdriver-cd/launcher.git#master"
	testDestination := "/tmp"

	clone = func(scmUrl, destination string) error {
		return fmt.Errorf("Spooky error")
	}
	setConfig = func(setting, value string) error {
		return fmt.Errorf("Should not get here")
	}
	mergePR = func(pr, branch string) error {
		return fmt.Errorf("Should not get here")
	}
	err := Setup(testScmURL, testDestination, testPrNumber)

	if err == nil {
		t.Errorf("Missing error from setup")
	}
	if err.Error() != "cloning repository: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestSetupBadConfigName(t *testing.T) {
	testPrNumber := "111"
	testScmURL := "https://github.com/screwdriver-cd/launcher.git#master"
	testDestination := "/tmp"

	clone = func(scmUrl, destination string) error {
		return nil
	}
	setConfig = func(setting, value string) error {
		if setting == "user.name" {
			return fmt.Errorf("Spooky error")
		}
		return fmt.Errorf("Should not get here")
	}
	mergePR = func(pr, branch string) error {
		return fmt.Errorf("Should not get here")
	}
	err := Setup(testScmURL, testDestination, testPrNumber)

	if err == nil {
		t.Errorf("Missing error from setup")
	}
	if err.Error() != "setting username: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestSetupBadConfigEmail(t *testing.T) {
	testPrNumber := "111"
	testScmURL := "https://github.com/screwdriver-cd/launcher.git#master"
	testDestination := "/tmp"

	clone = func(scmUrl, destination string) error {
		return nil
	}
	setConfig = func(setting, value string) error {
		if setting == "user.email" {
			return fmt.Errorf("Spooky error")
		}
		return nil
	}
	mergePR = func(pr, branch string) error {
		return fmt.Errorf("Should not get here")
	}
	err := Setup(testScmURL, testDestination, testPrNumber)

	if err == nil {
		t.Errorf("Missing error from setup")
	}
	if err.Error() != "setting email: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestSetupBadMerge(t *testing.T) {
	testPrNumber := "111"
	testScmURL := "https://github.com/screwdriver-cd/launcher.git#master"
	testDestination := "/tmp"

	clone = func(scmUrl, destination string) error {
		return nil
	}
	setConfig = func(setting, value string) error {
		return nil
	}
	mergePR = func(pr, branch string) error {
		return fmt.Errorf("Spooky error")
	}
	err := Setup(testScmURL, testDestination, testPrNumber)

	if err == nil {
		t.Errorf("Missing error from setup")
	}
	if err.Error() != "merging pr: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestCommandBadStart(t *testing.T) {
	oldStartCmd := startCmd
	defer func() { startCmd = oldStartCmd }()

	startCmd = func(c *exec.Cmd) error {
		return fmt.Errorf("Spooky error")
	}

	err := command("random", "arguments")

	if err.Error() != "starting git command: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestCommandBadWait(t *testing.T) {
	oldStartCmd := startCmd
	defer func() { startCmd = oldStartCmd }()

	oldWaitCmd := waitCmd
	defer func() { waitCmd = oldWaitCmd }()

	startCmd = func(c *exec.Cmd) error {
		return nil
	}

	waitCmd = func(c *exec.Cmd) error {
		return fmt.Errorf("Spooky error")
	}

	err := command("random", "arguments")

	if err.Error() != "running git command: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}
