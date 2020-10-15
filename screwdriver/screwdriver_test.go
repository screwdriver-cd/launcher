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

	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/assert"
)

const (
	testMaxRetries   = 4
	testRetryWaitMin = 10
	testRetryWaitMax = 10
	testHttpTimeout  = 10
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

		fmt.Println("in api")
		validateHeader(t, "Authorization", wantTokenHeader)
		v(r)

		w.WriteHeader(code)
		if code == 500 {
			time.Sleep(time.Duration(2) * time.Second)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func makeRetryableHttpClient(maxRetries, retryWaitMin, retryWaitMax, httpTimeout int) *retryablehttp.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = maxRetries
	client.RetryWaitMin = time.Duration(retryWaitMin) * time.Millisecond
	client.RetryWaitMax = time.Duration(retryWaitMax) * time.Millisecond
	client.Backoff = retryablehttp.LinearJitterBackoff
	client.HTTPClient.Timeout = time.Duration(httpTimeout) * time.Millisecond

	return client
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

	var testEnv []map[string]string
	testEnv = append(testEnv, map[string]string{"foo": "bar"})
	testEnv = append(testEnv, map[string]string{"baz": "bah"})

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
				ID:             1555,
				JobID:          3777,
				EventID:        8765,
				SHA:            "testSHA",
				Commands:       testCmds,
				Environment:    testEnv,
				Createtime:     "2020-04-28T20:34:01.907Z",
				QueueEntertime: "2020-05-01T20:15:31.508Z",
			},
			statusCode: 200,
			err:        nil,
		},
		{
			build:      Build{},
			statusCode: 500,
			err: errors.New("WARNING: received error from GET(http://fakeurl/v4/builds/0): " +
				"Get \"http://fakeurl/v4/builds/0\": " +
				"GET http://fakeurl/v4/builds/0 giving up after 5 attempts "),
		},
		{
			build:      Build{},
			statusCode: 404,
			err:        errors.New("WARNING: received response 404 from http://fakeurl/v4/builds/0 "),
		},
	}

	for _, test := range tests {
		JSON, err := json.Marshal(test.build)
		if err != nil {
			t.Fatalf("Unable to Marshal JSON: %v", err)
		}

		var client *retryablehttp.Client
		client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
		client.HTTPClient = makeFakeHTTPClient(t, test.statusCode, string(JSON))
		testAPI := api{"http://fakeurl", "faketoken", client}

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
				Creator: map[string]string{
					"username": "testUsername",
				},
			},
			statusCode: 200,
			err:        nil,
		},
		{
			event:      Event{},
			statusCode: 500,
			err: errors.New("WARNING: received error from GET(http://fakeurl/v4/events/0): " +
				"Get \"http://fakeurl/v4/events/0\": " +
				"GET http://fakeurl/v4/events/0 giving up after 5 attempts "),
		},
		{
			event:      Event{},
			statusCode: 404,
			err:        errors.New("WARNING: received response 404 from http://fakeurl/v4/events/0 "),
		},
	}

	for _, test := range tests {
		JSON, err := json.Marshal(test.event)
		if err != nil {
			t.Fatalf("Unable to Marshal JSON: %v", err)
		}

		var client *retryablehttp.Client
		client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
		client.HTTPClient = makeFakeHTTPClient(t, test.statusCode, string(JSON))
		testAPI := api{"http://fakeurl", "faketoken", client}

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
	envVars := map[string]interface{}{
		"SD_SONAR_AUTH_URL":   "https://api.screwdriver.cd/v4/coverage/token",
		"SD_SONAR_HOST":       "https://sonar.screwdriver.cd",
		"SD_SONAR_ENTERPRISE": false,
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
			err: errors.New("WARNING: received error from GET(http://fakeurl/v4/coverage/info?jobId=123&pipelineId=456&jobName=main&pipelineName=d2lam/mytest&scope=&prNum=&prParentJobId=): " +
				"Get \"http://fakeurl/v4/coverage/info?jobId=123&pipelineId=456&jobName=main&pipelineName=d2lam/mytest&scope=&prNum=&prParentJobId=\": " +
				"GET http://fakeurl/v4/coverage/info?jobId=123&pipelineId=456&jobName=main&pipelineName=d2lam/mytest&scope=&prNum=&prParentJobId= giving up after 5 attempts "),
		},
		{
			coverage:   Coverage{},
			statusCode: 404,
			err:        errors.New("WARNING: received response 404 from http://fakeurl/v4/coverage/info?jobId=123&pipelineId=456&jobName=main&pipelineName=d2lam/mytest&scope=&prNum=&prParentJobId= "),
		},
	}

	for _, test := range tests {
		JSON, err := json.Marshal(test.coverage)
		if err != nil {
			t.Fatalf("Unable to Marshal JSON: %v", err)
		}

		var client *retryablehttp.Client
		client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
		client.HTTPClient = makeFakeHTTPClient(t, test.statusCode, string(JSON))
		testAPI := api{"http://fakeurl", "faketoken", client}

		coverage, err := testAPI.GetCoverageInfo(123, 456, "main", "d2lam/mytest", "", "", "")

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
				Permutations: []JobPermutation{
					{
						Annotations: JobAnnotations{
							CoverageScope: "job",
						},
					},
				},
			},
			statusCode: 200,
			err:        nil,
		},
		{
			job:        Job{},
			statusCode: 500,
			err: errors.New("WARNING: received error from GET(http://fakeurl/v4/jobs/0): " +
				"Get \"http://fakeurl/v4/jobs/0\": " +
				"GET http://fakeurl/v4/jobs/0 giving up after 5 attempts "),
		},
		{
			job:        Job{},
			statusCode: 404,
			err:        errors.New("WARNING: received response 404 from http://fakeurl/v4/jobs/0 "),
		},
	}

	for _, test := range tests {
		JSON, err := json.Marshal(test.job)
		if err != nil {
			t.Fatalf("Unable to Marshal JSON: %v", err)
		}

		var client *retryablehttp.Client
		client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
		client.HTTPClient = makeFakeHTTPClient(t, test.statusCode, string(JSON))
		testAPI := api{"http://fakeurl", "faketoken", client}

		job, err := testAPI.JobFromID(test.job.ID)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from JobFromID: \n%v\n want \n%v", err, test.err)
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
			err: errors.New("WARNING: received error from GET(http://fakeurl/v4/pipelines/0): " +
				"Get \"http://fakeurl/v4/pipelines/0\": " +
				"GET http://fakeurl/v4/pipelines/0 giving up after 5 attempts "),
		},
		{
			pipeline:   Pipeline{},
			statusCode: 404,
			err:        errors.New("WARNING: received response 404 from http://fakeurl/v4/pipelines/0 "),
		},
	}

	for _, test := range tests {
		JSON, err := json.Marshal(test.pipeline)
		if err != nil {
			t.Fatalf("Unable to Marshal JSON: %v", err)
		}

		var client *retryablehttp.Client
		client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
		client.HTTPClient = makeFakeHTTPClient(t, test.statusCode, string(JSON))
		testAPI := api{"http://fakeurl", "faketoken", client}

		pipeline, err := testAPI.PipelineFromID(test.pipeline.ID)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from PipelineFromID: \n%v\n want \n%v", err, test.err)
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
		{Success, meta, 500, errors.New("Posting to Build Status: " +
			"WARNING: received error from PUT(http://fakeurl/v4/builds/15): " +
			"Put \"http://fakeurl/v4/builds/15\": " +
			"PUT http://fakeurl/v4/builds/15 giving up after 5 attempts ")},
	}

	for _, test := range tests {
		var client *retryablehttp.Client
		client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
		client.HTTPClient = makeFakeHTTPClient(t, test.statusCode, "{}")
		testAPI := api{"http://fakeurl", "faketoken", client}

		err := testAPI.UpdateBuildStatus(test.status, test.meta, 15)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from UpdateBuildStatus: \n%v\n want \n%v", err, test.err)
		}
	}
}

func TestUpdateStepStart(t *testing.T) {
	var client *retryablehttp.Client
	client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
	client.HTTPClient = makeValidatedFakeHTTPClient(t, 200, "{}", func(r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		want := regexp.MustCompile(`{"startTime":"[\d-]+T[\d:.(Z-|Z+)]+"}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})
	testAPI := api{"http://fakeurl", "faketoken", client}

	err := testAPI.UpdateStepStart(999, "step1")

	if err != nil {
		t.Errorf("Unexpected error from UpdateStepStart: %v", err)
	}
}

func TestUpdateStepStop(t *testing.T) {
	var client *retryablehttp.Client
	client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
	client.HTTPClient = makeValidatedFakeHTTPClient(t, 200, "{}", func(r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		want := regexp.MustCompile(`{"endTime":"[\d-]+T[\d:.(Z-|Z+)]+","code":10}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})
	testAPI := api{"http://fakeurl", "faketoken", client}

	err := testAPI.UpdateStepStop(999, "step1", 10)

	if err != nil {
		t.Errorf("Unexpected error from UpdateStepStop: %v", err)
	}
}

func TestGetAPIURL(t *testing.T) {
	var client *retryablehttp.Client
	client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
	client.HTTPClient = makeValidatedFakeHTTPClient(t, 200, "{}", func(r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		want := regexp.MustCompile(`{"endTime":"[\d-]+T[\d:.Z-]+","code":10}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})
	testAPI := api{"http://fakeurl", "faketoken", client}
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

	var client *retryablehttp.Client
	client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
	client.HTTPClient = makeValidatedFakeHTTPClient(t, 200, testResponse, func(r *http.Request) {
		wantURL, _ := url.Parse("http://fakeurl/v4/builds/1555/secrets")
		if r.URL.String() != wantURL.String() {
			t.Errorf("Secrets URL=%q, want %q", r.URL, wantURL)
		}
	})
	testAPI := api{"http://fakeurl", "faketoken", client}

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

	var client *retryablehttp.Client
	client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
	client.HTTPClient = makeValidatedFakeHTTPClient(t, 200, testResponse, func(r *http.Request) {
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

	testAPI := api{"http://fakeurl", "faketoken", client}
	token, err := testAPI.GetBuildToken(testBuildID, testBuildTimeoutMinutes)
	if err != nil {
		t.Fatalf("Unexpected error from GetBuildToken: %v", err)
	}

	if !reflect.DeepEqual(token, wantToken) {
		t.Errorf("t=%q, want %q", token, wantToken)
	}
}

func TestNewDefaults(t *testing.T) {
	maxRetries = 5
	httpTimeout = time.Duration(20) * time.Second

	os.Setenv("SDAPI_TIMEOUT_SECS", "")
	os.Setenv("SDAPI_MAXRETRIES", "")
	_, _ = New("http://fakeurl", "fake")
	assert.Equal(t, httpTimeout, time.Duration(20)*time.Second)
	assert.Equal(t, maxRetries, 5)
}

func TestNew(t *testing.T) {
	os.Setenv("SDAPI_TIMEOUT_SECS", "10")
	os.Setenv("SDAPI_MAXRETRIES", "1")
	_, _ = New("http://fakeurl", "fake")
	assert.Equal(t, httpTimeout, time.Duration(10)*time.Second)
	assert.Equal(t, maxRetries, 1)
}
