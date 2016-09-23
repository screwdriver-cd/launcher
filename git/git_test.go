package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

const (
	testSHA = "1a2b3c4d5e6f7g"
)

type execFunc func(command string, args ...string) *exec.Cmd
type validatorFunc func(string, ...string)

func getFakeExecCommand(validator validatorFunc) execFunc {
	return func(command string, args ...string) *exec.Cmd {
		validator(command, args...)
		return fakeExecCommand(command, args...)
	}
}

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestHelperProcess(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args[:]
	for i, val := range os.Args { // Should become something like ["git", "clone"]
		args = os.Args[i:]
		if val == "--" {
			args = args[1:]
			break
		}
	}

	if len(args) > 1 {
		switch args[1] {
		case "clone":
			return
		case "config":
			return
		case "fetch":
			return
		case "merge":
			return
		case "reset":
			return
		}
	}
	os.Exit(255)
}

func TestGetPath(t *testing.T) {
	testRepo := repo{
		scmURL: "test.com",
		path:   "test/path",
		branch: "testBranch",
		logger: os.Stderr,
	}

	wantPath := "test/path"

	if path := testRepo.Path(); path != wantPath {
		t.Errorf("path = %q, want %q", path, wantPath)
	}
}

func TestMergePR(t *testing.T) {
	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()

	currDir, err := os.Getwd()
	if err != nil {
		t.Errorf("getting working directory: %v", err)
	}

	tests := []struct {
		r   repo
		pr  string
		sha string
		err error
	}{
		{
			r: repo{
				scmURL: "https://github.com/screwdriver-cd/launcher.git",
				path:   currDir,
				branch: "master",
				logger: os.Stderr,
			},
			pr:  "1",
			sha: "abc123",
			err: nil,
		},
		{
			r: repo{
				scmURL: "https://github.com/screwdriver-cd/launcher.git",
				path:   "bad path",
				branch: "master",
				logger: os.Stderr,
			},
			pr:  "1",
			sha: "abc123",
			err: errors.New("fetching pr: starting git command: " +
				"chdir bad path: no such file or directory"),
		},
	}

	for _, test := range tests {
		validator := func(command string, args ...string) {
			if command != "git" {
				t.Errorf("command = %q, want <git>", command)
			}
			if len(args) < 2 {
				t.Errorf("no arguments")
			}

			var wantArgs []string

			switch args[0] {
			case "fetch":
				wantArgs = []string{"origin", "pull/" + test.pr + "/head:pr"}
			case "merge":
				wantArgs = []string{"--no-edit", test.sha}
			default:
				t.Errorf("incorrect git command: %v", args[0])
			}
			if !reflect.DeepEqual(args[1:], wantArgs) {
				fmt.Println(wantArgs)
				fmt.Println(args[1:])
				t.Errorf("incorrect arguments for %v", args[0])
			}
		}
		execCommand = getFakeExecCommand(validator)

		err := test.r.MergePR(test.pr, test.sha)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Error = %v, want %v", err, test.err)
		}
	}
}

func TestCheckout(t *testing.T) {
	oldExecCommand := execCommand
	defer func() { execCommand = oldExecCommand }()

	currDir, err := os.Getwd()
	if err != nil {
		t.Errorf("getting working directory: %v", err)
	}

	tests := []struct {
		r   repo
		sha string
		err string
	}{
		{
			r: repo{
				scmURL: "https://github.com/screwdriver-cd/launcher.git",
				path:   currDir,
				branch: "master",
				sha:    testSHA,
				logger: os.Stderr,
			},
			sha: "abc123",
			err: "",
		},
		{
			r: repo{
				scmURL: "https://github.com/screwdriver-cd/launcher.git",
				path:   "bad path",
				branch: "master",
				logger: os.Stderr,
			},
			sha: "abc123",
			err: "chdir bad path: no such file or directory",
		},
	}

	for _, test := range tests {
		validator := func(command string, args ...string) {
			if command != "git" {
				t.Errorf("command = %q, want <git>", command)
			}
			if len(args) < 2 {
				t.Errorf("no arguments")
			}

			var wantArgs []string

			switch args[0] {
			case "clone":
				wantArgs = []string{"--quiet", "--progress", "--branch",
					test.r.branch, test.r.scmURL, test.r.path}
			case "config":
				switch args[1] {
				case "user.name":
					wantArgs = []string{"user.name", "sd-buildbot"}
				case "user.email":
					wantArgs = []string{"user.email", "dev-null@screwdriver.cd"}
				default:
					t.Errorf("incorrect argument for git config %v", args[1])
				}
			case "reset":
				wantArgs = []string{"--hard", test.r.sha}
			default:
				t.Errorf("unexpected git command: %v", args[0])
			}

			if !reflect.DeepEqual(args[1:], wantArgs) {
				t.Errorf("incorrect arguments for %v", args[0])
			}
		}
		execCommand = getFakeExecCommand(validator)

		err := test.r.Checkout()

		if test.err == "" {
			if err != nil {
				t.Errorf("Unexpected error from Checkout() for test %v: %v", test, err)
			}
		} else {
			if !strings.Contains(err.Error(), test.err) {
				t.Errorf("Want error %s, got %s for test %v", test.err, err.Error(), test)
			}
		}
	}
}

func TestNewRepo(t *testing.T) {
	testRepo := repo{
		scmURL: "test.com/org/repo",
		path:   "test/path",
		branch: "testBranch",
		sha:    testSHA,
		logger: os.Stderr,
	}
	testURL := fmt.Sprintf("%s#%s", testRepo.scmURL, testRepo.branch)
	r, err := New(testURL, testRepo.sha, testRepo.path, testRepo.logger)
	if err != nil {
		t.Fatalf("Error creating a new repo: %v", err)
	}

	if !reflect.DeepEqual(r, testRepo) {
		t.Errorf("repo=%v, want %v", r, testRepo)
	}
}
