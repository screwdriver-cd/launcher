package screwdriver

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

// API is a Screwdriver API endpoint
type API interface {
	BuildFromID(buildID string) (Build, error)
	JobFromID(jobID string) (Job, error)
	PipelineFromID(pipelineID string) (Pipeline, error)
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

// Pipeline is a Screwdriver Pipeline definition
type Pipeline struct {
	ID     string `json:"id"`
	ScmURL string `json:"scmUrl"`
}

// Job is a Screwdriver Job
type Job struct {
	ID         string `json:"id"`
	PipelineID string `json:"pipelineId"`
}

// Build is a Screwdriver Build
type Build struct {
	ID    string `json:"id"`
	JobID string `json:"jobId"`
}

func (a api) makeURL(path string) (*url.URL, error) {
	fullpath := fmt.Sprintf("%s/%s", a.baseURL, path)
	return url.Parse(fullpath)
}

func (a api) get(url *url.URL) ([]byte, error) {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("Generating request to Screwdriver: %v", err)
	}
	token := fmt.Sprintf("Bearer %s", a.token)
	req.Header.Set("Authorization", token)

	response, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Reading response from Screwdriver: %v", err)
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("Reading response Body from Screwdriver: %v", err)
	}

	if response.StatusCode/100 != 2 {
		var err SDError
		parserr := json.Unmarshal(body, &err)
		if parserr != nil {
			return nil, fmt.Errorf("Unparseable error response from Screwdriver: %v", parserr)
		}
		return nil, err
	}

	return body, nil
}

// BuildFromID fetches and returns a Build object from its ID
func (a api) BuildFromID(buildID string) (build Build, err error) {
	u, err := a.makeURL(fmt.Sprintf("builds/%s", buildID))
	fmt.Println(u)
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
