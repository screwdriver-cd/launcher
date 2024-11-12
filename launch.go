package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"

	"github.com/peterbourgon/mergemap"
	"github.com/urfave/cli"
	"gopkg.in/fatih/color.v1"

	"github.com/screwdriver-cd/launcher/executor"
	"github.com/screwdriver-cd/launcher/screwdriver"
)

// These variables get set by the build script via the LDFLAGS
// Detail about these variables are here: https://goreleaser.com/#builds
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var deepMergeJSON = mergemap.Merge
var mkdirAll = os.MkdirAll
var stat = os.Stat
var open = os.Open
var executorRun = executor.Run
var TerminateSleep = executor.TerminateSleep
var writeFile = ioutil.WriteFile
var readFile = ioutil.ReadFile
var newEmitter = screwdriver.NewEmitter
var marshal = json.Marshal
var unmarshal = json.Unmarshal
var cyanSprint = color.New(color.FgCyan).Add(color.Underline).SprintFunc()
var blackSprintf = color.New(color.FgHiBlack).SprintfFunc()
var pushgatewayURLTimeout = 15
var buildCreateTime time.Time
var queueEnterTime time.Time

var emitter screwdriver.Emitter
var defaultEnv map[string]string

var cleanExit = func() {
	os.Exit(0)
}

var client *retryablehttp.Client

const DefaultTimeout = 90 // 90 minutes

type scmPath struct {
	Host    string
	Org     string
	Repo    string
	Branch  string
	RootDir string
}

func init() {
	client = retryablehttp.NewClient()
}

/*
has HTTP or HTTPS protocol
targetURL => URL
*/
func hasHTTPProtocol(targetURL *url.URL) bool {
	return targetURL.Scheme == "http" || targetURL.Scheme == "https"
}

/*
make pushgateway url
baseURL => base url for pushgateway
buildID => sd build id
*/
func makePushgatewayURL(baseURL string, buildID int) (string, error) {
	var pushgatewayURL string = baseURL
	u, err := url.Parse(pushgatewayURL)
	if err != nil {
		log.Printf("makePushgatewayURL: failed to parse url [%v], buildId:[%v], error:[%v]", pushgatewayURL, buildID, err)
		return "", err
	}
	if !hasHTTPProtocol(u) {
		u, _ = url.Parse("http://" + pushgatewayURL)
	}
	u.Path = path.Join(u.Path, "/metrics/job/containerd/instance/"+strconv.Itoa(buildID))

	return u.String(), nil
}

/*
push metrics to prometheus
metrics - sd_build_completed, sd_build_run_duration_secs
status => sd build status
buildID => sd build id
*/
func pushMetrics(status string, buildID int) error {
	// push metrics if pushgateway url is available
	log.Printf("push metrics for buildID:[%v], status:[%v]", buildID, status)
	if strings.TrimSpace(os.Getenv("SD_PUSHGATEWAY_URL")) != "" && strings.TrimSpace(os.Getenv("CONTAINER_IMAGE")) != "" && strings.TrimSpace(os.Getenv("SD_PIPELINE_ID")) != "" && buildID > 0 {
		timeout := time.Duration(pushgatewayURLTimeout) * time.Second
		client.HTTPClient.Timeout = timeout
		pushgatewayURL, err := makePushgatewayURL(os.Getenv("SD_PUSHGATEWAY_URL"), buildID)
		if err != nil {
			log.Printf("pushMetrics: failed to make pushgateway url, buildId:[%v], error:[%v]", buildID, err)
			return err
		}
		defer client.HTTPClient.CloseIdleConnections()
		image := os.Getenv("CONTAINER_IMAGE")
		pipelineId := os.Getenv("SD_PIPELINE_ID")
		node := os.Getenv("NODE_ID")
		jobId := os.Getenv("SD_JOB_ID")
		jobName := os.Getenv("SD_JOB_NAME")
		scmURL := os.Getenv("SCM_URL")
		sdBuildPrefix := os.Getenv("SD_BUILD_PREFIX")
		cpu := os.Getenv("CONTAINER_CPU_LIMIT")
		ram := os.Getenv("CONTAINER_MEMORY_LIMIT")
		launcherStartTS, _ := strconv.ParseInt(os.Getenv("SD_LAUNCHER_START_TS"), 10, 64)
		buildStartTS, _ := strconv.ParseInt(os.Getenv("SD_BUILD_START_TS"), 10, 64)
		// build run end timestamp
		ts := time.Now().Unix()
		buildCreateTS := buildCreateTime.Unix()
		// if not able to get build create time, substitute with launcher start ts
		if buildCreateTS < 0 {
			buildCreateTS = launcherStartTS
		}
		queueEnterTS := queueEnterTime.Unix()
		// if not able to get build queue enter time, substitute with build create ts
		if queueEnterTS < 0 {
			queueEnterTS = launcherStartTS
		}
		buildRunTimeSecs := ts - launcherStartTS            // build run time => build end time - launcher start time
		buildTimeSecs := ts - queueEnterTS                  // overall build time => build end time - build queue enter time
		buildQueuedTimeSecs := queueEnterTS - buildCreateTS // queued time => build queue enter time - build create time
		buildSetupTimeSecs := buildStartTS - queueEnterTS   // setup time => build start - queue enter time

		// data need to be specified in this format for pushgateway
		data := `sd_build_status{image_name="` + image + `",pipeline_id="` + pipelineId + `",node="` + node + `",job_id="` + jobId + `",job_name="` + jobName + `",scm_url="` + scmURL + `",status="` + status + `",prefix="` + sdBuildPrefix + `"} 1
sd_build_run_time_secs{image_name="` + image + `",pipeline_id="` + pipelineId + `",node="` + node + `",job_id="` + jobId + `",job_name="` + jobName + `",scm_url="` + scmURL + `",status="` + status + `",prefix="` + sdBuildPrefix + `",cpu="` + cpu + `",ram="` + ram + `"} ` + strconv.FormatInt(buildRunTimeSecs, 10) + `
sd_build_time_secs{image_name="` + image + `",pipeline_id="` + pipelineId + `",node="` + node + `",job_id="` + jobId + `",job_name="` + jobName + `",scm_url="` + scmURL + `",status="` + status + `",prefix="` + sdBuildPrefix + `"} ` + strconv.FormatInt(buildTimeSecs, 10) + `
sd_build_queued_time_secs{image_name="` + image + `",pipeline_id="` + pipelineId + `",node="` + node + `",job_id="` + jobId + `",job_name="` + jobName + `",scm_url="` + scmURL + `",status="` + status + `",prefix="` + sdBuildPrefix + `"} ` + strconv.FormatInt(buildQueuedTimeSecs, 10) + `
sd_build_setup_time_secs{image_name="` + image + `",pipeline_id="` + pipelineId + `",node="` + node + `",job_id="` + jobId + `",job_name="` + jobName + `",scm_url="` + scmURL + `",status="` + status + `",prefix="` + sdBuildPrefix + `"} ` + strconv.FormatInt(buildSetupTimeSecs, 10) + `
`
		body := strings.NewReader(data)
		log.Printf("pushMetrics: post metrics to [%v]", pushgatewayURL)
		res, err := client.HTTPClient.Post(pushgatewayURL, "", body)
		if res != nil {
			defer res.Body.Close()
		}
		if err != nil {
			log.Printf("pushMetrics: failed to push metrics to [%v], buildId:[%v], error:[%v]", pushgatewayURL, buildID, err)
			return err
		}
		if res.StatusCode/100 != 2 {
			msg := fmt.Sprintf("pushMetrics: failed to push metrics to [%v], buildId:[%v], response status code:[%v]", pushgatewayURL, buildID, res.StatusCode)
			log.Printf(msg)
			return errors.New(msg)
		}
		log.Printf("pushMetrics: successfully pushed metrics for build:[%v]", buildID)
	} else {
		log.Printf("pushMetrics: pushgatewayURL:[%v], buildID:[%v], image: [%v], pipelineId: [%v] is empty", os.Getenv("SD_PUSHGATEWAY_URL"), buildID, os.Getenv("CONTAINER_IMAGE"), os.Getenv("SD_PIPELINE_ID"))
	}
	return nil
}

// prepareExit sets the build status before exit
func prepareExit(status screwdriver.BuildStatus, buildID int, api screwdriver.API, metaSpace string, statusMessage string) {
	_ = pushMetrics(status.String(), buildID)
	if api != nil {
		var metaInterface map[string]interface{}

		log.Printf("Loading meta from %q/meta.json", metaSpace)
		metaJSON, err := readFile(metaSpace + "/meta.json")
		if err != nil {
			log.Printf("Failed to load %q/meta.json: %v", metaSpace, err)
			metaInterface = make(map[string]interface{})
		} else {
			err = unmarshal(metaJSON, &metaInterface)
			if err != nil {
				log.Printf("Failed to load %q/meta.json: %v", metaSpace, err)
				metaInterface = make(map[string]interface{})
			}
		}
		log.Printf("Setting build status to %s", status)
		if err := api.UpdateBuildStatus(status, metaInterface, buildID, statusMessage); err != nil {
			log.Printf("Failed updating the build status: %v", err)
		}
	}
}

// e.g. scmUri: "github:123456:master", scmName: "screwdriver-cd/launcher"
func parseScmURI(scmURI, scmName string) (scmPath, error) {
	uri := strings.Split(scmURI, ":")
	orgRepo := strings.Split(scmName, "/")

	if len(uri) < 3 || len(orgRepo) != 2 {
		return scmPath{}, fmt.Errorf("Unable to parse scmUri %v and scmName %v", scmURI, scmName)
	}

	parsed := scmPath{
		Host:    uri[0],
		Org:     orgRepo[0],
		Repo:    orgRepo[1],
		Branch:  uri[2],
		RootDir: "",
	}

	if len(uri) > 3 {
		parsed.RootDir = strings.Join(uri[3:], ":")
	}

	return parsed, nil
}

// A Workspace is a description of the paths available to a Screwdriver build
type Workspace struct {
	Root      string
	Src       string
	Artifacts string
}

// createWorkspace makes a Scrwedriver workspace from path components
// e.g. ["github.com", "screwdriver-cd" "screwdriver"] creates
// /sd/workspace/src/github.com/screwdriver-cd/screwdriver
// /sd/workspace/artifacts
func createWorkspace(isLocal bool, rootDir string, srcPaths ...string) (Workspace, error) {
	srcPaths = append([]string{"src"}, srcPaths...)
	src := path.Join(srcPaths...)

	src = path.Join(rootDir, src)
	artifacts := path.Join(rootDir, "artifacts")

	paths := []string{
		src,
		artifacts,
	}
	for _, p := range paths {
		_, err := stat(p)
		if err == nil && !isLocal {
			msg := "Cannot create workspace path %q, path already exists."
			return Workspace{}, fmt.Errorf(msg, p)
		}
		err = mkdirAll(p, 0777)
		if err != nil {
			return Workspace{}, fmt.Errorf("Cannot create workspace path %q: %v", p, err)
		}
	}

	w := Workspace{
		Root:      rootDir,
		Src:       src,
		Artifacts: artifacts,
	}
	return w, nil
}

func createMetaSpace(metaSpace string) error {
	log.Printf("Creating Meta Space in %v", metaSpace)
	err := mkdirAll(metaSpace, 0777)
	if err != nil {
		return fmt.Errorf("Cannot create meta-space path %q: %v", metaSpace, err)
	}
	return nil
}

func writeMetafile(metaSpace, metaFile, metaLog string, mergedMeta map[string]interface{}) error {
	metaByte := []byte("")
	log.Println("Marshalling Merged Meta JSON in writeMetafile")
	metaByte, err := marshal(mergedMeta)

	if err != nil {
		return fmt.Errorf("Parsing Meta JSON: %v", err)
	}

	err = writeFile(metaSpace+"/"+metaFile, metaByte, 0666)
	if err != nil {
		return fmt.Errorf("Writing Parent %v Meta JSON: %v", metaLog, err)
	}
	return nil
}

// SetExternalMeta checks if parent build is external and sets meta in external file accordingly
func SetExternalMeta(api screwdriver.API, pipelineID, parentBuildID int, mergedMeta map[string]interface{}, metaSpace, metaLog string, join bool) (map[string]interface{}, error) {
	var resultMeta = mergedMeta
	log.Printf("Fetching Parent Build %d", parentBuildID)
	parentBuild, err := api.BuildFromID(parentBuildID)
	if err != nil {
		return resultMeta, fmt.Errorf("Fetching Parent Build ID %d: %v", parentBuildID, err)
	}

	log.Printf("Fetching Parent Job %d", parentBuild.JobID)
	parentJob, err := api.JobFromID(parentBuild.JobID)
	if err != nil {
		return resultMeta, fmt.Errorf("Fetching Job ID %d: %v", parentBuild.JobID, err)
	}

	if parentBuild.Meta != nil {
		// Check if build is from external pipeline
		if pipelineID != parentJob.PipelineID {
			// Write to "sd@123:component.json", where sd@123:component is the triggering job
			externalMetaFile := "sd@" + strconv.Itoa(parentJob.PipelineID) + ":" + parentJob.Name + ".json"
			writeMetafile(metaSpace, externalMetaFile, metaLog, parentBuild.Meta)
			if join {
				marshallValue, err := json.Marshal(parentBuild.Meta)
				if err != nil {
					return resultMeta, fmt.Errorf("Cloning meta of Parent Build ID %d: %v", parentBuildID, err)
				}
				var externalParentBuildMeta map[string]interface{}
				json.Unmarshal(marshallValue, &externalParentBuildMeta)

				// Always exclude parameters from external meta
				delete(externalParentBuildMeta, "parameters")

				resultMeta = deepMergeJSON(resultMeta, externalParentBuildMeta)
			}

			// delete local version of external meta
			pIDString := strconv.Itoa(parentJob.PipelineID)
			pjn := parentJob.Name
			if sdMeta, ok := resultMeta["sd"]; ok {
				if externalPipelineMeta, ok := sdMeta.(map[string]interface{})[pIDString]; ok {
					if _, ok := externalPipelineMeta.(map[string]interface{})[pjn]; ok {
						delete(externalPipelineMeta.(map[string]interface{}), pjn)
					}
				}
			}
		} else {
			resultMeta = deepMergeJSON(resultMeta, parentBuild.Meta)
		}
	}

	return resultMeta, nil
}

func writeArtifact(aDir string, fName string, artifact interface{}) error {
	data, err := json.MarshalIndent(artifact, "", strings.Repeat(" ", 4))
	if err != nil {
		return fmt.Errorf("Marshaling artifact: %v ", err)
	}

	pathToCreate := path.Join(aDir, fName)
	err = writeFile(pathToCreate, data, 0644)
	if err != nil {
		return fmt.Errorf("Creating file %q : %v", pathToCreate, err)
	}

	return nil
}

// prNumber checks to see if the job name is a pull request and returns its number
func prNumber(jobName string) string {
	r := regexp.MustCompile("^PR-([0-9]+)(?::[\\w-]+)?$")
	matched := r.FindStringSubmatch(jobName)
	if matched == nil || len(matched) != 2 {
		return ""
	}
	log.Println("Build is a PR: ", matched[1])
	return matched[1]
}

// convertToArray will convert the interface to an array of ints
func convertToArray(i interface{}) (array []int) {
	switch v := i.(type) {
	case float64:
		var arr = make([]int, 1)
		arr[0] = int(i.(float64))
		return arr
	case []interface{}:
		var arr = make([]int, len(v))
		for i, e := range v {
			arr[i] = int(e.(float64))
		}
		return arr
	default:
		var arr = make([]int, 0)
		return arr
	}
}

func launch(api screwdriver.API, buildID int, rootDir, emitterPath, metaSpace, storeURL, uiURL, shellBin string, buildTimeout int, buildToken, cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir string, cacheCompress, cacheMd5Check, isLocal bool, cacheMaxSizeInMB int64, cacheMaxGoThreads int64) (error, string, string) {
	var err error
	emitter, err = newEmitter(emitterPath)
	envFilepath := "/tmp/env"
	if err != nil {
		return err, "", ""
	}
	defer emitter.Close()

	if err = api.UpdateStepStart(buildID, "sd-setup-launcher"); err != nil {
		return fmt.Errorf("Updating sd-setup-launcher start: %v", err), "", ""
	}

	log.Print("Setting Build Status to RUNNING")
	emptyMeta := make(map[string]interface{}) // {"meta":null} are not accepted. This will be {"meta":{}}
	if err = api.UpdateBuildStatus(screwdriver.Running, emptyMeta, buildID, ""); err != nil {
		return fmt.Errorf("Updating build status to RUNNING: %v", err), "", ""
	}

	log.Printf("Fetching Build %d", buildID)
	build, err := api.BuildFromID(buildID)
	if err != nil {
		return fmt.Errorf("Fetching Build ID %d: %v", buildID, err), "", ""
	}

	buildCreateTime, _ = time.Parse(time.RFC3339, build.Createtime)
	queueEnterTime, _ = time.Parse(time.RFC3339, build.Stats.QueueEntertime)

	log.Printf("Fetching Job %d", build.JobID)
	job, err := api.JobFromID(build.JobID)
	if err != nil {
		return fmt.Errorf("Fetching Job ID %d: %v", build.JobID, err), "", ""
	}

	log.Printf("Fetching Pipeline %d", job.PipelineID)
	pipeline, err := api.PipelineFromID(job.PipelineID)
	if err != nil {
		return fmt.Errorf("Fetching Pipeline ID %d: %v", job.PipelineID, err), "", ""
	}

	log.Printf("Fetching Event %d", build.EventID)
	event, err := api.EventFromID(build.EventID)
	if err != nil {
		return fmt.Errorf("Fetching Event ID %d: %v", build.EventID, err), "", ""
	}

	metaByte := []byte("")
	metaFile := "meta.json" // Write to "meta.json" file
	metaLog := ""

	oldJobName := job.Name
	pr := prNumber(job.Name)

	scm, err := parseScmURI(pipeline.ScmURI, pipeline.ScmRepo.Name)
	if err != nil {
		return err, "", ""
	}

	pipelineName := pipeline.ScmRepo.Name
	if scm.RootDir != "" {
		pipelineName += ":" + scm.RootDir
	}

	coverageScope := ""
	if len(job.Permutations) > 0 {
		coverageScope = job.Permutations[0].Annotations.CoverageScope
	}
	coverageInfo, coverageErr := api.GetCoverageInfo(job.ID, job.PipelineID, job.Name, pipelineName, coverageScope, pr, strconv.Itoa(job.PrParentJobID))

	parentBuildIDs := convertToArray(build.ParentBuildID)

	mergedMeta := map[string]interface{}{}

	if build.Meta != nil {
		mergedMeta = deepMergeJSON(mergedMeta, build.Meta)
	}

	// Create meta space
	err = createMetaSpace(metaSpace)
	if err != nil {
		return err, "", ""
	}

	// Always merge parent event meta if available
	if event.ParentEventID != 0 {
		log.Printf("Fetching Parent Event %d", event.ParentEventID)
		parentEvent, err := api.EventFromID(event.ParentEventID)
		if err != nil {
			return fmt.Errorf("Fetching Parent Event ID %d: %v", event.ParentEventID, err), "", ""
		}

		if parentEvent.Meta != nil {
			mergedMeta = deepMergeJSON(mergedMeta, parentEvent.Meta)
		}

		metaLog = fmt.Sprintf(`Event(%v)`, parentEvent.ID)
	}

	// merge event meta if available
	// the parent event's metadata should be overwritten if a conflict occurs
	// meaning if the event has been updated with newer metadata from its associated builds.
	if len(event.Meta) > 0 {
		log.Printf("Fetching Event Meta JSON %v", event.ID)
		if event.Meta != nil {
			mergedMeta = deepMergeJSON(mergedMeta, event.Meta)
		}
	}

	if len(parentBuildIDs) > 1 { // If has multiple parent build IDs, merge their metadata (join case)
		// Get meta from all parent builds
		for _, pbID := range parentBuildIDs {
			mergedMeta, err = SetExternalMeta(api, pipeline.ID, pbID, mergedMeta, metaSpace, metaLog, true)
			if err != nil {
				return fmt.Errorf("Setting meta for Parent Build ID %d: %v", pbID, err), "", ""
			}
		}

		metaLog = fmt.Sprintf(`Builds(%v)`, parentBuildIDs)
	} else if len(parentBuildIDs) == 1 { // If has parent build, fetch from parent build
		mergedMeta, err = SetExternalMeta(api, pipeline.ID, parentBuildIDs[0], mergedMeta, metaSpace, metaLog, false)
		if err != nil {
			return fmt.Errorf("Setting meta for Parent Build ID %d: %v", parentBuildIDs[0], err), "", ""
		}

		metaLog = fmt.Sprintf(`Build(%v)`, parentBuildIDs[0])
	}

	// Initialize pr comments (Issue #1858)
	if metadata, ok := mergedMeta["meta"]; ok {
		delete(metadata.(map[string]interface{}), "summary")
	}

	// Set build parameter explicitly (Issue #2501)
	if build.Meta["parameters"] != nil {
		mergedMeta["parameters"] = build.Meta["parameters"]
	}

	buildMeta := map[string]interface{}{
		"pipelineId": strconv.Itoa(job.PipelineID),
		"eventId":    strconv.Itoa(build.EventID),
		"jobId":      strconv.Itoa(job.ID),
		"buildId":    strconv.Itoa(buildID),
		"jobName":    job.Name,
		"sha":        build.SHA,
	}

	if coverageErr == nil {
		buildMeta["coverageKey"] = coverageInfo.EnvVars["SD_SONAR_PROJECT_KEY"]
	}

	mergedBuildMeta := buildMeta
	if mergedMeta["build"] != nil {
		mergedBuildMeta = deepMergeJSON(mergedMeta["build"].(map[string]interface{}), buildMeta)
	}
	delete(mergedBuildMeta, "warning")
	mergedMeta["build"] = mergedBuildMeta

	eventMeta := map[string]interface{}{
		"creator": event.Creator["username"],
	}

	if mergedMeta["event"] != nil {
		mergedMeta["event"] = deepMergeJSON(mergedMeta["event"].(map[string]interface{}), eventMeta)
	} else {
		mergedMeta["event"] = eventMeta
	}

	log.Println("Marshalling Merged Meta JSON")
	metaByte, err = marshal(mergedMeta)

	if err != nil {
		return fmt.Errorf("Parsing Meta JSON: %v", err), "", ""
	}

	err = writeFile(metaSpace+"/"+metaFile, metaByte, 0666)
	if err != nil {
		return fmt.Errorf("Writing Parent %v Meta JSON: %v", metaLog, err), "", ""
	}

	log.Printf("Creating Workspace in %v", rootDir)
	w, err := createWorkspace(isLocal, rootDir, scm.Host, scm.Org, scm.Repo)
	if err != nil {
		return err, "", ""
	}
	sourceDir := w.Src
	if scm.RootDir != "" {
		sourceDir = sourceDir + "/" + scm.RootDir
	}

	infoMessages := []string{
		cyanSprint("Screwdriver Launcher information"),
		blackSprintf("Version:        v%s", version),
		blackSprintf("Pipeline:       #%d", job.PipelineID),
		blackSprintf("Job:            %s", job.Name),
		blackSprintf("Build:          #%d", buildID),
		blackSprintf("Workspace Dir:  %s", w.Root),
		blackSprintf("Checkout Dir:   %s", w.Src),
		blackSprintf("Source Dir:     %s", sourceDir),
		blackSprintf("Artifacts Dir:  %s", w.Artifacts),
	}

	for _, v := range infoMessages {
		if isLocal {
			log.Print(v)
		} else {
			fmt.Fprintf(emitter, "%s\n", v)
		}
	}

	if pr != "" {
		job.Name = "main"
	}

	err = writeArtifact(w.Artifacts, "steps.json", build.Commands)
	if err != nil {
		return fmt.Errorf("Creating steps.json artifact: %v", err), "", ""
	}

	err = writeArtifact(w.Artifacts, "environment.json", build.Environment)
	if err != nil {
		return fmt.Errorf("Creating environment.json artifact: %v", err), "", ""
	}

	apiURL, _ := api.GetAPIURL()

	isCI := strconv.FormatBool(!isLocal)

	isScheduler := strconv.FormatBool(event.Creator["username"] == "sd:scheduler")

	defaultEnv = map[string]string{
		"PS1":                     "",
		"SCREWDRIVER":             isCI,
		"CI":                      isCI,
		"GIT_PAGER":               "cat",  // https://github.com/screwdriver-cd/screwdriver/issues/1583#issuecomment-539677403
		"GIT_ASKPASS":             "echo", // https://github.com/screwdriver-cd/screwdriver/issues/1583#issuecomment-2451234577
		"CONTINUOUS_INTEGRATION":  isCI,
		"SD_JOB_NAME":             oldJobName,
		"SD_PIPELINE_NAME":        pipeline.ScmRepo.Name,
		"SD_BUILD_ID":             strconv.Itoa(buildID),
		"SD_JOB_ID":               strconv.Itoa(job.ID),
		"SD_EVENT_ID":             strconv.Itoa(build.EventID),
		"SD_PIPELINE_ID":          strconv.Itoa(job.PipelineID),
		"SD_PARENT_BUILD_ID":      fmt.Sprintf("%v", parentBuildIDs),
		"SD_PR_PARENT_JOB_ID":     strconv.Itoa(job.PrParentJobID),
		"SD_PARENT_EVENT_ID":      strconv.Itoa(event.ParentEventID),
		"SD_SOURCE_DIR":           sourceDir,
		"SD_CHECKOUT_DIR":         w.Src,
		"SD_ROOT_DIR":             w.Root,
		"SD_ARTIFACTS_DIR":        w.Artifacts,
		"SD_META_DIR":             metaSpace,
		"SD_META_PATH":            metaSpace + "/meta.json",
		"SD_BUILD_SHA":            build.SHA,
		"SD_PULL_REQUEST":         pr,
		"SD_API_URL":              apiURL,
		"SD_BUILD_URL":            apiURL + "builds/" + strconv.Itoa(buildID),
		"SD_STORE_URL":            fmt.Sprintf("%s/%s/", storeURL, "v1"),
		"SD_UI_URL":               fmt.Sprintf("%s/", uiURL),
		"SD_UI_BUILD_URL":         fmt.Sprintf("%s/pipelines/%s/builds/%s", uiURL, strconv.Itoa(job.PipelineID), strconv.Itoa(buildID)),
		"SD_BUILD_CLUSTER_NAME":   build.BuildClusterName,
		"SD_TOKEN":                buildToken,
		"SD_CACHE_STRATEGY":       cacheStrategy,
		"SD_PIPELINE_CACHE_DIR":   pipelineCacheDir,
		"SD_JOB_CACHE_DIR":        jobCacheDir,
		"SD_EVENT_CACHE_DIR":      eventCacheDir,
		"SD_CACHE_COMPRESS":       fmt.Sprintf("%v", cacheCompress),
		"SD_CACHE_MD5CHECK":       fmt.Sprintf("%v", cacheMd5Check),
		"SD_CACHE_MAX_SIZE_MB":    fmt.Sprintf("%v", cacheMaxSizeInMB),
		"SD_CACHE_MAX_GO_THREADS": fmt.Sprintf("%v", cacheMaxGoThreads),
		"SD_SCHEDULED_BUILD":      isScheduler,
		"SD_PRIVATE_PIPELINE":     strconv.FormatBool(pipeline.ScmRepo.Private),
	}

	// Add coverage env vars
	if coverageErr != nil {
		log.Printf("Failed to get coverage info for build %v so skip it: %v\n", build.ID, err)
	} else {
		for key, value := range coverageInfo.EnvVars {
			defaultEnv[key] = fmt.Sprintf("%v", value)
		}
	}

	// Get secrets for build
	secrets, err := api.SecretsForBuild(build)
	if err != nil {
		return fmt.Errorf("Fetching secrets for build %v", build.ID), "", ""
	}

	env, userShellBin := createEnvironment(defaultEnv, secrets, build)
	if userShellBin != "" {
		shellBin = userShellBin
	}

	return executorRun(w.Src, env, emitter, build, api, buildID, shellBin, buildTimeout, envFilepath, sourceDir), sourceDir, shellBin
}

func createEnvironment(base map[string]string, secrets screwdriver.Secrets, build screwdriver.Build) ([]string, string) {
	var userShellBin string

	// Add the default environment values
	for k, v := range base {
		os.Setenv(k, os.ExpandEnv(v))
	}

	for _, s := range secrets {
		os.Setenv(s.Name, s.Value)
	}

	for _, env := range build.Environment {
		for k, v := range env {
			os.Setenv(k, os.ExpandEnv(v))

			if k == "USER_SHELL_BIN" {
				userShellBin = v
			}
		}
	}

	envMap := map[string]string{}

	// Go through environment and make an environment variables map
	for _, e := range os.Environ() {
		pieces := strings.SplitAfterN(e, "=", 2)
		if len(pieces) != 2 {
			log.Printf("WARN: bad environment value from base environment: %s", e)
			continue
		}

		k := pieces[0][:len(pieces[0])-1] // Drop the "=" off the end
		v := pieces[1]

		envMap[k] = v
	}

	env := []string{}
	for k, v := range envMap {
		env = append(env, strings.Join([]string{k, v}, "="))
	}

	return env, userShellBin
}

// Executes the command based on arguments from the CLI
func launchAction(api screwdriver.API, buildID int, rootDir, emitterPath, metaSpace, storeURI, uiURI, shellBin string, buildTimeout int, buildToken, cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir string, cacheCompress, cacheMd5Check, isLocal bool, cacheMaxSizeInMB int64, cacheMaxGoThreads int64) error {
	log.Printf("Starting Build %v\n", buildID)
	log.Printf("Cache strategy & directories (pipeline, job, event), compress, md5check, maxsize: %v, %v, %v, %v, %v, %v, %v \n", cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir, cacheCompress, cacheMd5Check, cacheMaxSizeInMB)

	err, sourceDir, launchShellBin := launch(api, buildID, rootDir, emitterPath, metaSpace, storeURI, uiURI, shellBin, buildTimeout, buildToken, cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir, cacheCompress, cacheMd5Check, isLocal, cacheMaxSizeInMB, cacheMaxGoThreads)
	if err != nil {
		if _, ok := err.(executor.ErrStatus); ok {
			log.Printf("Failure due to non-zero exit code: %v\n", err)
		} else {
			log.Printf("Error running launcher: %v\n", err)
		}

		prepareExit(screwdriver.Failure, buildID, api, metaSpace, "")
		TerminateSleep(launchShellBin, sourceDir, true)
		cleanExit()
		return nil
	}

	prepareExit(screwdriver.Success, buildID, api, metaSpace, "")
	cleanExit()

	return nil
}

func recoverPanic(buildID int, api screwdriver.API, metaSpace string) {
	if p := recover(); p != nil {
		filename := fmt.Sprintf("launcher-stacktrace-%s", time.Now().Format(time.RFC3339))
		tracefile := filepath.Join(os.TempDir(), filename)

		log.Printf("ERROR: Internal Screwdriver error. Please file a bug about this: %v", p)
		log.Printf("ERROR: Writing StackTrace to %s", tracefile)
		err := ioutil.WriteFile(tracefile, debug.Stack(), 0600)
		if err != nil {
			log.Printf("ERROR: Unable to write stacktrace to file: %v", err)
		}

		prepareExit(screwdriver.Failure, buildID, api, metaSpace, "")
		cleanExit()
	}
}

// finalRecover makes one last attempt to recover from a panic.
// This should only happen if the previous recovery caused a panic.
func finalRecover() {
	if p := recover(); p != nil {
		fmt.Fprintln(os.Stderr, "ERROR: Something terrible has happened. Please file a ticket with this info:")
		fmt.Fprintf(os.Stderr, "ERROR: %v\n%v\n", p, debug.Stack())
	}
	cleanExit()
}

func main() {
	defer finalRecover()
	defer recoverPanic(0, nil, "")

	app := cli.NewApp()
	app.Name = "launcher"
	app.Usage = "launch a Screwdriver build"
	app.UsageText = "launch [options] build-id"
	app.Version = fmt.Sprintf("%v, commit %v, built at %v", version, commit, date)

	if date != "unknown" {
		// date is passed in from GoReleaser which uses RFC3339 format
		t, _ := time.Parse(time.RFC3339, date)
		date = t.Format("2006")
	}
	app.Copyright = "(c) 2016-" + date + " Yahoo Inc."

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "api-uri",
			Usage: "API URI for Screwdriver",
			Value: "http://localhost:8080",
		},
		cli.StringFlag{
			Name:   "token",
			Usage:  "JWT used for accessing Screwdriver's API",
			EnvVar: "SD_TOKEN",
		},
		cli.StringFlag{
			Name:  "workspace",
			Usage: "Location for checking out and running code",
			Value: "/sd/workspace",
		},
		cli.StringFlag{
			Name:  "emitter",
			Usage: "Location for writing log lines to",
			Value: "/var/run/sd/emitter",
		},
		cli.StringFlag{
			Name:  "meta-space",
			Usage: "Location of meta temporarily",
			Value: "/sd/meta",
		},
		cli.StringFlag{
			Name:  "store-uri",
			Usage: "API URI for Store",
			Value: "http://localhost:8081",
		},
		cli.StringFlag{
			Name:  "ui-uri",
			Usage: "UI URI for Screwdriver",
			Value: "http://localhost:4200",
		},
		cli.StringFlag{
			Name:   "shell-bin",
			Usage:  "Shell to use when executing commands",
			Value:  "/bin/sh",
			EnvVar: "SD_SHELL_BIN",
		},
		cli.IntFlag{
			Name:   "build-timeout",
			Usage:  "Maximum number of minutes to allow a build to run",
			Value:  DefaultTimeout,
			EnvVar: "SD_BUILD_TIMEOUT",
		},
		cli.BoolFlag{
			Name:  "only-fetch-token",
			Usage: "Only fetching build token",
		},
		cli.StringFlag{
			Name:  "cache-strategy",
			Usage: "Cache strategy",
		},
		cli.StringFlag{
			Name:  "pipeline-cache-dir",
			Usage: "Pipeline cache directory",
		},
		cli.StringFlag{
			Name:  "job-cache-dir",
			Usage: "Job cache directory",
		},
		cli.StringFlag{
			Name:  "event-cache-dir",
			Usage: "Event cache directory",
		},
		cli.StringFlag{
			Name:  "cache-compress",
			Usage: "To compress and store cache",
		},
		cli.StringFlag{
			Name:  "cache-md5check",
			Usage: "Do md5 check",
		},
		cli.StringFlag{
			Name:  "cache-max-size-mb",
			Usage: "Cache allowed max size in mb",
			Value: "0",
		},
		cli.StringFlag{
			Name:  "cache-max-go-threads",
			Usage: "Cache allowed max go threads",
			Value: "10000",
		},
		cli.BoolFlag{
			Name:  "local-mode",
			Usage: "Enable local mode",
		},
		cli.StringFlag{
			//Pass JSON in the same format as payload from /v4/builds/{id}
			Name:  "local-build-json",
			Usage: "Build information for local mode",
		},
		cli.StringFlag{
			Name:  "local-job-name",
			Usage: "Job name for local mode",
		},
		cli.BoolFlag{
			Name:  "container-error",
			Usage: "container error",
		},
	}

	app.Action = func(c *cli.Context) error {
		apiURL := c.String("api-uri")
		token := c.String("token")
		workspace := c.String("workspace")
		emitterPath := c.String("emitter")
		metaSpace := c.String("meta-space")
		storeURL := c.String("store-uri")
		uiURL := c.String("ui-uri")
		shellBin := c.String("shell-bin")
		buildID, err := strconv.Atoi(c.Args().Get(0))
		buildTimeoutSeconds := c.Int("build-timeout") * 60
		fetchFlag := c.Bool("only-fetch-token")
		cacheStrategy := c.String("cache-strategy")
		pipelineCacheDir := c.String("pipeline-cache-dir")
		jobCacheDir := c.String("job-cache-dir")
		eventCacheDir := c.String("event-cache-dir")
		cacheCompress, _ := strconv.ParseBool(c.String("cache-compress"))
		cacheMd5Check, _ := strconv.ParseBool(c.String("cache-md5check"))
		cacheMaxSizeInMB := c.Int64("cache-max-size-mb")
		cacheMaxGoThreads := c.Int64("cache-max-go-threads")
		isLocal := c.Bool("local-mode")
		localBuildJson := c.String("local-build-json")
		localJobName := c.String("local-job-name")
		containerError := c.Bool("container-error")

		if err != nil {
			return cli.ShowAppHelp(c)
		}

		log.Printf("cache strategy, directories (pipeline, job, event), compress, md5check, maxsize: %v, %v, %v, %v, %v, %v, %v \n", cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir, cacheCompress, cacheMd5Check, cacheMaxSizeInMB)

		if !isLocal && len(token) == 0 {
			log.Println("Error: token is not passed.")
			cleanExit()
		}

		if containerError {
			temporalAPI, err := screwdriver.New(apiURL, token)
			if err != nil {
				log.Printf("Error creating temporal Screwdriver API %v: %v", buildID, err)
				prepareExit(screwdriver.Failure, buildID, nil, metaSpace, "")
				cleanExit()
			}
			prepareExit(screwdriver.Failure, buildID, temporalAPI, metaSpace, "Error: Build failed to start. Please check if your image is valid with curl, openssh installed and default user root or sudo NOPASSWD enabled.")
			cleanExit()
		}

		if fetchFlag {
			temporalAPI, err := screwdriver.New(apiURL, token)
			if err != nil {
				log.Printf("Error creating temporal Screwdriver API %v: %v", buildID, err)
				prepareExit(screwdriver.Failure, buildID, nil, metaSpace, "")
				cleanExit()
			}

			buildToken, err := temporalAPI.GetBuildToken(buildID, c.Int("build-timeout"))
			if err != nil {
				log.Printf("Error getting Build Token %v: %v", buildID, err)
				prepareExit(screwdriver.Failure, buildID, nil, metaSpace, "")
				cleanExit()
			}

			log.Printf("Launcher process only fetch token.")
			fmt.Printf("%s", buildToken)
			cleanExit()
		}
		var api screwdriver.API
		if isLocal {
			if len(localBuildJson) == 0 {
				log.Println("Error: local-build-json is not passed.")
				cleanExit()
			}

			var localBuild screwdriver.Build
			err := json.Unmarshal([]byte(localBuildJson), &localBuild)
			if err != nil {
				log.Printf("Failed to parse localBuildJson: %v", err)
				cleanExit()
			}

			api, err = screwdriver.NewLocal(apiURL, localJobName, localBuild)
		} else {
			api, err = screwdriver.New(apiURL, token)
		}

		if err != nil {
			log.Printf("Error creating Screwdriver API %v: %v", buildID, err)
			prepareExit(screwdriver.Failure, buildID, nil, metaSpace, "")
			cleanExit()
		}

		defer recoverPanic(buildID, api, metaSpace)

		launchAction(api, buildID, workspace, emitterPath, metaSpace, storeURL, uiURL, shellBin, buildTimeoutSeconds, token, cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir, cacheCompress, cacheMd5Check, isLocal, cacheMaxSizeInMB, cacheMaxGoThreads)

		// This should never happen...
		log.Println("Unexpected return in launcher. Failing the build.")
		prepareExit(screwdriver.Failure, buildID, api, metaSpace, "")
		cleanExit()
		return nil
	}
	app.Run(os.Args)
}
