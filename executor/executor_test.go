package executor

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"github.com/screwdriver-cd/launcher/screwdriver"
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

	if len(args) == 3 {
		switch args[2] {
		case "make":
			os.Exit(0)
		case "npm install":
			os.Exit(0)
		case "failer":
			os.Exit(7)
		}
	}
	os.Exit(255)
}

func TestRunSingle(t *testing.T) {
	var tests = []struct {
		command string
		err     error
	}{
		{"make", nil},
		{"npm install", nil},
		{"failer", fmt.Errorf("exit 7")},
	}

	for _, test := range tests {
		testCmds := []screwdriver.CommandDef{
			screwdriver.CommandDef{
				Name: "test",
				Cmd:  test.command,
			},
		}

		called := false
		execCommand = getFakeExecCommand(func(cmd string, args ...string) {
			called = true
			if cmd != "sh" {
				t.Errorf("Run() ran %v, want 'sh'", cmd)
			}

			if len(args) != 2 {
				t.Errorf("Expected 2 arguments to exec, got %d: %v", len(args), args)
			}

			if args[0] != "-c" {
				t.Errorf("Expected sh -c to be called, got sh %v", args[0])
			}

			if args[1] != test.command {
				t.Errorf("sh -c %v called, want sh -c %v", args[1], test.command)
			}
		})

		err := Run(testCmds)
		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from Run(%#v): %v", testCmds, err)
		}

		if !called {
			t.Errorf("Exec command was never called for %q.", test.command)
		}
	}
}

func TestRunMulti(t *testing.T) {
	var tests = []struct {
		command string
		err     error
	}{
		{"make", nil},
		{"npm install", nil},
		{"failer", fmt.Errorf("exit 7")},
		{"neverexecuted", nil},
	}

	testCmds := []screwdriver.CommandDef{}
	for _, test := range tests {
		testCmds = append(testCmds, screwdriver.CommandDef{
			Name: "test",
			Cmd:  test.command,
		})
	}

	called := []string{}
	execCommand = getFakeExecCommand(func(cmd string, args ...string) {
		called = append(called, args[1:]...)
	})
	err := Run(testCmds)

	if len(called) < len(tests)-1 {
		t.Fatalf("%d commands called, want %d", len(called), len(tests)-1)
	}

	if !reflect.DeepEqual(err, fmt.Errorf("exit 7")) {
		t.Errorf("Unexpected error: %v", err)
	}

	for i, test := range tests {
		if i >= len(tests)-1 {
			break
		}
		if called[i] != test.command {
			t.Errorf("Exec called with %v, want %v", called[i], test.command)
		}
	}
}
