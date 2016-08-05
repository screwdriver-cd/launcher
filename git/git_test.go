package git

import (
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
	execCommand = getFakeExecCommand(func(cmd string, args ...string) {
		want := []string{
			"clone", wantRepo, wantDest,
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
		}
	}
	os.Exit(255)
}
