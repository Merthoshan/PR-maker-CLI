package updater

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
	"github.com/Merthoshan/PR-maker-CLI/internal/version"
)

func TestNewValidatesDependencies(t *testing.T) {
	validRunner := &updateRunner{t: t}
	validProgress := &updateProgress{}
	tests := []struct {
		name           string
		runner         command.Runner
		input          io.Reader
		output         io.Writer
		progress       progressReporter
		currentVersion string
		wantError      string
	}{
		{
			name:           "requires runner",
			input:          strings.NewReader(""),
			output:         &bytes.Buffer{},
			progress:       validProgress,
			currentVersion: "v1.0.0",
			wantError:      "runner is required",
		},
		{
			name:           "requires input",
			runner:         validRunner,
			output:         &bytes.Buffer{},
			progress:       validProgress,
			currentVersion: "v1.0.0",
			wantError:      "input is required",
		},
		{
			name:           "requires output",
			runner:         validRunner,
			input:          strings.NewReader(""),
			progress:       validProgress,
			currentVersion: "v1.0.0",
			wantError:      "output is required",
		},
		{
			name:           "requires progress",
			runner:         validRunner,
			input:          strings.NewReader(""),
			output:         &bytes.Buffer{},
			currentVersion: "v1.0.0",
			wantError:      "progress reporter is required",
		},
		{
			name:      "requires current version",
			runner:    validRunner,
			input:     strings.NewReader(""),
			output:    &bytes.Buffer{},
			progress:  validProgress,
			wantError: "current version is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updater, err := New(
				test.runner,
				test.input,
				test.output,
				test.progress,
				test.currentVersion,
			)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("New() error = %v, want %q", err, test.wantError)
			}
			if updater != nil {
				t.Fatalf("New() updater = %+v, want nil", updater)
			}
		})
	}
}

func TestRunSkipsDevelopmentBuild(t *testing.T) {
	runner := &updateRunner{t: t}
	output := &bytes.Buffer{}
	progress := &updateProgress{}
	updater := mustNewUpdater(
		t,
		runner,
		strings.NewReader("yes\n"),
		output,
		progress,
		version.Development,
	)

	outcome, err := updater.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if outcome.CurrentVersion != version.Development || outcome.Updated {
		t.Fatalf("Run() outcome = %+v, want skipped development build", outcome)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want zero", runner.calls)
	}
	if !strings.Contains(output.String(), "will not be overwritten") {
		t.Fatalf("output missing development guidance:\n%s", output.String())
	}
}

func TestRunReportsAlreadyCurrent(t *testing.T) {
	runner := &updateRunner{
		t: t,
		results: []updateRun{{
			want:   latestVersionSpec(),
			result: command.Result{Stdout: "v1.2.3\n"},
		}},
	}
	output := &bytes.Buffer{}
	progress := &updateProgress{}
	updater := mustNewUpdater(
		t,
		runner,
		strings.NewReader("yes\n"),
		output,
		progress,
		"v1.2.3",
	)

	outcome, err := updater.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if outcome.Updated || outcome.LatestVersion != "v1.2.3" {
		t.Fatalf("Run() outcome = %+v, want already current", outcome)
	}
	if !strings.Contains(output.String(), "already up to date") {
		t.Fatalf("output missing current-version message:\n%s", output.String())
	}
	runner.assertComplete()
	progress.assertBalanced(t)
}

func TestRunCancelsUnlessUserExplicitlyConfirms(t *testing.T) {
	for _, input := range []string{"", "\n", "n\n", "later\n"} {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			runner := &updateRunner{
				t: t,
				results: []updateRun{{
					want:   latestVersionSpec(),
					result: command.Result{Stdout: "v2.0.0"},
				}},
			}
			output := &bytes.Buffer{}
			progress := &updateProgress{}
			updater := mustNewUpdater(
				t,
				runner,
				strings.NewReader(input),
				output,
				progress,
				"v1.0.0",
			)

			outcome, err := updater.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}
			if !outcome.Cancelled || outcome.Updated {
				t.Fatalf("Run() outcome = %+v, want cancellation", outcome)
			}
			if !strings.Contains(output.String(), "Update cancelled") {
				t.Fatalf("output missing cancellation:\n%s", output.String())
			}
			runner.assertComplete()
			progress.assertBalanced(t)
		})
	}
}

func TestRunInstallsExactVersionAfterConfirmation(t *testing.T) {
	for _, confirmation := range []string{"y\n", "YES\n"} {
		t.Run(strings.TrimSpace(confirmation), func(t *testing.T) {
			runner := &updateRunner{
				t: t,
				results: []updateRun{
					{
						want:   latestVersionSpec(),
						result: command.Result{Stdout: "v1.4.0"},
					},
					{
						want: command.Spec{
							Name: "go",
							Args: []string{
								"install",
								packagePath + "@v1.4.0",
							},
						},
					},
				},
			}
			output := &bytes.Buffer{}
			progress := &updateProgress{}
			updater := mustNewUpdater(
				t,
				runner,
				strings.NewReader(confirmation),
				output,
				progress,
				"v1.3.9",
			)

			outcome, err := updater.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}
			if !outcome.Updated || outcome.LatestVersion != "v1.4.0" {
				t.Fatalf("Run() outcome = %+v, want successful update", outcome)
			}
			if !strings.Contains(output.String(), "Successfully updated to v1.4.0") {
				t.Fatalf("output missing success:\n%s", output.String())
			}
			runner.assertComplete()
			progress.assertBalanced(t)
		})
	}
}

func TestRunRejectsInvalidPublishedVersion(t *testing.T) {
	runner := &updateRunner{
		t: t,
		results: []updateRun{{
			want:   latestVersionSpec(),
			result: command.Result{Stdout: "v1.2.3-beta"},
		}},
	}
	updater := mustNewUpdater(
		t,
		runner,
		strings.NewReader("yes\n"),
		&bytes.Buffer{},
		&updateProgress{},
		"v1.0.0",
	)

	_, err := updater.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "publish a vMAJOR.MINOR.PATCH tag") {
		t.Fatalf("Run() error = %v, want invalid release guidance", err)
	}
}

func TestRunWrapsVersionCheckAndInstallFailures(t *testing.T) {
	sentinel := errors.New("command failed")
	tests := []struct {
		name      string
		results   []updateRun
		wantError string
	}{
		{
			name: "version check",
			results: []updateRun{{
				want:   latestVersionSpec(),
				result: command.Result{Stderr: "network unavailable"},
				err:    sentinel,
			}},
			wantError: "check latest champu-pr version",
		},
		{
			name: "installation",
			results: []updateRun{
				{
					want:   latestVersionSpec(),
					result: command.Result{Stdout: "v2.0.0"},
				},
				{
					want: command.Spec{
						Name: "go",
						Args: []string{"install", packagePath + "@v2.0.0"},
					},
					result: command.Result{Stderr: "permission denied"},
					err:    sentinel,
				},
			},
			wantError: "install champu-pr v2.0.0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updater := mustNewUpdater(
				t,
				&updateRunner{t: t, results: test.results},
				strings.NewReader("yes\n"),
				&bytes.Buffer{},
				&updateProgress{},
				"v1.0.0",
			)

			_, err := updater.Run(context.Background())
			if !errors.Is(err, sentinel) ||
				!strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Run() error = %v, want wrapped %q", err, test.wantError)
			}
		})
	}
}

type updateRun struct {
	want   command.Spec
	result command.Result
	err    error
}

type updateRunner struct {
	t       *testing.T
	results []updateRun
	calls   int
}

func (runner *updateRunner) Run(
	_ context.Context,
	spec command.Spec,
) (command.Result, error) {
	runner.t.Helper()
	if runner.calls >= len(runner.results) {
		runner.t.Fatalf("unexpected command: %+v", spec)
	}
	run := runner.results[runner.calls]
	runner.calls++
	actual := spec
	if len(run.want.Args) > 0 && run.want.Args[0] == "list" {
		if !strings.Contains(actual.Dir, "champu-pr-update-") {
			runner.t.Fatalf(
				"command directory = %q, want temporary update directory",
				actual.Dir,
			)
		}
		actual.Dir = ""
	}
	if !reflect.DeepEqual(actual, run.want) {
		runner.t.Fatalf("command = %+v, want %+v", actual, run.want)
	}
	return run.result, run.err
}

func (runner *updateRunner) assertComplete() {
	runner.t.Helper()
	if runner.calls != len(runner.results) {
		runner.t.Fatalf(
			"runner calls = %d, want %d",
			runner.calls,
			len(runner.results),
		)
	}
}

type updateProgress struct {
	started int
	stopped int
}

func (progress *updateProgress) Start(string) func() {
	progress.started++
	stopped := false
	return func() {
		if stopped {
			return
		}
		stopped = true
		progress.stopped++
	}
}

func (progress *updateProgress) assertBalanced(t *testing.T) {
	t.Helper()
	if progress.started != progress.stopped {
		t.Fatalf(
			"progress started = %d, stopped = %d",
			progress.started,
			progress.stopped,
		)
	}
}

func mustNewUpdater(
	t *testing.T,
	runner command.Runner,
	input *strings.Reader,
	output *bytes.Buffer,
	progress progressReporter,
	currentVersion string,
) *Updater {
	t.Helper()
	updater, err := New(runner, input, output, progress, currentVersion)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	return updater
}

func latestVersionSpec() command.Spec {
	return command.Spec{
		Name: "go",
		Args: []string{
			"list",
			"-m",
			"-f", "{{.Version}}",
			modulePath + "@latest",
		},
	}
}
