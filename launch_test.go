package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/screwdriver-cd/launcher/screwdriver"
)

type FakeBuild screwdriver.Build
type FakeJob screwdriver.Job
type FakePipeline screwdriver.Pipeline

func mockAPI(t *testing.T, testBuildID, testJobID, testPipelineID string) MockAPI {
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
			return screwdriver.Job(FakeJob{ID: testJobID, PipelineID: testPipelineID}), nil
		},
		pipelineFromID: func(pipelineID string) (screwdriver.Pipeline, error) {
			if pipelineID != testPipelineID {
				t.Errorf("pipelineID == %s, want %s", pipelineID, testPipelineID)
				// Panic to get the stacktrace
				panic(true)
			}
			return screwdriver.Pipeline(FakePipeline{}), nil
		},
	}
}

type MockAPI struct {
	buildFromID    func(string) (screwdriver.Build, error)
	jobFromID      func(string) (screwdriver.Job, error)
	pipelineFromID func(string) (screwdriver.Pipeline, error)
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

func TestMain(m *testing.M) {
	mkdirAll = func(path string, perm os.FileMode) (err error) { return nil }
	stat = func(path string) (info os.FileInfo, err error) { return nil, os.ErrExist }
	os.Exit(m.Run())
}

func TestBuildJobPipelineFromID(t *testing.T) {
	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testPipelineID := "PIPELINEID"
	api := mockAPI(t, testBuildID, testJobID, testPipelineID)
	launch(screwdriver.API(api), testBuildID)
}

func TestBuildFromIdError(t *testing.T) {
	api := MockAPI{
		buildFromID: func(buildID string) (screwdriver.Build, error) {
			err := fmt.Errorf("testing error returns")
			return screwdriver.Build(FakeBuild{}), err
		},
	}

	err := launch(screwdriver.API(api), "shoulderror")
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
	api := mockAPI(t, testBuildID, testJobID, "")
	api.jobFromID = func(jobID string) (screwdriver.Job, error) {
		err := fmt.Errorf("testing error returns")
		return screwdriver.Job(FakeJob{}), err
	}

	err := launch(screwdriver.API(api), testBuildID)
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
	api := mockAPI(t, testBuildID, testJobID, testPipelineID)
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		err := fmt.Errorf("testing error returns")
		return screwdriver.Pipeline(FakePipeline{}), err
	}

	err := launch(screwdriver.API(api), testBuildID)
	if err == nil {
		t.Fatalf("err should not be nil")
	}

	expected := fmt.Sprintf(`fetching Pipeline ID %q`, testPipelineID)
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("err == %q, want %q", err, expected)
	}
}

func TestParseScmURL(t *testing.T) {
	wantHost := "git@github.com"
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

	workspace, err := createWorkspace("screwdriver-cd", "launcher.git")

	if err != nil {
		t.Errorf("Unexpected error creating workspace: %v", err)
	}

	wantWorkspace := Workspace{
		Root:      "/sd/workspace",
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

func TestClone(t *testing.T) {
	testBuildID := "BUILDID"
	testJobID := "JOBID"
	testSCMURL := "git@github.com:screwdriver-cd/launcher#master"
	api := mockAPI(t, testBuildID, testJobID, "")
	api.pipelineFromID = func(pipelineID string) (screwdriver.Pipeline, error) {
		return screwdriver.Pipeline(FakePipeline{ScmURL: testSCMURL}), nil
	}
	gitClone = func(repo, dest string) error {
		if repo != testSCMURL {
			t.Errorf("Git clone was called with repo %q, want %q", repo, testSCMURL)
		}
		return nil
	}
	launch(screwdriver.API(api), testBuildID)
}
