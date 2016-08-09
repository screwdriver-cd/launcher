package screwdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
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
		Name:       "testName",
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
  }`

	mainJob := JobDef{
		Image: "node:4",
		Commands: []CommandDef{
			CommandDef{
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

func TestUpdateBuildStatus(t *testing.T) {
	http := makeFakeHTTPClient(t, 200, "{}")
	testAPI := api{"http://fakeurl", "faketoken", http}
	statuses := []BuildStatus{
		Success,
		Failure,
		Aborted,
		Running,
	}

	for _, status := range statuses {
		err := testAPI.UpdateBuildStatus(status)
		if err != nil {
			t.Errorf("Unexpected error from UpdateBuildStatus(%s): %v", status, err)
		}
	}
}

func TestBadUpdateBuildStatus(t *testing.T) {
	http := makeFakeHTTPClient(t, 200, "{}")
	testAPI := api{"http://fakeurl", "faketoken", http}
	err := testAPI.UpdateBuildStatus("NOTASTATUS")
	if err == nil {
		t.Fatalf("Expected an error from UpdateBuildStatus. Got none.")
	}
	if !strings.Contains(err.Error(), "invalid build status: NOTASTATUS") {
		t.Errorf("Unexpected error from UpdateBuildStatus: %v", err)
	}
}

func TestNewAPI(t *testing.T) {
	testURL := "test.com"
	testToken := "token"
	obj, err := New(testURL, testToken)

	if err != nil {
		t.Errorf("Unexpected error from New: %v", err)
	}

	if obj.(api).baseURL != testURL {
		t.Errorf("API baseURL = %v, want %v", obj.(api).baseURL, testURL)
	}

	if obj.(api).token != testToken {
		t.Errorf("API token = %v, want %v", obj.(api).token, testToken)
	}
}

func TestSDError(t *testing.T) {
	testSDError := SDError{
		StatusCode: 0,
		Reason:     "none",
		Message:    "testMessage",
	}
	wantString := "0 none: testMessage"

	errorString := testSDError.Error()

	if errorString != wantString {
		t.Errorf("SDError string = %v, want %v", errorString, wantString)
	}
}

func TestHandleResponseBadIoutilReadAll(t *testing.T) {
	oldReadAll := readAll
	defer func() { readAll = oldReadAll }()

	readAll = func(r io.Reader) ([]byte, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	http := makeFakeHTTPClient(t, 200, "")

	res, _ := http.Get("http://fakeurl")

	_, err := handleResponse(res)

	if err.Error() != "reading response Body from Screwdriver: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestHandleResponseBadJsonUnmarshal(t *testing.T) {
	oldReadAll := readAll
	defer func() { readAll = oldReadAll }()

	readAll = func(r io.Reader) ([]byte, error) {
		return nil, nil
	}

	oldUnmarshal := unmarshal
	defer func() { unmarshal = oldUnmarshal }()

	unmarshal = func(data []byte, v interface{}) error {
		return fmt.Errorf("Spooky error")
	}

	http := makeFakeHTTPClient(t, 400, "")

	res, _ := http.Get("http://fakeurl")

	_, err := handleResponse(res)

	if err.Error() != "unparseable error response from Screwdriver: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestGetBadHttpNewRequest(t *testing.T) {
	oldHTTPNewRequest := httpNewRequest
	defer func() { httpNewRequest = oldHTTPNewRequest }()

	httpNewRequest = func(method, urlStr string, body io.Reader) (*http.Request, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	http := makeFakeHTTPClient(t, 200, "")
	testAPI := api{"http://fakeurl", "faketoken", http}
	testURL, _ := url.Parse("http://fakeurl")

	_, err := testAPI.get(testURL)

	if err.Error() != "generating request to Screwdriver: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestGetBadHttpClientDo(t *testing.T) {
	oldHTTPClientDo := httpClientDo
	defer func() { httpClientDo = oldHTTPClientDo }()

	httpClientDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("Spooky Error")
	}

	http := makeFakeHTTPClient(t, 200, "")
	testAPI := api{"http://fakeurl", "faketoken", http}
	testURL, _ := url.Parse("http://fakeurl")

	_, err := testAPI.get(testURL)

	if err.Error() != "reading response from Screwdriver: Spooky Error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestPostBadHttpNewRequest(t *testing.T) {
	oldHTTPNewRequest := httpNewRequest
	defer func() { httpNewRequest = oldHTTPNewRequest }()

	httpNewRequest = func(method, urlStr string, body io.Reader) (*http.Request, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	http := makeFakeHTTPClient(t, 200, "")
	testAPI := api{"http://fakeurl", "faketoken", http}
	testURL, _ := url.Parse("http://fakeurl")

	_, err := testAPI.post(testURL, "", nil)

	if err.Error() != "generating request to Screwdriver: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestPostBadHttpClientDo(t *testing.T) {
	oldHTTPClientDo := httpClientDo
	defer func() { httpClientDo = oldHTTPClientDo }()

	httpClientDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	http := makeFakeHTTPClient(t, 200, "")
	testAPI := api{"http://fakeurl", "faketoken", http}
	testURL, _ := url.Parse("http://fakeurl")

	_, err := testAPI.post(testURL, "", nil)

	if err.Error() != "posting to Screwdriver endpoint http://fakeurl: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestBuildFromIdBadmakeURL(t *testing.T) {
	oldmakeURL := makeURL
	defer func() { makeURL = oldmakeURL }()

	makeURL = func(a *api, path string) (*url.URL, error) {
		return nil, fmt.Errorf("Spooky error")
	}

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
	_, err = testAPI.BuildFromID(want.ID)

	if err.Error() != "generating Screwdriver url for Build testId: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestBuildFromIdBadJsonUnmarshal(t *testing.T) {
	oldJSONUnmarshal := jsonUnmarshal
	defer func() { jsonUnmarshal = oldJSONUnmarshal }()

	jsonUnmarshal = func(data []byte, v interface{}) error {
		return fmt.Errorf("Spooky error")
	}

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
	_, err = testAPI.BuildFromID(want.ID)

	if err.Error() != "Parsing JSON response \"{\\\"id\\\":\\\"testId\\\",\\\"jobId\\\":\\\"testJob\\\"}\\n\": Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestJobFromIdBadmakeURL(t *testing.T) {
	oldmakeURL := makeURL
	defer func() { makeURL = oldmakeURL }()

	makeURL = func(a *api, path string) (*url.URL, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	want := Job{
		ID:         "testId",
		PipelineID: "testPipeline",
		Name:       "testName",
	}
	json, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Unable to Marshal JSON for test: %v", err)
	}

	http := makeFakeHTTPClient(t, 200, string(json))

	testAPI := api{"http://fakeurl", "faketoken", http}
	_, err = testAPI.JobFromID(want.ID)

	if err.Error() != "generating Screwdriver url for Job testId: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestJobFromIdBadJsonUnmarshal(t *testing.T) {
	oldJSONUnmarshal := jsonUnmarshal
	defer func() { jsonUnmarshal = oldJSONUnmarshal }()

	jsonUnmarshal = func(data []byte, v interface{}) error {
		return fmt.Errorf("Spooky error")
	}

	want := Job{
		ID:         "testId",
		PipelineID: "testPipeline",
		Name:       "testName",
	}
	json, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Unable to Marshal JSON for test: %v", err)
	}

	http := makeFakeHTTPClient(t, 200, string(json))

	testAPI := api{"http://fakeurl", "faketoken", http}
	_, err = testAPI.JobFromID(want.ID)

	if err.Error() != "Parsing JSON response \"{\\\"id\\\":\\\"testId\\\",\\\"pipelineId\\\":\\\"testPipeline\\\",\\\"name\\\":\\\"testName\\\"}\\n\": Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestPipelineFromIdBadmakeURL(t *testing.T) {
	oldmakeURL := makeURL
	defer func() { makeURL = oldmakeURL }()

	makeURL = func(a *api, path string) (*url.URL, error) {
		return nil, fmt.Errorf("Spooky error")
	}

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
	_, err = testAPI.PipelineFromID(want.ID)

	if err.Error() != "generating Screwdriver url for Pipeline testId: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestPipelineFromIdBadJsonUnmarshal(t *testing.T) {
	oldJSONUnmarshal := jsonUnmarshal
	defer func() { jsonUnmarshal = oldJSONUnmarshal }()

	jsonUnmarshal = func(data []byte, v interface{}) error {
		return fmt.Errorf("Spooky error")
	}

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
	_, err = testAPI.PipelineFromID(want.ID)

	if err.Error() != "Parsing JSON response \"{\\\"id\\\":\\\"testId\\\",\\\"scmUrl\\\":\\\"testScmURL\\\"}\\n\": Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestPipelineDefFromYamlBadmakeURL(t *testing.T) {
	oldmakeURL := makeURL
	defer func() { makeURL = oldmakeURL }()

	makeURL = func(a *api, path string) (*url.URL, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	json := `{
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
  }`

	yaml := bytes.NewBufferString("")
	http := makeFakeHTTPClient(t, 200, json)
	testAPI := api{"http://fakeurl", "faketoken", http}
	_, err := testAPI.PipelineDefFromYaml(yaml)

	if err.Error() != "generating Screwdriver url for Validator: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestPipelineDefFromYamlBadIoutilReadAll(t *testing.T) {
	oldReadAll := readAll
	defer func() { readAll = oldReadAll }()

	readAll = func(r io.Reader) ([]byte, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	json := `{
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
  }`

	yaml := bytes.NewBufferString("")
	http := makeFakeHTTPClient(t, 200, json)
	testAPI := api{"http://fakeurl", "faketoken", http}
	_, err := testAPI.PipelineDefFromYaml(yaml)

	if err.Error() != "reading Screwdriver YAML: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestPipelineDefFromYamlBadJsonMarshal(t *testing.T) {
	oldJSONMarshal := jsonMarshal
	defer func() { jsonMarshal = oldJSONMarshal }()

	jsonMarshal = func(v interface{}) ([]byte, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	json := `{
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
  }`

	yaml := bytes.NewBufferString("")
	http := makeFakeHTTPClient(t, 200, json)
	testAPI := api{"http://fakeurl", "faketoken", http}
	_, err := testAPI.PipelineDefFromYaml(yaml)

	if err.Error() != "marshaling JSON for Validator: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestPipelineDefFromYamlBadApiPost(t *testing.T) {
	oldAPIPost := apiPost
	defer func() { apiPost = oldAPIPost }()

	apiPost = func(a *api, url *url.URL, bodyType string, payload io.Reader) ([]byte, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	json := `{
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
  }`

	yaml := bytes.NewBufferString("")
	http := makeFakeHTTPClient(t, 200, json)
	testAPI := api{"http://fakeurl", "faketoken", http}
	_, err := testAPI.PipelineDefFromYaml(yaml)

	if err.Error() != "posting to Validator: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestPipelineDefFromYamlBadJsonUnmarshal(t *testing.T) {
	oldJSONUnmarshal := jsonUnmarshal
	defer func() { jsonUnmarshal = oldJSONUnmarshal }()

	jsonUnmarshal = func(data []byte, v interface{}) error {
		return fmt.Errorf("Spooky error")
	}

	json := `{
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
  }`

	yaml := bytes.NewBufferString("")
	http := makeFakeHTTPClient(t, 200, json)
	testAPI := api{"http://fakeurl", "faketoken", http}
	_, err := testAPI.PipelineDefFromYaml(yaml)

	if err.Error() != "parsing JSON response from the Validator: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestUpdateBuildStatusBadmakeURL(t *testing.T) {
	oldmakeURL := makeURL
	defer func() { makeURL = oldmakeURL }()

	makeURL = func(a *api, path string) (*url.URL, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	http := makeFakeHTTPClient(t, 200, "{}")
	testAPI := api{"http://fakeurl", "faketoken", http}
	err := testAPI.UpdateBuildStatus("SUCCESS")

	fmt.Println(err)

	if err.Error() != "generating Screwdriver url for Build Status: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestUpdateBuildStatusBadJsonMarshal(t *testing.T) {
	oldJSONMarshal := jsonMarshal
	defer func() { jsonMarshal = oldJSONMarshal }()

	jsonMarshal = func(v interface{}) ([]byte, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	http := makeFakeHTTPClient(t, 200, "{}")
	testAPI := api{"http://fakeurl", "faketoken", http}
	err := testAPI.UpdateBuildStatus("SUCCESS")

	if err.Error() != "marshaling JSON for Build Status: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}

func TestUpdateBuildStatusBadApiPost(t *testing.T) {
	oldAPIPost := apiPost
	defer func() { apiPost = oldAPIPost }()

	apiPost = func(a *api, url *url.URL, bodyType string, payload io.Reader) ([]byte, error) {
		return nil, fmt.Errorf("Spooky error")
	}

	http := makeFakeHTTPClient(t, 200, "{}")
	testAPI := api{"http://fakeurl", "faketoken", http}
	err := testAPI.UpdateBuildStatus("SUCCESS")

	if err.Error() != "posting to Build Status: Spooky error" {
		t.Errorf("Error is wrong, got %v", err)
	}
}
