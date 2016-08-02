package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/screwdriver-cd/launcher/screwdriver"
)

type NewAPI func(buildID string, token string) (screwdriver.API, error)
type FakeAPI screwdriver.API
type FakeBuild screwdriver.Build
type FakeJob screwdriver.Job

type MockAPI struct {
	buildFromID func(string) (screwdriver.Build, error)
	jobFromID   func(string) (screwdriver.Job, error)
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

func TestBuildFromId(t *testing.T) {
	testID := "TESTID"
	api := MockAPI{
		buildFromID: func(buildID string) (screwdriver.Build, error) {
			if buildID != testID {
				t.Errorf("buildID == %v, want %v", buildID, testID)
			}
			return screwdriver.Build(FakeBuild{}), nil
		},
	}

	launch(screwdriver.API(api), testID)
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

func TestJobFromID(t *testing.T) {
	testBuildID := "BUILDID"
	testJobID := "JOBID"
	api := MockAPI{
		buildFromID: func(buildID string) (screwdriver.Build, error) {
			return screwdriver.Build(FakeBuild{ID: testBuildID, JobID: testJobID}), nil
		},
		jobFromID: func(jobID string) (screwdriver.Job, error) {
			if jobID != testJobID {
				t.Errorf("jobID == %v, want %v", jobID, testJobID)
			}
			return screwdriver.Job(FakeJob{}), nil
		},
	}

	launch(screwdriver.API(api), testBuildID)
}

func TestJobFromIdError(t *testing.T) {
	testBuildID := "BUILDID"
	testJobID := "JOBID"
	api := MockAPI{
		buildFromID: func(buildID string) (screwdriver.Build, error) {
			return screwdriver.Build(FakeBuild{ID: testBuildID, JobID: testJobID}), nil
		},
		jobFromID: func(jobID string) (screwdriver.Job, error) {
			err := fmt.Errorf("testing error returns")
			return screwdriver.Job(FakeJob{}), err
		},
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
