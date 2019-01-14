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
	"regexp"
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

func makeValidatedFakeHTTPClient(t *testing.T, code int, body string, v func(r *http.Request)) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "faketoken"
		wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

		validateHeader(t, "Authorization", wantTokenHeader)
		v(r)

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
	var commands = []struct {
		command string
		err     error
	}{
		{"make", nil},
		{"npm install", nil},
		{"failer", fmt.Errorf("exit 7")},
		{"neverexecuted", nil},
	}

	testEnv := map[string]string{
		"foo": "bar",
		"baz": "bah",
	}

	testCmds := []CommandDef{}
	testCmds = append(testCmds, CommandDef{
		Name: "sd-setup-init",
	})
	for _, test := range commands {
		testCmds = append(testCmds, CommandDef{
			Name: "test",
			Cmd:  test.command,
		})
	}
	tests := []struct {
		build      Build
		statusCode int
		err        error
	}{
		{
			build: Build{
				ID:          1555,
				JobID:       3777,
				EventID:     8765,
				SHA:         "testSHA",
				Commands:    testCmds,
				Environment: testEnv,
			},
			statusCode: 200,
			err:        nil,
		},
		{
			build:      Build{},
			statusCode: 500,
			err: errors.New("After 5 attempts, Last error: " +
				"GET retries exhausted: 500 returned from GET http://fakeurl/v4/builds/0"),
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
			t.Errorf("Unexpected error from BuildFromID: \n%v\n want \n%v", err.Error(), test.err.Error())
		}

		if len(test.build.Commands) != 0 {
			test.build.Commands = test.build.Commands[1:]
		}

		if !reflect.DeepEqual(build, test.build) {
			t.Errorf("build == %#v, want %#v", build, test.build)
		}
	}
}

func TestEventFromID(t *testing.T) {
	tests := []struct {
		event      Event
		statusCode int
		err        error
	}{
		{
			event: Event{
				ID:            1555,
				ParentEventID: 8765,
			},
			statusCode: 200,
			err:        nil,
		},
		{
			event:      Event{},
			statusCode: 500,
			err: errors.New("After 5 attempts, Last error: " +
				"GET retries exhausted: 500 returned from GET http://fakeurl/v4/events/0"),
		},
		{
			event:      Event{},
			statusCode: 404,
			err: SDError{
				StatusCode: 404,
				Reason:     "Not Found",
				Message:    "Event does not exist",
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
			JSON, err = json.Marshal(test.event)
			if err != nil {
				t.Fatalf("Unable to Marshal JSON for SDErr: %v", err)
			}
		}

		http := makeFakeHTTPClient(t, test.statusCode, string(JSON))
		testAPI := api{"http://fakeurl", "faketoken", http}

		event, err := testAPI.EventFromID(test.event.ID)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from EventFromID: \n%v\n want \n%v", err.Error(), test.err.Error())
		}

		if !reflect.DeepEqual(event, test.event) {
			t.Errorf("event == %#v, want %#v", event, test.event)
		}
	}
}

func TestGetCoverageInfo(t *testing.T) {
	envVars := map[string]string{
		"SD_SONAR_AUTH_URL": "https://api.screwdriver.cd/v4/coverage/token",
		"SD_SONAR_HOST":     "https://sonar.screwdriver.cd",
	}
	tests := []struct {
		coverage   Coverage
		statusCode int
		err        error
	}{
		{
			coverage: Coverage{
				EnvVars: envVars,
			},
			statusCode: 200,
			err:        nil,
		},
		{
			coverage:   Coverage{},
			statusCode: 500,
			err: errors.New("After 5 attempts, Last error: " +
				"GET retries exhausted: 500 returned from GET http://fakeurl/v4/coverage/info"),
		},
		{
			coverage:   Coverage{},
			statusCode: 404,
			err: SDError{
				StatusCode: 404,
				Reason:     "Not Found",
				Message:    "Coverage info does not exist",
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
			JSON, err = json.Marshal(test.coverage)
			if err != nil {
				t.Fatalf("Unable to Marshal JSON for SDErr: %v", err)
			}
		}

		http := makeFakeHTTPClient(t, test.statusCode, string(JSON))
		testAPI := api{"http://fakeurl", "faketoken", http}

		coverage, err := testAPI.GetCoverageInfo()

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from GetCoverageInfo: \n%v\n want \n%v", err.Error(), test.err.Error())
		}

		if !reflect.DeepEqual(coverage, test.coverage) {
			t.Errorf("coverage == %#v, want %#v", coverage, test.coverage)
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
				ID:         1555,
				PipelineID: 2666,
				Name:       "testName",
			},
			statusCode: 200,
			err:        nil,
		},
		{
			job:        Job{},
			statusCode: 500,
			err: errors.New("After 5 attempts, Last error: " +
				"GET retries exhausted: 500 returned from GET http://fakeurl/v4/jobs/0"),
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
				ID:     1555,
				ScmURI: "github.com:123456:master",
				ScmRepo: ScmRepo{
					Name: "screwdriver-cd/launcher",
				},
			},
			statusCode: 200,
			err:        nil,
		},
		{
			pipeline:   Pipeline{},
			statusCode: 500,
			err: errors.New("After 5 attempts, Last error: " +
				"GET retries exhausted: 500 returned from GET http://fakeurl/v4/pipelines/0"),
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

func TestUpdateBuildStatus(t *testing.T) {
	var meta map[string]interface{}
	mockJSON := []byte("{\"hoge\":\"fuga\"}")
	_ = json.Unmarshal(mockJSON, &meta)

	tests := []struct {
		status     BuildStatus
		meta       map[string]interface{}
		statusCode int
		err        error
	}{
		{Success, meta, 200, nil},
		{Failure, meta, 200, nil},
		{Aborted, meta, 200, nil},
		{Running, meta, 200, nil},
		{"NOTASTATUS", meta, 200, errors.New("Invalid build status: NOTASTATUS")},
		{Success, meta, 500, errors.New("Posting to Build Status: After 5 attempts, " +
			"Last error: retries exhausted: 500 returned from http://fakeurl/v4/builds/15")},
	}

	for _, test := range tests {
		http := makeFakeHTTPClient(t, test.statusCode, "{}")
		testAPI := api{"http://fakeurl", "faketoken", http}

		err := testAPI.UpdateBuildStatus(test.status, test.meta, 15)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from UpdateBuildStatus: %v, want %v", err, test.err)
		}
	}
}

func TestUpdateStepStart(t *testing.T) {
	http := makeValidatedFakeHTTPClient(t, 200, "{}", func(r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		want := regexp.MustCompile(`{"startTime":"[\d-]+T[\d:.Z-]+"}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})
	testAPI := api{"http://fakeurl", "faketoken", http}

	err := testAPI.UpdateStepStart(999, "step1")

	if err != nil {
		t.Errorf("Unexpected error from UpdateStepStart: %v", err)
	}
}

func TestUpdateStepStop(t *testing.T) {
	http := makeValidatedFakeHTTPClient(t, 200, "{}", func(r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		want := regexp.MustCompile(`{"endTime":"[\d-]+T[\d:.Z-]+","code":10}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})
	testAPI := api{"http://fakeurl", "faketoken", http}

	err := testAPI.UpdateStepStop(999, "step1", 10)

	if err != nil {
		t.Errorf("Unexpected error from UpdateStepStop: %v", err)
	}
}

func TestGetAPIURL(t *testing.T) {
	http := makeValidatedFakeHTTPClient(t, 200, "{}", func(r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		want := regexp.MustCompile(`{"endTime":"[\d-]+T[\d:.Z-]+","code":10}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})
	testAPI := api{"http://fakeurl", "faketoken", http}
	url, _ := testAPI.GetAPIURL()

	if !reflect.DeepEqual(url, "http://fakeurl/v4/") {
		t.Errorf(`api.GetAPIURL() expected to return: "%v", instead returned "%v"`, "http://fakeurl/v4/", url)
	}
}

func TestSecretsForBuild(t *testing.T) {
	testBuild := Build{
		ID:      1555,
		JobID:   3777,
		EventID: 8765,
		SHA:     "testSHA",
	}
	testResponse := `[{"name": "foo", "value": "bar"}]`
	wantSecrets := Secrets{
		{Name: "foo", Value: "bar"},
	}

	http := makeValidatedFakeHTTPClient(t, 200, testResponse, func(r *http.Request) {
		wantURL, _ := url.Parse("http://fakeurl/v4/builds/1555/secrets")
		if r.URL.String() != wantURL.String() {
			t.Errorf("Secrets URL=%q, want %q", r.URL, wantURL)
		}
	})
	testAPI := api{"http://fakeurl", "faketoken", http}

	s, err := testAPI.SecretsForBuild(testBuild)
	if err != nil {
		t.Fatalf("Unexpected error from SecretsForBuild: %v", err)
	}

	if !reflect.DeepEqual(s, wantSecrets) {
		t.Errorf("s=%q, want %q", s, wantSecrets)
	}
}

func TestGetBuildToken(t *testing.T) {
	testBuildID := 1111
	testBuildTimeoutMinutes := 90
	testResponse := `{"token": "foobar"}`
	wantToken := "foobar"

	http := makeValidatedFakeHTTPClient(t, 200, testResponse, func(r *http.Request) {
		wantURL, _ := url.Parse("http://fakeurl/v4/builds/1111/token")
		if r.URL.String() != wantURL.String() {
			t.Errorf("Secrets URL=%q, want %q", r.URL, wantURL)
		}
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		want := regexp.MustCompile(`{"buildTimeout":\d+}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})

	testAPI := api{"http://fakeurl", "faketoken", http}
	token, err := testAPI.GetBuildToken(testBuildID, testBuildTimeoutMinutes)
	if err != nil {
		t.Fatalf("Unexpected error from GetBuildToken: %v", err)
	}

	if !reflect.DeepEqual(token, wantToken) {
		t.Errorf("t=%q, want %q", token, wantToken)
	}
}
