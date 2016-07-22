package main

import (
	"fmt"
	"os"
	"os/exec"
)

type shellOutInterface interface {
	run(string, ...string) ([]byte, error)
}

var shellOut shellOutInterface

type shellOutImpl struct{}

func init() {
	shellOut = shellOutImpl{}
}

func (s shellOutImpl) run(cmd string, args ...string) (out []byte, err error) {
	return exec.Command(cmd, args...).CombinedOutput()
}

func runLauncher(args ...string) (out []byte, err error) {
	cmdName := "/opt/screwdriver/launch.sh"
	return shellOut.run(cmdName, args...)
}

func main() {
	var out []byte
	var err error

	fmt.Println("Launching the launcher...")
	if out, err = runLauncher(os.Args[1:]...); err != nil {
		fmt.Fprintln(os.Stderr, "There was an error shelling out to the launcher: ", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
	fmt.Println("Launched successfully.")
}
