package main

import(
	"fmt"
	"os"
)

var SCRIPT_DIR string
var GIT_ORG string
var GIT_REPO string
var GIT_PATH string
var GIT_BRANCH string
var JOB_NAME string


func main(){
	//Set up arguments
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Missing arguments: {GIT_ORG, GIT_REPO, GIT_BRANCH, JOB_NAME}")
		os.Exit(1)
	}
	GIT_ORG = os.Args[1]
	GIT_REPO = os.Args[2]
	GIT_PATH = "github.com/" + GIT_ORG + "/" + GIT_REPO
	GIT_BRANCH = "master"
	if len(os.Args) >= 4 {
		GIT_BRANCH = os.Args[3]
	}
	JOB_NAME = "main"
	if len(os.Args) == 5 {
		JOB_NAME = os.Args[4]
	}

	//Shell commands to execute in order
	err := Clone(GIT_PATH, GIT_BRANCH)
	if err != nil {
		fmt.Fprintln(os.Stderr, "There was an error cloning the git repository: ", err)
		os.Exit(1)
	}

	_, err = FetchAndMergePR(GIT_BRANCH, JOB_NAME)
	if err != nil {
		fmt.Fprintln(os.Stderr, "There was a fetching or merging the pull request ", err)
		os.Exit(1)
	}

	err = ArtifactsDirSetUp()
	if err != nil {
		fmt.Fprintln(os.Stderr, "There was an error setting up the artifacts directory: ", err)
		os.Exit(1)
	}


	//Call remaining shell commands here:
}