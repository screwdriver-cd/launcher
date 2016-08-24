package executor

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/screwdriver-cd/launcher/screwdriver"
)

var defaultEnv = map[string]string{
	"SCREWDRIVER": "true",
	"CI":          "true",
	"CONTINUOUS_INTEGRATION": "true",
	"SD_JOB_NAME":            "main",
	"SD_PULL_REQUEST":        "",
	"SD_SOURCE_DIR":          "test/path",
	"SD_ARTIFACTS_DIR":       "/sd/workspace/artifacts",
}

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

	if len(args) == 4 {
		switch args[3] {
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

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestRunSingle(t *testing.T) {
	var tests = []struct {
		command string
		err     error
	}{
		{"make", nil},
		{"npm install", nil},
		{"failer", ErrStatus{7}},
	}

	for _, test := range tests {
		testCmds := []screwdriver.CommandDef{
			{
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

			if len(args) != 3 {
				t.Errorf("Expected 3 arguments to exec, got %d: %v", len(args), args)
			}

			if args[0] != "-e" {
				t.Errorf("Expected sh [-e -c] to be called, got sh %v", args)
			}

			if args[1] != "-c" {
				t.Errorf("Expected sh [-e -c] to be called, got sh %v", args)
			}

			if args[2] != test.command {
				t.Errorf("sh -e -c %v called, want sh -e -c %v", args[1], test.command)
			}
		})

		testJob := screwdriver.JobDef{
			Commands:    testCmds,
			Environment: map[string]string{},
		}
		err := Run(defaultEnv, "", nil, testJob)

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

	testEnv := map[string]string{
		"foo": "bar",
		"baz": "bah",
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
		called = append(called, args[2:]...)
	})

	testJob := screwdriver.JobDef{
		Commands:    testCmds,
		Environment: testEnv,
	}
	err := Run(defaultEnv, "", nil, testJob)

	if len(called) < len(tests)-1 {
		t.Fatalf("%d commands called, want %d", len(called), len(tests)-1)
	}

	if !reflect.DeepEqual(err, ErrStatus{7}) {
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

func TestUnmocked(t *testing.T) {
	execCommand = exec.Command
	var tests = []struct {
		command string
		err     error
	}{
		{"ls", nil},
		{"doesntexist", ErrStatus{127}},
		{"ls && ls", nil},
		{"ls && sh -c 'exit 5' && sh -c 'exit 2'", ErrStatus{5}},
	}

	var testEnv map[string]string
	for _, test := range tests {
		cmd := screwdriver.CommandDef{
			Cmd: test.command,
		}
		testJob := screwdriver.JobDef{
			Commands: []screwdriver.CommandDef{
				cmd,
			},
			Environment: testEnv,
		}
		err := Run(defaultEnv, "", nil, testJob)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error: %v, want %v", err, test.err)
		}
	}
}

func TestEnv(t *testing.T) {
	want := map[string]string{
		"var1": "foo",
		"var2": "bar",
		"VAR3": "baz",
	}

	cmds := []screwdriver.CommandDef{
		{
			Cmd: "env",
		},
	}

	job := screwdriver.JobDef{
		Commands:    cmds,
		Environment: want,
	}

	execCommand = exec.Command
	output := new(bytes.Buffer)
	err := Run(defaultEnv, "", output, job)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	found := map[string]string{}
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		line := scanner.Text()
		split := strings.Split(line, "=")
		found[split[0]] = split[1]
	}

	for k, v := range want {
		if found[k] != v {
			t.Errorf("%v=%v, want %v", k, v, want[k])
		}
	}

	for k, v := range defaultEnv {
		if found[k] != v {
			t.Errorf("%v=%v, want %v", k, v, defaultEnv[k])
		}
	}
}

func TestErrStatus(t *testing.T) {
	errText := ErrStatus{5}.Error()
	if errText != "exit 5" {
		t.Errorf("ErrStatus{5}.Error() == %q, want %q", errText, "exit 5")
	}
}
