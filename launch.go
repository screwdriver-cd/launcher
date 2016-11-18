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
	"strings"
	"time"

	"github.com/screwdriver-cd/launcher/executor"
	"github.com/screwdriver-cd/launcher/screwdriver"
	"github.com/urfave/cli"
)

// VERSION gets set by the build script via the LDFLAGS
var VERSION string

var mkdirAll = os.MkdirAll
var stat = os.Stat
var open = os.Open
var executorRun = executor.Run
var writeFile = ioutil.WriteFile
var newEmitter = screwdriver.NewEmitter

var cleanExit = func() {
	os.Exit(0)
}

// exit sets the build status and exits successfully
func exit(status screwdriver.BuildStatus, buildID string, api screwdriver.API) {
	if api != nil {
		log.Printf("Setting build status to %s", status)
		if err := api.UpdateBuildStatus(status, buildID); err != nil {
			log.Printf("Failed updating the build status: %v", err)
		}
	}

	cleanExit()
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

func launch(api screwdriver.API, buildID, rootDir, emitterPath string) error {
	emitter, err := newEmitter(emitterPath)
	if err != nil {
		return err
	}
	defer emitter.Close()

	if err = api.UpdateStepStart(buildID, "sd-setup"); err != nil {
		return fmt.Errorf("updating sd-setup start: %v", err)
	}

	log.Print("Setting Build Status to RUNNING")
	if err = api.UpdateBuildStatus(screwdriver.Running, buildID); err != nil {
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

	oldJobName := j.Name
	pr := prNumber(j.Name)
	if pr != "" {
		j.Name = "main"
	}

	defaultEnv := map[string]string{
		"SCREWDRIVER": "true",
		"CI":          "true",
		"CONTINUOUS_INTEGRATION": "true",
		"SD_JOB_NAME":            oldJobName,
		"SD_PULL_REQUEST":        pr,
	}

	secrets, err := api.SecretsForBuild(b)
	if err != nil {
		return fmt.Errorf("Fetching secrets for build %s", b.ID)
	}

	env := createEnvironment(defaultEnv, secrets, b)

	srcDir := os.Getenv("SD_SOURCE_DIR")
	artifactsDir := os.Getenv("SD_ARTIFACTS_DIR")

	if err := writeArtifact(artifactsDir, "steps.json", b.Commands); err != nil {
		return fmt.Errorf("creating steps.json artifact: %v", err)
	}

	if err := writeArtifact(artifactsDir, "environment.json", b.Environment); err != nil {
		return fmt.Errorf("creating environment.json artifact: %v", err)
	}

	if err := api.UpdateStepStop(buildID, "sd-setup", 0); err != nil {
		return fmt.Errorf("updating sd-setup stop: %v", err)
	}

	if err := executorRun(srcDir, env, emitter, b, api, buildID); err != nil {
		return err
	}

	return nil
}

func createEnvironment(base map[string]string, secrets screwdriver.Secrets, build screwdriver.Build) []string {
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

	// Delete any environment variables that we don't want the user to accidentally dump
	for _, k := range []string{
		"SD_TOKEN",
	} {
		delete(combined, k)
	}

	// Create the final string slice
	envStrings := []string{}
	for k, v := range combined {
		envStrings = append(envStrings, strings.Join([]string{k, v}, "="))
	}

	for k, v := range build.Environment {
		envStrings = append(envStrings, strings.Join([]string{k, v}, "="))
	}

	return envStrings
}

// Executes the command based on arguments from the CLI
func launchAction(api screwdriver.API, buildID, rootDir, emitterPath string) error {
	log.Printf("Starting Build %v\n", buildID)

	if err := launch(api, buildID, rootDir, emitterPath); err != nil {
		if _, ok := err.(executor.ErrStatus); ok {
			log.Printf("Failure due to non-zero exit code: %v\n", err)
		} else {
			log.Printf("Error running launcher: %v\n", err)
		}

		exit(screwdriver.Failure, buildID, api)
		return nil
	}

	exit(screwdriver.Success, buildID, api)
	return nil
}

func recoverPanic(buildID string, api screwdriver.API) {
	if p := recover(); p != nil {
		filename := fmt.Sprintf("launcher-stacktrace-%s", time.Now().Format(time.RFC3339))
		tracefile := filepath.Join(os.TempDir(), filename)

		log.Printf("ERROR: Internal Screwdriver error. Please file a bug about this: %v", p)
		log.Printf("ERROR: Writing StackTrace to %s", tracefile)
		err := ioutil.WriteFile(tracefile, debug.Stack(), 0600)
		if err != nil {
			log.Printf("ERROR: Unable to write stacktrace to file: %v", err)
		}

		exit(screwdriver.Failure, buildID, api)
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
	defer recoverPanic("", nil)

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
		cli.StringFlag{
			Name:  "emitter",
			Usage: "Location for writing log lines to",
			Value: "/var/run/sd/emitter",
		},
	}

	app.Action = func(c *cli.Context) error {
		url := c.String("api-uri")
		token := c.String("token")
		workspace := c.String("workspace")
		emitterPath := c.String("emitter")
		buildID := c.Args().Get(0)

		if buildID == "" {
			return cli.ShowAppHelp(c)
		}

		api, err := screwdriver.New(url, token)
		if err != nil {
			log.Printf("Error creating Screwdriver API %v: %v", buildID, err)
			exit(screwdriver.Failure, buildID, nil)
		}

		defer recoverPanic(buildID, api)

		launchAction(api, buildID, workspace, emitterPath)

		// This should never happen...
		log.Println("Unexpected return in launcher. Failing the build.")
		exit(screwdriver.Failure, buildID, api)
		return nil
	}
	app.Run(os.Args)
}
