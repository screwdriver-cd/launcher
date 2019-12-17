package screwdriver

import (
	"fmt"
	"net/url"
)

type localApi struct {
	baseURL    string
	jobName    string
	localBuild Build
}

func NewLocal(url, jobName string, localBuild Build) (API, error) {
	newapi := localApi{
		url,
		jobName,
		localBuild,
	}
	return API(newapi), nil
}

func (a localApi) makeURL(path string) (*url.URL, error) {
	version := "v4"
	fullpath := fmt.Sprintf("%s/%s/%s", a.baseURL, version, path)
	return url.Parse(fullpath)
}

func (a localApi) BuildFromID(buildID int) (build Build, err error) {
	return a.localBuild, nil
}

func (a localApi) EventFromID(eventID int) (event Event, err error) {
	event = Event{
		0,
		make(map[string]interface{}),
		0,
	}

	return event, nil
}

func (a localApi) JobFromID(jobID int) (job Job, err error) {
	job = Job{
		0,
		0,
		a.jobName,
		0,
	}

	return job, nil
}

func (a localApi) PipelineFromID(pipelineID int) (pipeline Pipeline, err error) {
	pipeline = Pipeline{
		0,
		ScmRepo{"screwdriver-cd/screwdriver"},
		"github.com:123456:master",
	}

	return pipeline, nil
}

func (a localApi) UpdateBuildStatus(status BuildStatus, meta map[string]interface{}, buildID int) error {
	return nil
}

func (a localApi) UpdateStepStart(buildID int, stepName string) error {
	return nil
}

func (a localApi) UpdateStepStop(buildID int, stepName string, exitCode int) error {
	return nil
}

func (a localApi) SecretsForBuild(build Build) (Secrets, error) {
	secrets := make(Secrets, 0)

	return secrets, nil
}

func (a localApi) GetAPIURL() (string, error) {
	url, err := a.makeURL("")

	return url.String(), err
}

func (a localApi) GetCoverageInfo() (Coverage, error) {
	coverage := Coverage{}

	return coverage, nil
}

func (a localApi) GetBuildToken(buildID int, buildTimeoutMinutes int) (string, error) {
	return "", nil
}

func (a localApi) IsLocal() bool {
	return true
}
