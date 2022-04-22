package screwdriver

import (
	"reflect"
	"testing"
)

func TestBuildFromIDLocal(t *testing.T) {
	testBuild := Build{
		ID:      1555,
		JobID:   3777,
		EventID: 8765,
		SHA:     "testSHA",
	}
	testAPI := localApi{"http://fakeurl", "testJob", testBuild}

	actual, err := testAPI.BuildFromID(0)
	if !reflect.DeepEqual(actual, testBuild) {
		t.Errorf("actual: %#v, expected: %#v", actual, testBuild)
	}
	if err != nil {
		t.Errorf("actual: %v\nexpected: %v", err.Error(), nil)
	}
}

func TestEventFromIDLocal(t *testing.T) {
	testAPI := localApi{"http://fakeurl", "testJob", Build{}}
	expected := Event{
		0,
		make(map[string]interface{}),
		0,
		make(map[string]string),
	}

	actual, err := testAPI.EventFromID(0)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("actual: %#v, expected: %#v", actual, expected)
	}
	if err != nil {
		t.Errorf("actual: %v\nexpected: %v", err.Error(), nil)
	}
}

func TestJobFromIDLocal(t *testing.T) {
	testAPI := localApi{"http://fakeurl", "testJob", Build{}}
	expected := Job{
		0,
		0,
		testAPI.jobName,
		0,
		[]JobPermutation{},
	}

	actual, err := testAPI.JobFromID(0)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("actual: %#v, expected: %#v", actual, expected)
	}
	if err != nil {
		t.Errorf("actual: %v\nexpected: %v", err.Error(), nil)
	}
}

func TestPipelineFromIDLocal(t *testing.T) {
	testAPI := localApi{"http://fakeurl", "testJob", Build{}}
	expected := Pipeline{
		0,
		ScmRepo{"sd-local/local-build", false},
		"screwdriver.cd:123456:master",
	}

	actual, err := testAPI.PipelineFromID(0)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("actual: %#v, expected: %#v", actual, expected)
	}
	if err != nil {
		t.Errorf("actual: %v\nexpected: %v", err.Error(), nil)
	}
}

func TestUpdateBuildStatusLocal(t *testing.T) {
	testAPI := localApi{"http://fakeurl", "testJob", Build{}}

	actual := testAPI.UpdateBuildStatus("", make(map[string]interface{}), 0, "")
	if actual != nil {
		t.Errorf("actual: %v, expected: %v", actual, nil)
	}
}

func TestUpdateStepStartLocal(t *testing.T) {
	testAPI := localApi{"http://fakeurl", "testJob", Build{}}

	actual := testAPI.UpdateStepStart(0, "")
	if actual != nil {
		t.Errorf("actual: %v, expected: %v", actual, nil)
	}
}

func TestUpdateStepStopLocal(t *testing.T) {
	testAPI := localApi{"http://fakeurl", "testJob", Build{}}

	actual := testAPI.UpdateStepStop(0, "", 0)
	if actual != nil {
		t.Errorf("actual: %v, expected: %v", actual, nil)
	}
}

func TestSecretsForBuildLocal(t *testing.T) {
	testAPI := localApi{"http://fakeurl", "testJob", Build{}}
	expected := make(Secrets, 0)

	actual, err := testAPI.SecretsForBuild(Build{})
	if err != nil {
		t.Fatalf("Unexpected error from SecretsForBuild: %v", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("actual: %q, expected: %q", actual, expected)
	}
}

func TestGetAPIURLLocal(t *testing.T) {
	testAPI := localApi{"http://fakeurl", "testJob", Build{}}
	url, _ := testAPI.GetAPIURL()

	if !reflect.DeepEqual(url, "http://fakeurl/v4/") {
		t.Errorf(`api.GetAPIURL() expected to return: "%v", instead returned "%v"`, "http://fakeurl/v4/", url)
	}
}

func TestGetCoverageInfoLocal(t *testing.T) {
	testAPI := localApi{"http://fakeurl", "testJob", Build{}}
	expected := Coverage{}

	actual, err := testAPI.GetCoverageInfo(123, 456, "main", "d2lam/mytest", "", "", "")
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("actual: %#v, expected: %#v", actual, expected)
	}
	if err != nil {
		t.Errorf("actual: %v\nexpected: %v", err.Error(), nil)
	}
}

func TestGetBuildTokenLocal(t *testing.T) {
	testAPI := localApi{"http://fakeurl", "testJob", Build{}}
	expected := ""

	actual, err := testAPI.GetBuildToken(0, 0)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("actual: %#v, expected: %#v", actual, expected)
	}
	if err != nil {
		t.Errorf("actual: %v\nexpected: %v", err.Error(), nil)
	}
}
