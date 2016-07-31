package main

import(
	"os"
	"os/exec"
	"strings"
	"testing"
)
var baseDir string

// Create a test directory and change into it
func setUp() {
	os.Mkdir("testDir", 0777)
	os.Chdir("testDir")
}

// Change to base dirrectory and remove the test directory
func cleanUp() {
	os.Chdir(baseDir)
	ShellCommand("rm", "-rf", "testDir")
}

// Test that a valid shell command will execute properly
func TestShellCommand(t *testing.T) {
	var wdByte []byte
	wdByte, _ = exec.Command("pwd").CombinedOutput()
	wd := string(wdByte)

	wdTest, _ := ShellCommand("pwd")

	// Working directories should be equal
	if strings.Compare(wd, wdTest) != 0 {
		t.Logf("Expected working directory to equal: %s, but was: %s", wd, wdTest)
		t.FailNow()
	}
}

// Test that an invalid shell command will return an error
func TestShellCommandBadInput(t *testing.T) {
	_, err := ShellCommand("pwda")

	// Error message should not be nil
	if err == nil {
		t.Logf("Expected err to not be: nil")
		t.Fail()
	}
}

// Test that Clone will return without errors and change into the cloned directory
func TestClone(t *testing.T) {
	setUp()
	wd, _ := ShellCommand("pwd")

	// Arguments
	GIT_PATH := "github.com/Filbird/pr"
	GIT_BRANCH := "master"

	err := Clone(GIT_PATH, GIT_BRANCH)
	// Error message should be nil
	if err != nil {
		t.Logf("Expected err to be: nil, but was %v", nil)
		cleanUp()
		t.FailNow()
	}

	wdTest, _ := ShellCommand("pwd")
	wd = strings.Trim(wd, "\n")
	wd = wd + "/" + GIT_PATH + "\n"
	// Working directory should be updated
	if strings.Compare(wd, wdTest) != 0 {
		t.Logf("Expected working directory to equal: %s, but was: %s", wd, wdTest)
		t.Fail()
	}

	cleanUp()
}

//Test that Clone will return an error if GIT_PATH or GIT_BRANCH is incorrect
func TestCloneBadInput(t *testing.T) {
	setUp()

	// Arguments
	GIT_PATH := "badPath"
	GIT_BRANCH := "badBranch"

	// Error message should not be nil
	err := Clone(GIT_PATH, GIT_BRANCH)
	if err == nil {
		t.Logf("Expected err to not be: nil")
		t.Fail()
	}

	cleanUp()
}

// // Test that FetchAndMergePR will return without errors and return the correct pull request number
// func TestFetchAndMergePR(t *testing.T) {
// 	setUp()

// 	// Arguments
// 	GIT_BRANCH := "master"
// 	JOB_NAME := "PR-1"

// 	prTest, err := FetchAndMergePR(GIT_BRANCH, JOB_NAME)

// 	// Error message should be nil
// 	if err != nil {
// 		t.Logf("Expected err to be: nil, but was %v", err)
// 		cleanUp()
// 		t.FailNow()
// 	}

// 	pr := "1"
// 	if strings.Compare(pr, prTest) != 0 {
// 		t.Logf("Expected pull request number to be: %s, but was %s",pr, prTest)
// 		cleanUp()
// 		t.FailNow()
// 	}
// }

// Test that ArtifactsDirSetUp will return without errors and exports $ARTIFACTS_DIR
func TestArtifactsDirSetUp(t *testing.T) {
	setUp()

	// Call Clone
	GIT_PATH := "github.com/Filbird/pr"
	GIT_BRANCH := "master"
	_ = Clone(GIT_PATH, GIT_BRANCH)

	err := ArtifactsDirSetUp()
	// Error message should be nil
	if err != nil {
		t.Logf("Expected err to be: nil, but was %v", err)
		cleanUp()
		t.FailNow()
	}

	wd, _ := ShellCommand("pwd")
	artifactsDir := os.Getenv("ARTIFACTS_DIR")
	artifactsDir = artifactsDir + "\n"
	// Artifacts directory should be exported
	if strings.Compare(wd, artifactsDir) != 0 {
		t.Logf("Expected artifacts directory to equal: %s, but was: %s", wd, artifactsDir)
		t.Fail()
	}

	cleanUp()
 }

 func TestMain(m *testing.M) {
 	baseDir, _ = ShellCommand("pwd")
 	baseDir = strings.Split(baseDir, "\n")[0]
	status := m.Run()
	cleanUp()
	os.Exit(status)
 }
