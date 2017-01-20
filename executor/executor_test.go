package executor

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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

type MockAPI struct {
	updateStepStart func(buildID int, stepName string) error
	updateStepStop  func(buildID int, stepName string, exitCode int) error
}

func (f MockAPI) BuildFromID(buildID int) (screwdriver.Build, error) {
	return screwdriver.Build{}, nil
}

func (f MockAPI) JobFromID(jobID int) (screwdriver.Job, error) {
	return screwdriver.Job{}, nil
}

func (f MockAPI) PipelineFromID(pipelineID int) (screwdriver.Pipeline, error) {
	return screwdriver.Pipeline{}, nil
}

func (f MockAPI) UpdateBuildStatus(status screwdriver.BuildStatus, buildID int) error {
	return nil
}

func (f MockAPI) SecretsForBuild(build screwdriver.Build) (screwdriver.Secrets, error) {
	return nil, nil
}

func (f MockAPI) UpdateStepStart(buildID int, stepName string) error {
	if f.updateStepStart != nil {
		return f.updateStepStart(buildID, stepName)
	}
	return nil
}

func (f MockAPI) UpdateStepStop(buildID int, stepName string, exitCode int) error {
	if f.updateStepStop != nil {
		return f.updateStepStop(buildID, stepName, exitCode)
	}
	return nil
}

type MockEmitter struct {
	startCmd func(screwdriver.CommandDef)
	write    func([]byte) (int, error)
	close    func() error
	found    []byte
}

func (e *MockEmitter) Error() error {
	return nil
}

func (e *MockEmitter) StartCmd(cmd screwdriver.CommandDef) {
	if e.startCmd != nil {
		e.startCmd(cmd)
	}
	return
}

func (e *MockEmitter) Write(b []byte) (int, error) {
	if e.write != nil {
		return e.write(b)
	}
	e.found = append(e.found, b...)
	return len(b), nil
}

func (e *MockEmitter) Close() error {
	if e.close != nil {
		return e.close()
	}
	return nil
}

func CreateTempDir(directory string, prefix string) string {
	tmp, err := ioutil.TempDir(directory, prefix)
	if err != nil {
		panic(fmt.Errorf("Couldn't create temp dir: %v", err))
	}
	return tmp
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

	if strings.HasPrefix(args[2], "source") {
		os.Exit(0)
	}

	os.Exit(255)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func ReadCommand(file string) string {
	// read the file that was written for the command
	fileread, err := ioutil.ReadFile(file)
	if err != nil {
		panic(fmt.Errorf("Couldn't read file: %v", err))
	}
	return strings.Split(string(fileread), "\n")[1] // expected input: "#!/bin/sh -e\n<COMMAND>"
}

func TestRunSingle(t *testing.T) {
	tmp := CreateTempDir("", "TestRunSingle")
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "output.sh")

	var tests = []string{"make", "npm install", "failer"}

	for _, test := range tests {
		testCmds := []screwdriver.CommandDef{
			{
				Name: "test",
				Cmd:  test,
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

			// if args[0] != "-e" {
			// 	t.Errorf("Expected sh [-e -c source] to be called, got sh %v", args)
			// }

			if args[0] != "-c" {
				t.Errorf("Expected sh [-e -c source] to be called, got sh %v", args)
			}

			executionCommand := strings.Split(args[1], " ")
			if executionCommand[0] != "source" {
				t.Errorf("Expected sh [-c 'source <file>; echo file $?'] to be called, got sh %v", args)
			}

			if executionCommand[2] != ";echo" {
				t.Errorf("Expected sh [-c 'source <file>; echo file $?'] to be called, got sh %v", args)
			}

			if executionCommand[4] != "$?" {
				t.Errorf("Expected sh [-c 'source <file>; echo file $?'] to be called, got sh %v", args)
			}
		})

		testBuild := screwdriver.Build{
			ID:          1234,
			Commands:    testCmds,
			Environment: map[string]string{},
		}
		testAPI := screwdriver.API(MockAPI{
			updateStepStart: func(buildID int, stepName string) error {
				if buildID != testBuild.ID {
					t.Errorf("wrong build id got %v, want %v", buildID, testBuild.ID)
				}
				if stepName != "test" {
					t.Errorf("wrong step name got %v, want %v", stepName, "test")
				}
				return nil
			},
		})
		Run(tmp, nil, &MockEmitter{}, testBuild, testAPI, testBuild.ID)
		command := ReadCommand(file)

		if !reflect.DeepEqual(command, test) {
			t.Errorf("Unexpected command from Run(%#v): %v", testCmds, command)
		}

		if !called {
			t.Errorf("Exec command was never called for %q.", test)
		}
	}
}

func TestRunMulti(t *testing.T) {
	tmp := CreateTempDir("", "TestRunMulti")
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "output.sh")

	var tests = []string{"make", "npm install", "failer", "neverexecuted"}

	testEnv := map[string]string{
		"foo": "bar",
		"baz": "bah",
	}

	testCmds := []screwdriver.CommandDef{}
	for _, test := range tests {
		testCmds = append(testCmds, screwdriver.CommandDef{
			Name: "test",
			Cmd:  test,
		})
	}

	called := []string{}
	execCommand = getFakeExecCommand(func(cmd string, args ...string) {
    fmt.Fprintf(os.Stderr, "Called %v\n", args[1:])
		called = append(called, args[1:]...)
	})

	testBuild := screwdriver.Build{
		ID:          12345,
		Commands:    testCmds,
		Environment: testEnv,
	}

	testAPI := screwdriver.API(MockAPI{
		updateStepStart: func(buildID int, stepName string) error {
			if buildID != testBuild.ID {
				t.Errorf("wrong build id got %v, want %v", buildID, testBuild.ID)
			}
			if stepName != "test" {
				t.Errorf("wrong step name got %v, want %v", stepName, "test")
			}
			return nil
		},
	})

	err := Run(tmp, nil, &MockEmitter{}, testBuild, testAPI, testBuild.ID)
	command := ReadCommand(file)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if command != "neverexecuted" {
		t.Errorf("Exec called with %v, want neverexecuted", command)
	}

	if len(called) < len(tests)-1 {
		t.Fatalf("%d commands called, want %d", len(called), len(tests)-1)
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

	tmp := CreateTempDir("", "TestUnmocked")
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "output.sh")

	for _, test := range tests {
		cmd := screwdriver.CommandDef{
			Cmd:  test.command,
			Name: "test",
		}
		testBuild := screwdriver.Build{
			ID: 12345,
			Commands: []screwdriver.CommandDef{
				cmd,
			},
			Environment: map[string]string{},
		}
		testAPI := screwdriver.API(MockAPI{
			updateStepStart: func(buildID int, stepName string) error {
				if buildID != testBuild.ID {
					t.Errorf("wrong build id got %v, want %v", buildID, testBuild.ID)
				}
				if stepName != "test" {
					t.Errorf("wrong step name got %v, want %v", stepName, "test")
				}
				return nil
			},
		})
		err := Run(tmp, nil, &MockEmitter{}, testBuild, testAPI, testBuild.ID)
		command := ReadCommand(file)

		if err != test.err {
			t.Fatalf("Unexpected error: %v - should be %v", err, test.err)
		}

		if !reflect.DeepEqual(command, test.command) {
			t.Errorf("Unexpected command from Run(%#v): %v", tests, command)
		}
	}
}

func TestEnv(t *testing.T) {
	tmp := CreateTempDir("", "TestEnv")
	defer os.RemoveAll(tmp)

	baseEnv := []string{
		"var1=foo",
		"var2=bar",
		"VAR3=baz",
	}

	want := map[string]string{
		"var1": "foo",
		"var2": "bar",
		"VAR3": "baz",
	}

	cmds := []screwdriver.CommandDef{
		{
			Cmd:  "env",
			Name: "test",
		},
	}

	testBuild := screwdriver.Build{
		ID:       9999,
		Commands: cmds,
	}

	execCommand = exec.Command
	output := MockEmitter{}
	testAPI := screwdriver.API(MockAPI{
		updateStepStart: func(buildID int, stepName string) error {
			if buildID != testBuild.ID {
				t.Errorf("wrong build id got %v, want %v", buildID, testBuild.ID)
			}
			if stepName != "test" {
				t.Errorf("wrong step name got %v, want %v", stepName, "test")
			}
			return nil
		},
	})
	wantFlattened := []string{}
	for k, v := range want {
		wantFlattened = append(wantFlattened, strings.Join([]string{k, v}, "="))
	}
	err := Run(tmp, baseEnv, &output, testBuild, testAPI, testBuild.ID)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	found := map[string]string{}
	var foundCmd string

	scanner := bufio.NewScanner(bytes.NewReader(output.found))
	for scanner.Scan() {
		line := scanner.Text()
		split := strings.Split(line, "=")
		if len(split) != 2 {
			foundCmd = line
			continue
		}
		found[split[0]] = split[1]
	}

	if !strings.Contains(foundCmd, "output.sh") {
		t.Errorf("foundCmd = %q, want %q", foundCmd, "<TMP FILEPATH>/output.sh 0")
	}

	for k, v := range want {
		if found[k] != v {
			t.Errorf("%v=%q, want %v", k, found[k], v)
		}
	}
}

func TestEmitter(t *testing.T) {
	tmp := CreateTempDir("", "TestUnmocked")
	defer os.RemoveAll(tmp)

	execCommand = exec.Command
	var tests = []struct {
		command string
		name    string
	}{
		{"ls", "name1"},
		{"ls && ls", "name2"},
	}

	testBuild := screwdriver.Build{
		ID:       9999,
		Commands: []screwdriver.CommandDef{},
	}
	for _, test := range tests {
		testBuild.Commands = append(testBuild.Commands, screwdriver.CommandDef{
			Name: test.name,
			Cmd:  test.command,
		})
	}

	var found []string
	emitter := MockEmitter{
		startCmd: func(cmd screwdriver.CommandDef) {
			found = append(found, cmd.Name)
		},
	}
	testAPI := screwdriver.API(MockAPI{
		updateStepStart: func(buildID int, stepName string) error {
			if buildID != testBuild.ID {
				t.Errorf("wrong build id got %v, want %v", buildID, testBuild.ID)
			}
			if (stepName != "name1") && (stepName != "name2") {
				t.Errorf("wrong step name got %v", stepName)
			}
			return nil
		},
	})

	err := Run(tmp, nil, &emitter, testBuild, testAPI, testBuild.ID)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(found) != len(tests) {
		t.Fatalf("Unexpected startCmds called. Want %v. Got %v", len(tests), len(found))
	}

	for i, test := range tests {
		if found[i] != test.name {
			t.Errorf("Unexpected order. Want %v. Got %v", found[i], test.name)
		}
	}
}

func TestErrStatus(t *testing.T) {
	errText := ErrStatus{5}.Error()
	if errText != "exit 5" {
		t.Errorf("ErrStatus{5}.Error() == %q, want %q", errText, "exit 5")
	}
}
