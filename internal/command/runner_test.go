package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestExecRunnerRun(t *testing.T) {
	runner := ExecRunner{}

	t.Run("rejects empty command name", func(t *testing.T) {
		result, err := runner.Run(context.Background(), Spec{})
		if err == nil {
			t.Fatal("Run() error = nil, want an error")
		}
		if !strings.Contains(err.Error(), "command name cannot be empty") {
			t.Fatalf("Run() error = %q, want empty-command error", err)
		}
		if result != (Result{}) {
			t.Fatalf("Run() result = %+v, want zero result", result)
		}
	})

	t.Run("captures stdout and stderr separately", func(t *testing.T) {
		result, err := runner.Run(
			context.Background(),
			helperCommand("output"),
		)
		if err != nil {
			t.Fatalf("Run() unexpected error: %v", err)
		}
		if result.Stdout != "stdout message\n" {
			t.Fatalf("Run() stdout = %q, want %q", result.Stdout, "stdout message\n")
		}
		if result.Stderr != "stderr message\n" {
			t.Fatalf("Run() stderr = %q, want %q", result.Stderr, "stderr message\n")
		}
	})

	t.Run("forwards stdin", func(t *testing.T) {
		spec := helperCommand("stdin")
		spec.Stdin = "input sent to child process\n"

		result, err := runner.Run(context.Background(), spec)
		if err != nil {
			t.Fatalf("Run() unexpected error: %v", err)
		}
		if result.Stdout != spec.Stdin {
			t.Fatalf("Run() stdout = %q, want stdin %q", result.Stdout, spec.Stdin)
		}
		if result.Stderr != "" {
			t.Fatalf("Run() stderr = %q, want empty stderr", result.Stderr)
		}
	})

	t.Run("uses specified working directory", func(t *testing.T) {
		workingDirectory := t.TempDir()
		spec := helperCommand("directory")
		spec.Dir = workingDirectory

		result, err := runner.Run(context.Background(), spec)
		if err != nil {
			t.Fatalf("Run() unexpected error: %v", err)
		}
		if strings.TrimSpace(result.Stdout) != workingDirectory {
			t.Fatalf(
				"Run() working directory = %q, want %q",
				strings.TrimSpace(result.Stdout),
				workingDirectory,
			)
		}
	})

	t.Run("returns output from failed command", func(t *testing.T) {
		spec := helperCommand("failure")
		result, err := runner.Run(context.Background(), spec)

		if err == nil {
			t.Fatal("Run() error = nil, want an exit error")
		}
		if !strings.Contains(err.Error(), spec.Name) {
			t.Fatalf("Run() error = %q, want command name %q", err, spec.Name)
		}

		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("Run() error type = %T, want wrapped *exec.ExitError", err)
		}
		if exitError.ExitCode() != 7 {
			t.Fatalf("Run() exit code = %d, want 7", exitError.ExitCode())
		}
		if result.Stdout != "stdout before failure\n" {
			t.Fatalf("Run() stdout = %q, want preserved failure stdout", result.Stdout)
		}
		if result.Stderr != "stderr before failure\n" {
			t.Fatalf("Run() stderr = %q, want preserved failure stderr", result.Stderr)
		}
	})
}

func TestWrapError(t *testing.T) {
	sentinel := errors.New("failed")
	err := WrapError(
		"create pull request",
		Result{Stderr: " authentication required\n"},
		sentinel,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("WrapError() = %v, want wrapped sentinel", err)
	}
	if !strings.Contains(err.Error(), "create pull request") ||
		!strings.Contains(err.Error(), "authentication required") {
		t.Fatalf("WrapError() = %q, want operation and stderr", err)
	}
	if err := WrapError("ignored", Result{}, nil); err != nil {
		t.Fatalf("WrapError(nil) = %v, want nil", err)
	}
}

func TestExecRunnerContextCancellation(t *testing.T) {
	runner := ExecRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	startedAt := time.Now()
	_, err := runner.Run(ctx, helperCommand("wait"))
	elapsed := time.Since(startedAt)

	if err == nil {
		t.Fatal("Run() error = nil, want cancellation error")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("context error = %v, want context deadline exceeded", ctx.Err())
	}
	if elapsed >= 2*time.Second {
		t.Fatalf("Run() cancellation took %v, want less than 2s", elapsed)
	}
}

func helperCommand(action string) Spec {
	return Spec{
		Name: os.Args[0],
		Args: []string{
			"-test.run=^TestHelperProcess$",
			"--",
			action,
		},
	}
}

// TestHelperProcess runs in a child copy of this test executable. The parent
// tests use it as a predictable external command without depending on a shell.
func TestHelperProcess(t *testing.T) {
	separatorIndex := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separatorIndex = index
			break
		}
	}
	if separatorIndex == -1 || separatorIndex+1 >= len(os.Args) {
		return
	}

	action := os.Args[separatorIndex+1]
	switch action {
	case "output":
		fmt.Fprintln(os.Stdout, "stdout message")
		fmt.Fprintln(os.Stderr, "stderr message")
		os.Exit(0)
	case "stdin":
		if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	case "directory":
		workingDirectory, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stdout, workingDirectory)
		os.Exit(0)
	case "failure":
		fmt.Fprintln(os.Stdout, "stdout before failure")
		fmt.Fprintln(os.Stderr, "stderr before failure")
		os.Exit(7)
	case "wait":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper action: %s\n", action)
		os.Exit(2)
	}
}
