package main

import (
	"fmt"
	"strings"
	"testing"
)

var shellOutSetup map[string]string

type shellOutMock struct{}

func (s shellOutMock) run(cmd string, args ...string) (out []byte, err error) {
	cmdslice := []string{cmd}
	cmdslice = append(cmdslice, args...)

	cmd = strings.Join(cmdslice, " ")
	output := shellOutSetup[cmd]
	if output == "" {
		return nil, fmt.Errorf("Error: %s not mocked", cmd)
	}

	return []byte(output), nil
}

func Test(t *testing.T) {
	shellOut = shellOutMock{}

	shellOutSetup = map[string]string{
		"/opt/screwdriver/launch.sh abc": "abc",
	}

	output, err := runLauncher("abc")
	if string(output) != shellOutSetup["/opt/screwdriver/launch.sh abc"] {
		errmsg := fmt.Sprintf("Expected %s, got %s", shellOutSetup["echo"], output)
		t.Error(errmsg)
	}
	if err != nil {
		errmsg := fmt.Sprintf("Error running command: %s", err)
		t.Error(errmsg)
	}
}
