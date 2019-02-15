package main

import (
	"encoding/json"
	"fmt"
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
	"github.com/screwdriver-cd/launcher/executor"
	"github.com/screwdriver-cd/launcher/screwdriver"
	"gopkg.in/fatih/color.v1"
	"gopkg.in/urfave/cli.v1"
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

var cleanExit = func() {
	os.Exit(0)
}

const DefaultTimeout = 90 // 90 minutes

// exit sets the build status and exits successfully
func exit(status screwdriver.BuildStatus, buildID int, api screwdriver.API, metaSpace string) {
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

type scmPath struct {
	Host   string
	Org    string
	Repo   string
	Branch string
}

// e.g. scmUri: "github:123456:master", scmName: "screwdriver-cd/launcher"
func parseScmURI(scmURI, scmName string) (scmPath, error) {
	uri := strings.Split(scmURI, ":")
	orgRepo := strings.Split(scmName, "/")

	if len(uri) != 3 || len(orgRepo) != 2 {
		return scmPath{}, fmt.Errorf("Unable to parse scmUri %v and scmName %v", scmURI, scmName)
	}

	return scmPath{
		Host:   uri[0],
		Org:    orgRepo[0],
		Repo:   orgRepo[1],
		Branch: uri[2],
	}, nil
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
func createWorkspace(rootDir string, srcPaths ...string) (Workspace, error) {
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
		if err == nil {
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

var createMetaSpace = func(metaSpace string) error {
	err := mkdirAll(metaSpace, 0777)
	if err != nil {
		return fmt.Errorf("Cannot create meta-space path %q: %v", metaSpace, err)
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

func launch(api screwdriver.API, buildID int, rootDir, emitterPath, metaSpace, storeURL, shellBin string, buildTimeout int, buildToken string) error {
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
	mergedMeta := map[string]interface{}{
		"pipelineId": strconv.Itoa(job.PipelineID),
		"eventId":    strconv.Itoa(build.EventID),
		"jobId":      strconv.Itoa(job.ID),
		"buildId":    strconv.Itoa(buildID),
		"jobName":    job.Name,
		"sha":        build.SHA,
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
			metaFile = "sd@" + strconv.Itoa(parentPipeline.ID) + ":" + parentJob.Name + ".json"
		}
		if parentBuild.Meta != nil {
			mergedMeta = deepMergeJSON(parentBuild.Meta, mergedMeta)
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

	// Create meta space

	log.Printf("Creating Meta Space in %v", metaSpace)
	err = createMetaSpace(metaSpace)
	if err != nil {
		return err
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
	w, err := createWorkspace(rootDir, scm.Host, scm.Org, scm.Repo)
	if err != nil {
		return err
	}

	cyanFprintf(emitter, "Screwdriver Launcher information\n")
	fmt.Fprintf(emitter, "%s%s\n", blackSprint("Version:        v"), version)
	fmt.Fprintf(emitter, "%s%d\n", blackSprint("Pipeline:       #"), job.PipelineID)
	fmt.Fprintf(emitter, "%s%s\n", blackSprint("Job:            "), job.Name)
	fmt.Fprintf(emitter, "%s%d\n", blackSprint("Build:          #"), buildID)
	fmt.Fprintf(emitter, "%s%s\n", blackSprint("Workspace Dir:  "), w.Root)
	fmt.Fprintf(emitter, "%s%s\n", blackSprint("Source Dir:     "), w.Src)
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

	defaultEnv := map[string]string{
		"PS1":         "",
		"SCREWDRIVER": "true",
		"CI":          "true",
		"CONTINUOUS_INTEGRATION": "true",
		"SD_BUILD_ID":            strconv.Itoa(buildID),
		"SD_JOB_ID":              strconv.Itoa(job.ID),
		"SD_PIPELINE_ID":         strconv.Itoa(job.PipelineID),
		"SD_EVENT_ID":            strconv.Itoa(build.EventID),
		"SD_JOB_NAME":            oldJobName,
		"SD_PIPELINE_NAME":       pipeline.ScmRepo.Name,
		"SD_PULL_REQUEST":        pr,
		"SD_PR_PARENT_JOB_ID":    strconv.Itoa(job.PrParentJobID),
		"SD_PARENT_BUILD_ID":     fmt.Sprintf("%v", build.ParentBuildID),
		"SD_PARENT_EVENT_ID":     strconv.Itoa(event.ParentEventID),
		"SD_SOURCE_DIR":          w.Src,
		"SD_ROOT_DIR":            w.Root,
		"SD_ARTIFACTS_DIR":       w.Artifacts,
		"SD_META_PATH":           metaSpace + "/" + metaFile,
		"SD_API_URL":             apiURL,
		"SD_BUILD_URL":           apiURL + "builds/" + strconv.Itoa(buildID),
		"SD_BUILD_SHA":           build.SHA,
		"SD_STORE_URL":           fmt.Sprintf("%s/%s/", storeURL, "v1"),
		"SD_TOKEN":               buildToken,
	}

	// Add coverage env vars
	coverageInfo, err := api.GetCoverageInfo()
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

	if err := api.UpdateStepStop(buildID, "sd-setup-launcher", 0); err != nil {
		return fmt.Errorf("Updating sd-setup-launcher stop: %v", err)
	}

	return executorRun(w.Src, env, emitter, build, api, buildID, shellBin, buildTimeout, envFilepath)
}

func createEnvironment(base map[string]string, secrets screwdriver.Secrets, build screwdriver.Build) ([]string, string) {
	var userShellBin string

	combined := map[string]string{}

	// Start with the current environment
	for _, e := range os.Environ() {
		pieces := strings.SplitAfterN(e, "=", 2)
		if len(pieces) != 2 {
			log.Printf("WARN: bad environment value from base environment: %s", e)
			continue
		}

		k := pieces[0][:len(pieces[0])-1] // Drop the "=" off the end
		v := pieces[1]

		combined[k] = v
	}

	// Add the default environment values
	for k, v := range base {
		combined[k] = v
	}

	// Add secrets to the environment
	for _, s := range secrets {
		combined[s.Name] = s.Value
	}

	// Create the final string slice
	envStrings := []string{}
	for k, v := range combined {
		envStrings = append(envStrings, strings.Join([]string{k, v}, "="))
	}

	for k, v := range build.Environment {
		envStrings = append(envStrings, strings.Join([]string{k, v}, "="))
		if k == "USER_SHELL_BIN" {
			userShellBin = v
		}
	}

	return envStrings, userShellBin
}

// Executes the command based on arguments from the CLI
func launchAction(api screwdriver.API, buildID int, rootDir, emitterPath, metaSpace, storeURI, shellBin string, buildTimeout int, buildToken string) error {
	log.Printf("Starting Build %v\n", buildID)

	if err := launch(api, buildID, rootDir, emitterPath, metaSpace, storeURI, shellBin, buildTimeout, buildToken); err != nil {
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
	}

	app.Action = func(c *cli.Context) error {
		url := c.String("api-uri")
		token := c.String("token")
		workspace := c.String("workspace")
		emitterPath := c.String("emitter")
		metaSpace := c.String("meta-space")
		storeURL := c.String("store-uri")
		shellBin := c.String("shell-bin")
		buildID, err := strconv.Atoi(c.Args().Get(0))
		buildTimeoutSeconds := c.Int("build-timeout") * 60
		fetchFlag := c.Bool("only-fetch-token")

		if err != nil {
			return cli.ShowAppHelp(c)
		}

		if len(token) == 0 {
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

		api, err := screwdriver.New(url, token)
		if err != nil {
			log.Printf("Error creating Screwdriver API %v: %v", buildID, err)
			exit(screwdriver.Failure, buildID, nil, metaSpace)
		}

		defer recoverPanic(buildID, api, metaSpace)

		launchAction(api, buildID, workspace, emitterPath, metaSpace, storeURL, shellBin, buildTimeoutSeconds, token)

		// This should never happen...
		log.Println("Unexpected return in launcher. Failing the build.")
		exit(screwdriver.Failure, buildID, api, metaSpace)
		return nil
	}
	app.Run(os.Args)
}
