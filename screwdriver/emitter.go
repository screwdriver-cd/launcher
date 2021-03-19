package screwdriver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Emitter is an io.WriteCloser that knows about CommandDef
type Emitter interface {
	StartCmd(cmd CommandDef)
	io.WriteCloser
	Error() error
}

type emitter struct {
	file   *os.File
	cmd    CommandDef
	buffer *bytes.Buffer
	reader io.Reader
	*io.PipeWriter
	err error
}

type logLine struct {
	Time    int64  `json:"t"`
	Message string `json:"m"`
	Step    string `json:"s"`
}

// Error gets the latest error from the emitter
func (e *emitter) Error() error {
	return e.err
}

// StartCmd switches the currently running step for the Emitter
func (e *emitter) StartCmd(cmd CommandDef) {
	e.cmd = cmd
}

// Returns a single line (without the ending \n) from the input buffered reader
// Pulled from https://stackoverflow.com/a/12206365
func readln(r *bufio.Reader) (string, error) {
	var (
		isPrefix = true
		err      error
		line, ln []byte
	)

	for isPrefix && err == nil {
		line, isPrefix, err = r.ReadLine()
		ln = append(ln, line...)
	}

	return string(ln), err
}

func (e *emitter) processPipe() {
	var line string
	var readErr error

	// temporary hack - without this delay the datetime is printing incorrectly for kata containers
	time.Sleep(15 * time.Second)
	//

	reader := bufio.NewReader(e.reader)
	encoder := json.NewEncoder(e.file)

	line, readErr = readln(reader)

	for readErr == nil {
		newLine := logLine{
			Time:    time.Now().UnixNano() / int64(time.Millisecond),
			Message: line,
			Step:    e.cmd.Name,
		}
		if err := encoder.Encode(newLine); err != nil {
			e.err = fmt.Errorf("Encoding json: %v", err)
		}

		line, readErr = readln(reader)
	}

	if readErr != nil && readErr.Error() != "EOF" {
		e.err = fmt.Errorf("Piping log line to emitter: %v", readErr)
	}

	if err := e.file.Close(); err != nil {
		e.err = err
	}
}

// NewEmitter returns an emitter object from an emitter destination path
func NewEmitter(path string) (Emitter, error) {
	r, w := io.Pipe()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("Failed opening emitter path %q: %v", path, err)
	}

	cmd := CommandDef{
		Name: "sd-setup-launcher",
	}

	e := &emitter{
		file:       file,
		buffer:     bytes.NewBuffer([]byte{}),
		reader:     r,
		PipeWriter: w,
		cmd:        cmd,
	}

	go e.processPipe()

	return e, nil
}
