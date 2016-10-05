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
	BuildFromID(buildID string) (Build, error)
	JobFromID(jobID string) (Job, error)
	PipelineFromID(pipelineID string) (Pipeline, error)
	UpdateBuildStatus(status BuildStatus, buildID string) error
	PipelineDefFromYaml(yaml io.Reader) (PipelineDef, error)
	UpdateStepStart(buildID, stepName string) error
	UpdateStepStop(buildID, stepName string, exitCode int) error
	SecretsForBuild(build Build) (Secrets, error)
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
	Status string `json:"status"`
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

// Validator is a Screwdriver Validator payload.
type Validator struct {
	Yaml string `json:"yaml"`
}

// Pipeline is a Screwdriver Pipeline definition.
type Pipeline struct {
	ID     string `json:"id"`
	ScmURL string `json:"scmUrl"`
}

// PipelineDef contains the step definitions and jobs for a Pipeline.
type PipelineDef struct {
	Jobs     map[string][]JobDef `json:"jobs"`
	Workflow []string            `json:"workflow"`
}

// JobDef contains the step and environment definitions of a single Job.
type JobDef struct {
	Image       string            `json:"image"`
	Commands    []CommandDef      `json:"commands"`
	Environment map[string]string `json:"environment"`
}

// Job is a Screwdriver Job.
type Job struct {
	ID         string `json:"id"`
	PipelineID string `json:"pipelineId"`
	Name       string `json:"name"`
}

// CommandDef is the definition of a single executable command.
type CommandDef struct {
	Name string `json:"name"`
	Cmd  string `json:"command"`
}

// Build is a Screwdriver Build
type Build struct {
	ID    string `json:"id"`
	JobID string `json:"jobId"`
	SHA   string `json:"sha"`
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
		return nil, fmt.Errorf("reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var err SDError
		parserr := json.Unmarshal(body, &err)
		if parserr != nil {
			return nil, fmt.Errorf("unparseable error response from Screwdriver: %v", parserr)
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
	return fmt.Errorf("after %d attempts, last error: %s", attempts, err)
}

func (a api) get(url *url.URL) ([]byte, error) {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("generating request to Screwdriver: %v", err)
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

// BuildFromID fetches and returns a Build object from its ID
func (a api) BuildFromID(buildID string) (build Build, err error) {
	u, err := a.makeURL(fmt.Sprintf("builds/%s", buildID))
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

// JobFromID fetches and returns a Job object from its ID
func (a api) JobFromID(jobID string) (job Job, err error) {
	u, err := a.makeURL(fmt.Sprintf("jobs/%s", jobID))
	if err != nil {
		return job, fmt.Errorf("generating Screwdriver url for Job %s: %v", jobID, err)
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
func (a api) PipelineFromID(pipelineID string) (pipeline Pipeline, err error) {
	u, err := a.makeURL(fmt.Sprintf("pipelines/%s", pipelineID))
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

func (a api) PipelineDefFromYaml(yaml io.Reader) (PipelineDef, error) {
	u, err := a.makeURL("validator")
	if err != nil {
		return PipelineDef{}, err
	}

	y, err := ioutil.ReadAll(yaml)
	if err != nil {
		return PipelineDef{}, fmt.Errorf("reading Screwdriver YAML: %v", err)
	}

	v := Validator{string(y)}
	payload, err := json.Marshal(v)
	if err != nil {
		return PipelineDef{}, fmt.Errorf("marshaling JSON for Validator: %v", err)
	}

	res, err := a.post(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return PipelineDef{}, fmt.Errorf("posting to Validator: %v", err)
	}

	var pipelineDef PipelineDef
	err = json.Unmarshal(res, &pipelineDef)
	if err != nil {
		return PipelineDef{}, fmt.Errorf("parsing JSON response from the Validator: %v", err)
	}

	return pipelineDef, nil
}

func (a api) UpdateBuildStatus(status BuildStatus, buildID string) error {
	switch status {
	case Running:
	case Success:
	case Failure:
	case Aborted:
	default:
		return fmt.Errorf("invalid build status: %s", status)
	}

	u, err := a.makeURL(fmt.Sprintf("builds/%s", buildID))
	if err != nil {
		return fmt.Errorf("creating url: %v", err)
	}

	bs := BuildStatusPayload{
		Status: status.String(),
	}
	payload, err := json.Marshal(bs)
	if err != nil {
		return fmt.Errorf("marshaling JSON for Build Status: %v", err)
	}

	_, err = a.put(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("posting to Build Status: %v", err)
	}

	return nil
}

func (a api) UpdateStepStart(buildID, stepName string) error {
	u, err := a.makeURL(fmt.Sprintf("builds/%s/steps/%s", buildID, stepName))
	if err != nil {
		return fmt.Errorf("creating url: %v", err)
	}

	bs := StepStartPayload{
		StartTime: time.Now(),
	}
	payload, err := json.Marshal(bs)
	if err != nil {
		return fmt.Errorf("marshaling JSON for Step Start: %v", err)
	}

	_, err = a.put(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("posting to Step Start: %v", err)
	}

	return nil
}

func (a api) UpdateStepStop(buildID, stepName string, exitCode int) error {
	u, err := a.makeURL(fmt.Sprintf("builds/%s/steps/%s", buildID, stepName))
	if err != nil {
		return fmt.Errorf("creating url: %v", err)
	}

	bs := StepStopPayload{
		EndTime:  time.Now(),
		ExitCode: exitCode,
	}
	payload, err := json.Marshal(bs)
	if err != nil {
		return fmt.Errorf("marshaling JSON for Step Stop: %v", err)
	}

	_, err = a.put(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("posting to Step Stop: %v", err)
	}

	return nil
}

func (a api) SecretsForBuild(build Build) (Secrets, error) {
	u, err := a.makeURL(fmt.Sprintf("builds/%s/secrets", build.ID))
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
