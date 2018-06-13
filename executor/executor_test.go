package executor

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/screwdriver-cd/launcher/screwdriver"
)

const TestBuildTimeout = 60

var stepFilePath = "/tmp/step.sh"

type MockAPI struct {
	updateStepStart func(buildID int, stepName string) error
	updateStepStop  func(buildID int, stepName string, exitCode int) error
}

func (f MockAPI) BuildFromID(buildID int) (screwdriver.Build, error) {
	return screwdriver.Build{}, nil
}

func (f MockAPI) EventFromID(eventID int) (screwdriver.Event, error) {
	return screwdriver.Event{}, nil
}

func (f MockAPI) JobFromID(jobID int) (screwdriver.Job, error) {
	return screwdriver.Job{}, nil
}

func (f MockAPI) PipelineFromID(pipelineID int) (screwdriver.Pipeline, error) {
	return screwdriver.Pipeline{}, nil
}

func (f MockAPI) UpdateBuildStatus(status screwdriver.BuildStatus, meta map[string]interface{}, buildID int) error {
	return nil
}

func (f MockAPI) SecretsForBuild(build screwdriver.Build) (screwdriver.Secrets, error) {
	return nil, nil
}

func (f MockAPI) GetAPIURL() (string, error) {
	return "http://foo.bar", nil
}

func (f MockAPI) GetCoverageInfo() (screwdriver.Coverage, error) {
	return screwdriver.Coverage{}, nil
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

func ReadCommand(file string) []string {
	// read the file that was written for the command
	fileread, err := ioutil.ReadFile(file)
	if err != nil {
		panic(fmt.Errorf("Couldn't read file: %v", err))
	}

	return strings.Split(string(fileread), "\n")
}

func TestUnmocked(t *testing.T) {
	var tests = []struct {
		command string
		err     error
		shell   string
	}{
		{"ls", nil, "/bin/sh"},
		{"sleep 1", nil, "/bin/sh"},
		{"ls && ls ", nil, "/bin/sh"},
		// Large single-line
		{"openssl rand -hex 1000000", nil, "/bin/sh"},
		{"doesntexist", fmt.Errorf("Launching command exit with code: %v", 127), "/bin/sh"},
		{"ls && sh -c 'exit 5' && sh -c 'exit 2'", fmt.Errorf("Launching command exit with code: %v", 5), "/bin/sh"},
		// Custom shell
		{"ls", nil, "/bin/bash"},
	}

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

		err := Run("", nil, &MockEmitter{}, testBuild, testAPI, testBuild.ID, test.shell, TestBuildTimeout)
		commands := ReadCommand(stepFilePath)

		if !reflect.DeepEqual(err, test.err) {
			t.Fatalf("Unexpected error: (%v) - should be (%v)", err, test.err)
		}

		if test.shell == "" {
			test.shell = "/bin/sh"
		}

		if !reflect.DeepEqual(commands[0], "#!"+test.shell+" -e") {
			t.Errorf("Unexpected shell from Run(%#v): %v", test, commands[0])
		}

		if !reflect.DeepEqual(commands[1], test.command) {
			t.Errorf("Unexpected command from Run(%#v): %v", test, commands[1])
		}
	}
}

func TestUnmockedMulti(t *testing.T) {
	commands := []screwdriver.CommandDef{
		{Cmd: "ls", Name: "test ls"},
		{Cmd: "export FOO=BAR", Name: "test export env"},
		{Cmd: `if [ -z "$FOO" ] ; then exit 1; fi`, Name: "test if env set"},
		{Cmd: "doesnotexit", Name: "test doesnotexit err"},
		{Cmd: "sleep 1", Name: "test sleep 1"},
		{Cmd: "echo upload artifacts", Name: "sd-teardown-artifacts"},
	}
	testBuild := screwdriver.Build{
		ID:          12345,
		Commands:    commands,
		Environment: map[string]string{},
	}
	runTeardown := false
	testAPI := screwdriver.API(MockAPI{
		updateStepStart: func(buildID int, stepName string) error {
			if buildID != testBuild.ID {
				t.Errorf("wrong build id got %v, want %v", buildID, testBuild.ID)
			}
			if stepName == "test sleep 1" {
				t.Errorf("step should never execute: %v", stepName)
			}
			if stepName == "sd-teardown-artifacts" {
				runTeardown = true
			}
			return nil
		},
		updateStepStop: func(buildID int, stepName string, code int) error {
			if buildID != testBuild.ID {
				t.Errorf("wrong build id got %v, want %v", buildID, testBuild.ID)
			}
			if stepName == "test sleep 1" {
				t.Errorf("Should not update step that never run: %v", stepName)
			}
			if stepName == "sd-teardown-artifacts" {
				runTeardown = true
			}
			return nil
		},
	})
	err := Run("", nil, &MockEmitter{}, testBuild, testAPI, testBuild.ID, "/bin/sh", TestBuildTimeout)
	expectedErr := fmt.Errorf("Launching command exit with code: %v", 127)
	if !runTeardown {
		t.Errorf("step teardown should run")
	}
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("Unexpected error: %v - should be %v", err, expectedErr)
	}
}

func TestTeardownfail(t *testing.T) {
	commands := []screwdriver.CommandDef{
		{Cmd: "ls", Name: "test ls"},
		{Cmd: "doesnotexit", Name: "sd-teardown-artifacts"},
	}
	testBuild := screwdriver.Build{
		ID:          12345,
		Commands:    commands,
		Environment: map[string]string{},
	}
	testAPI := screwdriver.API(MockAPI{
		updateStepStart: func(buildID int, stepName string) error {
			if buildID != testBuild.ID {
				t.Errorf("wrong build id got %v, want %v", buildID, testBuild.ID)
			}
			return nil
		},
	})
	err := Run("", nil, &MockEmitter{}, testBuild, testAPI, testBuild.ID, "/bin/sh", TestBuildTimeout)
	expectedErr := ErrStatus{127}
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("Unexpected error: %v - should be %v", err, expectedErr)
	}
}

func TestTimeout(t *testing.T) {
	commands := []screwdriver.CommandDef{
		{Cmd: "echo testing timeout", Name: "test timeout"},
		{Cmd: "sleep 3", Name: "sleep for a long time"},
		{Cmd: "echo woke up to snooze", Name: "snooze"},
		{Cmd: "sleep 3", Name: "sleep for another long time"},
		{Cmd: "exit 0", Name: "completed"},
	}
	testBuild := screwdriver.Build{
		ID:          12345,
		Commands:    commands,
		Environment: map[string]string{},
	}
	testAPI := screwdriver.API(MockAPI{
		updateStepStart: func(buildID int, stepName string) error {
			return nil
		},
		updateStepStop: func(buildID int, stepName string, code int) error {
			if stepName == "completed" {
				t.Errorf("Should not update step that never run: %v", stepName)
			}
			return nil
		},
	})
	emitter := MockEmitter{
		startCmd: func(cmd screwdriver.CommandDef) {
			if cmd.Cmd == "sleep 3" {
				time.Sleep(10000)
			}
			return
		},
	}
	testTimeout := 3
	err := Run("", nil, &emitter, testBuild, testAPI, testBuild.ID, "/bin/sh", testTimeout)
	expectedErr := fmt.Errorf("Timeout of %vs seconds exceeded", testTimeout)
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("Unexpected error: %v - should be %v", err, expectedErr)
	}
}

func TestEnv(t *testing.T) {
	baseEnv := []string{
		"var0=xxx",
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
	err := Run("", baseEnv, &output, testBuild, testAPI, testBuild.ID, "/bin/sh", TestBuildTimeout)
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
			// Capture that "$ env" output line
			if strings.HasPrefix(line, "$") {
				foundCmd = line
			}
			continue
		}
		found[split[0]] = split[1]
	}

	if foundCmd != "$ env" {
		t.Errorf("foundCmd = %q, want %q", foundCmd, "env")
	}

	for k, v := range want {
		if found[k] != v {
			t.Errorf("%v=%q, want %v", k, found[k], v)
		}
	}
}

func TestEmitter(t *testing.T) {
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

	err := Run("", nil, &emitter, testBuild, testAPI, testBuild.ID, "/bin/sh", TestBuildTimeout)

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
