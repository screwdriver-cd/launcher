package screwdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func makeFakeHTTPClient(t *testing.T, code int, body string) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "faketoken"
		wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

		validateHeader(t, "Authorization", wantTokenHeader)
		w.WriteHeader(code)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, body)
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func validateHeader(t *testing.T, key, value string) func(r *http.Request) {
	return func(r *http.Request) {
		headers, ok := r.Header[key]
		if !ok {
			t.Fatalf("No %s header sent in Screwdriver request", key)
		}
		header := headers[0]
		if header != value {
			t.Errorf("%s header = %q, want %q", key, header, value)
		}
	}
}

func TestFromBuildId(t *testing.T) {
	want := Build{
		ID:    "testId",
		JobID: "testJob",
	}
	json, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Unable to Marshal JSON for test: %v", err)
	}

	http := makeFakeHTTPClient(t, 200, string(json))

	testAPI := api{"http://fakeurl", "faketoken", http}
	build, err := testAPI.BuildFromID(want.ID)

	if err != nil {
		t.Errorf("Unexpected error from BuildFromID: %v", err)
	}

	if build != want {
		t.Errorf("build == %#v, want %#v", build, want)
	}
}

func TestBuild404(t *testing.T) {
	want := SDError{
		StatusCode: 404,
		Reason:     "Not Found",
		Message:    "Build does not exist",
	}

	json, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Unable to Marshal JSON for test: %v", err)
	}

	http := makeFakeHTTPClient(t, 404, string(json))
	testAPI := api{"http://fakeurl", "faketoken", http}
	_, err = testAPI.BuildFromID("doesntexist")

	if err != want {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestFromJobId(t *testing.T) {
	want := Job{
		ID:         "testId",
		PipelineID: "testPipeline",
	}
	json, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Unable to Marshal JSON for test: %v", err)
	}

	http := makeFakeHTTPClient(t, 200, string(json))

	testAPI := api{"http://fakeurl", "faketoken", http}
	job, err := testAPI.JobFromID(want.ID)

	if err != nil {
		t.Errorf("Unexpected error from JobFromID: %v", err)
	}

	if job != want {
		t.Errorf("job == %#v, want %#v", job, want)
	}
}

func TestJob404(t *testing.T) {
	want := SDError{
		StatusCode: 404,
		Reason:     "Not Found",
		Message:    "Job does not exist",
	}

	json, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Unable to Marshal JSON for test: %v", err)
	}

	http := makeFakeHTTPClient(t, 404, string(json))
	testAPI := api{"http://fakeurl", "faketoken", http}
	_, err = testAPI.JobFromID("doesntexist")

	if err != want {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestFromPipelineId(t *testing.T) {
	want := Pipeline{
		ID:     "testId",
		ScmURL: "testScmURL",
	}
	json, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Unable to Marshal JSON for test: %v", err)
	}

	http := makeFakeHTTPClient(t, 200, string(json))

	testAPI := api{"http://fakeurl", "faketoken", http}
	pipeline, err := testAPI.PipelineFromID(want.ID)

	if err != nil {
		t.Errorf("Unexpected error from JobFromID: %v", err)
	}

	if pipeline != want {
		t.Errorf("pipeline == %#v, want %#v", pipeline, want)
	}
}

func TestPipeline404(t *testing.T) {
	want := SDError{
		StatusCode: 404,
		Reason:     "Not Found",
		Message:    "Pipeline does not exist",
	}

	json, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Unable to Marshal JSON for test: %v", err)
	}

	http := makeFakeHTTPClient(t, 404, string(json))
	testAPI := api{"http://fakeurl", "faketoken", http}
	_, err = testAPI.PipelineFromID("doesntexist")

	if err != want {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestPipelineFromYaml(t *testing.T) {
	json := `{
	  "jobs": {
	    "main": [
	      {
	        "image": "node:4",
	        "steps": {
	          "install": "npm install"
	        },
	        "environment": {
	          "ARRAY": "[\"a\",\"b\"]",
	          "BOOL": "true",
	          "OBJECT": "{\"a\":\"cat\",\"b\":\"dog\"}",
	          "NUMBER": "3",
	          "DECIMAL": "3.1415927",
	          "STRING": "test"
	        }
	      }
	    ]
	  },
	  "workflow": []
	}`

	mainJob := JobDef{
		Image: "node:4",
		Steps: map[string]string{
			"install": "npm install",
		},
		Environment: map[string]string{
			"ARRAY":   `["a","b"]`,
			"BOOL":    "true",
			"OBJECT":  `{"a":"cat","b":"dog"}`,
			"NUMBER":  "3",
			"DECIMAL": "3.1415927",
			"STRING":  "test",
		},
	}

	wantJobs := map[string][]JobDef{
		"main": []JobDef{
			mainJob,
		},
	}

	yaml := bytes.NewBufferString("")
	http := makeFakeHTTPClient(t, 200, json)
	testAPI := api{"http://fakeurl", "faketoken", http}
	p, err := testAPI.PipelineDefFromYaml(yaml)
	if err != nil {
		t.Errorf("Unexpected error from PipelineDefFromYaml: %v", err)
	}

	if !reflect.DeepEqual(p.Jobs, wantJobs) {
		t.Errorf("PipelineDef.Jobs = %+v, \n want %+v", p.Jobs, wantJobs)
	}
}
