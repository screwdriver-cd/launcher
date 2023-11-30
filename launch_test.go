package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"path"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/screwdriver-cd/launcher/executor"
	"github.com/screwdriver-cd/launcher/screwdriver"
	"github.com/stretchr/testify/assert"
)

const (
	TestWorkspace        = "/sd/workspace"
	TestEmitter          = "./data/emitter"
	TestBuildID          = 1234
	TestBuildTimeout     = 60
	TestEventID          = 2234
	TestJobID            = 2345
	TestParentBuildID    = 1111
	TestParentEventID    = 3345
	TestPipelineID       = 3456
	TestParentJobID      = 1112
	TestParentPipelineID = 1113
	TestMetaSpace        = "./data/meta"
	TestShellBin         = "/bin/sh"
	TestStoreURL         = "https://store.screwdriver.cd"
	TestUIURL            = "https://screwdriver.cd"
	TestBuildToken       = "foobar"
	TestBuildClusterName = "test-build-cluster-name"

	TestScmURI = "github.com:123456:master"
	TestSHA    = "abc123"
)

var TestParentBuildIDFloat interface{} = float64(1111)
var TestParentBuildIDs = []float64{1111, 2222}
var IDs = make([]interface{}, len(TestParentBuildIDs))
var actual = make(map[string]interface{})
var TestEnvVars = make(map[string]interface{})
var TestScmRepo = screwdriver.ScmRepo(FakeScmRepo{
	Name: "screwdriver-cd/launcher",
})
var TestEventCreator = map[string]string{"username": "stjohn"}

type FakeBuild screwdriver.Build
type FakeCoverage screwdriver.Coverage
type FakeEvent screwdriver.Event
type FakeJob screwdriver.Job
type FakePipeline screwdriver.Pipeline
type FakeScmRepo screwdriver.ScmRepo

func initCoverageMeta() {
	TestEnvVars = map[string]interface{}{
		"SD_SONAR_AUTH_URL":    "https://api.screwdriver.cd/v4/coverage/token",
		"SD_SONAR_HOST":        "https://sonar.screwdriver.cd",
		"SD_SONAR_ENTERPRISE":  false,
		"SD_SONAR_PROJECT_KEY": "job:fake",
	}
}

func mockAPI(t *testing.T, testBuildID, testJobID, testPipelineID int, testStatus screwdriver.BuildStatus) MockAPI {
	return MockAPI{
		buildFromID: func(buildID int) (screwdriver.Build, error) {
			return screwdriver.Build(FakeBuild{ID: testBuildID, EventID: TestEventID, JobID: testJobID, SHA: TestSHA, ParentBuildID: float64(1234)}), nil
		},
		eventFromID: func(eventID int) (screwdriver.Event, error) {
			return screwdriver.Event(FakeEvent{ID: TestEventID, ParentEventID: TestParentEventID, Creator: TestEventCreator}), nil
		},
		jobFromID: func(jobID int) (screwdriver.Job, error) {
			if jobID != testJobID {
				t.Errorf("jobID == %d, want %d", jobID, testJobID)
				// Panic to get the stacktrace
				panic(true)
			}
			return screwdriver.Job(FakeJob{ID: testJobID, PipelineID: testPipelineID, Name: "main"}), nil
		},
		pipelineFromID: func(pipelineID int) (screwdriver.Pipeline, error) {
			if pipelineID != testPipelineID {
				t.Errorf("pipelineID == %d, want %d", pipelineID, testPipelineID)
				// Panic to get the stacktrace
				panic(true)
			}
			return screwdriver.Pipeline(FakePipeline{ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
		},
		updateBuildStatus: func(status screwdriver.BuildStatus, meta map[string]interface{}, buildID int, statusMessage string) error {
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
		getAPIURL: func() (string, error) {
			return "https://api.screwdriver.cd/v4/", nil
		},
		getCoverageInfo: func(jobID, pipelineID int, jobName, pipelineName, scope, prNum, prParentJobId string) (screwdriver.Coverage, error) {
			return screwdriver.Coverage(FakeCoverage{EnvVars: TestEnvVars}), nil
		},
		getBuildToken: func(buildID int, buildTimeoutMinutes int) (string, error) {
			if buildID != testBuildID {
				t.Errorf("buildID == %d, want %d", buildID, testBuildID)
				// Panic to get the stacktrace
				panic(true)
			}
			if buildTimeoutMinutes != TestBuildTimeout {
				t.Errorf("buildTimeout == %d, want %d", buildTimeoutMinutes, TestBuildTimeout)
				// Panic to get the stacktrace
				panic(true)
			}
			return "foobar", nil
		},
	}
}

type MockAPI struct {
	buildFromID       func(int) (screwdriver.Build, error)
	eventFromID       func(int) (screwdriver.Event, error)
	jobFromID         func(int) (screwdriver.Job, error)
	pipelineFromID    func(int) (screwdriver.Pipeline, error)
	updateBuildStatus func(screwdriver.BuildStatus, map[string]interface{}, int, string) error
	updateStepStart   func(buildID int, stepName string) error
	updateStepStop    func(buildID int, stepName string, exitCode int) error
	secretsForBuild   func(build screwdriver.Build) (screwdriver.Secrets, error)
	getAPIURL         func() (string, error)
	getCoverageInfo   func(jobID, pipelineID int, jobName, pipelineName, scope, prNum, prParentJobId string) (screwdriver.Coverage, error)
	getBuildToken     func(buildID int, buildTimeoutMinutes int) (string, error)
}

func (f MockAPI) GetAPIURL() (string, error) {
	return f.getAPIURL()
}

func (f MockAPI) GetCoverageInfo(jobID, pipelineID int, jobName, pipelineName, scope, prNum, prParentJobId string) (screwdriver.Coverage, error) {
	return f.getCoverageInfo(jobID, pipelineID, jobName, pipelineName, scope, prNum, prParentJobId)
}

func (f MockAPI) SecretsForBuild(build screwdriver.Build) (screwdriver.Secrets, error) {
	if f.secretsForBuild != nil {
		return f.secretsForBuild(build)
	}
	return nil, nil
}

func (f MockAPI) BuildFromID(buildID int) (screwdriver.Build, error) {
	if f.buildFromID != nil {
		return f.buildFromID(buildID)
	}
	return screwdriver.Build(FakeBuild{}), nil
}

func (f MockAPI) EventFromID(eventID int) (screwdriver.Event, error) {
	if f.eventFromID != nil {
		return f.eventFromID(eventID)
	}
	return screwdriver.Event(FakeEvent{}), nil
}

func (f MockAPI) JobFromID(jobID int) (screwdriver.Job, error) {
	if f.jobFromID != nil {
		return f.jobFromID(jobID)
	}
	return screwdriver.Job(FakeJob{}), nil
}

func (f MockAPI) PipelineFromID(pipelineID int) (screwdriver.Pipeline, error) {
	if f.pipelineFromID != nil {
		return f.pipelineFromID(pipelineID)
	}
	return screwdriver.Pipeline(FakePipeline{}), nil
}

func (f MockAPI) UpdateBuildStatus(status screwdriver.BuildStatus, meta map[string]interface{}, buildID int, statusMessage string) error {
	if f.updateBuildStatus != nil {
		return f.updateBuildStatus(status, nil, buildID, "")
	}
	return nil
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

func (f MockAPI) GetBuildToken(buildID int, buildTimeoutMinutes int) (string, error) {
	if f.getBuildToken != nil {
		return f.getBuildToken(buildID, buildTimeoutMinutes)
	}
	return "foobar", nil
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
	initCoverageMeta()

	mkdirAll = func(path string, perm os.FileMode) (err error) { return nil }
	stat = func(path string) (info os.FileInfo, err error) { return nil, os.ErrExist }
	open = func(f string) (*os.File, error) {
		return os.Open("data/screwdriver.yaml")
	}
	executorRun = func(path string, env []string, emitter screwdriver.Emitter, build screwdriver.Build, api screwdriver.API, buildID int, shellBin string, timeout int, envFilepath, sourceDir string) error {
		return nil
	}
	cleanExit = func() {}
	writeFile = func(string, []byte, os.FileMode) error { return nil }
	readFile = func(filename string) (data []byte, err error) { return nil, nil }
	unmarshal = func(data []byte, v interface{}) (err error) { return nil }
	os.Exit(m.Run())
}

func TestBuildJobPipelineFromID(t *testing.T) {
	testPipelineID := 9999
	api := mockAPI(t, TestBuildID, TestJobID, testPipelineID, "RUNNING")
	err, sourceDir, shellBin := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	if err != nil {
		t.Errorf("err should be nil")
	}

	expectSourceDir := "/sd/workspace/src/github.com/screwdriver-cd/launcher"
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := "/bin/sh"
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}
}

func TestBuildFromIdError(t *testing.T) {
	api := MockAPI{
		buildFromID: func(buildID int) (screwdriver.Build, error) {
			err := fmt.Errorf("testing error returns")
			return screwdriver.Build(FakeBuild{}), err
		},
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), 0, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	if err == nil {
		t.Errorf("err should not be nil")
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}

	expected := `Fetching Build ID 0`
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestEventFromIdError(t *testing.T) {
	api := MockAPI{
		eventFromID: func(eventID int) (screwdriver.Event, error) {
			err := fmt.Errorf("testing error returns")
			return screwdriver.Event(FakeEvent{}), err
		},
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), 0, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	if err == nil {
		t.Errorf("err should not be nil")
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}

	expected := `Fetching Event ID 0`
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestJobFromIdError(t *testing.T) {
	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		err := fmt.Errorf("testing error returns")
		return screwdriver.Job(FakeJob{}), err
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	if err == nil {
		t.Errorf("err should not be nil")
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}

	expected := fmt.Sprintf(`Fetching Job ID %d`, TestJobID)
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestPipelineFromIdError(t *testing.T) {
	testPipelineID := 9999
	api := mockAPI(t, TestBuildID, TestJobID, testPipelineID, "RUNNING")
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		err := fmt.Errorf("testing error returns")
		return screwdriver.Pipeline(FakePipeline{}), err
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	if err == nil {
		t.Fatalf("err should not be nil")
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}

	expected := fmt.Sprintf(`Fetching Pipeline ID %d`, testPipelineID)
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestParseScmURI(t *testing.T) {
	wantHost := "github.com"
	wantOrg := "screwdriver-cd"
	wantRepo := "launcher"
	wantBranch := "master"
	wantRootDir := "child:Directory"

	scmURI := "github.com:123456:master:child:Directory"
	scmName := "screwdriver-cd/launcher"
	parsedURL, err := parseScmURI(scmURI, scmName)
	host, org, repo, branch, rootDir := parsedURL.Host, parsedURL.Org, parsedURL.Repo, parsedURL.Branch, parsedURL.RootDir
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

	if rootDir != wantRootDir {
		t.Errorf("rootDir = %q, want %q", rootDir, wantRootDir)
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
	workspace, err := createWorkspace(false, TestWorkspace, "screwdriver-cd", "launcher")

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
	testJobName := "PR-1:main"
	wantPrNumber := "1"

	prNumber := prNumber(testJobName)
	if prNumber != wantPrNumber {
		t.Errorf("prNumber == %q, want %q", prNumber, wantPrNumber)
	}
}

func TestCreateWorkspaceError(t *testing.T) {
	oldMkdir := mkdirAll
	defer func() { mkdirAll = oldMkdir }()

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	mkdirAll = func(path string, perm os.FileMode) (err error) {
		return fmt.Errorf("Spooky error")
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		return nil
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)

	if err.Error() != "Cannot create meta-space path \"./data/meta\": Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}
}

func TestCreateWorkspaceBadStat(t *testing.T) {
	oldStat := stat
	defer func() { stat = oldStat }()

	stat = func(path string) (info os.FileInfo, err error) {
		return nil, nil
	}

	wantWorkspace := Workspace{}

	workspace, err := createWorkspace(false, TestWorkspace, "screwdriver-cd", "launcher")

	if err.Error() != "Cannot create workspace path \"/sd/workspace/src/screwdriver-cd/launcher\", path already exists." {
		t.Errorf("Error is wrong, got %v", err)
	}

	if workspace != wantWorkspace {
		t.Errorf("Workspace == %q, want %q", workspace, wantWorkspace)
	}
}

func TestCreateWorkspaceBadStatLocal(t *testing.T) {
	oldStat := stat
	defer func() { stat = oldStat }()

	stat = func(path string) (info os.FileInfo, err error) {
		return nil, nil
	}

	wantWorkspace := Workspace{
		Root:      TestWorkspace,
		Src:       "/sd/workspace/src/screwdriver-cd/launcher",
		Artifacts: "/sd/workspace/artifacts",
	}

	workspace, err := createWorkspace(true, TestWorkspace, "screwdriver-cd", "launcher")

	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}

	if workspace != wantWorkspace {
		t.Errorf("Workspace == %q, want %q", workspace, wantWorkspace)
	}
}

func TestUpdateBuildStatusError(t *testing.T) {
	api := mockAPI(t, TestBuildID, 0, 0, screwdriver.Running)
	api.updateBuildStatus = func(status screwdriver.BuildStatus, meta map[string]interface{}, buildID int, statusMessage string) error {
		return fmt.Errorf("Spooky error")
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)

	want := "Updating build status to RUNNING: Spooky error"
	if err.Error() != want {
		t.Errorf("Error is wrong. got %v, want %v", err, want)
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}
}

func TestUpdateBuildStatusSuccess(t *testing.T) {
	wantStatuses := []screwdriver.BuildStatus{
		screwdriver.Running,
		screwdriver.Success,
	}

	var gotStatuses []screwdriver.BuildStatus
	api := mockAPI(t, 1, 2, 3, "")
	api.updateBuildStatus = func(status screwdriver.BuildStatus, meta map[string]interface{}, buildID int, statusMessage string) error {
		gotStatuses = append(gotStatuses, status)
		return nil
	}

	oldMkdirAll := mkdirAll
	defer func() { mkdirAll = oldMkdirAll }()
	mkdirAll = os.MkdirAll

	tmp, cleanup := setupTempDirectoryAndSocket(t)
	defer cleanup()

	if err := launchAction(screwdriver.API(api), 0, tmp, path.Join(tmp, "socket"), TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000); err != nil {
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
	api := mockAPI(t, 1, 2, 3, "")
	api.updateBuildStatus = func(status screwdriver.BuildStatus, meta map[string]interface{}, buildID int, statusMessage string) error {
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
	executorRun = func(path string, env []string, out screwdriver.Emitter, build screwdriver.Build, a screwdriver.API, buildID int, shellBin string, timeout int, envFilepath, sourceDir string) error {
		return executor.ErrStatus{Status: 1}
	}

	err = launchAction(screwdriver.API(api), 1, tmp, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
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
	api := mockAPI(t, 1, 2, 3, screwdriver.Running)

	updCalled := false
	api.updateBuildStatus = func(status screwdriver.BuildStatus, meta map[string]interface{}, buildID int, statusMessage string) error {
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
		defer recoverPanic(0, api, TestMetaSpace)
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
		defer recoverPanic(0, nil, TestMetaSpace)
		panic("OH NOES!")
	}()

	if !exitCalled {
		t.Errorf("Explicit exit not called")
	}
}

func TestEmitterClose(t *testing.T) {
	api := mockAPI(t, 1, 2, 3, "")
	api.updateBuildStatus = func(status screwdriver.BuildStatus, meta map[string]interface{}, buildID int, statusMessage string) error {
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

	if err := launchAction(screwdriver.API(api), 1, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000); err != nil {
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
		"PS1":                    "",
		"SCREWDRIVER":            "true",
		"CI":                     "true",
		"CONTINUOUS_INTEGRATION": "true",
		"SD_JOB_NAME":            "PR-1",
		"SD_PIPELINE_NAME":       "screwdriver-cd/launcher",
		"SD_BUILD_ID":            "1234",
		"SD_JOB_ID":              "2345",
		"SD_EVENT_ID":            "2234",
		"SD_PIPELINE_ID":         "3456",
		"SD_PARENT_BUILD_ID":     "[1234]",
		"SD_PR_PARENT_JOB_ID":    "111",
		"SD_PARENT_EVENT_ID":     "3345",
		"SD_CHECKOUT_DIR":        "/sd/workspace/src/github.com/screwdriver-cd/launcher",
		"SD_SOURCE_DIR":          "/sd/workspace/src/github.com/screwdriver-cd/launcher",
		"SD_ROOT_DIR":            "/sd/workspace",
		"SD_ARTIFACTS_DIR":       "/sd/workspace/artifacts",
		"SD_META_DIR":            "./data/meta",
		"SD_META_PATH":           "./data/meta/meta.json",
		"SD_BUILD_SHA":           "abc123",
		"SD_PULL_REQUEST":        "1",
		"SD_API_URL":             "https://api.screwdriver.cd/v4/",
		"SD_BUILD_URL":           "https://api.screwdriver.cd/v4/builds/1234",
		"SD_STORE_URL":           "https://store.screwdriver.cd/v1/",
		"SD_UI_URL":              "https://screwdriver.cd/",
		"SD_UI_BUILD_URL":        "https://screwdriver.cd/pipelines/3456/builds/1234",
		"SD_TOKEN":               "foobar",
		"SD_BUILD_CLUSTER_NAME":  "",
		"SD_SONAR_AUTH_URL":      "https://api.screwdriver.cd/v4/coverage/token",
		"SD_SONAR_HOST":          "https://sonar.screwdriver.cd",
		"SD_PIPELINE_CACHE_DIR":  "",
		"SD_JOB_CACHE_DIR":       "",
		"SD_EVENT_CACHE_DIR":     "",
		"SD_SCHEDULED_BUILD":     "false",
		"SD_PRIVATE_PIPELINE":    "false",
	}

	api := mockAPI(t, TestBuildID, TestJobID, TestPipelineID, "RUNNING")
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{ID: TestJobID, Name: "PR-1", PipelineID: TestPipelineID, PrParentJobID: 111}), nil
	}

	foundEnv := map[string]string{}
	executorRun = func(path string, env []string, emitter screwdriver.Emitter, build screwdriver.Build, api screwdriver.API, buildID int, shellBin string, timeout int, envFilepath, sourceDir string) error {
		if len(env) == 0 {
			t.Fatalf("Unexpected empty environment passed to executorRun")
		}

		for _, e := range env {
			split := strings.SplitN(e, "=", 2)
			if len(split) != 2 {
				t.Fatalf("Bad environment value passed to executorRun: %s", e)
			}
			foundEnv[split[0]] = split[1]
		}

		return nil
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	if err != nil {
		t.Fatalf("Unexpected error from launch: %v", err)
	}
	for k, v := range tests {
		if foundEnv[k] != v {
			t.Fatalf("foundEnv[%s] = %s, want %s", k, foundEnv[k], v)
		}
	}

	expectSourceDir := "/sd/workspace/src/github.com/screwdriver-cd/launcher"
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := "/bin/sh"
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}

	// in case of no coverage plugins
	delete(tests, "SD_SONAR_AUTH_URL")
	delete(tests, "SD_SONAR_HOST")
	TestEnvVars = map[string]interface{}{}
	foundEnv = map[string]string{}
	err, sourceDir, shellBin = launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	if err != nil {
		t.Fatalf("Unexpected error from launch: %v", err)
	}
	for k, v := range tests {
		if foundEnv[k] != v {
			t.Fatalf("foundEnv[%s] = %s, want %s", k, foundEnv[k], v)
		}
	}

	// set SD_SOURCE_DIR correctly with scm.RootDir
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI + ":lib", ScmRepo: TestScmRepo}), nil
	}
	tests["SD_SOURCE_DIR"] = tests["SD_SOURCE_DIR"] + "/lib"
	tempShellBin := "/bin/bash"
	TestEnvVars = map[string]interface{}{}
	foundEnv = map[string]string{}
	err, sourceDir, shellBin = launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, tempShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	if err != nil {
		t.Fatalf("Unexpected error from launch: %v", err)
	}

	expectSourceDir = "/sd/workspace/src/github.com/screwdriver-cd/launcher/lib"
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin = "/bin/bash"
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}

	for k, v := range tests {
		if foundEnv[k] != v {
			t.Fatalf("foundEnv[%s] = %s, want %s", k, foundEnv[k], v)
		}
	}

	// in case of the build cluster exists
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: TestBuildID, EventID: TestEventID, JobID: TestJobID, BuildClusterName: TestBuildClusterName, SHA: TestSHA, ParentBuildID: float64(1234)}), nil
	}
	tests["SD_BUILD_CLUSTER_NAME"] = TestBuildClusterName
	TestEnvVars = map[string]interface{}{}
	foundEnv = map[string]string{}
	err, _, _ = launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	if err != nil {
		t.Fatalf("Unexpected error from launch: %v", err)
	}
	for k, v := range tests {
		if foundEnv[k] != v {
			t.Fatalf("foundEnv[%s] = %s, want %s", k, foundEnv[k], v)
		}
	}

	// in case of the pipeline is private
	TestPrivateScmRepo := screwdriver.ScmRepo(FakeScmRepo{
		Name:    "screwdriver-cd/launcher",
		Private: true,
	})
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI + ":lib", ScmRepo: TestPrivateScmRepo}), nil
	}
	tests["SD_PRIVATE_PIPELINE"] = "true"
	TestEnvVars = map[string]interface{}{}
	foundEnv = map[string]string{}
	err, _, _ = launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	if err != nil {
		t.Fatalf("Unexpected error from launch: %v", err)
	}

	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}

	for k, v := range tests {
		if foundEnv[k] != v {
			t.Fatalf("foundEnv[%s] = %s, want %s", k, foundEnv[k], v)
		}
	}
}

func TestEnvSecrets(t *testing.T) {
	api := mockAPI(t, TestBuildID, TestJobID, TestPipelineID, "RUNNING")
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{Name: "PR-1", PipelineID: TestPipelineID}), nil
	}

	testBuild := FakeBuild{ID: TestBuildID, JobID: TestJobID}
	api.secretsForBuild = func(build screwdriver.Build) (screwdriver.Secrets, error) {
		if !reflect.DeepEqual(build.ID, testBuild.ID) {
			t.Errorf("build.ID = %d, want %d", build.ID, testBuild.ID)
		}

		testSecrets := screwdriver.Secrets{
			{Name: "FOONAME", Value: "barvalue"},
		}
		return testSecrets, nil
	}

	foundEnv := map[string]string{}
	oldExecutorRun := executorRun
	defer func() { executorRun = oldExecutorRun }()
	executorRun = func(path string, env []string, emitter screwdriver.Emitter, build screwdriver.Build, api screwdriver.API, buildID int, shellBin string, timeout int, envFilepath, sourceDir string) error {
		if len(env) == 0 {
			t.Fatalf("Unexpected empty environment passed to executorRun")
		}

		for _, e := range env {
			split := strings.SplitN(e, "=", 2)
			if len(split) != 2 {
				t.Fatalf("Bad environment value passed to executorRun: %s", e)
			}
			foundEnv[split[0]] = split[1]
		}

		return nil
	}

	err, _, _ := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
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
		"SD_TOKEN":        "1234",
		"FOO":             "bar",
		"THINGWITHEQUALS": "abc=def",
		"GETSOVERRIDDEN":  "goesaway",
	}

	secrets := screwdriver.Secrets{
		{Name: "secret1", Value: "secret1value"},
		{Name: "GETSOVERRIDDEN", Value: "override"},
		{Name: "MYSECRETPATH", Value: "secretpath"},
		{Name: "WITHDOLLAR", Value: "$FOO"},
	}

	var buildEnv []map[string]string
	buildEnv = append(buildEnv, map[string]string{"GOPATH": "/go/path"})
	buildEnv = append(buildEnv, map[string]string{"EXPANDENV": "${GOPATH}/expand"})
	buildEnv = append(buildEnv, map[string]string{"EXPANDSECRET": "$MYSECRETPATH/home"})
	buildEnv = append(buildEnv, map[string]string{"SD_CACHE_STRATEGY": "disk"})
	buildEnv = append(buildEnv, map[string]string{"SD_PIPELINE_CACHE_DIR": "/opt/sd/cache/pipeline"})
	buildEnv = append(buildEnv, map[string]string{"SD_JOB_CACHE_DIR": "/opt/sd/cache/job"})
	buildEnv = append(buildEnv, map[string]string{"SD_EVENT_CACHE_DIR": "/opt/sd/cache/event"})

	testBuild := screwdriver.Build{
		ID:          12345,
		Environment: buildEnv,
	}
	env, userShellBin := createEnvironment(base, secrets, testBuild)

	if userShellBin != "" {
		t.Errorf("Default userShellBin should be empty string")
	}

	foundEnv := map[string]bool{}
	for _, i := range env {
		foundEnv[i] = true
	}

	for _, want := range []string{
		"SD_TOKEN=1234",
		"FOO=bar",
		"THINGWITHEQUALS=abc=def",
		"secret1=secret1value",
		"GETSOVERRIDDEN=override",
		"OSENVWITHEQUALS=foo=bar=",
		"GOPATH=/go/path",
		"EXPANDENV=/go/path/expand",
		"EXPANDSECRET=secretpath/home",
		"WITHDOLLAR=$FOO",
		"SD_PIPELINE_CACHE_DIR=/opt/sd/cache/pipeline",
		"SD_JOB_CACHE_DIR=/opt/sd/cache/job",
		"SD_EVENT_CACHE_DIR=/opt/sd/cache/event",
	} {
		if !foundEnv[want] {
			t.Errorf("Did not receive expected environment setting %q", want)
		}
	}

	if foundEnv["GETSOVERRIDDEN=goesaway"] {
		t.Errorf("Failed to override the base environment with a secret")
	}
}

func TestUserShellBin(t *testing.T) {
	base := map[string]string{}
	secrets := screwdriver.Secrets{}
	var buildEnv []map[string]string
	buildEnv = append(buildEnv, map[string]string{"USER_SHELL_BIN": "/bin/bash"})

	testBuild := screwdriver.Build{
		ID:          12345,
		Environment: buildEnv,
	}
	_, userShellBin := createEnvironment(base, secrets, testBuild)

	if userShellBin != "/bin/bash" {
		t.Errorf("userShellBin %v, expect %v", userShellBin, "/bin/bash")
	}
}

func TestFetchDefaultMeta(t *testing.T) {
	initCoverageMeta()
	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()
	var defaultMeta []byte

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: buildID, JobID: TestJobID}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestPipelineID, Name: "main"}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		if path == "./data/meta/meta.json" {
			defaultMeta = data
		}
		return nil
	}

	err, _, _ := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)

	want := fmt.Sprintf(`{
		"build": {
			"buildId": "1234",
			"coverageKey": "job:fake",
			"eventId": "0",
			"jobId": "2345",
			"jobName": "main",
			"pipelineId": "3456",
			"sha": ""
		},
		"event": {
			"creator": "%s"
		}
	}`, TestEventCreator["username"])

	assert.JSONEq(t, want, string(defaultMeta))
	if err != nil {
		t.Errorf(fmt.Sprintf("err returned: %s", err.Error()))
	}
}

func TestFetchParentBuildsMetaParseError(t *testing.T) {
	for i, s := range TestParentBuildIDs {
		IDs[i] = s
	}
	oldMarshal := marshal
	defer func() { marshal = oldMarshal }()

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: TestBuildID, JobID: TestJobID, ParentBuildID: IDs}), nil
	}

	marshal = func(v interface{}) (result []byte, err error) {
		return nil, fmt.Errorf("Testing parsing parent builds meta")
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	expected := fmt.Sprint("Parsing Meta JSON: Testing parsing parent builds meta")

	if err.Error() != expected {
		t.Errorf("Error is wrong, got '%v', expected '%v'", err, expected)
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}
}

func TestFetchParentEventMetaParseError(t *testing.T) {
	oldMarshal := marshal
	defer func() { marshal = oldMarshal }()

	api := mockAPI(t, TestEventID, TestJobID, 0, "RUNNING")
	api.eventFromID = func(eventID int) (screwdriver.Event, error) {
		if eventID == TestParentEventID {
			return screwdriver.Event(FakeEvent{ID: TestParentEventID}), nil
		}
		return screwdriver.Event(FakeEvent{ID: TestEventID, ParentEventID: TestParentEventID}), nil
	}
	marshal = func(v interface{}) (result []byte, err error) {
		return nil, fmt.Errorf("Testing parsing parent event meta")
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestEventID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	expected := fmt.Sprint("Parsing Meta JSON: Testing parsing parent event meta")

	if err.Error() != expected {
		t.Errorf("Error is wrong, got '%v', expected '%v'", err, expected)
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}
}

func TestFetchParentBuildMetaParseError(t *testing.T) {
	oldMarshal := marshal
	defer func() { marshal = oldMarshal }()

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		if buildID == 1111 {
			return screwdriver.Build(FakeBuild{ID: TestParentBuildID, JobID: TestParentJobID}), nil
		}
		return screwdriver.Build(FakeBuild{ID: TestBuildID, JobID: TestJobID, ParentBuildID: TestParentBuildIDFloat}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		if jobID == TestParentJobID {
			return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestParentPipelineID, Name: "component"}), nil
		}
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestPipelineID, Name: "main"}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		if pipelineID == TestParentPipelineID {
			return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
		}
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	marshal = func(v interface{}) (result []byte, err error) {
		return nil, fmt.Errorf("Testing parsing parent build meta")
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	expected := fmt.Sprint("Parsing Meta JSON: Testing parsing parent build meta")

	if err.Error() != expected {
		t.Errorf("Error is wrong, got '%v', expected '%v'", err, expected)
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}
}

func TestFetchParentBuildMetaWriteError(t *testing.T) {
	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		if buildID == TestParentBuildID {
			return screwdriver.Build(FakeBuild{ID: TestParentBuildID, JobID: TestParentJobID}), nil
		}
		return screwdriver.Build(FakeBuild{ID: TestBuildID, JobID: TestJobID, ParentBuildID: TestParentBuildIDFloat}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		if jobID == TestParentJobID {
			return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestParentPipelineID, Name: "component"}), nil
		}
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestPipelineID, Name: "main"}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		if pipelineID == TestParentPipelineID {
			return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
		}
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		return fmt.Errorf("Testing writing parent build meta")
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	expected := fmt.Sprintf(`Writing Parent Build(%d) Meta JSON: Testing writing parent build meta`, TestParentBuildID)

	if err.Error() != expected {
		t.Errorf("Error is wrong, got '%v', expected '%v'", err, expected)
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}
}

func TestFetchParentBuildsMeta(t *testing.T) {
	initCoverageMeta()
	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()
	var parentMeta []byte
	var parentMeta2 []byte
	var buildMeta []byte
	for i, s := range TestParentBuildIDs {
		IDs[i] = s
	}
	TestMetaDeep1 := map[string]interface{}{
		"cat":  "meow",
		"bird": "chirp",
	}
	TestMetaDeep2 := map[string]interface{}{
		"dog":  "woof",
		"bird": "twitter",
	}
	TestMeta1 := map[string]interface{}{
		"foo":    TestMetaDeep1,
		"batman": "robin",
	}
	TestMeta2 := map[string]interface{}{
		"foo":    TestMetaDeep2,
		"wonder": "woman",
	}

	oldMarshal := marshal
	defer func() { marshal = oldMarshal }()

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		if buildID == TestParentBuildID {
			return screwdriver.Build(FakeBuild{ID: buildID, JobID: TestParentJobID, Meta: TestMeta1}), nil
		}
		if buildID == 2222 {
			return screwdriver.Build(FakeBuild{ID: buildID, JobID: 1114, Meta: TestMeta2}), nil
		}
		return screwdriver.Build(FakeBuild{ID: buildID, JobID: TestJobID, ParentBuildID: IDs}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		if jobID == TestParentJobID {
			return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestParentPipelineID, Name: "component"}), nil
		}
		if jobID == 1114 {
			return screwdriver.Job(FakeJob{ID: jobID, PipelineID: 1115, Name: "component"}), nil
		}
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestPipelineID, Name: "main"}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		if pipelineID == TestParentPipelineID {
			return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
		}
		if pipelineID == 1115 {
			return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
		}
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		if path == "./data/meta/sd@1113:component.json" {
			parentMeta = data
			log.Printf("parentMeta: %v", parentMeta)
		}
		if path == "./data/meta/sd@1115:component.json" {
			parentMeta2 = data
			log.Printf("parentMeta: %v", parentMeta2)
		}
		if path == "./data/meta/meta.json" {
			buildMeta = data
		}
		return nil
	}

	err, _, _ := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)

	want := fmt.Sprintf(`{
		"batman": "robin",
		"build": {
			"buildId": "1234",
			"coverageKey": "job:fake",
			"eventId": "0",
			"jobId": "2345",
			"jobName": "main",
			"pipelineId": "3456",
			"sha": ""
		},
		"event": {
			"creator": "%s"
		},
		"foo": {
			"bird": "twitter",
			"cat": "meow",
			"dog": "woof"
		},
		"wonder": "woman"
	}`, TestEventCreator["username"])

	wantParent := `{
		"batman": "robin",
		"foo": {
			"bird": "chirp",
			"cat": "meow"
		}
	}`

	wantParent2 := `{
		"foo": {
			"bird": "twitter",
			"dog": "woof"
		},
		"wonder": "woman"
	}`

	assert.JSONEq(t, want, string(buildMeta))
	assert.JSONEq(t, wantParent, string(parentMeta))
	assert.JSONEq(t, wantParent2, string(parentMeta2))
	if err != nil {
		t.Errorf(fmt.Sprintf("err returned: %s", err.Error()))
	}
}

func TestNoParentEventID(t *testing.T) {
	oldMarshal := marshal
	defer func() { marshal = oldMarshal }()

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: TestBuildID, JobID: TestJobID}), nil
	}
	api.eventFromID = func(eventID int) (screwdriver.Event, error) {
		return screwdriver.Event(FakeEvent{ID: TestEventID}), nil
	}

	marshal = func(v interface{}) (result []byte, err error) {
		return nil, fmt.Errorf("Testing parsing parent event meta")
	}

	launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
}

func TestFetchParentEventMetaWriteError(t *testing.T) {
	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()

	oldMarshal := marshal
	defer func() { marshal = oldMarshal }()

	api := mockAPI(t, TestEventID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: TestBuildID, EventID: TestEventID, JobID: TestJobID, SHA: TestSHA}), nil
	}
	api.eventFromID = func(eventID int) (screwdriver.Event, error) {
		if eventID == TestParentEventID {
			return screwdriver.Event(FakeEvent{ID: TestParentEventID}), nil
		}
		return screwdriver.Event(FakeEvent{ID: TestEventID, ParentEventID: TestParentEventID}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		if jobID == TestParentJobID {
			return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestParentPipelineID, Name: "component"}), nil
		}
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestPipelineID, Name: "main"}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		if pipelineID == TestParentPipelineID {
			return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
		}
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	marshal = func(v interface{}) (result []byte, err error) {
		return nil, nil
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		return fmt.Errorf("Testing writing parent event meta")
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestEventID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	expected := fmt.Sprintf(`Writing Parent Event(%d) Meta JSON: Testing writing parent event meta`, TestParentEventID)

	if err.Error() != expected {
		t.Errorf("Error is wrong, got '%v', expected '%v'", err, expected)
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}
}

func TestFetchEventMetaMarshalError(t *testing.T) {
	oldMarshal := marshal
	defer func() { marshal = oldMarshal }()

	mockMeta := make(map[string]interface{})
	mockMeta["spooky"] = "ghost"

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.eventFromID = func(eventID int) (screwdriver.Event, error) {
		if eventID == TestEventID {
			return screwdriver.Event(FakeEvent{ID: TestEventID, Meta: mockMeta}), nil
		}
		return screwdriver.Event(FakeEvent{ID: TestEventID, ParentEventID: TestParentEventID}), nil
	}
	marshal = func(v interface{}) (result []byte, err error) {
		return nil, fmt.Errorf("Testing parsing event meta")
	}

	err, sourceDir, shellBin := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	expected := fmt.Sprint("Parsing Meta JSON: Testing parsing event meta")

	if err.Error() != expected {
		t.Errorf("Error is wrong, got '%v', expected '%v'", err, expected)
	}

	expectSourceDir := ""
	if sourceDir != expectSourceDir {
		t.Errorf("sourceDir == %s, want %s", sourceDir, expectSourceDir)
	}

	expectShellBin := ""
	if shellBin != expectShellBin {
		t.Errorf("shellBin == %s, want %s", shellBin, expectShellBin)
	}
}

func TestMetaWhenStartPipeline(t *testing.T) {
	initCoverageMeta()
	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()
	var defaultMeta []byte

	buildFromIDMeta := map[string]interface{}{
		"build_only": "build_value", // Remain
		"parameters": map[string]string{
			"build_only": "build_value", // Remain
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"build_only": "build_value",
			},
			"build_only": "build_value", // Remain
		},
		"build": map[string]interface{}{
			"buildId":     "build_value", // Overwrote by default value
			"jobId":       "build_value", // Overwrote by default value
			"eventId":     "build_value", // Overwrote by default value
			"pipelineId":  "build_value", // Overwrote by default value
			"sha":         "build_value", // Overwrote by default value
			"jobName":     "build_value", // Overwrote by default value
			"coverageKey": "build_value", // Overwrote by default value
			"build_only":  "build_value", // Remain
		},
		"event": map[string]interface{}{
			"creator": "build_value", // Overwrote by default value
		},
	}

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: buildID, JobID: TestJobID, Meta: buildFromIDMeta}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestPipelineID, Name: "main"}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		if path == "./data/meta/meta.json" {
			defaultMeta = data
		}
		return nil
	}

	err, _, _ := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	want := fmt.Sprintf(`{
		"build_only": "build_value",
		"build": {
			"buildId": "%d",
			"jobId": "%d",
			"eventId": "0",
			"pipelineId": "%d",
			"sha": "",
			"jobName": "main",
			"coverageKey": "%s",
			"build_only": "build_value"
		},
		"event": {
			"creator": "%s"
		},
		"meta": {
			"build_only": "build_value"
		},
		"parameters": {
			"build_only": "build_value"
		}
	}`, TestBuildID, TestJobID, TestPipelineID, TestEnvVars["SD_SONAR_PROJECT_KEY"], TestEventCreator["username"])

	assert.JSONEq(t, want, string(defaultMeta))
	if err != nil {
		t.Errorf(fmt.Sprintf("err returned: %s", err.Error()))
	}
}

func TestMetaWhenTriggeredFromParentBuildWithoutParentBuildMeta(t *testing.T) {
	initCoverageMeta()
	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()
	var defaultMeta []byte

	buildFromIDMeta := map[string]interface{}{
		"build_only":      "build_value", // Remain
		"build_and_event": "build_value", // Overwrote by event
		"parameters": map[string]string{
			"build_only":      "build_value", // Remain
			"build_and_event": "build_value", // Overwrote by event
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"build_only":      "build_value",
				"build_and_event": "build_value",
			},
			"build_only":      "build_value", // Remain
			"build_and_event": "build_value", // Overwrote by event
		},
		"build": map[string]interface{}{
			"buildId":         "build_value", // Overwrote by default value
			"jobId":           "build_value", // Overwrote by default value
			"eventId":         "build_value", // Overwrote by default value
			"pipelineId":      "build_value", // Overwrote by default value
			"sha":             "build_value", // Overwrote by default value
			"jobName":         "build_value", // Overwrote by default value
			"coverageKey":     "build_value", // Overwrote by default value
			"build_only":      "build_value", // Remain
			"build_and_event": "build_value", // Overwrote by event
		},
		"event": map[string]interface{}{
			"creator": "build_value", // Overwrote by default value
		},
	}

	eventFromIDMeta := map[string]interface{}{
		"event_only":      "event_value", // Remain
		"build_and_event": "event_value", // Remain
		"parameters": map[string]string{
			"event_only":      "event_value", // This should be deleted
			"build_and_event": "event_value", // Overwrote by build
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"event_only":      "event_value",
				"build_and_event": "event_value",
			},
			"event_only":      "event_value", // This should be deleted
			"build_and_event": "event_value", // Remain
		},
		"build": map[string]interface{}{
			"buildId":         "event_value", // Overwrote by default value
			"jobId":           "event_value", // Overwrote by default value
			"eventId":         "event_value", // Overwrote by default value
			"pipelineId":      "event_value", // Overwrote by default value
			"sha":             "event_value", // Overwrote by default value
			"jobName":         "event_value", // Overwrote by default value
			"coverageKey":     "event_value", // Overwrote by default value
			"event_only":      "event_value", // Remain
			"build_and_event": "event_value", // Remain
		},
		"event": map[string]interface{}{
			"creator": "event_value", // Overwrote by default value
		},
	}

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: buildID, JobID: TestJobID, Meta: buildFromIDMeta}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestPipelineID, Name: "main"}), nil
	}
	api.eventFromID = func(eventID int) (screwdriver.Event, error) {
		return screwdriver.Event(FakeEvent{ID: TestEventID, ParentEventID: TestParentEventID, Meta: eventFromIDMeta, Creator: TestEventCreator}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		if path == "./data/meta/meta.json" {
			defaultMeta = data
		}
		return nil
	}

	err, _, _ := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	want := fmt.Sprintf(`{
		"build_only": "build_value",
		"event_only": "event_value",
		"build_and_event": "event_value",
		"build":{
			"buildId": "%d",
			"jobId": "%d",
			"eventId": "0",
			"pipelineId": "%d",
			"sha": "",
			"jobName": "main",
			"coverageKey": "%s",
			"build_only": "build_value",
			"event_only": "event_value",
			"build_and_event": "event_value"
		},
		"event": {
			"creator": "%s"
		},
		"meta":{
			"build_only": "build_value",
			"event_only": "event_value",
			"build_and_event": "event_value"
		},
		"parameters":{
			"build_only": "build_value",
			"build_and_event": "build_value"
		}
	}`, TestBuildID, TestJobID, TestPipelineID, TestEnvVars["SD_SONAR_PROJECT_KEY"], TestEventCreator["username"])

	assert.JSONEq(t, want, string(defaultMeta))
	if err != nil {
		t.Errorf(fmt.Sprintf("err returned: %s", err.Error()))
	}
}

func TestMetaWhenTriggeredFromPipelinesByANDLogicWithParentBuildMeta(t *testing.T) {
	initCoverageMeta()
	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()
	var defaultMeta, externalMeta []byte

	for i, s := range TestParentBuildIDs {
		IDs[i] = s
	}
	var InnerParentBuildID = TestParentBuildID
	var InnerParentJobID = TestParentJobID
	var InnerPipelineID = TestPipelineID
	var ExternalParentBuildID = 2222
	var ExternalParentJobID = 1114
	var ExternalPipelineID = TestParentPipelineID

	buildFromIDMeta := map[string]interface{}{
		"build_only":                            "build_value", // Remain
		"build_and_event_and_inner_pipeline":    "build_value", // Overwrote by inner pipeline
		"build_and_event_and_external_pipeline": "build_value", // Overwrote by external pipeline
		"parameters": map[string]string{
			"build_only":                            "build_value", // Remain
			"build_and_event_and_inner_pipeline":    "build_value", // Remain
			"build_and_event_and_external_pipeline": "build_value", // Remain
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"build_only":                            "build_value",
				"build_and_event_and_inner_pipeline":    "build_value",
				"build_and_event_and_external_pipeline": "build_value",
			},
			"build_only":                            "build_value", // Remain
			"build_and_event_and_inner_pipeline":    "build_value", // Overwrote by inner pipeline
			"build_and_event_and_external_pipeline": "build_value", // Overwrote by external pipeline
		},
		"build": map[string]interface{}{
			"buildId":                               "build_value", // Overwrote by default value
			"jobId":                                 "build_value", // Overwrote by default value
			"eventId":                               "build_value", // Overwrote by default value
			"pipelineId":                            "build_value", // Overwrote by default value
			"sha":                                   "build_value", // Overwrote by default value
			"jobName":                               "build_value", // Overwrote by default value
			"coverageKey":                           "build_value", // Overwrote by default value
			"build_only":                            "build_value", // Remain
			"build_and_event_and_inner_pipeline":    "build_value", // Overwrote by inner pipeline
			"build_and_event_and_external_pipeline": "build_value", // Overwrote by external pipeline
		},
		"event": map[string]interface{}{
			"creator": "build_value", // Overwrote by default value
		},
		"sd": map[string]interface{}{
			"build_only": "build_value", // Remain
			strconv.Itoa(InnerPipelineID): map[string]string{
				"build_only":   "build_value", // Remain
				"inner_parent": "build_value", // Overwrote by inner pipeline
			},
			strconv.Itoa(ExternalPipelineID): map[string]string{
				"build_only":      "build_value", // Remain
				"external_parent": "build_value", // This should be deleted
			},
		},
	}

	eventFromIDMeta := map[string]interface{}{
		"event_only":                            "event_value", // Remain
		"build_and_event_and_inner_pipeline":    "event_value", // Overwrote by inner pipeline
		"build_and_event_and_external_pipeline": "event_value", // Overwrote by external pipeline
		"parameters": map[string]string{
			"event_only":                            "event_value", // This should be deleted
			"build_and_event_and_inner_pipeline":    "event_value", // Overwrote by build
			"build_and_event_and_external_pipeline": "event_value", // Overwrote by build
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"event_only":                            "event_value",
				"build_and_event_and_inner_pipeline":    "event_value",
				"build_and_event_and_external_pipeline": "event_value",
			},
			"event_only":                            "event_value", // Remain
			"build_and_event_and_inner_pipeline":    "event_value", // Overwrote by inner pipeline
			"build_and_event_and_external_pipeline": "event_value", // Overwrote by external pipeline
		},
		"build": map[string]interface{}{
			"buildId":                               "event_value", // Overwrote by default value
			"jobId":                                 "event_value", // Overwrote by default value
			"eventId":                               "event_value", // Overwrote by default value
			"pipelineId":                            "event_value", // Overwrote by default value
			"sha":                                   "event_value", // Overwrote by default value
			"jobName":                               "event_value", // Overwrote by default value
			"coverageKey":                           "event_value", // Overwrote by default value
			"event_only":                            "event_value", // Remain
			"build_and_event_and_inner_pipeline":    "event_value", // Overwrote by inner pipeline
			"build_and_event_and_external_pipeline": "event_value", // Overwrote by external pipeline
		},
		"event": map[string]interface{}{
			"creator": "event_value", // Overwrote by default value
		},
		"sd": map[string]interface{}{
			"event_only": "event_value", // Remain
			strconv.Itoa(InnerPipelineID): map[string]string{
				"event_only":   "event_value", // Remain
				"inner_parent": "event_value", // Overwrote by inner pipeline
			},
			strconv.Itoa(ExternalPipelineID): map[string]string{
				"event_only":      "event_value", // Remain
				"external_parent": "event_value", // This should be deleted
			},
		},
	}

	innerParentBuildMeta := map[string]interface{}{
		"inner_pipeline_only":                "inner_pipeline_value", // Remain
		"build_and_event_and_inner_pipeline": "inner_pipeline_value", // Remain
		"parameters": map[string]string{
			"inner_pipeline_only":                "inner_pipeline_value", // This should be deleted
			"build_and_event_and_inner_pipeline": "inner_pipeline_value", // Overwrote by build
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"inner_pipeline_only":                "inner_pipeline_value",
				"build_and_event_and_inner_pipeline": "inner_pipeline_value",
			},
			"inner_pipeline_only":                "inner_pipeline_value", // Remain
			"build_and_event_and_inner_pipeline": "inner_pipeline_value", // Remain
		},
		"build": map[string]interface{}{
			"buildId":                            "inner_pipeline_value", // Overwrote by default value
			"jobId":                              "inner_pipeline_value", // Overwrote by default value
			"eventId":                            "inner_pipeline_value", // Overwrote by default value
			"pipelineId":                         "inner_pipeline_value", // Overwrote by default value
			"sha":                                "inner_pipeline_value", // Overwrote by default value
			"jobName":                            "inner_pipeline_value", // Overwrote by default value
			"coverageKey":                        "inner_pipeline_value", // Overwrote by default value
			"inner_pipeline_only":                "inner_pipeline_value", // Remain
			"build_and_event_and_inner_pipeline": "inner_pipeline_value", // Remain
		},
		"event": map[string]interface{}{
			"creator": "inner_pipeline_value", // Overwrote by default value
		},
		"sd": map[string]interface{}{
			"inner_pipeline_only": "inner_pipeline_value", // Remain
			strconv.Itoa(InnerPipelineID): map[string]string{
				"inner_pipeline_only": "inner_pipeline_value", // Remain
				"inner_parent":        "inner_pipeline_value", // Remain
			},
			strconv.Itoa(ExternalPipelineID): map[string]string{
				"inner_pipeline_only": "inner_pipeline_value", // Remain
			},
		},
	}

	externalParentBuildMeta := map[string]interface{}{
		"external_pipeline_only":                "external_pipeline_value", // Remain
		"build_and_event_and_external_pipeline": "external_pipeline_value", // Remain
		"parameters": map[string]string{
			"external_pipeline_only":                "external_pipeline_value", // This should be deleted
			"build_and_event_and_external_pipeline": "external_pipeline_value", // Overwrote by build
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"external_pipeline_only":                "external_pipeline_value",
				"build_and_event_and_external_pipeline": "external_pipeline_value",
			},
			"external_pipeline_only":                "external_pipeline_value", // Remain
			"build_and_event_and_external_pipeline": "external_pipeline_value", // Remain
		},
		"build": map[string]interface{}{
			"buildId":                               "external_pipeline_value", // Overwrote by default value
			"jobId":                                 "external_pipeline_value", // Overwrote by default value
			"eventId":                               "external_pipeline_value", // Overwrote by default value
			"pipelineId":                            "external_pipeline_value", // Overwrote by default value
			"sha":                                   "external_pipeline_value", // Overwrote by default value
			"jobName":                               "external_pipeline_value", // Overwrote by default value
			"coverageKey":                           "external_pipeline_value", // Overwrote by default value
			"external_pipeline_only":                "external_pipeline_value", // Remain
			"build_and_event_and_external_pipeline": "external_pipeline_value", // Remain
		},
		"event": map[string]interface{}{
			"creator": "external_pipeline_value", // Overwrote by default value
		},
		"sd": map[string]interface{}{
			"external_pipeline_only": "external_pipeline_value", // Remain
			strconv.Itoa(InnerPipelineID): map[string]string{
				"external_pipeline_only": "external_pipeline_value", // Remain
			},
			strconv.Itoa(ExternalPipelineID): map[string]string{
				"external_pipeline_only": "external_pipeline_value", // Remain
				"external_parent":        "external_pipeline_value", // This should be deleted
			},
		},
	}

	oldMarshal := marshal
	defer func() { marshal = oldMarshal }()

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		if buildID == InnerParentBuildID {
			// inner parent build
			return screwdriver.Build(FakeBuild{ID: buildID, JobID: InnerParentJobID, Meta: innerParentBuildMeta}), nil
		}
		if buildID == ExternalParentBuildID {
			// external parent build
			return screwdriver.Build(FakeBuild{ID: buildID, JobID: ExternalParentJobID, Meta: externalParentBuildMeta}), nil
		}
		// target build
		return screwdriver.Build(FakeBuild{ID: buildID, JobID: TestJobID, ParentBuildID: IDs, Meta: buildFromIDMeta}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		if jobID == InnerParentJobID {
			// inner parent job
			return screwdriver.Job(FakeJob{ID: jobID, PipelineID: InnerPipelineID, Name: "inner_parent"}), nil
		}
		if jobID == ExternalParentJobID {
			// external parent job
			return screwdriver.Job(FakeJob{ID: jobID, PipelineID: ExternalPipelineID, Name: "external_parent"}), nil
		}
		// target build
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: InnerPipelineID, Name: "main"}), nil
	}
	api.eventFromID = func(eventID int) (screwdriver.Event, error) {
		return screwdriver.Event(FakeEvent{ID: TestEventID, ParentEventID: TestParentEventID, Meta: eventFromIDMeta, Creator: TestEventCreator}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		if path == "./data/meta/meta.json" {
			defaultMeta = data
		}
		if path == fmt.Sprintf("./data/meta/sd@%d:external_parent.json", ExternalPipelineID) {
			externalMeta = data
		}
		return nil
	}

	err, _, _ := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	want := fmt.Sprintf(`{
		"build_only": "build_value",
		"event_only": "event_value",
		"external_pipeline_only": "external_pipeline_value",
		"inner_pipeline_only": "inner_pipeline_value",
		"build_and_event_and_inner_pipeline": "inner_pipeline_value",
		"build_and_event_and_external_pipeline": "external_pipeline_value",
		"build":{
			"buildId": "%d",
			"jobId": "%d",
			"eventId": "0",
			"pipelineId": "%d",
			"sha": "",
			"jobName": "main",
			"coverageKey": "%s",
			"build_only": "build_value",
			"event_only": "event_value",
			"inner_pipeline_only": "inner_pipeline_value",
			"external_pipeline_only": "external_pipeline_value",
			"build_and_event_and_inner_pipeline": "inner_pipeline_value",
			"build_and_event_and_external_pipeline": "external_pipeline_value"
		},
		"event": {
			"creator": "%s"
		},
		"meta": {
			"build_only": "build_value",
			"event_only": "event_value",
			"external_pipeline_only": "external_pipeline_value",
			"inner_pipeline_only": "inner_pipeline_value",
			"build_and_event_and_inner_pipeline": "inner_pipeline_value",
			"build_and_event_and_external_pipeline": "external_pipeline_value"
		},
		"parameters": {
			"build_only": "build_value",
			"build_and_event_and_inner_pipeline": "build_value",
			"build_and_event_and_external_pipeline": "build_value"
		},
		"sd": {
			"build_only": "build_value",
			"event_only": "event_value",
			"inner_pipeline_only": "inner_pipeline_value",
			"external_pipeline_only": "external_pipeline_value",
			"%d":{
				"build_only": "build_value",
				"event_only": "event_value",
				"inner_pipeline_only": "inner_pipeline_value",
				"external_pipeline_only": "external_pipeline_value",
				"inner_parent": "inner_pipeline_value"
			},
			"%d":{
				"build_only": "build_value",
				"event_only": "event_value",
				"inner_pipeline_only": "inner_pipeline_value",
				"external_pipeline_only": "external_pipeline_value"
			}
		}
	}`, TestBuildID, TestJobID, TestPipelineID, TestEnvVars["SD_SONAR_PROJECT_KEY"], TestEventCreator["username"], InnerPipelineID, ExternalPipelineID)
	assert.JSONEq(t, want, string(defaultMeta))

	wantExternalMetaByte, _ := marshal(externalParentBuildMeta)
	assert.JSONEq(t, string(wantExternalMetaByte), string(externalMeta))

	if err != nil {
		t.Errorf(fmt.Sprintf("err returned: %s", err.Error()))
	}
}

func TestMetaWhenTriggeredFromInnerPipelineByORLogicWithParentBuildMeta(t *testing.T) {
	initCoverageMeta()
	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()
	var defaultMeta []byte

	buildFromIDMeta := map[string]interface{}{
		"build_only":                         "build_value", // Remain
		"build_and_event_and_inner_pipeline": "build_value", // Overwrote by inner pipeline
		"parameters": map[string]string{
			"build_only":                         "build_value", // Remain
			"build_and_event_and_inner_pipeline": "build_value", // Remain
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"build_only":                         "build_value",
				"build_and_event_and_inner_pipeline": "build_value",
			},
			"build_only":                         "build_value", // Remain
			"build_and_event_and_inner_pipeline": "build_value", // Overwrote by inner pipeline
		},
		"build": map[string]interface{}{
			"buildId":                            "build_value", // Overwrote by default value
			"jobId":                              "build_value", // Overwrote by default value
			"eventId":                            "build_value", // Overwrote by default value
			"pipelineId":                         "build_value", // Overwrote by default value
			"sha":                                "build_value", // Overwrote by default value
			"jobName":                            "build_value", // Overwrote by default value
			"coverageKey":                        "build_value", // Overwrote by default value
			"build_only":                         "build_value", // Remain
			"build_and_event_and_inner_pipeline": "build_value", // Overwrote by inner pipeline
		},
		"event": map[string]interface{}{
			"creator": "build_value", // Overwrote by default value
		},
	}

	eventFromIDMeta := map[string]interface{}{
		"event_only":                         "event_value", // Remain
		"build_and_event_and_inner_pipeline": "event_value", // Overwrote by inner pipeline
		"parameters": map[string]string{
			"event_only":                         "event_value", // This should be deleted
			"build_and_event_and_inner_pipeline": "event_value", // Overwrote by build
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"event_only":                         "event_value",
				"build_and_event_and_inner_pipeline": "event_value",
			},
			"event_only":                         "event_value", // Remain
			"build_and_event_and_inner_pipeline": "event_value", // Overwrote by inner pipeline
		},
		"build": map[string]interface{}{
			"buildId":                            "event_value", // Overwrote by default value
			"jobId":                              "event_value", // Overwrote by default value
			"eventId":                            "event_value", // Overwrote by default value
			"pipelineId":                         "event_value", // Overwrote by default value
			"sha":                                "event_value", // Overwrote by default value
			"jobName":                            "event_value", // Overwrote by default value
			"coverageKey":                        "event_value", // Overwrote by default value
			"event_only":                         "event_value", // Remain
			"build_and_event_and_inner_pipeline": "event_value", // Overwrote by inner pipeline
		},
		"event": map[string]interface{}{
			"creator": "event_value", // Overwrote by default value
		},
	}

	innerParentBuildMeta := map[string]interface{}{
		"inner_pipeline_only":                "inner_pipeline_value", // Remain
		"build_and_event_and_inner_pipeline": "inner_pipeline_value", // Remain
		"parameters": map[string]string{
			"inner_pipeline_only":                "inner_pipeline_value", // This should be deleted
			"build_and_event_and_inner_pipeline": "inner_pipeline_value", // Overwrote by build
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"inner_pipeline_only":                "inner_pipeline_value",
				"build_and_event_and_inner_pipeline": "inner_pipeline_value",
			},
			"inner_pipeline_only":                "inner_pipeline_value", // Remain
			"build_and_event_and_inner_pipeline": "inner_pipeline_value", // Remain
		},
		"build": map[string]interface{}{
			"buildId":                            "inner_pipeline_value", // Overwrote by default value
			"jobId":                              "inner_pipeline_value", // Overwrote by default value
			"eventId":                            "inner_pipeline_value", // Overwrote by default value
			"pipelineId":                         "inner_pipeline_value", // Overwrote by default value
			"sha":                                "inner_pipeline_value", // Overwrote by default value
			"jobName":                            "inner_pipeline_value", // Overwrote by default value
			"coverageKey":                        "inner_pipeline_value", // Overwrote by default value
			"inner_pipeline_only":                "inner_pipeline_value", // Remain
			"build_and_event_and_inner_pipeline": "inner_pipeline_value", // Remain
		},
		"event": map[string]interface{}{
			"creator": "inner_pipeline_value", // Overwrote by default value
		},
	}

	oldMarshal := marshal
	defer func() { marshal = oldMarshal }()

	var InnerParentBuildID = TestParentBuildID
	var InnerParentJobID = TestParentJobID
	var InnerPipelineID = TestPipelineID

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		if buildID == InnerParentBuildID {
			// inner parent build
			return screwdriver.Build(FakeBuild{ID: buildID, JobID: InnerParentJobID, Meta: innerParentBuildMeta}), nil
		}
		// target build
		return screwdriver.Build(FakeBuild{ID: buildID, JobID: TestJobID, ParentBuildID: TestParentBuildIDFloat, Meta: buildFromIDMeta}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		if jobID == InnerParentJobID {
			// inner parent job
			return screwdriver.Job(FakeJob{ID: jobID, PipelineID: InnerPipelineID, Name: "inner_parent"}), nil
		}
		// target build
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: InnerPipelineID, Name: "main"}), nil
	}
	api.eventFromID = func(eventID int) (screwdriver.Event, error) {
		return screwdriver.Event(FakeEvent{ID: TestEventID, ParentEventID: TestParentEventID, Meta: eventFromIDMeta, Creator: TestEventCreator}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		if path == "./data/meta/meta.json" {
			defaultMeta = data
		}
		return nil
	}

	err, _, _ := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	want := fmt.Sprintf(`{
		"build_only": "build_value",
		"event_only": "event_value",
		"inner_pipeline_only": "inner_pipeline_value",
		"build_and_event_and_inner_pipeline": "inner_pipeline_value",
		"build":{
			"buildId": "%d",
			"jobId": "%d",
			"eventId": "0",
			"pipelineId": "%d",
			"sha": "",
			"jobName": "main",
			"coverageKey": "%s",
			"build_only": "build_value",
			"event_only": "event_value",
			"inner_pipeline_only": "inner_pipeline_value",
			"build_and_event_and_inner_pipeline": "inner_pipeline_value"
		},
		"event": {
			"creator": "%s"
		},
		"meta": {
			"build_only": "build_value",
			"event_only": "event_value",
			"inner_pipeline_only": "inner_pipeline_value",
			"build_and_event_and_inner_pipeline": "inner_pipeline_value"
		},
		"parameters": {
			"build_only": "build_value",
			"build_and_event_and_inner_pipeline": "build_value"
		}
	}`, TestBuildID, TestJobID, TestPipelineID, TestEnvVars["SD_SONAR_PROJECT_KEY"], TestEventCreator["username"])
	assert.JSONEq(t, want, string(defaultMeta))

	if err != nil {
		t.Errorf(fmt.Sprintf("err returned: %s", err.Error()))
	}
}

func TestMetaWhenTriggeredFromExternalPipelineByORLogicWithParentBuildMeta(t *testing.T) {
	initCoverageMeta()
	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()
	var defaultMeta, externalMeta []byte

	var InnerPipelineID = TestPipelineID
	var ExternalParentBuildID = TestParentBuildID
	var ExternalParentJobID = TestParentJobID
	var ExternalPipelineID = TestParentPipelineID

	buildFromIDMeta := map[string]interface{}{
		"build_only":                            "build_value", // Remain
		"build_and_event_and_external_pipeline": "build_value", // Overwrote by event
		"parameters": map[string]string{
			"build_only":                            "build_value", // Remain
			"build_and_event_and_external_pipeline": "build_value", // Remain
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"build_only":                            "build_value",
				"build_and_event_and_external_pipeline": "build_value",
			},
			"build_only":                            "build_value", // Remain
			"build_and_event_and_external_pipeline": "build_value", // Overwrote by event
		},
		"build": map[string]interface{}{
			"buildId":                               "build_value", // Overwrote by default value
			"jobId":                                 "build_value", // Overwrote by default value
			"eventId":                               "build_value", // Overwrote by default value
			"pipelineId":                            "build_value", // Overwrote by default value
			"sha":                                   "build_value", // Overwrote by default value
			"jobName":                               "build_value", // Overwrote by default value
			"coverageKey":                           "build_value", // Overwrote by default value
			"build_only":                            "build_value", // Remain
			"build_and_event_and_external_pipeline": "build_value", // Overwrote by event
		},
		"event": map[string]interface{}{
			"creator": "build_value", // Overwrote by default value
		},
		"sd": map[string]interface{}{
			"build_only": "build_value", // Remain
			strconv.Itoa(ExternalPipelineID): map[string]string{
				"build_only":      "build_value", // Remain
				"external_parent": "build_value", // This should be deleted
			},
		},
	}

	eventFromIDMeta := map[string]interface{}{
		"event_only":                            "event_value", // Remain
		"build_and_event_and_external_pipeline": "event_value", // Remain
		"parameters": map[string]string{
			"event_only":                            "event_value", // This should be deleted
			"build_and_event_and_external_pipeline": "event_value", // Remain
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"event_only":                            "event_value",
				"build_and_event_and_external_pipeline": "event_value",
			},
			"event_only":                            "event_value", // Remain
			"build_and_event_and_external_pipeline": "event_value", // Remain
		},
		"build": map[string]interface{}{
			"buildId":                               "event_value", // Overwrote by default value
			"jobId":                                 "event_value", // Overwrote by default value
			"eventId":                               "event_value", // Overwrote by default value
			"pipelineId":                            "event_value", // Overwrote by default value
			"sha":                                   "event_value", // Overwrote by default value
			"jobName":                               "event_value", // Overwrote by default value
			"coverageKey":                           "event_value", // Overwrote by default value
			"event_only":                            "event_value", // Remain
			"build_and_event_and_external_pipeline": "event_value", // Remain
		},
		"event": map[string]interface{}{
			"creator": "event_value", // Overwrote by default value
		},
		"sd": map[string]interface{}{
			"event_only": "event_value", // Remain
			strconv.Itoa(ExternalPipelineID): map[string]string{
				"event_only":      "event_value", // Remain
				"external_parent": "event_value", // This should be deleted
			},
		},
	}

	externalParentBuildMeta := map[string]interface{}{
		"external_pipeline_only":                "external_pipeline_value", // This should be deleted
		"build_and_event_and_external_pipeline": "external_pipeline_value", // Overwrote by event
		"parameters": map[string]string{
			"external_pipeline_only":                "external_pipeline_value", // This should be deleted
			"build_and_event_and_external_pipeline": "external_pipeline_value", // Overwrote by build
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"external_pipeline_only":                "external_pipeline_value",
				"build_and_event_and_external_pipeline": "external_pipeline_value",
			},
			"external_pipeline_only":                "external_pipeline_value", // This should be deleted
			"build_and_event_and_external_pipeline": "external_pipeline_value", // Overwrote by event
		},
		"build": map[string]interface{}{
			"buildId":                               "external_pipeline_value", // Overwrote by default value
			"jobId":                                 "external_pipeline_value", // Overwrote by default value
			"eventId":                               "external_pipeline_value", // Overwrote by default value
			"pipelineId":                            "external_pipeline_value", // Overwrote by default value
			"sha":                                   "external_pipeline_value", // Overwrote by default value
			"jobName":                               "external_pipeline_value", // Overwrote by default value
			"coverageKey":                           "external_pipeline_value", // Overwrote by default value
			"external_pipeline_only":                "external_pipeline_value", // This should be deleted
			"build_and_event_and_external_pipeline": "external_pipeline_value", // Overwrote by event
		},
		"event": map[string]interface{}{
			"creator": "external_pipeline_value", // Overwrote by default value
		},
		"sd": map[string]interface{}{
			"external_pipeline_only": "external_pipeline_value", // This should be deleted
			strconv.Itoa(ExternalPipelineID): map[string]string{
				"external_pipeline_only": "external_pipeline_value", // This should be deleted
				"external_parent":        "external_pipeline_value", // This should be deleted
			},
		},
	}

	oldMarshal := marshal
	defer func() { marshal = oldMarshal }()

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		if buildID == ExternalParentBuildID {
			// external parent build
			return screwdriver.Build(FakeBuild{ID: buildID, JobID: ExternalParentJobID, Meta: externalParentBuildMeta}), nil
		}
		// target build
		return screwdriver.Build(FakeBuild{ID: buildID, JobID: TestJobID, ParentBuildID: TestParentBuildIDFloat, Meta: buildFromIDMeta}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		if jobID == ExternalParentJobID {
			// external parent job
			return screwdriver.Job(FakeJob{ID: jobID, PipelineID: ExternalPipelineID, Name: "external_parent"}), nil
		}
		// target build
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: InnerPipelineID, Name: "main"}), nil
	}
	api.eventFromID = func(eventID int) (screwdriver.Event, error) {
		return screwdriver.Event(FakeEvent{ID: TestEventID, ParentEventID: TestParentEventID, Meta: eventFromIDMeta, Creator: TestEventCreator}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		if path == "./data/meta/meta.json" {
			defaultMeta = data
		}
		if path == fmt.Sprintf("./data/meta/sd@%d:external_parent.json", ExternalPipelineID) {
			externalMeta = data
		}
		return nil
	}

	err, _, _ := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	want := fmt.Sprintf(`{
		"build_only": "build_value",
		"event_only": "event_value",
		"build_and_event_and_external_pipeline": "event_value",
		"build":{
			"buildId": "%d",
			"jobId": "%d",
			"eventId": "0",
			"pipelineId": "%d",
			"sha": "",
			"jobName": "main",
			"coverageKey": "%s",
			"build_only": "build_value",
			"event_only": "event_value",
			"build_and_event_and_external_pipeline": "event_value"
		},
		"event": {
			"creator": "%s"
		},
		"meta": {
			"build_only": "build_value",
			"event_only": "event_value",
			"build_and_event_and_external_pipeline": "event_value"
		},
		"parameters": {
			"build_only": "build_value",
			"build_and_event_and_external_pipeline": "build_value"
		},
		"sd": {
			"build_only": "build_value",
			"event_only": "event_value",
			"%d":{
				"build_only": "build_value",
				"event_only": "event_value"
			}
		}
	}`, TestBuildID, TestJobID, TestPipelineID, TestEnvVars["SD_SONAR_PROJECT_KEY"], TestEventCreator["username"], ExternalPipelineID)
	assert.JSONEq(t, want, string(defaultMeta))

	wantExternalMetaByte, _ := marshal(externalParentBuildMeta)
	assert.JSONEq(t, string(wantExternalMetaByte), string(externalMeta))

	if err != nil {
		t.Errorf(fmt.Sprintf("err returned: %s", err.Error()))
	}
}

func TestMetaWhenStartFromAnyJobWithParentEvent(t *testing.T) {
	initCoverageMeta()
	oldWriteFile := writeFile
	defer func() { writeFile = oldWriteFile }()
	var defaultMeta []byte

	buildFromIDMeta := map[string]interface{}{
		"build_only":                       "build_value", // Remain
		"build_and_event_and_parent_event": "build_value", // Overwrote parent event
		"parameters": map[string]string{
			"build_only":                       "build_value", // Remain
			"build_and_event_and_parent_event": "build_value", // Remain
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"build_only":                       "build_value",
				"build_and_event_and_parent_event": "build_value",
			},
			"build_only":                       "build_value", // Remain
			"build_and_event_and_parent_event": "build_value", // Overwrote by parent event
		},
		"build": map[string]interface{}{
			"buildId":                          "build_value", // Overwrote by default value
			"jobId":                            "build_value", // Overwrote by default value
			"eventId":                          "build_value", // Overwrote by default value
			"pipelineId":                       "build_value", // Overwrote by default value
			"sha":                              "build_value", // Overwrote by default value
			"jobName":                          "build_value", // Overwrote by default value
			"coverageKey":                      "build_value", // Overwrote by default value
			"build_only":                       "build_value", // Remain
			"build_and_event_and_parent_event": "build_value", // Overwrote by parent event
		},
		"event": map[string]interface{}{
			"creator": "build_value", // Overwrote by default value
		},
	}

	eventFromIDMeta := map[string]interface{}{
		"event_only":                       "event_value", // Remain
		"build_and_event_and_parent_event": "event_value", // Overwrote by parent event
		"parameters": map[string]string{
			"event_only":                       "event_value", // This should be deleted
			"build_and_event_and_parent_event": "event_value", // Overwrote by build
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"event_only":                       "event_value",
				"build_and_event_and_parent_event": "event_value",
			},
			"event_only":                       "event_value", // Remain
			"build_and_event_and_parent_event": "event_value", // Overwrote by parent event
		},
		"build": map[string]interface{}{
			"buildId":                          "event_value", // Overwrote by default value
			"jobId":                            "event_value", // Overwrote by default value
			"eventId":                          "event_value", // Overwrote by default value
			"pipelineId":                       "event_value", // Overwrote by default value
			"sha":                              "event_value", // Overwrote by default value
			"jobName":                          "event_value", // Overwrote by default value
			"coverageKey":                      "event_value", // Overwrote by default value
			"event_only":                       "event_value", // Remain
			"build_and_event_and_parent_event": "event_value", // Overwrote by parent event
		},
		"event": map[string]interface{}{
			"creator": "event_value", // Overwrote by default value
		},
	}

	parentEventMeta := map[string]interface{}{
		"parent_event_only":                "parent_event_value", // Remain
		"build_and_event_and_parent_event": "parent_event_value", // Remain
		"parameters": map[string]string{
			"parent_event_only":                "parent_event_value", // This should be deleted
			"build_and_event_and_parent_event": "parent_event_value", // Overwrote by build
		},
		"meta": map[string]interface{}{
			"summary": map[string]string{ // This should be deleted
				"parent_event_only":                "parent_event_value",
				"build_and_event_and_parent_event": "parent_event_value",
			},
			"parent_event_only":                "parent_event_value", // Remain
			"build_and_event_and_parent_event": "parent_event_value", // Remain
		},
		"build": map[string]interface{}{
			"buildId":                          "parent_event_value", // Overwrote by default value
			"jobId":                            "parent_event_value", // Overwrote by default value
			"eventId":                          "parent_event_value", // Overwrote by default value
			"pipelineId":                       "parent_event_value", // Overwrote by default value
			"sha":                              "parent_event_value", // Overwrote by default value
			"jobName":                          "parent_event_value", // Overwrote by default value
			"coverageKey":                      "parent_event_value", // Overwrote by default value
			"parent_event_only":                "parent_event_value", // Remain
			"build_and_event_and_parent_event": "parent_event_value", // Remain
			"warning":                          "parent_event_value", // Removed from merged meta
		},
		"event": map[string]interface{}{
			"creator": "parent_event_value", // Overwrote by default value
		},
	}

	api := mockAPI(t, TestBuildID, TestJobID, 0, "RUNNING")
	api.buildFromID = func(buildID int) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: buildID, JobID: TestJobID, Meta: buildFromIDMeta}), nil
	}
	api.jobFromID = func(jobID int) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{ID: jobID, PipelineID: TestPipelineID, Name: "main"}), nil
	}
	api.eventFromID = func(eventID int) (screwdriver.Event, error) {
		if eventID == TestParentEventID {
			// parent event
			return screwdriver.Event(FakeEvent{ID: TestEventID, Meta: parentEventMeta}), nil
		}
		return screwdriver.Event(FakeEvent{ID: TestEventID, ParentEventID: TestParentEventID, Meta: eventFromIDMeta, Creator: TestEventCreator}), nil
	}
	api.pipelineFromID = func(pipelineID int) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ID: pipelineID, ScmURI: TestScmURI, ScmRepo: TestScmRepo}), nil
	}
	writeFile = func(path string, data []byte, perm os.FileMode) (err error) {
		if path == "./data/meta/meta.json" {
			defaultMeta = data
		}
		return nil
	}

	err, _, _ := launch(screwdriver.API(api), TestBuildID, TestWorkspace, TestEmitter, TestMetaSpace, TestStoreURL, TestUIURL, TestShellBin, TestBuildTimeout, TestBuildToken, "", "", "", "", false, false, false, 0, 10000)
	want := fmt.Sprintf(`{
		"build_only": "build_value",
		"event_only": "event_value",
		"parent_event_only": "parent_event_value",
		"build_and_event_and_parent_event": "parent_event_value",
		"build":{
			"buildId": "%d",
			"jobId": "%d",
			"eventId": "0",
			"pipelineId": "%d",
			"sha": "",
			"jobName": "main",
			"coverageKey": "%s",
			"build_only": "build_value",
			"event_only": "event_value",
			"parent_event_only": "parent_event_value",
			"build_and_event_and_parent_event": "parent_event_value"
		},
		"event": {
			"creator": "%s"
		},
		"meta":{
			"build_only": "build_value",
			"event_only": "event_value",
			"parent_event_only": "parent_event_value",
			"build_and_event_and_parent_event": "parent_event_value"
		},
		"parameters":{
			"build_only": "build_value",
			"build_and_event_and_parent_event": "build_value"
		}
	}`, TestBuildID, TestJobID, TestPipelineID, TestEnvVars["SD_SONAR_PROJECT_KEY"], TestEventCreator["username"])

	assert.JSONEq(t, want, string(defaultMeta))
	if err != nil {
		t.Errorf(fmt.Sprintf("err returned: %s", err.Error()))
	}
}

func fakeHttpClient() *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := strings.Split(r.URL.String(), "&")
		code, _ := strconv.Atoi(data[1])
		body := data[1]
		timeout, _ := strconv.Atoi(data[2])
		timeoutDuration := time.Duration(timeout) * time.Second
		w.WriteHeader(code)
		if timeout > 0 {
			time.Sleep(timeoutDuration)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(body))
		}
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func TestMakePushgatewayURL(t *testing.T) {
	// SD_PUSHGATEWAY_URL https protocol
	expected := "https://fake.pushgateway.url:9001/metrics/job/containerd/instance/1"
	pushgatewayURL, err := makePushgatewayURL("https://fake.pushgateway.url:9001/", 1)
	assert.Equal(t, expected, pushgatewayURL)
	if err != nil {
		assert.Fail(t, "Failed to parse url")
	}

	// SD_PUSHGATEWAY_URL no protocol
	expected = "http://fake.pushgateway.url/metrics/job/containerd/instance/1"
	pushgatewayURL, err = makePushgatewayURL("fake.pushgateway.url", 1)
	assert.Equal(t, expected, pushgatewayURL)
	if err != nil {
		assert.Fail(t, "Failed to parse url")
	}

	// SD_PUSHGATEWAY_URL has port and no protocol
	expected = "http://fake.pushgateway.url:9001/metrics/job/containerd/instance/1"
	pushgatewayURL, err = makePushgatewayURL("fake.pushgateway.url:9001", 1)
	assert.Equal(t, expected, pushgatewayURL)
	if err != nil {
		assert.Fail(t, "Failed to parse url")
	}

	// SD_PUSHGATEWAY_URL Invalid url
	pushgatewayURL, err = makePushgatewayURL("http://\nfake.pushgateway.url", 1)
	assert.Error(t, err, "Valid url")

}

func TestPushMetrics(t *testing.T) {
	httpTest := fakeHttpClient()
	client.HTTPClient = httpTest
	ts := time.Now().Unix() - 2000
	os.Setenv("CONTAINER_IMAGE", "dummy_image")

	// SD_PUSHGATEWAY_URL null
	os.Setenv("SD_PUSHGATEWAY_URL", "")
	err := pushMetrics("success", 1)
	if err != nil {
		t.Errorf("Push metrics expect to return [nil] but got [%v]", err)
	}

	// SD_PUSHGATEWAY_URL no protocol
	os.Setenv("SD_PUSHGATEWAY_URL", "fake.pushgateway.url&200&0")
	err = pushMetrics("success", 1)
	if err != nil {
		t.Errorf("Push metrics expect to return [nil] but got [%v]", err)
	}

	// SD_PUSHGATEWAY_URL Invalid url
	os.Setenv("SD_PUSHGATEWAY_URL", "http://\nfake.pushgateway.url&200&0")
	err = pushMetrics("success", 1)
	assert.Error(t, err, "Push metrics expect to return error")

	// build id 0
	os.Setenv("SD_PUSHGATEWAY_URL", "http://fake.pushgateway.url&200&0")
	err = pushMetrics("success", 0)
	if err != nil {
		t.Errorf("Push metrics expect to return [nil] but got [%v]", err)
	}

	// 200 success
	os.Setenv("SD_PUSHGATEWAY_URL", "http://fake.pushgateway.url&200&0")
	os.Setenv("SD_LAUNCHER_END_TS", strconv.FormatInt(ts, 10))
	err = pushMetrics("success", 1)
	if err != nil {
		t.Errorf("Push metrics expect to return [nil] but got [%v]", err)
	}

	// 200 success, launcher end timestamp null / blank
	os.Setenv("SD_PUSHGATEWAY_URL", "http://fake.pushgateway.url&200&0")
	os.Setenv("SD_LAUNCHER_END_TS", "")
	err = pushMetrics("success", 1)
	if err != nil {
		t.Errorf("Push metrics expect to return [nil] but got [%v]", err)
	}

	// 400
	os.Setenv("SD_PUSHGATEWAY_URL", "http://fake.pushgateway.url&400&0")
	os.Setenv("SD_LAUNCHER_END_TS", strconv.FormatInt(ts, 10))
	err = pushMetrics("success", 1)
	if err != nil {
		expected := "pushMetrics: failed to push metrics to [http://fake.pushgateway.url&400&0/metrics/job/containerd/instance/1], buildId:[1], response status code:[400]"
		assert.Equal(t, expected, err.Error())
	} else {
		assert.Fail(t, "Push metrics expect to return error")
	}

	// 200 success
	os.Setenv("SD_PUSHGATEWAY_URL", "http://fake.pushgateway.url&200&0")
	os.Setenv("SD_LAUNCHER_END_TS", strconv.FormatInt(ts, 10))
	err = pushMetrics("failed", 1)
	if err != nil {
		t.Errorf("Push metrics expect to return [nil] but got [%v]", err)
	}

	// 500
	os.Setenv("SD_PUSHGATEWAY_URL", "http://fake.pushgateway.url&500&0")
	os.Setenv("SD_LAUNCHER_END_TS", strconv.FormatInt(ts, 10))
	err = pushMetrics("failed", 1)
	if err != nil {
		expected := "pushMetrics: failed to push metrics to [http://fake.pushgateway.url&500&0/metrics/job/containerd/instance/1], buildId:[1], response status code:[500]"
		assert.Equal(t, expected, err.Error())
	} else {
		assert.Fail(t, "Push metrics expect to return error")
	}

	// 504
	pushgatewayURLTimeout = 1
	os.Setenv("SD_PUSHGATEWAY_URL", "http://fake.pushgateway.url&504&3")
	os.Setenv("SD_LAUNCHER_END_TS", strconv.FormatInt(ts, 10))
	err = pushMetrics("success", 1)
	if err != nil {
		expected := "pushMetrics: failed to push metrics to [http://fake.pushgateway.url&504&3/metrics/job/containerd/instance/1], buildId:[1], response status code:[504]"
		assert.Equal(t, expected, err.Error())
	} else {
		assert.Fail(t, "Push metrics expect to return error")
	}
}

func TestSignalsAreDelivered(t *testing.T) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)
	defer signal.Stop(c)
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	t.Logf("sigterm...")
	main()
	t.Logf("sigterm...")
}
