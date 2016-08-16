package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/screwdriver-cd/launcher/executor"
	"github.com/screwdriver-cd/launcher/git"
	"github.com/screwdriver-cd/launcher/screwdriver"
	"github.com/urfave/cli"
)

// VERSION gets set by the build script via the LDFLAGS
var VERSION string

var mkdirAll = os.MkdirAll
var stat = os.Stat
var gitSetup = git.Setup
var open = os.Open
var executorRun = executor.Run
var writeFile = ioutil.WriteFile

var cleanExit = func() {
	os.Exit(0)
}

// exit sets the build status and exits successfully
func exit(status screwdriver.BuildStatus, api screwdriver.API) {
	if api != nil {
		log.Printf("Setting build status to %s", status)
		if err := api.UpdateBuildStatus(status); err != nil {
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

func (s scmPath) String() string {
	return fmt.Sprintf("git@%s:%s/%s#%s", s.Host, s.Org, s.Repo, s.Branch)
}

func (s scmPath) httpsString() string {
	return fmt.Sprintf("https://%s/%s/%s#%s", s.Host, s.Org, s.Repo, s.Branch)
}

// e.g. "git@github.com:screwdriver-cd/launch.git#master"
func parseScmURL(url string) (scmPath, error) {
	r := regexp.MustCompile("^git@(.*):(.*)/(.*)#(.*)$")
	matched := r.FindStringSubmatch(url)
	if matched == nil || len(matched) != 5 {
		return scmPath{}, fmt.Errorf("Unable to parse SCM URL %v, match: %q", url, matched)
	}

	return scmPath{
		Host:   matched[1],
		Org:    matched[2],
		Repo:   matched[3],
		Branch: matched[4],
	}, nil
}

// A Workspace is a description of the paths available to a Screwdriver build
type Workspace struct {
	Root      string
	Src       string
	Artifacts string
}

// createWorkspace makes a Scrwedriver workspace from path components
// e.g. ["screwdriver-cd" "screwdriver"] creates
//     /sd/workspace/src/screwdriver-cd/screwdriver
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

func writeArtifact(aDir string, fName string, artifact interface{}) error {
	data, err := json.MarshalIndent(artifact, "", strings.Repeat(" ", 4))
	if err != nil {
		return fmt.Errorf("marshaling artifact: %v ", err)
	}

	pathToCreate := path.Join(aDir, fName)
	err = writeFile(pathToCreate, data, 0644)
	if err != nil {
		return fmt.Errorf("creating file %q : %v", pathToCreate, err)
	}

	return nil
}

// prNumber checks to see if the job name is a pull request and returns its number
func prNumber(jobName string) string {
	r := regexp.MustCompile("^PR-([0-9]+)$")
	matched := r.FindStringSubmatch(jobName)
	if matched == nil || len(matched) != 2 {
		return ""
	}
	log.Println("Build is a PR: ", matched[1])
	return matched[1]
}

func launch(api screwdriver.API, buildID string, rootDir string) error {
	log.Print("Setting Build Status to RUNNING")
	err := api.UpdateBuildStatus(screwdriver.Running)
	if err != nil {
		return fmt.Errorf("updating build status to RUNNING: %v", err)
	}

	log.Printf("Fetching Build %v", buildID)
	b, err := api.BuildFromID(buildID)
	if err != nil {
		return fmt.Errorf("fetching build ID %q: %v", buildID, err)
	}

	log.Printf("Fetching Job %v", b.JobID)
	j, err := api.JobFromID(b.JobID)
	if err != nil {
		return fmt.Errorf("fetching Job ID %q: %v", b.JobID, err)
	}

	log.Printf("Fetching Pipeline %v", j.PipelineID)
	p, err := api.PipelineFromID(j.PipelineID)
	if err != nil {
		return fmt.Errorf("fetching Pipeline ID %q: %v", j.PipelineID, err)
	}

	scm, err := parseScmURL(p.ScmURL)
	log.Printf("Creating Workspace in %v", rootDir)
	w, err := createWorkspace(rootDir, scm.Org, scm.Repo)
	if err != nil {
		return err
	}

	pr := prNumber(j.Name)
	if pr != "" {
		j.Name = "main"
	}

	err = gitSetup(scm.httpsString(), w.Src, pr, b.SHA)
	if err != nil {
		return err
	}

	var yaml io.ReadCloser

	yaml, err = open(path.Join(w.Src, "screwdriver.yaml"))
	if err != nil {
		return fmt.Errorf("opening screwdriver.yaml: %v", err)
	}

	defer yaml.Close()

	pipelineDef, err := api.PipelineDefFromYaml(yaml)
	if err != nil {
		return err
	}

	jobs := pipelineDef.Jobs[j.Name]
	if len(jobs) == 0 {
		log.Printf("ERROR: Launcher currently only supports 1 matrix job. 0 jobs are configured for %q\n", j.Name)
		exit(screwdriver.Failure, api)
	}

	if len(jobs) != 1 {
		log.Printf("WARNING: Launcher currently only supports 1 matrix job. %d jobs are configured for %q\n", len(jobs), j.Name)
	}

	// TODO: Select the specific child build by looking at the decimal value of the build number
	currentJob := pipelineDef.Jobs[j.Name][0]

	err = writeArtifact(w.Artifacts, "steps.json", currentJob.Commands)
	if err != nil {
		return fmt.Errorf("creating steps.json artifact: %v", err)
	}

	err = writeArtifact(w.Artifacts, "environment.json", currentJob.Environment)
	if err != nil {
		return fmt.Errorf("creating environment.json artifact: %v", err)
	}

	err = executorRun(currentJob.Commands)
	if err != nil {
		return err
	}

	return nil
}

// Executes the command based on arguments from the CLI
func launchAction(api screwdriver.API, buildID string, rootDir string) error {
	log.Printf("Starting Build %v\n", buildID)

	if err := launch(api, buildID, rootDir); err != nil {
		if _, ok := err.(executor.ErrStatus); ok {
			log.Printf("Failure due to non-zero exit code: %v\n", err)
		} else {
			log.Printf("Error running launcher: %v\n", err)
		}

		exit(screwdriver.Failure, api)
		return nil
	}

	exit(screwdriver.Success, api)
	return nil
}

func recoverPanic(api screwdriver.API) {
	if p := recover(); p != nil {
		filename := fmt.Sprintf("launcher-stacktrace-%s", time.Now().Format(time.RFC3339))
		tracefile := filepath.Join(os.TempDir(), filename)

		log.Printf("ERROR: Internal Screwdriver error. Please file a bug about this: %v", p)
		log.Printf("ERROR: Writing StackTrace to %s", tracefile)
		err := ioutil.WriteFile(tracefile, debug.Stack(), 0600)
		if err != nil {
			log.Printf("ERROR: Unable to write stacktrace to file: %v", err)
		}

		exit(screwdriver.Failure, api)
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
	defer recoverPanic(nil)

	app := cli.NewApp()
	app.Name = "launcher"
	app.Usage = "launch a Screwdriver build"
	app.UsageText = "launch [options] build-id"
	app.Copyright = "(c) 2016 Yahoo Inc."

	if VERSION == "" {
		VERSION = "0.0.0"
	}
	app.Version = VERSION

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
	}

	app.Action = func(c *cli.Context) error {
		url := c.String("api-uri")
		token := c.String("token")
		workspace := c.String("workspace")
		buildID := c.Args().Get(0)

		if buildID == "" {
			return cli.ShowAppHelp(c)
		}

		api, err := screwdriver.New(url, token)
		if err != nil {
			log.Printf("Error creating Screwdriver API %v: %v", buildID, err)
			exit(screwdriver.Failure, nil)
		}

		defer recoverPanic(api)

		launchAction(api, buildID, workspace)

		// This should never happen...
		log.Println("Unexpected return in launcher. Failing the build.")
		exit(screwdriver.Failure, api)
		return nil
	}
	app.Run(os.Args)
}
