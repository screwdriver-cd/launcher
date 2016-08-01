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

type MockAPI struct {
	buildFromID func(string) (screwdriver.Build, error)
}

func (f MockAPI) BuildFromID(buildID string) (screwdriver.Build, error) {
	if f.buildFromID != nil {
		return f.buildFromID(buildID)
	}
	return screwdriver.Build(FakeBuild{}), nil
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
