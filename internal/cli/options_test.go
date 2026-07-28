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
	for _, argument := range []string{"-h", "--help"} {
		t.Run(argument, func(t *testing.T) {
			_, err := ParseOptions([]string{argument})
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
		"champu-pr [options]",
		"--base string",
		`(default "main")`,
		"--pr int",
		"--ready",
		"--dry-run",
		"-h, --help",
		"Refinement commands:",
		"exclude F2",
		"tests passed: go test ./...",
		"Workflow controls:",
		"apply",
		"quit",
		"Examples:",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
}
