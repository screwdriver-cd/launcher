package screwdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var sleep = time.Sleep

// BuildStatus is the status of a Screwdriver build
type BuildStatus string

// These are the set of valid statuses that a build can be set to
const (
	Running BuildStatus = "RUNNING"
	Success             = "SUCCESS"
	Failure             = "FAILURE"
	Aborted             = "ABORTED"
)

const maxAttempts = 5

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
	GetCoverageInfo() (Coverage, error)
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
	client  *http.Client
}

// New returns a new API object
func New(url, token string) (API, error) {
	newapi := api{
		url,
		token,
		&http.Client{Timeout: 10 * time.Second},
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

// Job is a Screwdriver Job.
type Job struct {
	ID         int    `json:"id"`
	PipelineID int    `json:"pipelineId"`
	Name       string `json:"name"`
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
	ID            int                    `json:"id"`
	JobID         int                    `json:"jobId"`
	SHA           string                 `json:"sha"`
	Commands      []CommandDef           `json:"steps"`
	Environment   map[string]string      `json:"environment"`
	ParentBuildID IntOrArray             `json:"parentBuildId"`
	Meta          map[string]interface{} `json:"meta"`
	EventID       int                    `json:"eventId"`
}

// Coverage is a Coverage object returned when getInfo is called
type Coverage struct {
	EnvVars map[string]string `json:"envVars"`
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

func (a api) makeURL(path string) (*url.URL, error) {
	version := "v4"
	fullpath := fmt.Sprintf("%s/%s/%s", a.baseURL, version, path)
	return url.Parse(fullpath)
}

func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

func handleResponse(res *http.Response) ([]byte, error) {
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var err SDError
		parserr := json.Unmarshal(body, &err)
		if parserr != nil {
			return nil, fmt.Errorf("Unparseable error response from Screwdriver: %v", parserr)
		}
		return nil, err
	}
	return body, nil
}

func retry(attempts int, callback func() error) (err error) {
	for i := 0; ; i++ {
		err = callback()
		if err == nil {
			return nil
		}

		if i >= (attempts - 1) {
			break
		}

		//Exponential backoff of 2 seconds
		duration := time.Duration(math.Pow(2, float64(i+1)))
		sleep(duration * time.Second)
	}
	return fmt.Errorf("After %d attempts, Last error: %s", attempts, err)
}

func (a api) get(url *url.URL) ([]byte, error) {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Generating request to Screwdriver: %v", err)
	}
	req.Header.Set("Authorization", tokenHeader(a.token))

	res := &http.Response{}
	attemptNumber := 0

	err = retry(maxAttempts, func() error {
		attemptNumber++
		res, err = a.client.Do(req)
		if err != nil {
			log.Printf("WARNING: received error from GET(%s): %v "+
				"(attempt %d of %d)", url.String(), err, attemptNumber, maxAttempts)
			return err
		}

		if res.StatusCode/100 == 5 {
			log.Printf("WARNING: received response %d from GET %s "+
				"(attempt %d of %d)", res.StatusCode, url.String(), attemptNumber, maxAttempts)
			return fmt.Errorf("GET retries exhausted: %d returned from GET %s",
				res.StatusCode, url.String())
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	return handleResponse(res)
}

func (a api) write(url *url.URL, requestType string, bodyType string, payload io.Reader) ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.ReadFrom(payload)
	p := buf.String()

	res := &http.Response{}
	req := &http.Request{}
	attemptNumber := 0

	err := retry(maxAttempts, func() error {
		attemptNumber++
		var err error
		req, err = http.NewRequest(requestType, url.String(), strings.NewReader(p))
		if err != nil {
			log.Printf("WARNING: received error generating new request for %s(%s): %v "+
				"(attempt %v of %v)", requestType, url.String(), err, attemptNumber, maxAttempts)
			return err
		}

		req.Header.Set("Authorization", tokenHeader(a.token))
		req.Header.Set("Content-Type", bodyType)

		res, err = a.client.Do(req)
		if err != nil {
			log.Printf("WARNING: received error from %s(%s): %v "+
				"(attempt %d of %d)", requestType, url.String(), err, attemptNumber, maxAttempts)
			return err
		}

		if res.StatusCode/100 == 5 {
			log.Printf("WARNING: received response %d from %s "+
				"(attempt %d of %d)", res.StatusCode, url.String(), attemptNumber, maxAttempts)
			return fmt.Errorf("retries exhausted: %d returned from %s",
				res.StatusCode, url.String())
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	return handleResponse(res)
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
func (a api) GetCoverageInfo() (coverage Coverage, err error) {
	url, err := a.makeURL(fmt.Sprintf("/coverage/info"))
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
