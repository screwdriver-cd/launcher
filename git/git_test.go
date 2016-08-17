package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"testing"
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
		}
	}
	os.Exit(255)
}

func TestGetPath(t *testing.T) {
	testRepo := repo{
		ScmURL: "test.com",
		Path:   "test/path",
		Branch: "testBranch",
	}

	wantPath := "test/path"

	if path := testRepo.GetPath(); path != wantPath {
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
				ScmURL: "https://github.com/screwdriver-cd/launcher.git",
				Path:   currDir,
				Branch: "master",
			},
			pr:  "1",
			sha: "abc123",
			err: nil,
		},
		{
			r: repo{
				ScmURL: "https://github.com/screwdriver-cd/launcher.git",
				Path:   "bad path",
				Branch: "master",
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
		err error
	}{
		{
			r: repo{
				ScmURL: "https://github.com/screwdriver-cd/launcher.git",
				Path:   currDir,
				Branch: "master",
			},
			sha: "abc123",
			err: nil,
		},
		{
			r: repo{
				ScmURL: "https://github.com/screwdriver-cd/launcher.git",
				Path:   "bad path",
				Branch: "master",
			},
			sha: "abc123",
			err: errors.New("setting user name: starting git command: " +
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
			case "clone":
				wantArgs = []string{"--quiet", "--progress", "--branch",
					test.r.Branch, test.r.ScmURL, test.r.Path}
			case "config":
				switch args[1] {
				case "user.name":
					wantArgs = []string{"user.name", "sd-buildbot"}
				case "user.email":
					wantArgs = []string{"user.email", "dev-null@screwdriver.cd"}
				default:
					t.Errorf("incorrect argument for git config %v", args[1])
				}
			default:
				t.Errorf("incorrect git command: %v", args[0])
			}

			if !reflect.DeepEqual(args[1:], wantArgs) {
				t.Errorf("incorrect arguments for %v", args[0])
			}
		}
		execCommand = getFakeExecCommand(validator)

		err := test.r.Checkout()

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Error = %v, want %v", err, test.err)
		}
	}
}
