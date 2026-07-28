package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Spec describes one external process invocation.
type Spec struct {
	Name  string
	Args  []string
	Dir   string
	Stdin string
}

// Result contains the output produced by an external process.
type Result struct {
	Stdout string
	Stderr string
}

// Runner executes external commands.
type Runner interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}

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
	if strings.TrimSpace(spec.Name) == "" {
		return Result{}, errors.New("command name cannot be empty")
	}

	cmd := exec.CommandContext(ctx, spec.Name, spec.Args...)
	if spec.Dir != "" {
		cmd.Dir = spec.Dir
	}
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		return result, fmt.Errorf("run %q: %w", spec.Name, err)
	}

	return result, nil
}
