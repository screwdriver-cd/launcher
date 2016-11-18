package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/screwdriver-cd/launcher/executor"
	"github.com/screwdriver-cd/launcher/screwdriver"
)

const (
	TestWorkspace  = "/sd/workspace"
	TestEmitter    = "./data/emitter"
	TestBuildID    = "BUILDID"
	TestJobID      = "JOBID"
	TestPipelineID = "PIPELINEID"

	TestScmURI = "github.com:123456:master"
	TestSHA    = "abc123"
)

var TestScmRepo = screwdriver.ScmRepo(FakeScmRepo{
	Name: "screwdriver-cd/launcher",
})

type FakeBuild screwdriver.Build
type FakeJob screwdriver.Job
type FakePipeline screwdriver.Pipeline
type FakeScmRepo screwdriver.ScmRepo

func mockAPI(t *testing.T, testBuildID, testJobID, testPipelineID string, testStatus screwdriver.BuildStatus) MockAPI {
	return MockAPI{
		buildFromID: func(buildID string) (screwdriver.Build, error) {
			return screwdriver.Build(FakeBuild{ID: testBuildID, JobID: testJobID}), nil
		},
		jobFromID: func(jobID string) (screwdriver.Job, error) {
			if jobID != testJobID {
				t.Errorf("jobID == %s, want %s", jobID, testJobID)
				// Panic to get the stacktrace
				panic(true)
			}
			return screwdriver.Job(FakeJob{ID: testJobID, PipelineID: testPipelineID, Name: "main"}), nil
		},
		pipelineFromID: func(pipelineID string) (screwdriver.Pipeline, error) {
			if pipelineID != testPipelineID {
				t.Errorf("pipelineID == %s, want %s", pipelineID, testPipelineID)
				// Panic to get the stacktrace
				panic(true)
			}
			return screwdriver.Pipeline(FakePipeline{ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
		},
		updateBuildStatus: func(status screwdriver.BuildStatus, buildID string) error {
			if buildID != testBuildID {
				t.Errorf("status == %s, want %s", status, testStatus)
				// Panic to get the stacktrace
				panic(true)
			}
			if status != testStatus {
				t.Errorf("status == %s, want %s", status, testStatus)
				// Panic to get the stacktrace
				panic(true)
			}
			return nil
		},
	}
}

type MockAPI struct {
	buildFromID       func(string) (screwdriver.Build, error)
	jobFromID         func(string) (screwdriver.Job, error)
	pipelineFromID    func(string) (screwdriver.Pipeline, error)
	updateBuildStatus func(screwdriver.BuildStatus, string) error
	updateStepStart   func(buildID, stepName string) error
	updateStepStop    func(buildID, stepName string, exitCode int) error
	secretsForBuild   func(build screwdriver.Build) (screwdriver.Secrets, error)
}

func (f MockAPI) SecretsForBuild(build screwdriver.Build) (screwdriver.Secrets, error) {
	if f.secretsForBuild != nil {
		return f.secretsForBuild(build)
	}
	return nil, nil
}

func (f MockAPI) BuildFromID(buildID string) (screwdriver.Build, error) {
	if f.buildFromID != nil {
		return f.buildFromID(buildID)
	}
	return screwdriver.Build(FakeBuild{}), nil
}

func (f MockAPI) JobFromID(jobID string) (screwdriver.Job, error) {
	if f.jobFromID != nil {
		return f.jobFromID(jobID)
	}
	return screwdriver.Job(FakeJob{}), nil
}

func (f MockAPI) PipelineFromID(pipelineID string) (screwdriver.Pipeline, error) {
	if f.pipelineFromID != nil {
		return f.pipelineFromID(pipelineID)
	}
	return screwdriver.Pipeline(FakePipeline{}), nil
}

func (f MockAPI) UpdateBuildStatus(status screwdriver.BuildStatus, buildID string) error {
	if f.updateBuildStatus != nil {
		return f.updateBuildStatus(status, buildID)
	}
	return nil
}

func (f MockAPI) UpdateStepStart(buildID, stepName string) error {
	if f.updateStepStart != nil {
		return f.updateStepStart(buildID, stepName)
	}
	return nil
}

func (f MockAPI) UpdateStepStop(buildID, stepName string, exitCode int) error {
	if f.updateStepStop != nil {
		return f.updateStepStop(buildID, stepName, exitCode)
	}
	return nil
}

type MockEmitter struct {
	startCmd func(screwdriver.CommandDef)
	write    func([]byte) (int, error)
	close    func() error
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
	return len(b), nil
}

func (e *MockEmitter) Close() error {
	if e.close != nil {
		return e.close()
	}
	return nil
}

func setupTempDirectoryAndSocket(t *testing.T) (dir string, cleanup func()) {
	tmp, err := ioutil.TempDir("", "ArtifactDir")
	if err != nil {
		t.Fatalf("Couldn't create temp dir: %v", err)
	}

	emitterPath := path.Join(tmp, "socket")
	if _, err = os.Create(emitterPath); err != nil {
		t.Fatalf("Error creating test socket: %v", err)
	}
	return tmp, func() {
		os.RemoveAll(tmp)
	}
}

func TestMain(m *testing.M) {
	mkdirAll = func(path string, perm os.FileMode) (err error) { return nil }
	stat = func(path string) (info os.FileInfo, err error) { return nil, os.ErrExist }
	open = func(f string) (*os.File, error) {
		return os.Open("data/screwdriver.yaml")
	}
	executorRun = func(path string, env []string, emitter screwdriver.Emitter, build screwdriver.Build, api screwdriver.API, buildID string) error {
		return nil
	}
	cleanExit = func() {}
	writeFile = func(string, []byte, os.FileMode) error { return nil }
	os.Exit(m.Run())
}

func TestBuildJobPipelineFromID(t *testing.T) {
	testPipelineID := "PIPELINEID"
	api := mockAPI(t, TestBuildID, TestJobID, testPipelineID, "RUNNING")
	launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter)
}

func TestBuildFromIdError(t *testing.T) {
	api := MockAPI{
		buildFromID: func(buildID string) (screwdriver.Build, error) {
			err := fmt.Errorf("testing error returns")
			return screwdriver.Build(FakeBuild{}), err
		},
	}

	err := launch(screwdriver.API(api), "shoulderror", TestWorkspace, TestEmitter)
	if err == nil {
		t.Errorf("err should not be nil")
	}

	expected := `fetching build ID "shoulderror"`
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestJobFromIdError(t *testing.T) {
	api := mockAPI(t, TestBuildID, TestJobID, "", "RUNNING")
	api.jobFromID = func(jobID string) (screwdriver.Job, error) {
		err := fmt.Errorf("testing error returns")
		return screwdriver.Job(FakeJob{}), err
	}

	err := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter)
	if err == nil {
		t.Errorf("err should not be nil")
	}

	expected := fmt.Sprintf(`fetching Job ID %q`, TestJobID)
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestPipelineFromIdError(t *testing.T) {
	testPipelineID := "PIPELINEID"
	api := mockAPI(t, TestBuildID, TestJobID, testPipelineID, "RUNNING")
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		err := fmt.Errorf("testing error returns")
		return screwdriver.Pipeline(FakePipeline{}), err
	}

	err := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter)
	if err == nil {
		t.Fatalf("err should not be nil")
	}

	expected := fmt.Sprintf(`fetching Pipeline ID %q`, testPipelineID)
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestParseScmURI(t *testing.T) {
	wantHost := "github.com"
	wantOrg := "screwdriver-cd"
	wantRepo := "launcher"
	wantBranch := "master"

	scmURI := "github.com:123456:master"
	scmName := "screwdriver-cd/launcher"
	parsedURL, err := parseScmURI(scmURI, scmName)
	host, org, repo, branch := parsedURL.Host, parsedURL.Org, parsedURL.Repo, parsedURL.Branch
	if err != nil {
		t.Errorf("Unexpected error parsing SCM URI %q: %v", scmURI, err)
	}

	if host != wantHost {
		t.Errorf("host = %q, want %q", host, wantHost)
	}

	if org != wantOrg {
		t.Errorf("org = %q, want %q", org, wantOrg)
	}

	if repo != wantRepo {
		t.Errorf("repo = %q, want %q", repo, wantRepo)
	}

	if branch != wantBranch {
		t.Errorf("branch = %q, want %q", branch, wantBranch)
	}
}

func TestCreateWorkspace(t *testing.T) {
	oldMkdir := mkdirAll
	defer func() { mkdirAll = oldMkdir }()

	madeDirs := map[string]os.FileMode{}
	mkdirAll = func(path string, perm os.FileMode) (err error) {
		madeDirs[path] = perm
		return nil
	}

	workspace, err := createWorkspace(TestWorkspace, "screwdriver-cd", "launcher")

	if err != nil {
		t.Errorf("Unexpected error creating workspace: %v", err)
	}

	wantWorkspace := Workspace{
		Root:      TestWorkspace,
		Src:       "/sd/workspace/src/screwdriver-cd/launcher",
		Artifacts: "/sd/workspace/artifacts",
	}
	if workspace != wantWorkspace {
		t.Errorf("workspace = %q, want %q", workspace, wantWorkspace)
	}

	wantDirs := map[string]os.FileMode{
		"/sd/workspace/src/screwdriver-cd/launcher": 0777,
		"/sd/workspace/artifacts":                   0777,
	}
	for d, p := range wantDirs {
		if _, ok := madeDirs[d]; !ok {
			t.Errorf("Directory %s not created. Made: %v", d, madeDirs)
		} else {
			if perm := madeDirs[d]; perm != p {
				t.Errorf("Directory %s permissions %v, want %v", d, perm, p)
			}
		}
	}
}

func TestPRNumber(t *testing.T) {
	testJobName := "PR-1"
	wantPrNumber := "1"

	prNumber := prNumber(testJobName)
	if prNumber != wantPrNumber {
		t.Errorf("prNumber == %q, want %q", prNumber, wantPrNumber)
	}
}

func TestCreateWorkspaceError(t *testing.T) {
	oldMkdir := mkdirAll
	defer func() { mkdirAll = oldMkdir }()

	api := mockAPI(t, TestBuildID, TestJobID, "", "RUNNING")
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	mkdirAll = func(path string, perm os.FileMode) (err error) {
		return fmt.Errorf("Spooky error")
	}

	err := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter)

	if err.Error() != "Cannot create workspace path \"/sd/workspace/src/github.com/screwdriver-cd/launcher\": Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestCreateWorkspaceBadStat(t *testing.T) {
	oldStat := stat
	defer func() { stat = oldStat }()

	stat = func(path string) (info os.FileInfo, err error) {
		return nil, nil
	}

	wantWorkspace := Workspace{}

	workspace, err := createWorkspace(TestWorkspace, "screwdriver-cd", "launcher")

	if err.Error() != "Cannot create workspace path \"/sd/workspace/src/screwdriver-cd/launcher\", path already exists." {
		t.Errorf("Error is wrong, got %v", err)
	}

	if workspace != wantWorkspace {
		t.Errorf("Workspace == %q, want %q", workspace, wantWorkspace)
	}
}

func TestUpdateBuildStatusError(t *testing.T) {
	api := mockAPI(t, TestBuildID, "", "", screwdriver.Running)
	api.updateBuildStatus = func(status screwdriver.BuildStatus, buildID string) error {
		return fmt.Errorf("Spooky error")
	}

	err := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter)

	want := "updating build status to RUNNING: Spooky error"
	if err.Error() != want {
		t.Errorf("Error is wrong. got %v, want %v", err, want)
	}
}

func TestUpdateBuildStatusSuccess(t *testing.T) {
	wantStatuses := []screwdriver.BuildStatus{
		screwdriver.Running,
		screwdriver.Success,
	}

	var gotStatuses []screwdriver.BuildStatus
	api := mockAPI(t, "TestBuildID", "TestJobID", "testPipelineID", "")
	api.updateBuildStatus = func(status screwdriver.BuildStatus, buildID string) error {
		gotStatuses = append(gotStatuses, status)
		return nil
	}

	oldMkdirAll := mkdirAll
	defer func() { mkdirAll = oldMkdirAll }()
	mkdirAll = os.MkdirAll

	tmp, cleanup := setupTempDirectoryAndSocket(t)
	defer cleanup()

	if err := launchAction(screwdriver.API(api), "TestBuildID", tmp, path.Join(tmp, "socket")); err != nil {
		t.Errorf("Unexpected error from launch: %v", err)
	}

	if !reflect.DeepEqual(gotStatuses, wantStatuses) {
		t.Errorf("Set statuses %q, want %q", gotStatuses, wantStatuses)
	}
}

func TestUpdateBuildNonZeroFailure(t *testing.T) {
	wantStatuses := []screwdriver.BuildStatus{
		screwdriver.Running,
		screwdriver.Failure,
	}

	var gotStatuses []screwdriver.BuildStatus
	api := mockAPI(t, "TestBuildID", "TestJobID", "testPipelineID", "")
	api.updateBuildStatus = func(status screwdriver.BuildStatus, buildID string) error {
		gotStatuses = append(gotStatuses, status)
		return nil
	}

	oldMkdirAll := mkdirAll
	defer func() { mkdirAll = oldMkdirAll }()
	mkdirAll = os.MkdirAll
	tmp, err := ioutil.TempDir("", "ArtifactDir")
	if err != nil {
		t.Fatalf("Couldn't create temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)

	oldRun := executorRun
	defer func() { executorRun = oldRun }()
	executorRun = func(path string, env []string, out screwdriver.Emitter, build screwdriver.Build, a screwdriver.API, buildID string) error {
		return executor.ErrStatus{Status: 1}
	}

	err = launchAction(screwdriver.API(api), "TestBuildID", tmp, TestEmitter)
	if err != nil {
		t.Errorf("Unexpected error from launch: %v", err)
	}

	if !reflect.DeepEqual(gotStatuses, wantStatuses) {
		t.Errorf("Set statuses %q, want %q", gotStatuses, wantStatuses)
	}
}

func TestWriteCommandArtifact(t *testing.T) {
	sdCommand := []screwdriver.CommandDef{
		{
			Name: "install",
			Cmd:  "npm install",
		},
	}
	var sdCommandUnmarshal []screwdriver.CommandDef
	fName := "steps.json"

	tmp, err := ioutil.TempDir("", "ArtifactDir")
	if err != nil {
		t.Fatalf("Couldn't create temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)

	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()
	writeFile = ioutil.WriteFile

	err = writeArtifact(tmp, fName, sdCommand)
	if err != nil {
		t.Errorf("Expected error to be nil: %v", err)
	}

	filePath := path.Join(tmp, fName)

	if _, err = os.Stat(filePath); err != nil {
		t.Fatalf("file not found: %s", err)
	}

	fileContents, err := ioutil.ReadFile(filePath)

	if err != nil {
		t.Fatalf("reading file error: %v", err)
	}

	err = json.Unmarshal(fileContents, &sdCommandUnmarshal)

	if err != nil {
		t.Fatalf("unmarshalling file contents: %v", err)
	}

	if !reflect.DeepEqual(sdCommand, sdCommandUnmarshal) {
		t.Fatalf("Did not write file correctly. Wanted %v. Got %v", sdCommand, sdCommandUnmarshal)
	}
}

func TestWriteEnvironmentArtifact(t *testing.T) {
	sdEnv := map[string]string{
		"NUMBER":  "3",
		"NUMBER1": "4",
		"BOOL":    "false",
	}
	var sdEnvUnmarshal map[string]string
	fName := "environment.json"

	tmp, err := ioutil.TempDir("", "ArtifactDir")
	if err != nil {
		t.Fatalf("Couldn't create temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)

	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()
	writeFile = ioutil.WriteFile

	err = writeArtifact(tmp, fName, sdEnv)
	if err != nil {
		t.Fatalf("Expected error to be nil: %v", err)
	}

	filePath := path.Join(tmp, fName)

	if _, err = os.Stat(filePath); err != nil {
		t.Fatalf("file not found: %s", err)
	}

	fileContents, err := ioutil.ReadFile(filePath)

	if err != nil {
		t.Fatalf("reading file error: %v", err)
	}

	err = json.Unmarshal(fileContents, &sdEnvUnmarshal)

	if err != nil {
		t.Fatalf("unmarshalling file contents: %v", err)
	}

	if !reflect.DeepEqual(sdEnv, sdEnvUnmarshal) {
		t.Fatalf("Did not write file correctly. Wanted %v. Got %v", sdEnv, sdEnvUnmarshal)
	}
}

func TestRecoverPanic(t *testing.T) {
	api := mockAPI(t, "TestBuildID", "", "", screwdriver.Running)

	updCalled := false
	api.updateBuildStatus = func(status screwdriver.BuildStatus, buildID string) error {
		updCalled = true
		fmt.Printf("Status set: %v\n", status)
		if status != screwdriver.Failure {
			t.Errorf("Status set to %v, want %v", status, screwdriver.Failure)
		}
		return nil
	}

	exitCalled := false
	cleanExit = func() {
		exitCalled = true
	}

	func() {
		defer recoverPanic("", api)
		panic("OH NOES!")
	}()

	if !updCalled {
		t.Errorf("Build status not updated to FAILURE")
	}

	if !exitCalled {
		t.Errorf("Explicit exit not called")
	}
}

func TestRecoverPanicNoAPI(t *testing.T) {
	exitCalled := false
	cleanExit = func() {
		exitCalled = true
	}

	func() {
		defer recoverPanic("", nil)
		panic("OH NOES!")
	}()

	if !exitCalled {
		t.Errorf("Explicit exit not called")
	}
}

func TestEmitterClose(t *testing.T) {
	api := mockAPI(t, "TestBuildID", "TestJobID", "testPipelineID", "")
	api.updateBuildStatus = func(status screwdriver.BuildStatus, buildID string) error {
		return nil
	}

	called := false

	newEmitter = func(path string) (screwdriver.Emitter, error) {
		return &MockEmitter{
			close: func() error {
				called = true
				return nil
			},
		}, nil
	}

	if err := launchAction(screwdriver.API(api), "TestBuildID", TestWorkspace, TestEmitter); err != nil {
		t.Errorf("Unexpected error from launch: %v", err)
	}

	if !called {
		t.Errorf("Did not close the emitter")
	}
}

func TestSetEnv(t *testing.T) {
	oldExecutorRun := executorRun
	defer func() { executorRun = oldExecutorRun }()

	tests := map[string]string{
		"SCREWDRIVER": "true",
		"CI":          "true",
		"CONTINUOUS_INTEGRATION": "true",
		"SD_JOB_NAME":            "PR-1",
		"SD_PULL_REQUEST":        "1",
		"SD_SOURCE_DIR":          "/sd/workspace/src/github.com/screwdriver-cd/launcher",
		"SD_ARTIFACTS_DIR":       "/sd/workspace/artifacts",
	}

	api := mockAPI(t, TestBuildID, TestJobID, "", "RUNNING")
	api.jobFromID = func(jobID string) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{Name: "PR-1"}), nil
	}

	foundEnv := map[string]string{}
	executorRun = func(path string, env []string, emitter screwdriver.Emitter, build screwdriver.Build, api screwdriver.API, buildID string) error {
		if len(env) == 0 {
			t.Fatalf("Unexpected empty environment passed to executorRun")
		}

		for _, e := range env {
			split := strings.Split(e, "=")
			if len(split) != 2 {
				t.Fatalf("Bad environment value passed to executorRun: %s", e)
			}
			foundEnv[split[0]] = split[1]
		}

		return nil
	}

	err := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter)
	if err != nil {
		t.Fatalf("Unexpected error from launch: %v", err)
	}
	for k, v := range tests {
		if foundEnv[k] != v {
			t.Fatalf("foundEnv[%s] = %s, want %s", k, foundEnv[k], v)
			// t.Errorf("Invalid value for environment variable %v: got %v, want %v", test.key, value, test.value)
		}
	}
}

func TestEnvSecrets(t *testing.T) {
	api := mockAPI(t, TestBuildID, TestJobID, "", "RUNNING")
	api.jobFromID = func(jobID string) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{Name: "PR-1"}), nil
	}

	testBuild := FakeBuild{ID: TestBuildID, JobID: TestJobID}
	api.secretsForBuild = func(build screwdriver.Build) (screwdriver.Secrets, error) {
		if !reflect.DeepEqual(build.ID, testBuild.ID) {
			t.Errorf("build.ID = %v, want %v", build.ID, testBuild.ID)
		}

		testSecrets := screwdriver.Secrets{
			{Name: "FOONAME", Value: "barvalue"},
		}
		return testSecrets, nil
	}

	foundEnv := map[string]string{}
	oldExecutorRun := executorRun
	defer func() { executorRun = oldExecutorRun }()
	executorRun = func(path string, env []string, emitter screwdriver.Emitter, build screwdriver.Build, api screwdriver.API, buildID string) error {
		if len(env) == 0 {
			t.Fatalf("Unexpected empty environment passed to executorRun")
		}

		for _, e := range env {
			split := strings.Split(e, "=")
			if len(split) != 2 {
				t.Fatalf("Bad environment value passed to executorRun: %s", e)
			}
			foundEnv[split[0]] = split[1]
		}

		return nil
	}

	err := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter)
	if err != nil {
		t.Fatalf("Unexpected error from launch: %v", err)
	}

	if foundEnv["FOONAME"] != "barvalue" {
		t.Errorf("secret not set in environment %v, want FOONAME=barvalue", foundEnv)
	}
}

func TestCreateEnvironment(t *testing.T) {
	os.Setenv("OSENVWITHEQUALS", "foo=bar=")
	base := map[string]string{
		"FOO":             "bar",
		"THINGWITHEQUALS": "abc=def",
		"GETSOVERRIDDEN":  "goesaway",
	}

	secrets := screwdriver.Secrets{
		{Name: "secret1", Value: "secret1value"},
		{Name: "GETSOVERRIDDEN", Value: "override"},
	}

	buildEnv := map[string]string{
		"GOPATH": "/go/path",
	}

	testBuild := screwdriver.Build{
		ID:          "build",
		Environment: buildEnv,
	}
	env := createEnvironment(base, secrets, testBuild)

	foundEnv := map[string]bool{}
	for _, i := range env {
		foundEnv[i] = true
	}

	for _, want := range []string{
		"FOO=bar",
		"THINGWITHEQUALS=abc=def",
		"secret1=secret1value",
		"GETSOVERRIDDEN=override",
		"OSENVWITHEQUALS=foo=bar=",
		"GOPATH=/go/path",
	} {
		if !foundEnv[want] {
			t.Errorf("Did not receive expected environment setting %q", want)
		}
	}

	if foundEnv["GETSOVERRIDDEN=goesaway"] {
		t.Errorf("Failed to override the base environment with a secret")
	}
}
