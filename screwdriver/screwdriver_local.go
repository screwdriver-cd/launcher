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
		ID:            0,
		Meta:          make(map[string]interface{}),
		ParentEventID: 0,
		Creator:       make(map[string]string),
	}

	return event, nil
}

func (a localApi) JobFromID(jobID int) (job Job, err error) {
	job = Job{
		ID:            0,
		PipelineID:    0,
		Name:          a.jobName,
		PrParentJobID: 0,
		Permutations:  []JobPermutation{},
	}

	return job, nil
}

func (a localApi) PipelineFromID(pipelineID int) (pipeline Pipeline, err error) {
	pipeline = Pipeline{
		ID:      0,
		ScmRepo: ScmRepo{"sd-local/local-build"},
		ScmURI:  "screwdriver.cd:123456:master",
	}

	return pipeline, nil
}

func (a localApi) UpdateBuildStatus(status BuildStatus, meta map[string]interface{}, buildID int, statusMessag string) error {
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

func (a localApi) GetCoverageInfo(jobID, pipelineID int, jobName, pipelineName, scope, prNum, prParentJobId string) (Coverage, error) {
	coverage := Coverage{}

	return coverage, nil
}

func (a localApi) GetBuildToken(buildID int, buildTimeoutMinutes int) (string, error) {
	return "", nil
}

func (a localApi) LatestBuildFromJob(jobID int) (build Build, err error) {
	return a.localBuild, nil
}

func (a localApi) GetLatestBuildForMeta(pipelineID, parentBuildID int) (build Build, err error) {
	return a.localBuild, nil
}
