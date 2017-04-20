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

	"github.com/screwdriver-cd/launcher/screwdriver"
)

var stepFilePath = "/tmp/step.sh"

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

func (f MockAPI) UpdateBuildStatus(status screwdriver.BuildStatus, meta map[string]interface{}, buildID int) error {
	return nil
}

func (f MockAPI) SecretsForBuild(build screwdriver.Build) (screwdriver.Secrets, error) {
	return nil, nil
}

func (f MockAPI) GetAPIURL() (string, error) {
	return "http://foo.bar", nil
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

func ReadCommand(file string) string {
	// read the file that was written for the command
	fileread, err := ioutil.ReadFile(file)
	if err != nil {
		panic(fmt.Errorf("Couldn't read file: %v", err))
	}
	return strings.Split(string(fileread), "\n")[1] // expected input: "#!/bin/sh -e\n<COMMAND>"
}

func TestUnmocked(t *testing.T) {
	var tests = []struct {
		command string
		err     error
	}{
		{"ls", nil},
		{"sleep 1", nil},
		{"ls && ls ", nil},
		{"doesntexist", fmt.Errorf("Launching command exit with code: %v", 127)},
		{"ls && sh -c 'exit 5' && sh -c 'exit 2'", fmt.Errorf("Launching command exit with code: %v", 5)},
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
		err := Run("", nil, &MockEmitter{}, testBuild, testAPI, testBuild.ID)
		command := ReadCommand(stepFilePath)

		if !reflect.DeepEqual(err, test.err) {
			t.Fatalf("Unexpected error: (%v) - should be (%v)", err, test.err)
		}

		if !reflect.DeepEqual(command, test.command) {
			t.Errorf("Unexpected command from Run(%#v): %v", tests, command)
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
			if stepName == "test sleep 1" {
				t.Errorf("step should never execute: %v", stepName)
			}
			return nil
		},
	})
	err := Run("", nil, &MockEmitter{}, testBuild, testAPI, testBuild.ID)
	expectedErr := fmt.Errorf("Launching command exit with code: %v", 127)
	if !reflect.DeepEqual(err, expectedErr) {
		t.Fatalf("Unexpected error: %v - should be %v", err, expectedErr)
	}
}

func TestAlwaysRun(t *testing.T) {
	commands := []screwdriver.CommandDef{
		{Cmd: "doesnotexit", Name: "test doesnotexit err"},
		{Cmd: "sleep 1", Name: "test sleep 1", AlwaysRun: false},
		{Cmd: "echo hello", Name: "test alwaysRun", AlwaysRun: true},
	}
	testBuild := screwdriver.Build{
		ID:          12345,
		Commands:    commands,
		Environment: map[string]string{},
	}
	testAPI := screwdriver.API(MockAPI{
		updateStepStart: func(buildID int, stepName string) error {
			if stepName == "test sleep 1" {
				t.Errorf("step should never execute: %v", stepName)
			}
			if stepName == "test alwaysRun" {
				fmt.Printf("Testing alwaysRun: step \"%v\" successfully ran despite an earlier step failing.\n", stepName)
				t.SkipNow()
			}
			return nil
		},
	})
	Run("", nil, &MockEmitter{}, testBuild, testAPI, testBuild.ID)
	t.Fatalf("Always Run step did not run")
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
			Cmd:       "env",
			Name:      "test",
			AlwaysRun: false,
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
	err := Run("", baseEnv, &output, testBuild, testAPI, testBuild.ID)
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

	err := Run("", nil, &emitter, testBuild, testAPI, testBuild.ID)

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
