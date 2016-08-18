package screwdriver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"
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

func TestMain(m *testing.M) {
	sleep = func(d time.Duration) {}
	os.Exit(m.Run())
}

func TestBuildFromID(t *testing.T) {
	tests := []struct {
		build      Build
		statusCode int
		err        error
	}{
		{
			build: Build{
				ID:    "testId",
				JobID: "testJob",
				SHA:   "testSHA",
			},
			statusCode: 200,
			err:        nil,
		},
		{
			build:      Build{},
			statusCode: 500,
			err: errors.New("timeout on request: after 5 attempts, last error: " +
				"GET retries exhausted: 500 returned from GET http://fakeurl/v3/builds/"),
		},
		{
			build:      Build{},
			statusCode: 404,
			err: SDError{
				StatusCode: 404,
				Reason:     "Not Found",
				Message:    "Build does not exist",
			},
		},
	}

	for _, test := range tests {
		JSON := []byte{}
		err := errors.New("")

		if reflect.TypeOf(test.err) == reflect.TypeOf(SDError{}) {
			JSON, err = json.Marshal(test.err)
			if err != nil {
				t.Fatalf("Unable to Marshal JSON for SDErr: %v", err)
			}
		} else {
			JSON, err = json.Marshal(test.build)
			if err != nil {
				t.Fatalf("Unable to Marshal JSON for SDErr: %v", err)
			}
		}

		http := makeFakeHTTPClient(t, test.statusCode, string(JSON))
		testAPI := api{"http://fakeurl", "faketoken", http}

		build, err := testAPI.BuildFromID(test.build.ID)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from BuildFromID: %v, want %v", err, test.err)
		}

		if !reflect.DeepEqual(build, test.build) {
			t.Errorf("build == %#v, want %#v", build, test.build)
		}
	}
}

func TestJobFromID(t *testing.T) {
	tests := []struct {
		job        Job
		statusCode int
		err        error
	}{
		{
			job: Job{
				ID:         "testId",
				PipelineID: "testPipeline",
				Name:       "testName",
			},
			statusCode: 200,
			err:        nil,
		},
		{
			job:        Job{},
			statusCode: 500,
			err: errors.New("timeout on request: after 5 attempts, last error: " +
				"GET retries exhausted: 500 returned from GET http://fakeurl/v3/jobs/"),
		},
		{
			job:        Job{},
			statusCode: 404,
			err: SDError{
				StatusCode: 404,
				Reason:     "Not Found",
				Message:    "Job does not exist",
			},
		},
	}

	for _, test := range tests {
		JSON := []byte{}
		err := errors.New("")

		if reflect.TypeOf(test.err) == reflect.TypeOf(SDError{}) {
			JSON, err = json.Marshal(test.err)
			if err != nil {
				t.Fatalf("Unable to Marshal JSON for SDErr: %v", err)
			}
		} else {
			JSON, err = json.Marshal(test.job)
			if err != nil {
				t.Fatalf("Unable to Marshal JSON for SDErr: %v", err)
			}
		}

		http := makeFakeHTTPClient(t, test.statusCode, string(JSON))
		testAPI := api{"http://fakeurl", "faketoken", http}

		job, err := testAPI.JobFromID(test.job.ID)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from JobFromID: %v, want %v", err, test.err)
		}

		if !reflect.DeepEqual(job, test.job) {
			t.Errorf("job == %#v, want %#v", job, test.job)
		}
	}
}

func TestPipelineFromID(t *testing.T) {
	tests := []struct {
		pipeline   Pipeline
		statusCode int
		err        error
	}{
		{
			pipeline: Pipeline{
				ID:     "testId",
				ScmURL: "testScmURL",
			},
			statusCode: 200,
			err:        nil,
		},
		{
			pipeline:   Pipeline{},
			statusCode: 500,
			err: errors.New("timeout on request: after 5 attempts, last error: " +
				"GET retries exhausted: 500 returned from GET http://fakeurl/v3/pipelines/"),
		},
		{
			pipeline:   Pipeline{},
			statusCode: 404,
			err: SDError{
				StatusCode: 404,
				Reason:     "Not Found",
				Message:    "Pipeline does not exist",
			},
		},
	}

	for _, test := range tests {
		JSON := []byte{}
		err := errors.New("")

		if reflect.TypeOf(test.err) == reflect.TypeOf(SDError{}) {
			JSON, err = json.Marshal(test.err)
			if err != nil {
				t.Fatalf("Unable to Marshal JSON for SDErr: %v", err)
			}
		} else {
			JSON, err = json.Marshal(test.pipeline)
			if err != nil {
				t.Fatalf("Unable to Marshal JSON for SDErr: %v", err)
			}
		}

		http := makeFakeHTTPClient(t, test.statusCode, string(JSON))
		testAPI := api{"http://fakeurl", "faketoken", http}

		pipeline, err := testAPI.PipelineFromID(test.pipeline.ID)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from PipelineFromID: %v, want %v", err, test.err)
		}

		if !reflect.DeepEqual(pipeline, test.pipeline) {
			t.Errorf("pipeline == %#v, want %#v", pipeline, test.pipeline)
		}
	}
}

func TestPipelineDefFromYaml(t *testing.T) {
	tests := []struct {
		json       string
		jobs       map[string][]JobDef
		statusCode int
		err        error
	}{
		{
			json: `{
		    "jobs": {
		      "main": [
		        {
		          "image": "node:4",
		          "commands": [
		            {
		              "name": "install",
		              "command": "npm install"
		            }
		          ],
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
		  }`,
			jobs: map[string][]JobDef{
				"main": []JobDef{
					{
						Image: "node:4",
						Commands: []CommandDef{
							{
								Name: "install",
								Cmd:  "npm install",
							},
						},
						Environment: map[string]string{
							"ARRAY":   `["a","b"]`,
							"BOOL":    "true",
							"OBJECT":  `{"a":"cat","b":"dog"}`,
							"NUMBER":  "3",
							"DECIMAL": "3.1415927",
							"STRING":  "test",
						},
					},
				},
			},
			statusCode: 200,
			err:        nil,
		},
		{
			json:       "",
			jobs:       map[string][]JobDef{},
			statusCode: 500,
			err: errors.New("posting to Validator: timeout on request after 5 attempts, " +
				"last error: POST retries exhausted: 500 returned from POST http://fakeurl/v3/validator"),
		},
	}

	for _, test := range tests {
		yaml := bytes.NewBufferString("")
		http := makeFakeHTTPClient(t, test.statusCode, test.json)
		testAPI := api{"http://fakeurl", "faketoken", http}

		p, err := testAPI.PipelineDefFromYaml(yaml)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from PipelineDefFromYaml: %v, want %v", err, test.err)
		}

		if !reflect.DeepEqual(p.Jobs, test.jobs) && len(p.Jobs) != len(test.jobs) {
			t.Errorf("PipelineDef.Jobs = %+v, \n want %+v", p.Jobs, test.jobs)
		}
	}
}

func TestUpdateBuildStatus(t *testing.T) {
	tests := []struct {
		status     BuildStatus
		statusCode int
		err        error
	}{
		{Success, 200, nil},
		{Failure, 200, nil},
		{Aborted, 200, nil},
		{Running, 200, nil},
		{"NOTASTATUS", 200, errors.New("invalid build status: NOTASTATUS")},
		{Success, 500, errors.New("posting to Build Status: timeout on request after 5 attempts, " +
			"last error: POST retries exhausted: 500 returned from POST http://fakeurl/v3/webhooks/build")},
	}

	for _, test := range tests {
		http := makeFakeHTTPClient(t, test.statusCode, "{}")
		testAPI := api{"http://fakeurl", "faketoken", http}

		err := testAPI.UpdateBuildStatus(test.status)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from UpdateBuildStatus: %v, want %v", err, test.err)
		}
	}

}
