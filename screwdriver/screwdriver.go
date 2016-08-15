package screwdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

// BuildStatus is the status of a Screwdriver build
type BuildStatus string

// These are the set of valid statuses that a build can be set to
const (
	Running BuildStatus = "RUNNING"
	Success             = "SUCCESS"
	Failure             = "FAILURE"
	Aborted             = "ABORTED"
)

func (b BuildStatus) String() string {
	return string(b)
}

var readAll = ioutil.ReadAll
var unmarshal = jsonUnmarshal
var httpClientDo = do
var httpNewRequest = http.NewRequest
var jsonMarshal = json.Marshal
var jsonUnmarshal = json.Unmarshal
var makeURL = URLMake
var apiPost = postWithAPI

// API is a Screwdriver API endpoint
type API interface {
	BuildFromID(buildID string) (Build, error)
	JobFromID(jobID string) (Job, error)
	PipelineFromID(pipelineID string) (Pipeline, error)
	UpdateBuildStatus(status BuildStatus) error
	PipelineDefFromYaml(yaml io.Reader) (PipelineDef, error)
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
	api := api{
		url,
		token,
		&http.Client{},
	}
	return API(api), nil
}

// BuildStatusPayload is a Screwdriver Build Status payload.
type BuildStatusPayload struct {
	Status string `json:"status"`
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
}

func URLMake(a *api, path string) (*url.URL, error) {
	return a.makeURL(path)
}

func (a api) makeURL(path string) (*url.URL, error) {
	version := "v3"
	fullpath := fmt.Sprintf("%s/%s/%s", a.baseURL, version, path)
	return url.Parse(fullpath)
}

func do(client *http.Client, req *http.Request) (resp *http.Response, err error) {
	return client.Do(req)
}

func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

func handleResponse(res *http.Response) ([]byte, error) {
	body, err := readAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var err SDError
		parserr := unmarshal(body, &err)
		if parserr != nil {
			return nil, fmt.Errorf("unparseable error response from Screwdriver: %v", parserr)
		}
		return nil, err
	}
	return body, nil
}

func (a api) get(url *url.URL) ([]byte, error) {
	req, err := httpNewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("generating request to Screwdriver: %v", err)
	}
	req.Header.Set("Authorization", tokenHeader(a.token))

	res, err := httpClientDo(a.client, req)
	if err != nil {
		return nil, fmt.Errorf("reading response from Screwdriver: %v", err)
	}
	defer res.Body.Close()

	return handleResponse(res)
}

func (a api) post(url *url.URL, bodyType string, payload io.Reader) ([]byte, error) {
	req, err := httpNewRequest("POST", url.String(), payload)
	if err != nil {
		return nil, fmt.Errorf("generating request to Screwdriver: %v", err)
	}
	req.Header.Set("Content-Type", bodyType)
	req.Header.Set("Authorization", tokenHeader(a.token))

	res, err := httpClientDo(a.client, req)
	if err != nil {
		return nil, fmt.Errorf("posting to Screwdriver endpoint %v: %v", url, err)
	}
	defer res.Body.Close()

	return handleResponse(res)
}

func postWithAPI(a *api, url *url.URL, bodyType string, payload io.Reader) ([]byte, error) {
	return a.post(url, bodyType, payload)
}

// BuildFromID fetches and returns a Build object from its ID
func (a api) BuildFromID(buildID string) (build Build, err error) {
	u, err := makeURL(&a, fmt.Sprintf("builds/%s", buildID))
	if err != nil {
		return build, fmt.Errorf("generating Screwdriver url for Build %v: %v", buildID, err)
	}
	body, err := a.get(u)
	if err != nil {
		return build, err
	}

	err = jsonUnmarshal(body, &build)
	if err != nil {
		return build, fmt.Errorf("Parsing JSON response %q: %v", body, err)
	}
	return build, nil
}

// JobFromID fetches and returns a Job object from its ID
func (a api) JobFromID(jobID string) (job Job, err error) {
	u, err := makeURL(&a, fmt.Sprintf("jobs/%s", jobID))
	if err != nil {
		return job, fmt.Errorf("generating Screwdriver url for Job %v: %v", jobID, err)
	}

	body, err := a.get(u)
	if err != nil {
		return job, err
	}

	err = jsonUnmarshal(body, &job)
	if err != nil {
		return job, fmt.Errorf("Parsing JSON response %q: %v", body, err)
	}
	return job, nil
}

// PipelineFromID fetches and returns a Pipeline object from its ID
func (a api) PipelineFromID(pipelineID string) (pipeline Pipeline, err error) {
	u, err := makeURL(&a, fmt.Sprintf("pipelines/%s", pipelineID))
	if err != nil {
		return pipeline, fmt.Errorf("generating Screwdriver url for Pipeline %v: %v", pipelineID, err)
	}

	body, err := a.get(u)
	if err != nil {
		return pipeline, err
	}

	err = jsonUnmarshal(body, &pipeline)
	if err != nil {
		return pipeline, fmt.Errorf("Parsing JSON response %q: %v", body, err)
	}
	return pipeline, nil
}

// PipelineDefFromYaml returns a PipelineDef object from a valid Yaml
func (a api) PipelineDefFromYaml(yaml io.Reader) (PipelineDef, error) {
	u, err := makeURL(&a, "validator")
	if err != nil {
		return PipelineDef{}, fmt.Errorf("generating Screwdriver url for Validator: %v", err)
	}

	y, err := readAll(yaml)
	if err != nil {
		return PipelineDef{}, fmt.Errorf("reading Screwdriver YAML: %v", err)
	}

	v := Validator{string(y)}
	payload, err := jsonMarshal(v)
	if err != nil {
		return PipelineDef{}, fmt.Errorf("marshaling JSON for Validator: %v", err)
	}

	res, err := apiPost(&a, u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return PipelineDef{}, fmt.Errorf("posting to Validator: %v", err)
	}

	var pipelineDef PipelineDef
	err = jsonUnmarshal(res, &pipelineDef)
	if err != nil {
		return PipelineDef{}, fmt.Errorf("parsing JSON response from the Validator: %v", err)
	}

	return pipelineDef, nil
}

func (a api) UpdateBuildStatus(status BuildStatus) error {
	switch status {
	case Running:
	case Success:
	case Failure:
	case Aborted:
	default:
		return fmt.Errorf("invalid build status: %s", status)
	}

	fmt.Println("REACHED")
	u, err := makeURL(&a, "webhooks/build")
	if err != nil {
		return fmt.Errorf("generating Screwdriver url for Build Status: %v", err)
	}

	bs := BuildStatusPayload{
		Status: status.String(),
	}
	payload, err := jsonMarshal(bs)
	if err != nil {
		return fmt.Errorf("marshaling JSON for Build Status: %v", err)
	}

	_, err = apiPost(&a, u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("posting to Build Status: %v", err)
	}

	return nil
}
