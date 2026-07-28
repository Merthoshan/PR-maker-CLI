package gitcontext

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"champu-pr/internal/command"
)

func TestCollectorCollectEvidence(t *testing.T) {
	runner := newSuccessfulEvidenceRunner(t)
	collector := mustNewEvidenceCollector(t, runner)

	evidence, err := collector.CollectEvidence(
		context.Background(),
		"  /repo/gallery  ",
		"  main  ",
	)
	if err != nil {
		t.Fatalf("CollectEvidence() unexpected error: %v", err)
	}

	want := Evidence{
		BaseBranch:   "main",
		BaseRef:      "refs/remotes/origin/main",
		MergeBaseSHA: "base123",
		CommitLog:    "head456\tAdd collection workflow",
		ChangedFiles: "M\tinternal/workflow/target.go",
		Diff:         "diff --git a/target.go b/target.go",
	}
	if evidence != want {
		t.Fatalf("CollectEvidence() = %+v, want %+v", evidence, want)
	}
	runner.assertComplete()
}

func TestCollectorCollectEvidenceRequiresRunner(t *testing.T) {
	_, err := (Collector{}).CollectEvidence(
		context.Background(),
		"/repo/gallery",
		"main",
	)
	if err == nil || !strings.Contains(err.Error(), "runner is required") {
		t.Fatalf("CollectEvidence() error = %v, want runner validation", err)
	}
}

func TestCollectorCollectEvidenceValidation(t *testing.T) {
	tests := []struct {
		name           string
		repositoryRoot string
		baseBranch     string
		wantError      string
	}{
		{
			name:       "requires repository root",
			baseBranch: "main",
			wantError:  "repository root is required",
		},
		{
			name:           "rejects whitespace repository root",
			repositoryRoot: " \n\t ",
			baseBranch:     "main",
			wantError:      "repository root is required",
		},
		{
			name:           "requires base branch",
			repositoryRoot: "/repo/gallery",
			wantError:      "base branch is required",
		},
		{
			name:           "rejects whitespace base branch",
			repositoryRoot: "/repo/gallery",
			baseBranch:     " \n\t ",
			wantError:      "base branch is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &evidenceRunner{t: t}
			collector := mustNewEvidenceCollector(t, runner)

			evidence, err := collector.CollectEvidence(
				context.Background(),
				test.repositoryRoot,
				test.baseBranch,
			)
			if err == nil {
				t.Fatal("CollectEvidence() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("CollectEvidence() error = %q, want %q", err, test.wantError)
			}
			if evidence != (Evidence{}) {
				t.Fatalf("CollectEvidence() = %+v, want zero evidence", evidence)
			}
			if runner.nextIndex != 0 {
				t.Fatalf("runner calls = %d, want 0", runner.nextIndex)
			}
		})
	}
}

func TestCollectorCollectEvidenceRequiresMergeBase(t *testing.T) {
	runner := newSuccessfulEvidenceRunner(t)
	runner.steps[1].result.Stdout = " \n"
	runner.steps = runner.steps[:2]
	collector := mustNewEvidenceCollector(t, runner)

	evidence, err := collector.CollectEvidence(
		context.Background(),
		"/repo/gallery",
		"main",
	)
	if err == nil {
		t.Fatal("CollectEvidence() error = nil, want empty merge-base error")
	}
	if !strings.Contains(err.Error(), `merge base for "main" is empty`) {
		t.Fatalf("CollectEvidence() error = %q, want empty merge-base context", err)
	}
	if evidence != (Evidence{}) {
		t.Fatalf("CollectEvidence() = %+v, want zero evidence", evidence)
	}
	runner.assertComplete()
}

func TestCollectorCollectEvidenceStopsAtGitFailure(t *testing.T) {
	operations := []struct {
		name        string
		errorDetail string
	}{
		{name: "fetch base", errorDetail: "refresh base branch"},
		{name: "find merge base", errorDetail: "find merge base"},
		{name: "collect commits", errorDetail: "collect commit log"},
		{name: "collect changed files", errorDetail: "collect changed files"},
		{name: "collect diff", errorDetail: "collect textual diff"},
	}

	for failureIndex, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			sentinel := errors.New("git failed")
			runner := newSuccessfulEvidenceRunner(t)
			runner.steps[failureIndex].err = sentinel
			runner.steps[failureIndex].result.Stderr = "fatal: example failure\n"
			runner.steps = runner.steps[:failureIndex+1]
			collector := mustNewEvidenceCollector(t, runner)

			evidence, err := collector.CollectEvidence(
				context.Background(),
				"/repo/gallery",
				"main",
			)
			if err == nil {
				t.Fatal("CollectEvidence() error = nil, want Git command error")
			}
			if !errors.Is(err, sentinel) {
				t.Fatalf("CollectEvidence() error = %v, want wrapped sentinel error", err)
			}
			if !strings.Contains(err.Error(), operation.errorDetail) ||
				!strings.Contains(err.Error(), "fatal: example failure") {
				t.Fatalf(
					"CollectEvidence() error = %q, want operation and stderr context",
					err,
				)
			}
			if evidence != (Evidence{}) {
				t.Fatalf("CollectEvidence() = %+v, want zero evidence", evidence)
			}
			runner.assertComplete()
		})
	}
}

type evidenceStep struct {
	want   command.Spec
	result command.Result
	err    error
}

type evidenceRunner struct {
	t         *testing.T
	steps     []evidenceStep
	nextIndex int
}

func (runner *evidenceRunner) Run(
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

func (runner *evidenceRunner) assertComplete() {
	runner.t.Helper()
	if runner.nextIndex != len(runner.steps) {
		runner.t.Fatalf(
			"executed %d commands, want %d",
			runner.nextIndex,
			len(runner.steps),
		)
	}
}

func mustNewEvidenceCollector(
	t *testing.T,
	runner command.Runner,
) Collector {
	t.Helper()

	collector, err := NewCollector(runner)
	if err != nil {
		t.Fatalf("NewCollector() unexpected error: %v", err)
	}

	return collector
}

func newSuccessfulEvidenceRunner(t *testing.T) *evidenceRunner {
	t.Helper()

	const repositoryRoot = "/repo/gallery"
	const baseRef = "refs/remotes/origin/main"
	const revisionRange = "base123..HEAD"

	return &evidenceRunner{
		t: t,
		steps: []evidenceStep{
			{
				want: command.Spec{
					Name: "git",
					Args: []string{
						"fetch",
						"--quiet",
						"origin",
						"refs/heads/main:refs/remotes/origin/main",
					},
					Dir: repositoryRoot,
				},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{
						"merge-base",
						"HEAD",
						baseRef,
					},
					Dir: repositoryRoot,
				},
				result: command.Result{Stdout: " base123\n"},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{
						"log",
						"--format=%H%x09%s",
						revisionRange,
					},
					Dir: repositoryRoot,
				},
				result: command.Result{
					Stdout: "head456\tAdd collection workflow\n",
				},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{
						"diff",
						"--name-status",
						"--find-renames",
						revisionRange,
						"--",
					},
					Dir: repositoryRoot,
				},
				result: command.Result{
					Stdout: "M\tinternal/workflow/target.go\n",
				},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{
						"diff",
						"--no-ext-diff",
						"--no-color",
						"--find-renames",
						revisionRange,
						"--",
					},
					Dir: repositoryRoot,
				},
				result: command.Result{
					Stdout: "diff --git a/target.go b/target.go\n",
				},
			},
		},
	}
}
