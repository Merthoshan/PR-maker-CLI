package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// Options contains the command-line choices that affect one champu-pr run.
type Options struct {
	Base     string
	PRNumber int
	Ready    bool
	DryRun   bool
}

func ParseOptions(args []string) (Options, error) {
	options := defaultOptions()
	flagSet := newFlagSet(&options)

	if err := flagSet.Parse(args); err != nil {
		return Options{}, fmt.Errorf("parse command-line options: %w", err)
	}

	if flagSet.NArg() > 0 {
		return Options{}, fmt.Errorf(
			"unexpected arguments: %s",
			strings.Join(flagSet.Args(), " "),
		)
	}

	provided := collectProvidedFlags(flagSet)

	if provided["pr"] && provided["base"] {
		return Options{}, fmt.Errorf("cannot specify both --pr and --base")
	}
	if provided["pr"] {
		if options.PRNumber <= 0 {
			return Options{}, fmt.Errorf("pull request number must be positive")
		}
		options.Base = ""
	}
	if !provided["pr"] {
		options.Base = strings.TrimSpace(options.Base)
		if options.Base == "" {
			return Options{}, fmt.Errorf("base branch cannot be empty")
		}
	}
	return options, nil
}

// WriteHelp writes the CLI usage, options, and interactive commands.
func WriteHelp(output io.Writer) error {
	var flagHelp strings.Builder
	options := defaultOptions()
	flagSet := newFlagSet(&options)
	flagSet.SetOutput(&flagHelp)
	flagSet.PrintDefaults()

	formattedFlags := strings.ReplaceAll(flagHelp.String(), "  -", "  --")
	_, err := fmt.Fprintf(
		output,
		`champu-pr — generate and publish evidence-backed PR descriptions

Usage:
  champu-pr [options]
  champu-pr update
  champu-pr --version

Commands:
  update
        check for a newer release and install it after confirmation

Options:
%s  -h, --help
        show this help
  --version
        show the installed version

Refinement commands:
  Write refinement instructions in normal English. The forms below are optional shortcuts.
  exclude F2
  include F2.C1
  combine F1.C1 F1.C2
  separate F1.C1
  make the summary shorter
  make the summary more detailed
  focus the title on F3.C1
  tests passed: go test ./...
  tests failed: go test ./...
  tests: not run
  make description
  reset
  preview

Workflow controls:
  apply   Publish after make description
  quit    Exit without changing GitHub

Examples:
  champu-pr
  champu-pr --base develop
  champu-pr --pr 123
  champu-pr --base main --ready
  champu-pr --pr 123 --ready
  champu-pr --dry-run
  champu-pr --version
  champu-pr update
`,
		formattedFlags,
	)
	if err != nil {
		return fmt.Errorf("write help: %w", err)
	}
	return nil
}

func defaultOptions() Options {
	return Options{Base: "main"}
}

func newFlagSet(options *Options) *flag.FlagSet {
	flagSet := flag.NewFlagSet("champu-pr", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	flagSet.StringVar(
		&options.Base,
		"base",
		options.Base,
		"base branch for the pull request",
	)
	flagSet.IntVar(
		&options.PRNumber,
		"pr",
		0,
		"existing pull request number; works from any local branch",
	)
	flagSet.BoolVar(
		&options.Ready,
		"ready",
		false,
		"create or mark the pull request as ready",
	)
	flagSet.BoolVar(
		&options.DryRun,
		"dry-run",
		false,
		"preview without changing GitHub",
	)
	return flagSet
}

// collectProvidedFlags reports flags explicitly supplied by the user.
func collectProvidedFlags(flagSet *flag.FlagSet) map[string]bool {
	provided := make(map[string]bool)
	flagSet.Visit(func(f *flag.Flag) {
		provided[f.Name] = true
	})
	return provided
}
