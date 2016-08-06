package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"regexp"

	"github.com/screwdriver-cd/launcher/git"
	"github.com/screwdriver-cd/launcher/screwdriver"
	"github.com/urfave/cli"
)

// VERSION gets set by the build script via the LDFLAGS
var VERSION string

type scmPath struct {
	Host   string
	Org    string
	Repo   string
	Branch string
}

var mkdirAll = os.MkdirAll
var stat = os.Stat
var gitSetup = git.Setup

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

// prNumber checks to see if the job name is a pull request and returns its number
func prNumber(jobName string) string {
	r := regexp.MustCompile("^PR-([0-9]+)$")
	matched := r.FindStringSubmatch(jobName)
	if matched == nil || len(matched) != 2 {
		return ""
	}

	return matched[1]
}

func launch(api screwdriver.API, buildID string, rootDir string) error {
	log.Print("Update Build Status to RUNNING")
	err := api.UpdateBuildStatus("RUNNING")
	if err != nil {
		return fmt.Errorf("updating build status: %v", err)
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

	err = gitSetup(scm.httpsString(), w.Src, pr)
	if err != nil {
		return err
	}

	return nil
}

// Executes the command based on arguments from the CLI
func launchAction(c *cli.Context) error {
	url := c.String("api-uri")
	token := c.String("token")
	workspace := c.String("workspace")
	buildID := c.Args().Get(0)

	if buildID == "" {
		return cli.ShowAppHelp(c)
	}

	log.Printf("Starting Build %v\n", buildID)

	api, err := screwdriver.New(url, token)
	if err != nil {
		log.Fatalf("Error creating Screwdriver API %v: %v", buildID, err)
	}

	if err = launch(api, buildID, workspace); err != nil {
		log.Fatalf("Error running launcher: %v\n", err)
		os.Exit(1)
	}

	return nil
}

func main() {
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

	app.Action = launchAction
	app.Run(os.Args)
}
