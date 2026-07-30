package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"champu-pr/internal/command"
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

type integrationRunner struct {
	t         *testing.T
	root      string
	draft     string
	mutations int
}

func (runner *integrationRunner) Run(
	_ context.Context,
	spec command.Spec,
) (command.Result, error) {
	runner.t.Helper()
	if spec.Name == "gh" && len(spec.Args) >= 2 &&
		spec.Args[0] == "pr" &&
		spec.Args[1] != "list" {
		runner.mutations++
	}
	if spec.Name == "codex" {
		return command.Result{Stdout: runner.draft}, nil
	}
	key := spec.Name + " " + strings.Join(spec.Args, " ")
	outputs := map[string]string{
		"git rev-parse --show-toplevel": runner.root,
		"git branch --show-current":     "feature",
		"git rev-parse HEAD":            "head123",
		"git remote get-url origin":     "git@github.com:acme/repo.git",
		"git status --porcelain":        "",
		"gh pr list --head feature --state open --json number,title,url,body,baseRefName,headRefName,isDraft": "[]",
		"git fetch --quiet origin refs/heads/main:refs/remotes/origin/main":                                   "",
		"git merge-base HEAD refs/remotes/origin/main":                                                        "base123",
		"git log --format=%H%x09%s base123..HEAD":                                                             "head123\tAdd workflow",
		"git diff --name-status --find-renames base123..HEAD --":                                              "M\tmain.go",
		"git diff --no-ext-diff --no-color --find-renames base123..HEAD --":                                   "diff --git a/main.go b/main.go",
	}
	output, ok := outputs[key]
	if !ok {
		runner.t.Fatalf("unexpected command: %+v", spec)
	}
	return command.Result{Stdout: output}, nil
}
