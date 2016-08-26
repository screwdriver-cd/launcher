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

// Close closes the emitter and its underlying handles
func (e *emitter) Close() error {
	return e.PipeWriter.Close()
}

func (e *emitter) processPipe() {
	scanner := bufio.NewScanner(e.reader)
	encoder := json.NewEncoder(e.file)

	for scanner.Scan() {
		newLine := logLine{
			Time:    time.Now().UnixNano() / 1000,
			Message: scanner.Text(),
			Step:    e.cmd.Name,
		}
		if err := encoder.Encode(newLine); err != nil {
			e.err = fmt.Errorf("encoding json: %v", err)
		}
	}

	if err := e.file.Close(); err != nil {
		e.err = err
	}
}

// NewEmitter returns an emitter object from an emitter destination path
func NewEmitter(path string) (Emitter, error) {
	r, w := io.Pipe()
	file, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed opening emitter path %q: %v", path, err)
	}

	e := &emitter{
		file:       file,
		buffer:     bytes.NewBuffer([]byte{}),
		reader:     r,
		PipeWriter: w,
	}

	go e.processPipe()

	return e, nil
}
