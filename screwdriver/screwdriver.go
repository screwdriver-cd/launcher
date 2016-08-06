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

// API is a Screwdriver API endpoint
type API interface {
	BuildFromID(buildID string) (Build, error)
	JobFromID(jobID string) (Job, error)
	PipelineFromID(pipelineID string) (Pipeline, error)
	UpdateBuildStatus(status string) error
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

// BuildStatus is a Screwdriver Build Status payload
type BuildStatus struct {
	Status string `json:"status"`
}

// Validator is a Screwdriver Validator payload.
type Validator struct {
	Yaml string `json:"yaml"`
}

// Pipeline is a Screwdriver Pipeline definition
type Pipeline struct {
	ID     string `json:"id"`
	ScmURL string `json:"scmUrl"`
}

// PipelineDef contains the step definitions and jobs for a Pipeline
type PipelineDef struct {
	Jobs     map[string][]JobDef `json:"jobs"`
	Workflow []string            `json:"workflow"`
}

// JobDef contains the step and environment definitions of a single Job
type JobDef struct {
	Image       string            `json:"image"`
	Steps       map[string]string `json:"steps"`
	Environment map[string]string `json:"environment"`
}

// Job is a Screwdriver Job
type Job struct {
	ID         string `json:"id"`
	PipelineID string `json:"pipelineId"`
	Name       string `json:"name"`
}

// Build is a Screwdriver Build
type Build struct {
	ID    string `json:"id"`
	JobID string `json:"jobId"`
}

func (a api) makeURL(path string) (*url.URL, error) {
	version := "v3"
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

func (a api) get(url *url.URL) ([]byte, error) {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("generating request to Screwdriver: %v", err)
	}
	req.Header.Set("Authorization", tokenHeader(a.token))

	res, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading response from Screwdriver: %v", err)
	}
	defer res.Body.Close()

	return handleResponse(res)
}

func (a api) post(url *url.URL, bodyType string, payload io.Reader) ([]byte, error) {
	req, err := http.NewRequest("POST", url.String(), payload)
	if err != nil {
		return nil, fmt.Errorf("generating request to Screwdriver: %v", err)
	}
	req.Header.Set("Content-Type", bodyType)
	req.Header.Set("Authorization", tokenHeader(a.token))

	res, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("posting to Screwdriver endpoint %v: %v", url, err)
	}
	defer res.Body.Close()

	return handleResponse(res)
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
		return job, fmt.Errorf("generating Screwdriver url for Job %v: %v", jobID, err)
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

func (a api) UpdateBuildStatus(status string) error {
	u, err := a.makeURL("webhooks/build")
	if err != nil {
		return fmt.Errorf("creating url: %v", err)
	}

	bs := BuildStatus{
		Status: status,
	}
	payload, err := json.Marshal(bs)
	if err != nil {
		return fmt.Errorf("marshaling JSON for Build Status: %v", err)
	}

	_, err = a.post(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("posting to Build Status: %v", err)
	}

	return nil
}
