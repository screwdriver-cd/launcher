package screwdriver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"
)

func fakeCmd(name string) CommandDef {
	return CommandDef{
		Name: name,
		Cmd:  name,
	}

}

func TestEmitter(t *testing.T) {
	tmp, err := ioutil.TempDir("", "emitter")
	if err != nil {
		t.Fatalf("Couldn't create temp dir: %v", err)
	}
	fmt.Println("TEMPDIR:", tmp)
	defer os.RemoveAll(tmp)

	emitterpath := path.Join(tmp, "socket")
	if _, err = os.Create(emitterpath); err != nil {
		t.Fatalf("Error creating test socket: %v", err)
	}

	emitter, err := NewEmitter(emitterpath)
	if err != nil {
		t.Fatalf("Error creating emitter: %v", err)
	}
	defer emitter.Close()

	type testlist []struct {
		message string
		step    string
	}
	var tests = testlist{
		{"line1", "step1"},
		{"line2", "step1"},
		{"line3", "step2"},
		{"line4", "step2"},
	}

	fmt.Fprintln(emitter, "running sd-setup-launcher step")
	time.Sleep(1 * time.Millisecond)
	for _, test := range tests {
		emitter.StartCmd(fakeCmd(test.step))
		fmt.Fprintln(emitter, test.message)
		time.Sleep(1 * time.Millisecond)
	}
	emitter.Write([]byte("This should not be processed. It has no newline."))

	tests = append(testlist{{"running sd-setup-launcher step", "sd-setup-launcher"}}, tests...)

	f, err := os.Open(emitterpath)
	if err != nil {
		t.Fatalf("Error opening file: %v", err)
	}

	_, _ = ioutil.ReadFile(emitterpath)
	scanner := bufio.NewScanner(f)
	line := 0
	var log logLine
	var prev int64

	for scanner.Scan() {
		text := scanner.Text()
		err := json.Unmarshal([]byte(text), &log)
		if err != nil {
			t.Errorf("error unmarshalling %v", err)
		}

		if line >= len(tests) {
			t.Fatalf("Too many lines received. Want %d, got %d", len(tests), line+1)
		}

		if log.Step != tests[line].step {
			t.Errorf("step is incorrect. Wanted %v. Got %v", tests[line].step, log.Step)
		}
		if log.Message != tests[line].message {
			t.Errorf("message is incorrect. Wanted %v. Got %v", tests[line].message, log.Message)
		}
		if log.Time <= prev {
			t.Errorf("timestamp is decreasing. Wanted %v greater than %v", log.Time, prev)
		}
		prev = log.Time
		line++
	}

	if line != len(tests) {
		t.Errorf("file does not contain correct number lines. Wanted %v. Got %v", len(tests), line)
	}
}
