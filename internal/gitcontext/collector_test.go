package gitcontext

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"champu-pr/internal/command"
)

func TestCollectorCollect(t *testing.T) {
	t.Run("collects clean repository context", func(t *testing.T) {
		runner := newGitRunner(
			t,
			"/work/gallery",
			"/repo/gallery\n",
			"feature/crop\n",
			"abc123\n",
			"git@github.com:aftershootco/gallery-go-backend.git\n",
			"",
		)

		collector := mustNewCollector(t, runner)
		repository, err := collector.Collect(
			context.Background(),
			"/work/gallery",
		)
		if err != nil {
			t.Fatalf("Collect() unexpected error: %v", err)
		}

		want := Repository{
			Root:      "/repo/gallery",
			Branch:    "feature/crop",
			HeadSHA:   "abc123",
			RemoteURL: "git@github.com:aftershootco/gallery-go-backend.git",
			Dirty:     false,
		}
		if repository != want {
			t.Fatalf("Collect() = %+v, want %+v", repository, want)
		}
		runner.assertComplete()
	})

	t.Run("marks repository dirty", func(t *testing.T) {
		runner := newGitRunner(
			t,
			"/work/gallery",
			"/repo/gallery",
			"feature/crop",
			"abc123",
			"https://github.com/aftershootco/gallery-go-backend.git",
			" M internal/gitcontext/collector.go\n?? notes.txt\n",
		)

		collector := mustNewCollector(t, runner)
		repository, err := collector.Collect(
			context.Background(),
			"/work/gallery",
		)
		if err != nil {
			t.Fatalf("Collect() unexpected error: %v", err)
		}
		if !repository.Dirty {
			t.Fatal("Collect() Dirty = false, want true")
		}
		runner.assertComplete()
	})
}

func TestCollectorCollectValidation(t *testing.T) {
	t.Run("constructor requires runner", func(t *testing.T) {
		_, err := NewCollector(nil)
		if err == nil {
			t.Fatal("NewCollector() error = nil, want runner validation error")
		}
		if !strings.Contains(err.Error(), "runner is required") {
			t.Fatalf("NewCollector() error = %q, want runner validation error", err)
		}
	})

	t.Run("zero-value collector requires runner", func(t *testing.T) {
		_, err := (Collector{}).Collect(context.Background(), "/work/gallery")
		if err == nil {
			t.Fatal("Collect() error = nil, want runner validation error")
		}
		if !strings.Contains(err.Error(), "runner is required") {
			t.Fatalf("Collect() error = %q, want runner validation error", err)
		}
	})

	t.Run("rejects empty repository root", func(t *testing.T) {
		runner := &scriptedRunner{
			t: t,
			steps: []runnerStep{
				{
					want: command.Spec{
						Name: "git",
						Args: []string{"rev-parse", "--show-toplevel"},
						Dir:  "/work/gallery",
					},
					result: command.Result{Stdout: " \n"},
				},
			},
		}

		collector := mustNewCollector(t, runner)
		_, err := collector.Collect(context.Background(), "/work/gallery")
		if err == nil {
			t.Fatal("Collect() error = nil, want empty-root error")
		}
		if !strings.Contains(err.Error(), "repository root is empty") {
			t.Fatalf("Collect() error = %q, want empty-root error", err)
		}
		runner.assertComplete()
	})

	t.Run("rejects detached HEAD", func(t *testing.T) {
		runner := &scriptedRunner{
			t: t,
			steps: []runnerStep{
				{
					want: command.Spec{
						Name: "git",
						Args: []string{"rev-parse", "--show-toplevel"},
						Dir:  "/work/gallery",
					},
					result: command.Result{Stdout: "/repo/gallery\n"},
				},
				{
					want: command.Spec{
						Name: "git",
						Args: []string{"branch", "--show-current"},
						Dir:  "/repo/gallery",
					},
					result: command.Result{},
				},
			},
		}

		collector := mustNewCollector(t, runner)
		_, err := collector.Collect(context.Background(), "/work/gallery")
		if err == nil {
			t.Fatal("Collect() error = nil, want detached-HEAD error")
		}
		if !strings.Contains(err.Error(), "detached HEAD") {
			t.Fatalf("Collect() error = %q, want detached-HEAD error", err)
		}
		runner.assertComplete()
	})
}

func TestCollectorCollectStopsAtGitFailure(t *testing.T) {
	operations := []struct {
		name string
		args []string
	}{
		{name: "repository root", args: []string{"rev-parse", "--show-toplevel"}},
		{name: "current branch", args: []string{"branch", "--show-current"}},
		{name: "HEAD SHA", args: []string{"rev-parse", "HEAD"}},
		{name: "origin URL", args: []string{"remote", "get-url", "origin"}},
		{name: "working tree status", args: []string{"status", "--porcelain"}},
	}

	for failureIndex, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			sentinel := errors.New("git command failed")
			runner := newGitRunner(
				t,
				"/work/gallery",
				"/repo/gallery",
				"feature/crop",
				"abc123",
				"git@github.com:aftershootco/gallery-go-backend.git",
				"",
			)
			runner.steps[failureIndex].err = sentinel
			runner.steps = runner.steps[:failureIndex+1]

			collector := mustNewCollector(t, runner)
			repository, err := collector.Collect(
				context.Background(),
				"/work/gallery",
			)
			if err == nil {
				t.Fatal("Collect() error = nil, want Git command error")
			}
			if !errors.Is(err, sentinel) {
				t.Fatalf("Collect() error = %v, want wrapped sentinel error", err)
			}
			if !strings.Contains(err.Error(), strings.Join(operation.args, " ")) {
				t.Fatalf(
					"Collect() error = %q, want Git arguments %q",
					err,
					strings.Join(operation.args, " "),
				)
			}
			if repository != (Repository{}) {
				t.Fatalf("Collect() repository = %+v, want zero value", repository)
			}
			runner.assertComplete()
		})
	}
}

type runnerStep struct {
	want   command.Spec
	result command.Result
	err    error
}

type scriptedRunner struct {
	t         *testing.T
	steps     []runnerStep
	nextIndex int
}

func (runner *scriptedRunner) Run(
	_ context.Context,
	spec command.Spec,
) (command.Result, error) {
	runner.t.Helper()

	if runner.nextIndex >= len(runner.steps) {
		runner.t.Fatalf("unexpected command: %+v", spec)
	}

	step := runner.steps[runner.nextIndex]
	runner.nextIndex++

	if spec.Name != step.want.Name ||
		spec.Dir != step.want.Dir ||
		spec.Stdin != step.want.Stdin ||
		!slices.Equal(spec.Args, step.want.Args) {
		runner.t.Fatalf("command = %+v, want %+v", spec, step.want)
	}

	return step.result, step.err
}

func (runner *scriptedRunner) assertComplete() {
	runner.t.Helper()
	if runner.nextIndex != len(runner.steps) {
		runner.t.Fatalf(
			"executed %d commands, want %d",
			runner.nextIndex,
			len(runner.steps),
		)
	}
}

func mustNewCollector(t *testing.T, runner command.Runner) Collector {
	t.Helper()

	collector, err := NewCollector(runner)
	if err != nil {
		t.Fatalf("NewCollector() unexpected error: %v", err)
	}
	if collector.runner == nil {
		t.Fatal("NewCollector() runner is nil")
	}

	return collector
}

func newGitRunner(t *testing.T, workingDirectory string, root string, branch string, headSHA string, remoteURL string, status string) *scriptedRunner {
	t.Helper()

	return &scriptedRunner{
		t: t,
		steps: []runnerStep{
			{
				want: command.Spec{
					Name: "git",
					Args: []string{"rev-parse", "--show-toplevel"},
					Dir:  workingDirectory,
				},
				result: command.Result{Stdout: root},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{"branch", "--show-current"},
					Dir:  strings.TrimSpace(root),
				},
				result: command.Result{Stdout: branch},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{"rev-parse", "HEAD"},
					Dir:  strings.TrimSpace(root),
				},
				result: command.Result{Stdout: headSHA},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{"remote", "get-url", "origin"},
					Dir:  strings.TrimSpace(root),
				},
				result: command.Result{Stdout: remoteURL},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{"status", "--porcelain"},
					Dir:  strings.TrimSpace(root),
				},
				result: command.Result{Stdout: status},
			},
		},
	}
}
