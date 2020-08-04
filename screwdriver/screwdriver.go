package screwdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

var sleep = time.Sleep

const CoverageURL = "coverage/info?jobId=%d&pipelineId=%d&jobName=%s&pipelineName=%s&scope=%s&prNum=%s&prParentJobId=%s"

// BuildStatus is the status of a Screwdriver build
type BuildStatus string

// These are the set of valid statuses that a build can be set to
const (
	Running BuildStatus = "RUNNING"
	Success             = "SUCCESS"
	Failure             = "FAILURE"
	Aborted             = "ABORTED"
)

const defaultBuildTimeoutBuffer = 30 // 30 minutes
const retryWaitMin = 100
const retryWaitMax = 300

var maxRetries = 5
var httpTimeout = time.Duration(20) * time.Second

func (b BuildStatus) String() string {
	return string(b)
}

// API is a Screwdriver API endpoint
type API interface {
	BuildFromID(buildID int) (Build, error)
	EventFromID(eventID int) (Event, error)
	JobFromID(jobID int) (Job, error)
	PipelineFromID(pipelineID int) (Pipeline, error)
	UpdateBuildStatus(status BuildStatus, meta map[string]interface{}, buildID int) error
	UpdateStepStart(buildID int, stepName string) error
	UpdateStepStop(buildID int, stepName string, exitCode int) error
	SecretsForBuild(build Build) (Secrets, error)
	GetAPIURL() (string, error)
	GetCoverageInfo(jobID, pipelineID int, jobName, pipelineName, scope, prNum, prParentJobId string) (Coverage, error)
	GetBuildToken(buildID int, buildTimeoutMinutes int) (string, error)
}

// SDError is an error response from the Screwdriver API
type SDError struct {
	StatusCode int    `json:"statusCode"`
	Reason     string `json:"error"`
	Message    string `json:"message"`
}

func (e SDError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.StatusCode, e.Reason, e.Message)
}

type api struct {
	baseURL string
	token   string
	client  *retryablehttp.Client
}

// New returns a new API object
func New(url, token string) (API, error) {
	// read config from env variables
	if strings.TrimSpace(os.Getenv("SDAPI_TIMEOUT_SECS")) != "" {
		apiTimeout, _ := strconv.Atoi(os.Getenv("SDAPI_TIMEOUT_SECS"))
		httpTimeout = time.Duration(apiTimeout) * time.Second
	}

	if strings.TrimSpace(os.Getenv("SDAPI_MAXRETRIES")) != "" {
		maxRetries, _ = strconv.Atoi(os.Getenv("SDAPI_MAXRETRIES"))
	}

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = maxRetries
	retryClient.RetryWaitMin = time.Duration(retryWaitMin) * time.Millisecond
	retryClient.RetryWaitMax = time.Duration(retryWaitMax) * time.Millisecond
	retryClient.Backoff = retryablehttp.LinearJitterBackoff
	retryClient.HTTPClient.Timeout = httpTimeout
	newapi := api{
		url,
		token,
		retryClient,
	}
	return API(newapi), nil
}

// BuildStatusPayload is a Screwdriver Build Status payload.
type BuildStatusPayload struct {
	Status string                 `json:"status"`
	Meta   map[string]interface{} `json:"meta"`
}

// StepStartPayload is a Screwdriver Step Start payload.
type StepStartPayload struct {
	StartTime time.Time `json:"startTime"`
}

// StepStopPayload is a Screwdriver Step Stop payload.
type StepStopPayload struct {
	EndTime  time.Time `json:"endTime"`
	ExitCode int       `json:"code"`
}

// BuildTokenPayload is a Screwdriver Build Token payload.
type BuildTokenPayload struct {
	BuildTimeout int `json:"buildTimeout"`
}

// Pipeline is a Screwdriver Pipeline definition.
type Pipeline struct {
	ID      int     `json:"id"`
	ScmRepo ScmRepo `json:"scmRepo"`
	ScmURI  string  `json:"scmUri"`
}

// ScmRepo contains the full name of the repository for a Pipeline, e.g. "screwdriver-cd/launcher"
type ScmRepo struct {
	Name string `json:"name"`
}

type JobAnnotations struct {
	CoverageScope string `json:"screwdriver.cd/coverageScope,omitempty" default:""`
}

type JobPermutation struct {
	Annotations JobAnnotations `json:"annotations"`
}

// Job is a Screwdriver Job.
type Job struct {
	ID            int              `json:"id"`
	PipelineID    int              `json:"pipelineId"`
	Name          string           `json:"name"`
	PrParentJobID int              `json:"prParentJobId"`
	Permutations  []JobPermutation `json:"permutations,omitempty"`
}

// CommandDef is the definition of a single executable command.
type CommandDef struct {
	Name string `json:"name"`
	Cmd  string `json:"command"`
}

// Need a generic interface to take in an int or array of ints
type IntOrArray interface{}

// Build is a Screwdriver Build
type Build struct {
	ID             int                    `json:"id"`
	JobID          int                    `json:"jobId"`
	SHA            string                 `json:"sha"`
	Commands       []CommandDef           `json:"steps"`
	Environment    []map[string]string    `json:"environment"`
	ParentBuildID  IntOrArray             `json:"parentBuildId"`
	Meta           map[string]interface{} `json:"meta"`
	EventID        int                    `json:"eventId"`
	Createtime     string                 `json:"createTime"`
	QueueEntertime string                 `json:"stats.queueEnterTime"`
}

// Coverage is a Coverage object returned when getInfo is called
type Coverage struct {
	EnvVars map[string]interface{} `json:"envVars"`
}

// Event is a Screwdriver Event
type Event struct {
	ID            int                    `json:"id"`
	Meta          map[string]interface{} `json:"meta"`
	ParentEventID int                    `json:"parentEventId"`
}

// Secret is a Screwdriver build secret.
type Secret struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Secrets is the collection of secrets for a Screwdriver build
type Secrets []Secret

// Token is a Screwdriver API token.
type Token struct {
	Token string `json:"token"`
}

func (a api) makeURL(path string) (*url.URL, error) {
	version := "v4"
	fullpath := fmt.Sprintf("%s/%s/%s", a.baseURL, version, path)
	return url.Parse(fullpath)
}

func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

func (a api) get(url *url.URL) ([]byte, error) {
	requestType := "GET"
	req, err := http.NewRequest(requestType, url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Generating request to Screwdriver: %v", err)
	}

	defer a.client.HTTPClient.CloseIdleConnections()

	req.Header.Set("Authorization", tokenHeader(a.token))

	res, err := a.client.StandardClient().Do(req)

	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		log.Printf("WARNING: received error from %s(%s): %v ", requestType, url.String(), err)
		return nil, fmt.Errorf("WARNING: received error from %s(%s): %v ", requestType, url.String(), err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("reading response Body from Screwdriver: %v", err)
		return nil, fmt.Errorf("reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var errParse SDError
		parseError := json.Unmarshal(body, &errParse)
		if parseError != nil {
			log.Printf("unparseable error response from Screwdriver: %v", parseError)
			return nil, fmt.Errorf("unparseable error response from Screwdriver: %v", parseError)
		}

		log.Printf("WARNING: received response %d from %s ", res.StatusCode, url.String())
		return nil, fmt.Errorf("WARNING: received response %d from %s ", res.StatusCode, url.String())
	}

	return body, nil
}

func (a api) write(url *url.URL, requestType string, bodyType string, payload io.Reader) ([]byte, error) {
	req := &http.Request{}
	buf := new(bytes.Buffer)

	size, err := buf.ReadFrom(payload)
	if err != nil {
		log.Printf("WARNING: error:[%v], not able to read payload: %v", err, payload)
		return nil, fmt.Errorf("WARNING: error:[%v], not able to read payload: %v", err, payload)
	}
	p := buf.String()

	req, err = http.NewRequest(requestType, url.String(), strings.NewReader(p))
	if err != nil {
		log.Printf("WARNING: received error generating new request for %s(%s): %v ", requestType, url.String(), err)
		return nil, fmt.Errorf("WARNING: received error generating new request for %s(%s): %v ", requestType, url.String(), err)
	}

	defer a.client.HTTPClient.CloseIdleConnections()

	req.Header.Set("Authorization", tokenHeader(a.token))
	req.Header.Set("Content-Type", bodyType)
	req.ContentLength = size

	res, err := a.client.StandardClient().Do(req)
	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		log.Printf("WARNING: received error from %s(%s): %v ", requestType, url.String(), err)
		return nil, fmt.Errorf("WARNING: received error from %s(%s): %v ", requestType, url.String(), err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("reading response Body from Screwdriver: %v", err)
		return nil, fmt.Errorf("reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var errParse SDError
		parseError := json.Unmarshal(body, &errParse)
		if parseError != nil {
			log.Printf("unparseable error response from Screwdriver: %v", parseError)
			return nil, fmt.Errorf("unparseable error response from Screwdriver: %v", parseError)
		}

		log.Printf("WARNING: received response %d from %s ", res.StatusCode, url.String())
		return nil, fmt.Errorf("WARNING: received response %d from %s ", res.StatusCode, url.String())
	}

	return body, nil
}

func (a api) post(url *url.URL, bodyType string, payload io.Reader) ([]byte, error) {
	return a.write(url, "POST", bodyType, payload)
}

func (a api) put(url *url.URL, bodyType string, payload io.Reader) ([]byte, error) {
	return a.write(url, "PUT", bodyType, payload)
}

func (a api) GetAPIURL() (string, error) {
	url, err := a.makeURL("")
	return url.String(), err
}

// Get coverage object with coverage information
func (a api) GetCoverageInfo(jobID, pipelineID int, jobName, pipelineName, scope, prNum, prParentJobId string) (coverage Coverage, err error) {
	url, err := a.makeURL(fmt.Sprintf(CoverageURL, jobID, pipelineID, jobName, pipelineName, scope, prNum, prParentJobId))
	body, err := a.get(url)
	if err != nil {
		return coverage, err
	}

	err = json.Unmarshal(body, &coverage)
	if err != nil {
		return coverage, fmt.Errorf("Parsing JSON response %q: %v", body, err)
	}

	return coverage, nil
}

// BuildFromID fetches and returns a Build object from its ID
func (a api) BuildFromID(buildID int) (build Build, err error) {
	u, err := a.makeURL(fmt.Sprintf("builds/%d", buildID))
	body, err := a.get(u)
	if err != nil {
		return build, err
	}

	err = json.Unmarshal(body, &build)
	if err != nil {
		return build, fmt.Errorf("Parsing JSON response %q: %v", body, err)
	}

	initCmd := build.Commands[0]
	if initCmd.Name == "sd-setup-init" {
		build.Commands = build.Commands[1:]
	}

	return build, nil
}

// EventFromID fetches and returns a Event object from its ID
func (a api) EventFromID(eventID int) (event Event, err error) {
	u, err := a.makeURL(fmt.Sprintf("events/%d", eventID))
	body, err := a.get(u)
	if err != nil {
		return event, err
	}

	err = json.Unmarshal(body, &event)
	if err != nil {
		return event, fmt.Errorf("Parsing JSON response %q: %v", body, err)
	}
	return event, nil
}

// JobFromID fetches and returns a Job object from its ID
func (a api) JobFromID(jobID int) (job Job, err error) {
	u, err := a.makeURL(fmt.Sprintf("jobs/%d", jobID))
	if err != nil {
		return job, fmt.Errorf("Generating Screwdriver url for Job %d: %v", jobID, err)
	}

	body, err := a.get(u)
	if err != nil {
		return job, err
	}

	err = json.Unmarshal(body, &job)
	if err != nil {
		return job, fmt.Errorf("Parsing JSON response %q: %v", body, err)
	}
	return job, nil
}

// PipelineFromID fetches and returns a Pipeline object from its ID
func (a api) PipelineFromID(pipelineID int) (pipeline Pipeline, err error) {
	u, err := a.makeURL(fmt.Sprintf("pipelines/%d", pipelineID))
	if err != nil {
		return pipeline, err
	}

	body, err := a.get(u)
	if err != nil {
		return pipeline, err
	}

	err = json.Unmarshal(body, &pipeline)
	if err != nil {
		return pipeline, fmt.Errorf("Parsing JSON response %q: %v", body, err)
	}
	return pipeline, nil
}

func (a api) UpdateBuildStatus(status BuildStatus, meta map[string]interface{}, buildID int) error {
	switch status {
	case Running:
	case Success:
	case Failure:
	case Aborted:
	default:
		return fmt.Errorf("Invalid build status: %s", status)
	}

	u, err := a.makeURL(fmt.Sprintf("builds/%d", buildID))
	if err != nil {
		return fmt.Errorf("creating url: %v", err)
	}

	bs := BuildStatusPayload{
		Status: status.String(),
		Meta:   meta,
	}
	payload, err := json.Marshal(bs)
	if err != nil {
		return fmt.Errorf("Marshaling JSON for Build Status: %v", err)
	}

	_, err = a.put(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("Posting to Build Status: %v", err)
	}

	return nil
}

func (a api) UpdateStepStart(buildID int, stepName string) error {
	u, err := a.makeURL(fmt.Sprintf("builds/%d/steps/%s", buildID, stepName))
	if err != nil {
		return fmt.Errorf("Creating url: %v", err)
	}

	bs := StepStartPayload{
		StartTime: time.Now(),
	}
	payload, err := json.Marshal(bs)
	if err != nil {
		return fmt.Errorf("Marshaling JSON for Step Start: %v", err)
	}

	_, err = a.put(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("Posting to Step Start: %v", err)
	}

	return nil
}

func (a api) UpdateStepStop(buildID int, stepName string, exitCode int) error {
	u, err := a.makeURL(fmt.Sprintf("builds/%d/steps/%s", buildID, stepName))
	if err != nil {
		return fmt.Errorf("Creating url: %v", err)
	}

	bs := StepStopPayload{
		EndTime:  time.Now(),
		ExitCode: exitCode,
	}
	payload, err := json.Marshal(bs)
	if err != nil {
		return fmt.Errorf("Marshaling JSON for Step Stop: %v", err)
	}

	_, err = a.put(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("Posting to Step Stop: %v", err)
	}

	return nil
}

func (a api) SecretsForBuild(build Build) (Secrets, error) {
	u, err := a.makeURL(fmt.Sprintf("builds/%d/secrets", build.ID))
	if err != nil {
		return nil, err
	}

	body, err := a.get(u)
	if err != nil {
		return nil, err
	}

	secrets := Secrets{}

	err = json.Unmarshal(body, &secrets)
	if err != nil {
		return secrets, fmt.Errorf("Parsing JSON response %q: %v", body, err)
	}

	return secrets, nil
}

func (a api) GetBuildToken(buildID int, buildTimeoutMinutes int) (string, error) {
	u, err := a.makeURL(fmt.Sprintf("builds/%d/token", buildID))
	if err != nil {
		return a.token, fmt.Errorf("Creating url: %v", err)
	}

	bs := BuildTokenPayload{
		BuildTimeout: buildTimeoutMinutes + defaultBuildTimeoutBuffer,
	}
	payload, err := json.Marshal(bs)
	if err != nil {
		return a.token, fmt.Errorf("Marshaling JSON for Build Token: %v", err)
	}

	body, err := a.post(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return a.token, fmt.Errorf("Posting to Build Token: %v", err)
	}

	buildToken := Token{}

	err = json.Unmarshal(body, &buildToken)
	if err != nil {
		return a.token, fmt.Errorf("Parsing JSON response %q: %v", body, err)
	}

	return buildToken.Token, nil
}
