package main

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
	"github.com/Merthoshan/PR-maker-CLI/internal/version"
)

func TestRunDryRunIntegration(t *testing.T) {
	root := t.TempDir()
	draft := map[string]any{
		"title":   "Add PR workflow",
		"summary": []string{"Generate a PR description."},
		"changes": []map[string]any{{
			"id":        "F1.C1",
			"file":      "main.go",
			"operation": "modified",
			"element":   "main",
			"summary":   "Run the workflow.",
		}},
		"testing": []string{"Not run (no test results provided)."},
	}
	draftJSON, _ := json.Marshal(draft)
	runner := &integrationRunner{
		t:     t,
		root:  root,
		draft: string(draftJSON),
	}
	var output bytes.Buffer
	var errorOutput bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{"--dry-run"},
		root,
		strings.NewReader(""),
		&output,
		&errorOutput,
		runner,
	)
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %s", exitCode, errorOutput.String())
	}
	if !strings.Contains(output.String(), "Dry run: GitHub was not changed") {
		t.Fatalf("output missing dry-run result:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "File-wise changelog:") {
		t.Fatalf("output missing changelog label:\n%s", output.String())
	}
	if runner.mutations != 0 {
		t.Fatalf("GitHub mutations = %d, want zero", runner.mutations)
	}
	for _, status := range []string{
		"Inspecting Git repository...",
		"Finding open pull requests...",
		"Collecting Git evidence...",
		"Generating PR description with Codex...",
	} {
		if !strings.Contains(errorOutput.String(), status) {
			t.Fatalf(
				"progress output missing %q:\n%s",
				status,
				errorOutput.String(),
			)
		}
	}
	if strings.Contains(output.String(), "Inspecting Git repository") {
		t.Fatalf("stdout contains progress output:\n%s", output.String())
	}
}

func TestRunCancellationDoesNotExposeCodexPayload(t *testing.T) {
	const sensitivePayload = "private diff and existing PR body"
	runner := &integrationRunner{
		t:           t,
		root:        t.TempDir(),
		codexStderr: sensitivePayload,
		codexErr:    context.Canceled,
	}
	var output bytes.Buffer
	var errorOutput bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{"--dry-run"},
		runner.root,
		strings.NewReader(""),
		&output,
		&errorOutput,
		runner,
	)

	if exitCode != interruptedExitCode {
		t.Fatalf("run() exit code = %d, want %d", exitCode, interruptedExitCode)
	}
	if strings.Contains(output.String(), sensitivePayload) ||
		strings.Contains(errorOutput.String(), sensitivePayload) {
		t.Fatalf(
			"cancellation exposed Codex payload:\nstdout=%q\nstderr=%q",
			output.String(),
			errorOutput.String(),
		)
	}
	if !strings.HasSuffix(errorOutput.String(), "Cancelled.\n") {
		t.Fatalf("stderr = %q, want final cancellation message", errorOutput.String())
	}
	if runner.mutations != 0 {
		t.Fatalf("GitHub mutations = %d, want zero", runner.mutations)
	}
}

func TestRunPullRequestDryRunFromDifferentBranch(t *testing.T) {
	root := t.TempDir()
	draftJSON, _ := json.Marshal(map[string]any{
		"title":   "Fix link pricesheet",
		"summary": []string{"Use the updated pricesheet link data."},
		"changes": []map[string]any{{
			"id":        "F1.C1",
			"file":      "controllers/pricesheet.go",
			"operation": "modified",
			"element":   "LinkPricesheet",
			"summary":   "Use the updated pricesheet link data.",
		}},
		"testing": []string{"Not run (no test results provided)."},
	})
	runner := &integrationRunner{
		t:     t,
		root:  root,
		draft: string(draftJSON),
	}
	var output bytes.Buffer
	var errorOutput bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{"--pr", "888", "--dry-run"},
		root,
		strings.NewReader(""),
		&output,
		&errorOutput,
		runner,
	)
	if exitCode != 0 {
		t.Fatalf(
			"run() exit code = %d, stderr = %s",
			exitCode,
			errorOutput.String(),
		)
	}
	if runner.mutations != 0 {
		t.Fatalf("GitHub mutations = %d, want zero", runner.mutations)
	}
	if !strings.Contains(errorOutput.String(), "Finding pull request #888") {
		t.Fatalf("stderr missing direct PR lookup:\n%s", errorOutput.String())
	}
	if !strings.Contains(output.String(), "Fix link pricesheet") {
		t.Fatalf("stdout missing PR draft:\n%s", output.String())
	}
}

func TestRunBranchListIntegration(t *testing.T) {
	runner := &sequenceRunner{
		t: t,
		runs: []sequenceRun{
			{
				want: command.Spec{
					Name: "git",
					Args: []string{"rev-parse", "--show-toplevel"},
					Dir:  "/work",
				},
				result: command.Result{Stdout: "/repo\n"},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{"branch", "--show-current"},
					Dir:  "/repo",
				},
				result: command.Result{Stdout: "feature/payments\n"},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{
						"for-each-ref",
						"--sort=-committerdate",
						"--format=%(refname:short)%00%(committerdate:unix)%00%(committerdate:short)",
						"refs/heads",
					},
					Dir: "/repo",
				},
				result: command.Result{Stdout: strings.Join([]string{
					"feature/payments\x001786705200\x002026-08-14",
					"main\x001786190400\x002026-08-08",
				}, "\n") + "\n"},
			},
			{
				want: command.Spec{
					Name: "git",
					Args: []string{
						"for-each-ref",
						"--format=%(refname:short)",
						"--merged=refs/heads/main",
						"refs/heads",
					},
					Dir: "/repo",
				},
				result: command.Result{Stdout: "main\n"},
			},
		},
	}
	var output bytes.Buffer
	var errorOutput bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{"branch"},
		"/work",
		strings.NewReader(""),
		&output,
		&errorOutput,
		runner,
	)
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %s", exitCode, errorOutput.String())
	}
	if !strings.Contains(output.String(), "Current branch: feature/payments") ||
		!strings.Contains(output.String(), "Local branches:") ||
		!strings.Contains(output.String(), "2026-08-14") {
		t.Fatalf("branch output missing details:\n%s", output.String())
	}
	runner.assertComplete()
}

func TestRunRejectsInvalidOptions(t *testing.T) {
	var output bytes.Buffer
	var errorOutput bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{"--pr", "0"},
		"/repo",
		strings.NewReader(""),
		&output,
		&errorOutput,
		&integrationRunner{t: t},
	)
	if exitCode != 2 {
		t.Fatalf("run() exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(errorOutput.String(), "must be positive") {
		t.Fatalf("stderr = %q, want validation message", errorOutput.String())
	}
}

func TestRunReviewIntegration(t *testing.T) {
	root := t.TempDir()
	runner := &mainReviewRunner{t: t, root: root}
	var output bytes.Buffer
	var errorOutput bytes.Buffer

	exitCode := run(
		context.Background(),
		[]string{"review", "123"},
		root,
		strings.NewReader(""),
		&output,
		&errorOutput,
		runner,
	)
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %s", exitCode, errorOutput.String())
	}
	if !strings.Contains(output.String(), "PR review #123 (standard)") ||
		!strings.Contains(output.String(), "# Potential Issues and Risks") ||
		!strings.Contains(output.String(), "Used by this review:       1,500 tokens") ||
		!strings.Contains(output.String(), "Credits consumed:          unavailable") ||
		!strings.Contains(output.String(), "Account credit balance:    unavailable") {
		t.Fatalf("review output =\n%s", output.String())
	}
	if strings.Contains(output.String(), "\x1b[") || strings.Contains(errorOutput.String(), "\x1b[") {
		t.Fatalf("redirected review output contains ANSI escapes:\nstdout=%q\nstderr=%q", output.String(), errorOutput.String())
	}
	for _, status := range []string{
		"[  5%] Inspecting Git repository...",
		"[ 15%] Resolving pull-request metadata...",
		"[ 25%] Collecting pull-request evidence...",
		"[ 38%] Selecting evidence within the review budget...",
		"[ 45%] Reading account credit balance before review...",
		"[ 50%] Starting Codex review...",
		"[ 58%] Streaming or processing Codex events...",
		"[ 75%] Validating review findings...",
		"[ 85%] Reconciling account usage after review...",
		"[ 95%] Rendering the review report...",
	} {
		if !strings.Contains(errorOutput.String(), status) {
			t.Fatalf("progress output missing %q:\n%s", status, errorOutput.String())
		}
	}
	if runner.calls != 5 {
		t.Fatalf("runner calls = %d, want 5", runner.calls)
	}
}

func TestRunHelpExitsSuccessfullyWithoutCommands(t *testing.T) {
	for _, argument := range []string{"-h", "--help"} {
		t.Run(argument, func(t *testing.T) {
			var output bytes.Buffer
			var errorOutput bytes.Buffer
			exitCode := run(
				context.Background(),
				[]string{argument},
				"/repo",
				strings.NewReader(""),
				&output,
				&errorOutput,
				&integrationRunner{t: t},
			)
			if exitCode != 0 {
				t.Fatalf("run() exit code = %d, want 0", exitCode)
			}
			if !strings.Contains(output.String(), "Refinement commands:") {
				t.Fatalf("stdout missing help:\n%s", output.String())
			}
			if errorOutput.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", errorOutput.String())
			}
		})
	}
}

func TestRunVersionExitsSuccessfullyWithoutCommands(t *testing.T) {
	tests := []struct {
		name           string
		argument       string
		currentVersion string
		want           string
	}{
		{
			name:           "release flag",
			argument:       "--version",
			currentVersion: "v1.2.3",
			want:           "champu v1.2.3\n",
		},
		{
			name:           "development command",
			argument:       "version",
			currentVersion: version.Development,
			want:           "champu development build\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			var errorOutput bytes.Buffer
			exitCode := runWithVersion(
				context.Background(),
				[]string{test.argument},
				"/repo",
				strings.NewReader(""),
				&output,
				&errorOutput,
				&integrationRunner{t: t},
				test.currentVersion,
			)
			if exitCode != 0 {
				t.Fatalf("runWithVersion() exit code = %d, want 0", exitCode)
			}
			if output.String() != test.want {
				t.Fatalf("stdout = %q, want %q", output.String(), test.want)
			}
			if errorOutput.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", errorOutput.String())
			}
		})
	}
}

func TestRunUpdateInstallsConfirmedRelease(t *testing.T) {
	modulePath := "github.com/Merthoshan/PR-maker-CLI"
	runner := &sequenceRunner{
		t: t,
		runs: []sequenceRun{
			{
				want: command.Spec{
					Name: "go",
					Args: []string{
						"list",
						"-m",
						"-f", "{{.Version}}",
						modulePath + "@latest",
					},
				},
				result: command.Result{Stdout: "v1.1.0"},
			},
			{
				want: command.Spec{
					Name: "go",
					Args: []string{
						"install",
						modulePath + "/cmd/champu@v1.1.0",
					},
				},
			},
		},
	}
	var output bytes.Buffer
	var errorOutput bytes.Buffer

	exitCode := runWithVersion(
		context.Background(),
		[]string{"update"},
		"/repo",
		strings.NewReader("yes\n"),
		&output,
		&errorOutput,
		runner,
		"v1.0.0",
	)
	if exitCode != 0 {
		t.Fatalf(
			"runWithVersion() exit code = %d, stderr = %s",
			exitCode,
			errorOutput.String(),
		)
	}
	if !strings.Contains(output.String(), "Successfully updated to v1.1.0") {
		t.Fatalf("stdout missing update result:\n%s", output.String())
	}
	if !strings.Contains(errorOutput.String(), "Checking for champu updates") ||
		!strings.Contains(errorOutput.String(), "Installing champu v1.1.0") {
		t.Fatalf("stderr missing update progress:\n%s", errorOutput.String())
	}
	runner.assertComplete()
}

func TestRunUpdateRejectsArguments(t *testing.T) {
	var errorOutput bytes.Buffer
	exitCode := runWithVersion(
		context.Background(),
		[]string{"update", "now"},
		"/repo",
		strings.NewReader(""),
		&bytes.Buffer{},
		&errorOutput,
		&integrationRunner{t: t},
		"v1.0.0",
	)
	if exitCode != 2 {
		t.Fatalf("runWithVersion() exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(errorOutput.String(), "does not accept arguments") {
		t.Fatalf("stderr = %q, want argument error", errorOutput.String())
	}
}

type integrationRunner struct {
	t           *testing.T
	root        string
	draft       string
	codexStderr string
	codexErr    error
	mutations   int
}

type mainReviewRunner struct {
	t     *testing.T
	root  string
	calls int
}

func (runner *mainReviewRunner) Run(_ context.Context, spec command.Spec) (command.Result, error) {
	runner.t.Helper()
	runner.calls++
	key := spec.Name + " " + strings.Join(spec.Args, " ")
	switch key {
	case "git rev-parse --show-toplevel":
		return command.Result{Stdout: runner.root}, nil
	case "git remote get-url origin":
		return command.Result{Stdout: "git@github.com:acme/service.git"}, nil
	case "gh pr view 123 --json number,state,title,url,body,baseRefName,headRefName,isDraft,labels,files":
		return command.Result{Stdout: `{"number":123,"state":"OPEN","title":"Review","url":"https://github.com/acme/service/pull/123","baseRefName":"main","headRefName":"feature","labels":[],"files":[{"path":"main.go","additions":1,"deletions":0}]}`}, nil
	case "gh pr diff 123 --color=never":
		if spec.StdoutLimit <= 0 {
			runner.t.Fatal("review diff command has no stdout limit")
		}
		return command.Result{Stdout: "diff --git a/main.go b/main.go\n@@ -1 +1 @@\n-old\n+new\n"}, nil
	}
	if spec.Name == "codex" {
		review := `{"overview":"No actionable issue was found.","code_quality_and_style":[],"specific_suggestions":[],"potential_issues_and_risks":[]}`
		encoded, _ := json.Marshal(review)
		events := `{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":` + string(encoded) + `}}` + "\n" +
			`{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":400,"output_tokens":300}}` + "\n"
		return command.Result{Stdout: events}, nil
	}
	runner.t.Fatalf("unexpected command: %+v", spec)
	return command.Result{}, nil
}

type sequenceRun struct {
	want   command.Spec
	result command.Result
}

type sequenceRunner struct {
	t     *testing.T
	runs  []sequenceRun
	calls int
}

func (runner *sequenceRunner) Run(
	_ context.Context,
	spec command.Spec,
) (command.Result, error) {
	runner.t.Helper()
	if runner.calls >= len(runner.runs) {
		runner.t.Fatalf("unexpected command: %+v", spec)
	}
	run := runner.runs[runner.calls]
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
	return run.result, nil
}

func (runner *sequenceRunner) assertComplete() {
	runner.t.Helper()
	if runner.calls != len(runner.runs) {
		runner.t.Fatalf(
			"runner calls = %d, want %d",
			runner.calls,
			len(runner.runs),
		)
	}
}

func (runner *integrationRunner) Run(
	_ context.Context,
	spec command.Spec,
) (command.Result, error) {
	runner.t.Helper()
	if spec.Name == "gh" && len(spec.Args) >= 2 && spec.Args[0] == "pr" {
		switch spec.Args[1] {
		case "create", "edit", "ready":
			runner.mutations++
		}
	}
	if spec.Name == "codex" {
		return command.Result{
			Stdout: runner.draft,
			Stderr: runner.codexStderr,
		}, runner.codexErr
	}
	key := spec.Name + " " + strings.Join(spec.Args, " ")
	outputs := map[string]string{
		"git rev-parse --show-toplevel": runner.root,
		"git branch --show-current":     "feature",
		"git rev-parse HEAD":            "head123",
		"git remote get-url origin":     "git@github.com:acme/repo.git",
		"git status --porcelain":        "",
		"gh pr list --head feature --state open --json number,title,url,body,baseRefName,headRefName,isDraft": "[]",
		"gh pr view 888 --json number,state,title,url,body,baseRefName,headRefName,isDraft":                   `{"number":888,"state":"OPEN","title":"Fix link pricesheet","url":"https://example.test/pr/888","body":"Existing body","baseRefName":"main","headRefName":"fix-link-pricesheet","isDraft":false}`,
		"git fetch --quiet origin refs/heads/main:refs/remotes/origin/main":                                   "",
		"git merge-base HEAD refs/remotes/origin/main":                                                        "base123",
		"git log --format=%H%x09%s base123..HEAD":                                                             "head123\tAdd workflow",
		"git diff --name-status --find-renames base123..HEAD --":                                              "M\tmain.go",
		"git diff --no-ext-diff --no-color --find-renames base123..HEAD --":                                   "diff --git a/main.go b/main.go",
		"git fetch --quiet origin +refs/pull/888/head:refs/champu-pr/pulls/888/head":                          "",
		"git merge-base refs/champu-pr/pulls/888/head refs/remotes/origin/main":                               "base123",
		"git log --format=%H%x09%s base123..refs/champu-pr/pulls/888/head":                                    "pr888\tFix link pricesheet",
		"git diff --name-status --find-renames base123..refs/champu-pr/pulls/888/head --":                     "M\tcontrollers/pricesheet.go",
		"git diff --no-ext-diff --no-color --find-renames base123..refs/champu-pr/pulls/888/head --":          "diff --git a/controllers/pricesheet.go b/controllers/pricesheet.go",
	}
	output, ok := outputs[key]
	if !ok {
		runner.t.Fatalf("unexpected command: %+v", spec)
	}
	return command.Result{Stdout: output}, nil
}
