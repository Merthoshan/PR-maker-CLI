package cli

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    Options
		wantErr string
	}{
		{
			name: "defaults",
			args: []string{},
			want: Options{
				Base: "main",
			},
		},
		{
			name: "branch list",
			args: []string{"branch"},
			want: Options{Branch: true},
		},
		{
			name: "branch cleanup",
			args: []string{"branch", "cleanup"},
			want: Options{Branch: true, BranchCleanup: true},
		},
		{
			name:    "branch rejects arguments",
			args:    []string{"branch", "cleanup", "fix"},
			wantErr: "accepts only the cleanup subcommand",
		},
		{
			name: "custom base",
			args: []string{
				"--base", "  develop  ",
			},
			want: Options{
				Base: "develop",
			},
		},
		{
			name: "pull request mode",
			args: []string{
				"--pr", "42",
			},
			want: Options{
				PRNumber: 42,
			},
		},
		{
			name: "workflow flags with base mode",
			args: []string{
				"--base", "develop",
				"--ready",
				"--dry-run",
			},
			want: Options{
				Base:   "develop",
				Ready:  true,
				DryRun: true,
			},
		},
		{
			name:    "blank base",
			args:    []string{"--base", ""},
			wantErr: "base branch cannot be empty",
		},
		{
			name:    "whitespace base",
			args:    []string{"--base", " \n\t "},
			wantErr: "base branch cannot be empty",
		},
		{
			name:    "mutually exclusive selection flags",
			args:    []string{"--pr", "42", "--base", "develop"},
			wantErr: "cannot specify both --pr and --base",
		},
		{
			name:    "zero PR number",
			args:    []string{"--pr", "0"},
			wantErr: "pull request number must be positive",
		},
		{
			name:    "negative PR number",
			args:    []string{"--pr", "-1"},
			wantErr: "pull request number must be positive",
		},
		{
			name:    "unexpected positional argument",
			args:    []string{"unexpected"},
			wantErr: "unexpected arguments",
		},
		{
			name: "standard review",
			args: []string{"review", "123"},
			want: Options{Review: true, ReviewTarget: "123", ReviewDepth: "standard"},
		},
		{
			name: "deep review after target",
			args: []string{"review", "https://github.com/acme/service/pull/123", "--depth", "deep"},
			want: Options{Review: true, ReviewTarget: "https://github.com/acme/service/pull/123", ReviewDepth: "deep"},
		},
		{
			name: "deep review before target with instructions",
			args: []string{"review", "--depth=deep", "--instructions", ".champu-pr/review.md", "123"},
			want: Options{
				Review:             true,
				ReviewTarget:       "123",
				ReviewDepth:        "deep",
				ReviewInstructions: ".champu-pr/review.md",
			},
		},
		{
			name:    "invalid review depth",
			args:    []string{"review", "123", "--depth", "wide"},
			wantErr: "review depth must be standard or deep",
		},
		{
			name:    "missing review depth",
			args:    []string{"review", "123", "--depth"},
			wantErr: "review depth is required",
		},
		{
			name:    "missing review instructions",
			args:    []string{"review", "123", "--instructions="},
			wantErr: "review instructions path is required",
		},
		{
			name:    "unknown review option",
			args:    []string{"review", "123", "--deph", "deep"},
			wantErr: `unknown review option "--deph"`,
		},
		{
			name:    "multiple review targets",
			args:    []string{"review", "123", "456"},
			wantErr: "requires one pull request",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseOptions(test.args)

			if test.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseOptions() error = nil, want error containing %q", test.wantErr)
				}
				if !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("ParseOptions() error = %q, want error containing %q", err, test.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseOptions() unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("ParseOptions() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseOptionsRecognizesHelp(t *testing.T) {
	for _, arguments := range [][]string{
		{"-h"},
		{"--help"},
		{"branch", "-h"},
		{"branch", "--help"},
		{"review", "-h"},
		{"review", "--help"},
	} {
		t.Run(strings.Join(arguments, " "), func(t *testing.T) {
			_, err := ParseOptions(arguments)
			if !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("ParseOptions() error = %v, want flag.ErrHelp", err)
			}
		})
	}
}

func TestWriteHelp(t *testing.T) {
	var output bytes.Buffer
	if err := WriteHelp(&output); err != nil {
		t.Fatalf("WriteHelp() unexpected error: %v", err)
	}

	help := output.String()
	for _, want := range []string{
		"Usage:",
		"champu [options]",
		"--base string",
		`(default "main")`,
		"--pr int",
		"works from any local branch",
		"--ready",
		"--dry-run",
		"--version",
		"-h, --help",
		"Commands:",
		"champu branch",
		"champu branch cleanup",
		"safely merged local branches",
		"review <number-or-url>",
		"champu update",
		"champu review",
		"--instructions path",
		"Refinement commands:",
		"instructions in normal English",
		"exclude F2",
		"tests passed: go test ./...",
		"Workflow controls:",
		"y, yes",
		"apply",
		"quit",
		"Ctrl-C",
		"without printing request evidence",
		"Examples:",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "\n  champu-pr") ||
		strings.Contains(help, "make description") {
		t.Fatalf("help output contains a legacy command:\n%s", help)
	}
}
