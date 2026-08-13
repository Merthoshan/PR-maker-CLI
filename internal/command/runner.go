package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// ExecRunner executes commands using the local operating system.
type ExecRunner struct{}

// WrapError adds operation context and captured stderr to a command failure.
func WrapError(operation string, result Result, err error) error {
	if err == nil {
		return nil
	}
	stderr := strings.TrimSpace(result.Stderr)
	if stderr != "" {
		return fmt.Errorf("%s: %w: %s", operation, err, stderr)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

// Run executes one command and captures its standard output and standard error.
func (ExecRunner) Run(ctx context.Context, spec Spec) (Result, error) {
	return runExec(ctx, spec, nil)
}

// RunStreaming executes one command and reports complete stdout lines without
// waiting for the process to exit.
func (ExecRunner) RunStreaming(
	ctx context.Context,
	spec Spec,
	onStdoutLine func(string),
) (Result, error) {
	return runExec(ctx, spec, onStdoutLine)
}

func runExec(
	ctx context.Context,
	spec Spec,
	onStdoutLine func(string),
) (Result, error) {
	if strings.TrimSpace(spec.Name) == "" {
		return Result{}, errors.New("command name cannot be empty")
	}
	if spec.StdoutLimit < 0 {
		return Result{}, errors.New("command stdout limit cannot be negative")
	}

	cmd := exec.CommandContext(ctx, spec.Name, spec.Args...)
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}
	if len(spec.Env) > 0 {
		cmd.Env = append(cmd.Environ(), spec.Env...)
	}
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}

	var stdout bytes.Buffer
	var limitedStdout *limitedBuffer
	var stdoutWriter io.Writer = &stdout
	if spec.StdoutLimit > 0 {
		limitedStdout = &limitedBuffer{limit: spec.StdoutLimit}
		stdoutWriter = limitedStdout
	}
	var lines *lineWriter
	if onStdoutLine != nil {
		lines = &lineWriter{
			destination: stdoutWriter,
			onLine:      onStdoutLine,
		}
		stdoutWriter = lines
	}
	var stderr bytes.Buffer

	cmd.Stdout = stdoutWriter
	cmd.Stderr = &stderr

	err := cmd.Run()
	if lines != nil {
		lines.Flush()
	}

	result := Result{Stderr: stderr.String()}
	if limitedStdout != nil {
		result.Stdout = limitedStdout.String()
		result.StdoutTruncated = limitedStdout.truncated
	} else {
		result.Stdout = stdout.String()
	}

	if err != nil {
		return result, fmt.Errorf("run %q: %w", spec.Name, err)
	}

	return result, nil
}

// limitedBuffer drains all command output while retaining at most limit bytes.
// Returning the original write length prevents a verbose child process from
// blocking after the retained review evidence reaches its memory budget.
type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int64
	truncated bool
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := buffer.limit - int64(buffer.buffer.Len())
	if remaining <= 0 {
		if written > 0 {
			buffer.truncated = true
		}
		return written, nil
	}

	retained := int64(written)
	if retained > remaining {
		retained = remaining
		buffer.truncated = true
	}
	_, _ = buffer.buffer.Write(value[:int(retained)])
	return written, nil
}

func (buffer *limitedBuffer) String() string {
	return buffer.buffer.String()
}

type lineWriter struct {
	destination io.Writer
	onLine      func(string)
	pending     []byte
}

func (writer *lineWriter) Write(value []byte) (int, error) {
	written, err := writer.destination.Write(value)
	if err != nil {
		return written, err
	}
	if written != len(value) {
		return written, io.ErrShortWrite
	}
	writer.pending = append(writer.pending, value...)
	for {
		newline := bytes.IndexByte(writer.pending, '\n')
		if newline < 0 {
			break
		}
		writer.emit(writer.pending[:newline])
		writer.pending = writer.pending[newline+1:]
	}
	return len(value), nil
}

func (writer *lineWriter) Flush() {
	if len(writer.pending) == 0 {
		return
	}
	writer.emit(writer.pending)
	writer.pending = nil
}

func (writer *lineWriter) emit(line []byte) {
	writer.onLine(strings.TrimSuffix(string(line), "\r"))
}
