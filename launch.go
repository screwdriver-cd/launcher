package main

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

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
var writeFile = ioutil.WriteFile
var readFile = ioutil.ReadFile
var newEmitter = screwdriver.NewEmitter
var marshal = json.Marshal
var unmarshal = json.Unmarshal
var cyanFprintf = color.New(color.FgCyan).Add(color.Underline).FprintfFunc()
var blackSprint = color.New(color.FgHiBlack).SprintFunc()
var pushgatewayUrlTimeout = 15
var buildCreateTime time.Time
var queueEnterTime time.Time

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

/* push metrics to prometheus
metrics - sd_build_completed, sd_build_run_duration_secs
status => sd build status
buildID => sd build id
*/
func pushMetrics(status string, buildID int) error {
	// push metrics if pushgateway url is available
	log.Printf("push metrics for buildID:[%v], status:[%v]", buildID, status)
	if strings.TrimSpace(os.Getenv("SD_PUSHGATEWAY_URL")) != "" && strings.TrimSpace(os.Getenv("CONTAINER_IMAGE")) != "" && strings.TrimSpace(os.Getenv("SD_PIPELINE_ID")) != "" && buildID > 0 {
		timeout := time.Duration(pushgatewayUrlTimeout) * time.Second
		client.HTTPClient.Timeout = timeout
		url := "http://" + os.Getenv("SD_PUSHGATEWAY_URL") + "/metrics/job/containerd/instance/" + strconv.Itoa(buildID)
		defer client.HTTPClient.CloseIdleConnections()
		image := os.Getenv("CONTAINER_IMAGE")
		pipelineId := os.Getenv("SD_PIPELINE_ID")
		node := os.Getenv("NODE_ID")
		jobId := os.Getenv("SD_JOB_ID")
		jobName := os.Getenv("SD_JOB_NAME")
		scmUrl := os.Getenv("SCM_URL")
		sdBuildPrefix := os.Getenv("SD_BUILD_PREFIX")
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
		data := `sd_build_status{image_name="` + image + `",pipeline_id="` + pipelineId + `",node="` + node + `",job_id="` + jobId + `",job_name="` + jobName + `",scm_url="` + scmUrl + `",status="` + status + `",prefix="` + sdBuildPrefix + `"} 1
sd_build_run_time_secs{image_name="` + image + `",pipeline_id="` + pipelineId + `",node="` + node + `",job_id="` + jobId + `",job_name="` + jobName + `",scm_url="` + scmUrl + `",status="` + status + `",prefix="` + sdBuildPrefix + `"} ` + strconv.FormatInt(buildRunTimeSecs, 10) + `
sd_build_time_secs{image_name="` + image + `",pipeline_id="` + pipelineId + `",node="` + node + `",job_id="` + jobId + `",job_name="` + jobName + `",scm_url="` + scmUrl + `",status="` + status + `",prefix="` + sdBuildPrefix + `"} ` + strconv.FormatInt(buildTimeSecs, 10) + `
sd_build_queued_time_secs{image_name="` + image + `",pipeline_id="` + pipelineId + `",node="` + node + `",job_id="` + jobId + `",job_name="` + jobName + `",scm_url="` + scmUrl + `",status="` + status + `",prefix="` + sdBuildPrefix + `"} ` + strconv.FormatInt(buildQueuedTimeSecs, 10) + `
sd_build_setup_time_secs{image_name="` + image + `",pipeline_id="` + pipelineId + `",node="` + node + `",job_id="` + jobId + `",job_name="` + jobName + `",scm_url="` + scmUrl + `",status="` + status + `",prefix="` + sdBuildPrefix + `"} ` + strconv.FormatInt(buildSetupTimeSecs, 10) + `
`
		body := strings.NewReader(data)
		log.Printf("pushMetrics: post metrics to [%v]", url)
		res, err := client.HTTPClient.Post(url, "", body)
		if res != nil {
			defer res.Body.Close()
		}
		if err != nil {
			log.Printf("pushMetrics: failed to push metrics to [%v], buildId:[%v], error:[%v]", url, buildID, err)
			return nil
		}
		if res.StatusCode/100 != 2 {
			log.Printf("pushMetrics: failed to push metrics to[%v], buildId:[%v], respose status code:[%v]", url, buildID, res.StatusCode)
			return nil
		}
		log.Printf("pushMetrics: successfully pushed metrics for build:[%v]", buildID)
	} else {
		log.Printf("pushMetrics: pushgatewayUrl:[%v], buildID:[%v], image: [%v], pipelineId: [%v] is empty ", os.Getenv("SD_PUSHGATEWAY_URL"), buildID, os.Getenv("CONTAINER_IMAGE"), os.Getenv("SD_PIPELINE_ID"))
	}
	return nil
}

// exit sets the build status and exits successfully
func exit(status screwdriver.BuildStatus, buildID int, api screwdriver.API, metaSpace string) {
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
		if err := api.UpdateBuildStatus(status, metaInterface, buildID); err != nil {
			log.Printf("Failed updating the build status: %v", err)
		}
	}
	cleanExit()
}

// e.g. scmUri: "github:123456:master", scmName: "screwdriver-cd/launcher"
func parseScmURI(scmURI, scmName string) (scmPath, error) {
	uri := strings.Split(scmURI, ":")
	orgRepo := strings.Split(scmName, "/")

	if (len(uri) != 3 && len(uri) != 4) || len(orgRepo) != 2 {
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
		parsed.RootDir = uri[3]
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
//     /sd/workspace/src/github.com/screwdriver-cd/screwdriver
//     /sd/workspace/artifacts
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
	log.Println("Marshalling Merged Meta JSON")
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

func launch(api screwdriver.API, buildID int, rootDir, emitterPath, metaSpace, storeURL, uiURL, shellBin string, buildTimeout int, buildToken, cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir string, cacheCompress, cacheMd5Check, isLocal bool, cacheMaxSizeInMB int64) error {
	emitter, err := newEmitter(emitterPath)
	envFilepath := "/tmp/env"
	if err != nil {
		return err
	}
	defer emitter.Close()

	if err = api.UpdateStepStart(buildID, "sd-setup-launcher"); err != nil {
		return fmt.Errorf("Updating sd-setup-launcher start: %v", err)
	}

	log.Print("Setting Build Status to RUNNING")
	emptyMeta := make(map[string]interface{}) // {"meta":null} are not accepted. This will be {"meta":{}}
	if err = api.UpdateBuildStatus(screwdriver.Running, emptyMeta, buildID); err != nil {
		return fmt.Errorf("Updating build status to RUNNING: %v", err)
	}

	log.Printf("Fetching Build %d", buildID)
	build, err := api.BuildFromID(buildID)
	if err != nil {
		return fmt.Errorf("Fetching Build ID %d: %v", buildID, err)
	}

	buildCreateTime, _ = time.Parse(time.RFC3339, build.Createtime)
	queueEnterTime, _ = time.Parse(time.RFC3339, build.QueueEntertime)

	log.Printf("Fetching Job %d", build.JobID)
	job, err := api.JobFromID(build.JobID)
	if err != nil {
		return fmt.Errorf("Fetching Job ID %d: %v", build.JobID, err)
	}

	log.Printf("Fetching Pipeline %d", job.PipelineID)
	pipeline, err := api.PipelineFromID(job.PipelineID)
	if err != nil {
		return fmt.Errorf("Fetching Pipeline ID %d: %v", job.PipelineID, err)
	}

	log.Printf("Fetching Event %d", build.EventID)
	event, err := api.EventFromID(build.EventID)
	if err != nil {
		return fmt.Errorf("Fetching Event ID %d: %v", build.EventID, err)
	}

	metaByte := []byte("")
	metaFile := "meta.json" // Write to "meta.json" file
	metaLog := ""

	parentBuildIDs := convertToArray(build.ParentBuildID)
	buildMeta := map[string]interface{}{
		"pipelineId": strconv.Itoa(job.PipelineID),
		"eventId":    strconv.Itoa(build.EventID),
		"jobId":      strconv.Itoa(job.ID),
		"buildId":    strconv.Itoa(buildID),
		"jobName":    job.Name,
		"sha":        build.SHA,
	}
	mergedMeta := map[string]interface{}{
		"build": buildMeta,
	}
	if build.Meta != nil {
		mergedMeta = deepMergeJSON(build.Meta, mergedMeta)
	}

	// Create meta space
	err = createMetaSpace(metaSpace)
	if err != nil {
		return err
	}

	if len(parentBuildIDs) > 1 { // If has multiple parent build IDs, merge their metadata
		// Get meta from all parent builds
		for _, pbID := range parentBuildIDs {
			pb, err := api.BuildFromID(pbID)
			if err != nil {
				return fmt.Errorf("Fetching Parent Build ID %d: %v", pbID, err)
			}
			if pb.Meta != nil {
				mergedMeta = deepMergeJSON(pb.Meta, mergedMeta)
			}
		}

		metaLog = fmt.Sprintf(`Builds(%v)`, parentBuildIDs)
	} else if len(parentBuildIDs) == 1 { // If has parent build, fetch from parent build
		log.Printf("Fetching Parent Build %d", parentBuildIDs[0])
		parentBuild, err := api.BuildFromID(parentBuildIDs[0])
		if err != nil {
			return fmt.Errorf("Fetching Parent Build ID %d: %v", parentBuildIDs[0], err)
		}

		log.Printf("Fetching Parent Job %d", parentBuild.JobID)
		parentJob, err := api.JobFromID(parentBuild.JobID)
		if err != nil {
			return fmt.Errorf("Fetching Job ID %d: %v", parentBuild.JobID, err)
		}

		log.Printf("Fetching Parent Pipeline %d", parentJob.PipelineID)
		parentPipeline, err := api.PipelineFromID(parentJob.PipelineID)
		if err != nil {
			return fmt.Errorf("Fetching Pipeline ID %d: %v", parentJob.PipelineID, err)
		}

		// If build is triggered by an external pipeline, write to "sd@123:component.json"
		// where sd@123:component is the triggering job
		if pipeline.ID != parentPipeline.ID {
			externalMetaFile := "sd@" + strconv.Itoa(parentPipeline.ID) + ":" + parentJob.Name + ".json"
			if parentBuild.Meta != nil {
				writeMetafile(metaSpace, externalMetaFile, metaLog, parentBuild.Meta)
			}
		} else {
			if parentBuild.Meta != nil {
				mergedMeta = deepMergeJSON(parentBuild.Meta, mergedMeta)
			}
		}
		metaLog = fmt.Sprintf(`Build(%v)`, parentBuild.ID)
	} else if event.ParentEventID != 0 { // If has parent event, fetch meta from parent event
		log.Printf("Fetching Parent Event %d", event.ParentEventID)
		parentEvent, err := api.EventFromID(event.ParentEventID)
		if err != nil {
			return fmt.Errorf("Fetching Parent Event ID %d: %v", event.ParentEventID, err)
		}

		if parentEvent.Meta != nil {
			mergedMeta = deepMergeJSON(parentEvent.Meta, mergedMeta)
		}
		metaLog = fmt.Sprintf(`Event(%v)`, parentEvent.ID)
	} else if len(event.Meta) > 0 { // If has meta, marshal it
		log.Printf("Fetching Event Meta JSON %v", event.ID)
		if event.Meta != nil {
			mergedMeta = deepMergeJSON(event.Meta, mergedMeta)
		}
	}

	log.Println("Marshalling Merged Meta JSON")
	metaByte, err = marshal(mergedMeta)

	if err != nil {
		return fmt.Errorf("Parsing Meta JSON: %v", err)
	}

	err = writeFile(metaSpace+"/"+metaFile, metaByte, 0666)
	if err != nil {
		return fmt.Errorf("Writing Parent %v Meta JSON: %v", metaLog, err)
	}

	scm, err := parseScmURI(pipeline.ScmURI, pipeline.ScmRepo.Name)
	if err != nil {
		return err
	}

	log.Printf("Creating Workspace in %v", rootDir)
	w, err := createWorkspace(isLocal, rootDir, scm.Host, scm.Org, scm.Repo)
	if err != nil {
		return err
	}
	sourceDir := w.Src
	if scm.RootDir != "" {
		sourceDir = sourceDir + "/" + scm.RootDir
	}

	cyanFprintf(emitter, "Screwdriver Launcher information\n")
	fmt.Fprintf(emitter, "%s%s\n", blackSprint("Version:        v"), version)
	fmt.Fprintf(emitter, "%s%d\n", blackSprint("Pipeline:       #"), job.PipelineID)
	fmt.Fprintf(emitter, "%s%s\n", blackSprint("Job:            "), job.Name)
	fmt.Fprintf(emitter, "%s%d\n", blackSprint("Build:          #"), buildID)
	fmt.Fprintf(emitter, "%s%s\n", blackSprint("Workspace Dir:  "), w.Root)
	fmt.Fprintf(emitter, "%s%s\n", blackSprint("Checkout Dir:     "), w.Src)
	fmt.Fprintf(emitter, "%s%s\n", blackSprint("Source Dir:     "), sourceDir)
	fmt.Fprintf(emitter, "%s%s\n", blackSprint("Artifacts Dir:  "), w.Artifacts)

	oldJobName := job.Name
	pr := prNumber(job.Name)
	if pr != "" {
		job.Name = "main"
	}

	err = writeArtifact(w.Artifacts, "steps.json", build.Commands)
	if err != nil {
		return fmt.Errorf("Creating steps.json artifact: %v", err)
	}

	err = writeArtifact(w.Artifacts, "environment.json", build.Environment)
	if err != nil {
		return fmt.Errorf("Creating environment.json artifact: %v", err)
	}

	apiURL, _ := api.GetAPIURL()

	isCI := strconv.FormatBool(!isLocal)

	defaultEnv := map[string]string{
		"PS1":                    "",
		"SCREWDRIVER":            isCI,
		"CI":                     isCI,
		"GIT_PAGER":              "cat", // https://github.com/screwdriver-cd/screwdriver/issues/1583#issuecomment-539677403
		"CONTINUOUS_INTEGRATION": isCI,
		"SD_JOB_NAME":            oldJobName,
		"SD_PIPELINE_NAME":       pipeline.ScmRepo.Name,
		"SD_BUILD_ID":            strconv.Itoa(buildID),
		"SD_JOB_ID":              strconv.Itoa(job.ID),
		"SD_EVENT_ID":            strconv.Itoa(build.EventID),
		"SD_PIPELINE_ID":         strconv.Itoa(job.PipelineID),
		"SD_PARENT_BUILD_ID":     fmt.Sprintf("%v", parentBuildIDs),
		"SD_PR_PARENT_JOB_ID":    strconv.Itoa(job.PrParentJobID),
		"SD_PARENT_EVENT_ID":     strconv.Itoa(event.ParentEventID),
		"SD_SOURCE_DIR":          sourceDir,
		"SD_CHECKOUT_DIR":        w.Src,
		"SD_ROOT_DIR":            w.Root,
		"SD_ARTIFACTS_DIR":       w.Artifacts,
		"SD_META_DIR":            metaSpace,
		"SD_META_PATH":           metaSpace + "/meta.json",
		"SD_BUILD_SHA":           build.SHA,
		"SD_PULL_REQUEST":        pr,
		"SD_API_URL":             apiURL,
		"SD_BUILD_URL":           apiURL + "builds/" + strconv.Itoa(buildID),
		"SD_STORE_URL":           fmt.Sprintf("%s/%s/", storeURL, "v1"),
		"SD_UI_URL":              fmt.Sprintf("%s/", uiURL),
		"SD_UI_BUILD_URL":        fmt.Sprintf("%s/pipelines/%s/builds/%s", uiURL, strconv.Itoa(job.PipelineID), strconv.Itoa(buildID)),
		"SD_TOKEN":               buildToken,
		"SD_CACHE_STRATEGY":      cacheStrategy,
		"SD_PIPELINE_CACHE_DIR":  pipelineCacheDir,
		"SD_JOB_CACHE_DIR":       jobCacheDir,
		"SD_EVENT_CACHE_DIR":     eventCacheDir,
		"SD_CACHE_COMPRESS":      fmt.Sprintf("%v", cacheCompress),
		"SD_CACHE_MD5CHECK":      fmt.Sprintf("%v", cacheMd5Check),
		"SD_CACHE_MAX_SIZE_MB":   fmt.Sprintf("%v", cacheMaxSizeInMB),
	}

	// Add coverage env vars
	coverageScope := ""
	if len(job.Permutations) > 0 {
		coverageScope = job.Permutations[0].Annotations.CoverageScope
	}
	coverageInfo, err := api.GetCoverageInfo(job.ID, job.PipelineID, job.Name, pipeline.ScmRepo.Name, coverageScope, pr, strconv.Itoa(job.PrParentJobID))
	if err != nil {
		log.Printf("Failed to get coverage info for build %v so skip it\n", build.ID)
	} else {
		for key, value := range coverageInfo.EnvVars {
			defaultEnv[key] = value
		}
	}

	// Get secrets for build
	secrets, err := api.SecretsForBuild(build)
	if err != nil {
		return fmt.Errorf("Fetching secrets for build %v", build.ID)
	}

	env, userShellBin := createEnvironment(defaultEnv, secrets, build)
	if userShellBin != "" {
		shellBin = userShellBin
	}

	return executorRun(w.Src, env, emitter, build, api, buildID, shellBin, buildTimeout, envFilepath, sourceDir)
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
func launchAction(api screwdriver.API, buildID int, rootDir, emitterPath, metaSpace, storeURI, uiURI, shellBin string, buildTimeout int, buildToken, cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir string, cacheCompress, cacheMd5Check, isLocal bool, cacheMaxSizeInMB int64) error {
	log.Printf("Starting Build %v\n", buildID)
	log.Printf("Cache strategy & directories (pipeline, job, event), compress, md5check, maxsize: %v, %v, %v, %v, %v, %v, %v \n", cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir, cacheCompress, cacheMd5Check, cacheMaxSizeInMB)

	if err := launch(api, buildID, rootDir, emitterPath, metaSpace, storeURI, uiURI, shellBin, buildTimeout, buildToken, cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir, cacheCompress, cacheMd5Check, isLocal, cacheMaxSizeInMB); err != nil {
		if _, ok := err.(executor.ErrStatus); ok {
			log.Printf("Failure due to non-zero exit code: %v\n", err)
		} else {
			log.Printf("Error running launcher: %v\n", err)
		}

		exit(screwdriver.Failure, buildID, api, metaSpace)
		return nil
	}

	exit(screwdriver.Success, buildID, api, metaSpace)
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

		exit(screwdriver.Failure, buildID, api, metaSpace)
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
	}

	app.Action = func(c *cli.Context) error {
		url := c.String("api-uri")
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
		isLocal := c.Bool("local-mode")
		localBuildJson := c.String("local-build-json")
		localJobName := c.String("local-job-name")

		if err != nil {
			return cli.ShowAppHelp(c)
		}

		log.Printf("cache strategy, directories (pipeline, job, event), compress, md5check, maxsize: %v, %v, %v, %v, %v, %v, %v \n", cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir, cacheCompress, cacheMd5Check, cacheMaxSizeInMB)

		if !isLocal && len(token) == 0 {
			log.Println("Error: token is not passed.")
			cleanExit()
		}

		if fetchFlag {
			temporalApi, err := screwdriver.New(url, token)
			if err != nil {
				log.Printf("Error creating temporal Screwdriver API %v: %v", buildID, err)
				exit(screwdriver.Failure, buildID, nil, metaSpace)
			}

			buildToken, err := temporalApi.GetBuildToken(buildID, c.Int("build-timeout"))
			if err != nil {
				log.Printf("Error getting Build Token %v: %v", buildID, err)
				exit(screwdriver.Failure, buildID, nil, metaSpace)
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

			api, err = screwdriver.NewLocal(url, localJobName, localBuild)
		} else {
			api, err = screwdriver.New(url, token)
		}
		if err != nil {
			log.Printf("Error creating Screwdriver API %v: %v", buildID, err)
			exit(screwdriver.Failure, buildID, nil, metaSpace)
		}

		defer recoverPanic(buildID, api, metaSpace)

		launchAction(api, buildID, workspace, emitterPath, metaSpace, storeURL, uiURL, shellBin, buildTimeoutSeconds, token, cacheStrategy, pipelineCacheDir, jobCacheDir, eventCacheDir, cacheCompress, cacheMd5Check, isLocal, cacheMaxSizeInMB)

		// This should never happen...
		log.Println("Unexpected return in launcher. Failing the build.")
		exit(screwdriver.Failure, buildID, api, metaSpace)
		return nil
	}
	app.Run(os.Args)
}
