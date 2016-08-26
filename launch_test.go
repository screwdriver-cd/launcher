package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/screwdriver-cd/launcher/executor"
	"github.com/screwdriver-cd/launcher/git"
	"github.com/screwdriver-cd/launcher/screwdriver"
)

type FakeBuild screwdriver.Build
type FakeJob screwdriver.Job
type FakePipeline screwdriver.Pipeline
type FakePipelineDef screwdriver.PipelineDef

type MockRepo struct {
	checkout func() error
	mergePR  func(string, string) error
	getPath  func() string
}

func (r MockRepo) Checkout() error {
	if r.checkout != nil {
		return r.checkout()
	}
	return nil
}

func (r MockRepo) MergePR(prNumber, sha string) error {
	if r.mergePR != nil {
		return r.mergePR(prNumber, sha)
	}
	return nil
}

func (r MockRepo) GetPath() string {
	if r.getPath != nil {
		return r.getPath()
	}
	return ""
}

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
			return screwdriver.Pipeline(FakePipeline{}), nil
		},
		updateBuildStatus: func(status screwdriver.BuildStatus) error {
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
	buildFromID         func(string) (screwdriver.Build, error)
	jobFromID           func(string) (screwdriver.Job, error)
	pipelineFromID      func(string) (screwdriver.Pipeline, error)
	updateBuildStatus   func(screwdriver.BuildStatus) error
	pipelineDefFromYaml func(io.Reader) (screwdriver.PipelineDef, error)
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

func (f MockAPI) UpdateBuildStatus(status screwdriver.BuildStatus) error {
	if f.updateBuildStatus != nil {
		return f.updateBuildStatus(status)
	}
	return nil
}

func fakePipelineDef() screwdriver.PipelineDef {
	jobDef := screwdriver.JobDef{
		Commands: []screwdriver.CommandDef{
			{
				Name: "testcmd",
				Cmd:  "cmd",
			},
		},
	}
	def := FakePipelineDef{
		Jobs: map[string][]screwdriver.JobDef{
			"main": {
				jobDef,
			},
		},
	}

	return screwdriver.PipelineDef(def)
}

func (f MockAPI) PipelineDefFromYaml(yaml io.Reader) (screwdriver.PipelineDef, error) {
	if f.pipelineDefFromYaml != nil {
		return f.pipelineDefFromYaml(yaml)
	}
	return screwdriver.PipelineDef(fakePipelineDef()), nil
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

	oldEmitterPath := emitterPath
	emitterPath = path.Join(tmp, "socket")
	if _, err = os.Create(emitterPath); err != nil {
		t.Fatalf("Error creating test socket: %v", err)
	}
	return tmp, func() {
		os.RemoveAll(tmp)
		emitterPath = oldEmitterPath
	}
}

func TestMain(m *testing.M) {
	mkdirAll = func(path string, perm os.FileMode) (err error) { return nil }
	stat = func(path string) (info os.FileInfo, err error) { return nil, os.ErrExist }
	newRepo = func(scmURL, path string) (git.Repo, error) {
		repo := MockRepo{}
		return git.Repo(repo), nil
	}
	open = func(f string) (*os.File, error) {
		return os.Open("data/screwdriver.yaml")
	}
	executorRun = func(path string, emitter screwdriver.Emitter, jobDef screwdriver.JobDef) error { return nil }
	cleanExit = func() {}
	writeFile = func(string, []byte, os.FileMode) error { return nil }
	os.Exit(m.Run())
}

func TestBuildJobPipelineFromID(t *testing.T) {
	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testPipelineID := "PIPELINEID"
	testRoot := "/sd/workspace"
	api := mockAPI(t, testBuildID, testJobID, testPipelineID, "RUNNING")
	launch(screwdriver.API(api), testBuildID, testRoot)
}

func TestBuildFromIdError(t *testing.T) {
	api := MockAPI{
		buildFromID: func(buildID string) (screwdriver.Build, error) {
			err := fmt.Errorf("testing error returns")
			return screwdriver.Build(FakeBuild{}), err
		},
	}

	err := launch(screwdriver.API(api), "shoulderror", "/sd/workspace")
	if err == nil {
		t.Errorf("err should not be nil")
	}

	expected := `fetching build ID "shoulderror"`
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestJobFromIdError(t *testing.T) {
	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testRoot := "/sd/workspace"
	api := mockAPI(t, testBuildID, testJobID, "", "RUNNING")
	api.jobFromID = func(jobID string) (screwdriver.Job, error) {
		err := fmt.Errorf("testing error returns")
		return screwdriver.Job(FakeJob{}), err
	}

	err := launch(screwdriver.API(api), testBuildID, testRoot)
	if err == nil {
		t.Errorf("err should not be nil")
	}

	expected := fmt.Sprintf(`fetching Job ID %q`, testJobID)
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestPipelineFromIdError(t *testing.T) {
	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testPipelineID := "PIPELINEID"
	testRoot := "/sd/workspace"
	api := mockAPI(t, testBuildID, testJobID, testPipelineID, "RUNNING")
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		err := fmt.Errorf("testing error returns")
		return screwdriver.Pipeline(FakePipeline{}), err
	}

	err := launch(screwdriver.API(api), testBuildID, testRoot)
	if err == nil {
		t.Fatalf("err should not be nil")
	}

	expected := fmt.Sprintf(`fetching Pipeline ID %q`, testPipelineID)
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestParseScmURL(t *testing.T) {
	wantHost := "github.com"
	wantOrg := "screwdriver-cd"
	wantRepo := "launcher.git"
	wantBranch := "master"

	scmURL := "git@github.com:screwdriver-cd/launcher.git#master"
	parsedURL, err := parseScmURL(scmURL)
	host, org, repo, branch := parsedURL.Host, parsedURL.Org, parsedURL.Repo, parsedURL.Branch
	if err != nil {
		t.Errorf("Unexpected error parsing SCM URL %q: %v", scmURL, err)
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

	if parsedURL.String() != scmURL {
		t.Errorf("parsedURL.String() == %q, want %q", parsedURL.String(), scmURL)
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
	testRoot := "/sd/workspace"

	workspace, err := createWorkspace(testRoot, "screwdriver-cd", "launcher.git")

	if err != nil {
		t.Errorf("Unexpected error creating workspace: %v", err)
	}

	wantWorkspace := Workspace{
		Root:      testRoot,
		Src:       "/sd/workspace/src/screwdriver-cd/launcher.git",
		Artifacts: "/sd/workspace/artifacts",
	}
	if workspace != wantWorkspace {
		t.Errorf("workspace = %q, want %q", workspace, wantWorkspace)
	}

	wantDirs := map[string]os.FileMode{
		"/sd/workspace/src/screwdriver-cd/launcher.git": 0777,
		"/sd/workspace/artifacts":                       0777,
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

func TestHttpsString(t *testing.T) {
	testScmPath := scmPath{
		Host:   "github.com",
		Org:    "screwdriver-cd",
		Repo:   "launcher.git",
		Branch: "master",
	}

	wantHTTPSString := "https://github.com/screwdriver-cd/launcher.git#master"

	httpsString := testScmPath.httpsString()

	if httpsString != wantHTTPSString {
		t.Errorf("httpsString == %q, want %q", httpsString, wantHTTPSString)
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

func TestNonPR(t *testing.T) {
	oldNewRepo := newRepo
	defer func() { newRepo = oldNewRepo }()

	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testSCMURL := "git@github.com:screwdriver-cd/launcher.git#master"
	testRoot := "/sd/workspace"
	testSHA := "abc123"

	api := mockAPI(t, testBuildID, testJobID, "", "RUNNING")
	api.buildFromID = func(buildID string) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: testBuildID, JobID: testJobID, SHA: testSHA}), nil
	}
	api.jobFromID = func(jobID string) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{Name: "main"}), nil
	}
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ScmURL: testSCMURL}), nil
	}
	newRepo = func(scmURL, path string) (git.Repo, error) {
		repo := MockRepo{}
		return repo, nil
	}

	launch(screwdriver.API(api), testBuildID, testRoot)
}

func TestPR(t *testing.T) {
	oldNewRepo := newRepo
	defer func() { newRepo = oldNewRepo }()

	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testSCMURL := "git@github.com:screwdriver-cd/launcher.git#master"
	testPrNumber := "1"
	testRoot := "/sd/workspace"
	testSHA := "abc123"
	api := mockAPI(t, testBuildID, testJobID, "", "RUNNING")
	api.buildFromID = func(buildID string) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: testBuildID, JobID: testJobID, SHA: testSHA}), nil
	}
	api.jobFromID = func(jobID string) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{Name: "PR-1"}), nil
	}
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ScmURL: testSCMURL}), nil
	}
	newRepo = func(scmURL, path string) (git.Repo, error) {
		repo := MockRepo{}
		repo.mergePR = func(prNumber, sha string) error {
			if prNumber != testPrNumber {
				t.Errorf("pr number is %q, want %q", prNumber, testPrNumber)
			}
			if sha != testSHA {
				t.Errorf("sha is %q, want %q", sha, testSHA)
			}
			return nil
		}
		return repo, nil
	}

	launch(screwdriver.API(api), testBuildID, testRoot)
}

func TestCreateWorkspaceError(t *testing.T) {
	oldMkdir := mkdirAll
	defer func() { mkdirAll = oldMkdir }()

	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testSCMURL := "git@github.com:screwdriver-cd/launcher.git#master"
	testRoot := "/sd/workspace"
	api := mockAPI(t, testBuildID, testJobID, "", "RUNNING")
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ScmURL: testSCMURL}), nil
	}
	mkdirAll = func(path string, perm os.FileMode) (err error) {
		return fmt.Errorf("Spooky error")
	}

	err := launch(screwdriver.API(api), testBuildID, testRoot)

	if err.Error() != "Cannot create workspace path \"/sd/workspace/src/screwdriver-cd/launcher.git\": Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestCreateWorkspaceBadStat(t *testing.T) {
	oldStat := stat
	defer func() { stat = oldStat }()

	stat = func(path string) (info os.FileInfo, err error) {
		return nil, nil
	}
	testRoot := "/sd/workspace"

	wantWorkspace := Workspace{}

	workspace, err := createWorkspace(testRoot, "screwdriver-cd", "launcher.git")

	if err.Error() != "Cannot create workspace path \"/sd/workspace/src/screwdriver-cd/launcher.git\", path already exists." {
		t.Errorf("Error is wrong, got %v", err)
	}

	if workspace != wantWorkspace {
		t.Errorf("Workspace == %q, want %q", workspace, wantWorkspace)
	}
}

func TestUpdateBuildStatusError(t *testing.T) {
	testBuildID := "BUILDID"
	testRoot := "/sd/workspace"
	api := mockAPI(t, testBuildID, "", "", screwdriver.Running)
	api.updateBuildStatus = func(status screwdriver.BuildStatus) error {
		return fmt.Errorf("Spooky error")
	}

	err := launch(screwdriver.API(api), testBuildID, testRoot)

	want := "updating build status to RUNNING: Spooky error"
	if err.Error() != want {
		t.Errorf("Error is wrong. got %v, want %v", err, want)
	}
}

func TestPipelineDefFromYaml(t *testing.T) {
	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testRoot := "/sd/workspace"
	testPath := "test/path"

	mainJob := screwdriver.JobDef{
		Image: "node:4",
		Commands: []screwdriver.CommandDef{
			{
				Name: "install",
				Cmd:  "npm install",
			},
		},
		Environment: map[string]string{
			"NUMBER": "3",
		},
	}
	wantJobs := map[string][]screwdriver.JobDef{
		"main": {
			mainJob,
		},
	}

	oldNewRepo := newRepo
	defer func() { newRepo = oldNewRepo }()

	newRepo = func(scmURL, path string) (git.Repo, error) {
		repo := MockRepo{}
		repo.getPath = func() string {
			return "test/path"
		}
		return repo, nil
	}

	oldOpen := open
	defer func() { open = oldOpen }()
	open = func(f string) (*os.File, error) {
		if f != "/sd/workspace/src/screwdriver.yaml" {
			t.Errorf("File name not correct: %q", f)
		}

		return os.Open("data/screwdriver.yaml")
	}

	_, cleanup := setupTempDirectoryAndSocket(t)
	defer cleanup()

	api := mockAPI(t, testBuildID, testJobID, "", screwdriver.Running)

	api.jobFromID = func(jobID string) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{Name: "main"}), nil
	}
	api.pipelineDefFromYaml = func(yaml io.Reader) (screwdriver.PipelineDef, error) {
		return screwdriver.PipelineDef(FakePipelineDef{
			Jobs: wantJobs,
		}), nil
	}

	oldRun := executorRun
	defer func() { executorRun = oldRun }()
	executorRun = func(path string, out screwdriver.Emitter, jobDef screwdriver.JobDef) error {
		cmdDefs := jobDef.Commands
		env := jobDef.Environment
		if path != testPath {
			t.Errorf("Executor run path = %v, want %v", path, testPath)
		}
		if !reflect.DeepEqual(cmdDefs, wantJobs["main"][0].Commands) {
			t.Errorf("Executor run jobDef = %+v, \n want %+v", cmdDefs, wantJobs["main"])
		}
		if !reflect.DeepEqual(env, wantJobs["main"][0].Environment) {
			t.Errorf("Executor run env = %+v, \n want %+v", env, wantJobs["main"][0].Environment)
		}
		return nil
	}

	if err := launch(screwdriver.API(api), testBuildID, testRoot); err != nil {
		t.Errorf("Launch returned error: %v", err)
	}
}

func TestUpdateBuildStatusSuccess(t *testing.T) {
	wantStatuses := []screwdriver.BuildStatus{
		screwdriver.Running,
		screwdriver.Success,
	}

	var gotStatuses []screwdriver.BuildStatus
	api := mockAPI(t, "testBuildID", "testJobID", "testPipelineID", "")
	api.updateBuildStatus = func(status screwdriver.BuildStatus) error {
		gotStatuses = append(gotStatuses, status)
		return nil
	}

	oldMkdirAll := mkdirAll
	defer func() { mkdirAll = oldMkdirAll }()
	mkdirAll = os.MkdirAll

	tmp, cleanup := setupTempDirectoryAndSocket(t)
	defer cleanup()

	if err := launchAction(screwdriver.API(api), "testBuildID", tmp); err != nil {
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
	api := mockAPI(t, "testBuildID", "testJobID", "testPipelineID", "")
	api.updateBuildStatus = func(status screwdriver.BuildStatus) error {
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

	executorRun = func(path string, out screwdriver.Emitter, jobDef screwdriver.JobDef) error {
		return executor.ErrStatus{Status: 1}
	}

	err = launchAction(screwdriver.API(api), "testBuildID", tmp)
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
	api := mockAPI(t, "testBuildID", "", "", screwdriver.Running)

	updCalled := false
	api.updateBuildStatus = func(status screwdriver.BuildStatus) error {
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
		defer recoverPanic(api)
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
		defer recoverPanic(nil)
		panic("OH NOES!")
	}()

	if !exitCalled {
		t.Errorf("Explicit exit not called")
	}
}

func TestNewRepoError(t *testing.T) {
	oldNewRepo := newRepo
	defer func() { newRepo = oldNewRepo }()

	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testSCMURL := "git@github.com:screwdriver-cd/launcher.git#master"
	testRoot := "/sd/workspace"
	testSHA := "abc123"

	api := mockAPI(t, testBuildID, testJobID, "", "RUNNING")
	api.buildFromID = func(buildID string) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: testBuildID, JobID: testJobID, SHA: testSHA}), nil
	}
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ScmURL: testSCMURL}), nil
	}
	newRepo = func(scmURL, path string) (git.Repo, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	err := launch(screwdriver.API(api), testBuildID, testRoot)
	if err.Error() != "Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}
func TestCheckoutError(t *testing.T) {
	oldNewRepo := newRepo
	defer func() { newRepo = oldNewRepo }()

	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testSCMURL := "git@github.com:screwdriver-cd/launcher.git#master"
	testRoot := "/sd/workspace"
	testSHA := "abc123"

	api := mockAPI(t, testBuildID, testJobID, "", "RUNNING")
	api.buildFromID = func(buildID string) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: testBuildID, JobID: testJobID, SHA: testSHA}), nil
	}
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ScmURL: testSCMURL}), nil
	}
	newRepo = func(scmURL, path string) (git.Repo, error) {
		repo := MockRepo{}
		repo.checkout = func() error {
			return fmt.Errorf("Spooky error")
		}
		return repo, nil
	}

	err := launch(screwdriver.API(api), testBuildID, testRoot)
	if err.Error() != "Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestMergePRError(t *testing.T) {
	oldNewRepo := newRepo
	defer func() { newRepo = oldNewRepo }()

	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testSCMURL := "git@github.com:screwdriver-cd/launcher.git#master"
	testRoot := "/sd/workspace"
	testSHA := "abc123"

	api := mockAPI(t, testBuildID, testJobID, "", "RUNNING")
	api.buildFromID = func(buildID string) (screwdriver.Build, error) {
		return screwdriver.Build(FakeBuild{ID: testBuildID, JobID: testJobID, SHA: testSHA}), nil
	}
	api.jobFromID = func(jobID string) (screwdriver.Job, error) {
		return screwdriver.Job(FakeJob{Name: "PR-1"}), nil
	}
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ScmURL: testSCMURL}), nil
	}

	newRepo = func(scmURL, path string) (git.Repo, error) {
		repo := MockRepo{}
		repo.mergePR = func(prNumber, sha string) error {
			return fmt.Errorf("Spooky error")
		}
		return repo, nil
	}

	err := launch(screwdriver.API(api), testBuildID, testRoot)
	if err.Error() != "Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestEmitterClose(t *testing.T) {
	testRoot := "/sd/workspace"
	api := mockAPI(t, "testBuildID", "testJobID", "testPipelineID", "")
	api.updateBuildStatus = func(status screwdriver.BuildStatus) error {
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

	if err := launchAction(screwdriver.API(api), "testBuildID", testRoot); err != nil {
		t.Errorf("Unexpected error from launch: %v", err)
	}

	if !called {
		t.Errorf("Did not close the emitter")
	}
}
