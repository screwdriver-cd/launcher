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
}

type emitter struct {
	file   *os.File
	cmd    CommandDef
	buffer *bytes.Buffer
}

type logLine struct {
	Time    int64  `json:"t"`
	Message string `json:"m"`
	Step    string `json:"s"`
}

// Write implements the io.Writer interface for writing to the emitter
func (e *emitter) Write(p []byte) (int, error) {
	n, err := e.buffer.Write(p)

	encoder := json.NewEncoder(e.file)

	scanner := bufio.NewScanner(e.buffer)
	for scanner.Scan() {
		newLine := logLine{
			Time:    time.Now().UnixNano() / 1000,
			Message: scanner.Text(),
			Step:    e.cmd.Name,
		}
		if err = encoder.Encode(newLine); err != nil {
			return n, fmt.Errorf("encoding json: %v", err)
		}
	}
	return n, err
}

// StartCmd switches the currently running step for the Emitter
func (e *emitter) StartCmd(cmd CommandDef) {
	e.cmd = cmd
}

// Close closes the emitter and its underlying file handle
func (e *emitter) Close() error {
	return e.file.Close()
}

// NewEmitter returns an emitter object from an emitter destination path
func NewEmitter(path string) (Emitter, error) {
	file, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed opening emitter path %q: %v", path, err)
	}

	e := emitter{
		file:   file,
		buffer: bytes.NewBuffer([]byte{}),
	}

	return &e, nil
}
