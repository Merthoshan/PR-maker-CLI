package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

func ParseOptions(args []string) (Options, error) {
	if len(args) > 0 && args[0] == "branch" {
		return parseBranchOptions(args[1:])
	}
	if len(args) > 0 && args[0] == "review" {
		return parseReviewOptions(args[1:])
	}
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

func parseBranchOptions(args []string) (Options, error) {
	options := Options{Branch: true}
	if len(args) == 0 {
		return options, nil
	}
	if len(args) == 1 {
		switch args[0] {
		case "cleanup":
			options.BranchCleanup = true
			return options, nil
		case "-h", "--help", "-help":
			return Options{}, flag.ErrHelp
		}
	}
	return Options{}, fmt.Errorf(
		"branch accepts only the cleanup subcommand",
	)
}

func parseReviewOptions(args []string) (Options, error) {
	options := Options{Review: true, ReviewDepth: "standard"}
	positionals := make([]string, 0, 1)
	instructionsProvided := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "-h" || argument == "--help" || argument == "-help":
			return Options{}, flag.ErrHelp
		case argument == "--":
			positionals = append(positionals, args[index+1:]...)
			index = len(args)
		case argument == "--depth" || argument == "-depth":
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return Options{}, fmt.Errorf("review depth is required")
			}
			options.ReviewDepth = strings.TrimSpace(args[index+1])
			index++
		case strings.HasPrefix(argument, "--depth="):
			options.ReviewDepth = strings.TrimSpace(strings.TrimPrefix(argument, "--depth="))
		case strings.HasPrefix(argument, "-depth="):
			options.ReviewDepth = strings.TrimSpace(strings.TrimPrefix(argument, "-depth="))
		case argument == "--instructions" || argument == "-instructions":
			instructionsProvided = true
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return Options{}, fmt.Errorf("review instructions path is required")
			}
			options.ReviewInstructions = strings.TrimSpace(args[index+1])
			index++
		case strings.HasPrefix(argument, "--instructions="):
			instructionsProvided = true
			options.ReviewInstructions = strings.TrimSpace(strings.TrimPrefix(argument, "--instructions="))
		case strings.HasPrefix(argument, "-instructions="):
			instructionsProvided = true
			options.ReviewInstructions = strings.TrimSpace(strings.TrimPrefix(argument, "-instructions="))
		case strings.HasPrefix(argument, "-"):
			return Options{}, fmt.Errorf("unknown review option %q", argument)
		default:
			positionals = append(positionals, argument)
		}
	}
	if len(positionals) != 1 {
		return Options{}, fmt.Errorf("review requires one pull request number or URL")
	}
	if options.ReviewDepth == "" {
		return Options{}, fmt.Errorf("review depth is required")
	}
	if options.ReviewDepth != "standard" && options.ReviewDepth != "deep" {
		return Options{}, fmt.Errorf("review depth must be standard or deep")
	}
	options.ReviewTarget = strings.TrimSpace(positionals[0])
	if options.ReviewTarget == "" {
		return Options{}, fmt.Errorf("review pull request target cannot be empty")
	}
	if instructionsProvided && options.ReviewInstructions == "" {
		return Options{}, fmt.Errorf("review instructions path is required")
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
		`champu-pr — generate PR descriptions and review pull requests

Usage:
  champu-pr [options]
  champu-pr branch
  champu-pr branch cleanup
  champu-pr review <number-or-url> [--depth standard|deep] [--instructions path]
  champu-pr update
  champu-pr --version

Commands:
  branch
        list local branches ordered by their latest commit
  branch cleanup
        interactively preview and delete safely merged local branches
  review <number-or-url>
        review an existing pull request without changing GitHub
  update
        check for a newer release and install it after confirmation

Options:
%s  -h, --help
        show this help
  --version
        show the installed version

Review:
  standard  daily review mode; checks correctness, security, performance,
            database calls in loops, nesting, error handling, and tests
  deep      larger evidence budget with higher reasoning effort
  --instructions path
            explicitly add review guidance from a regular, non-symlinked file
            inside the repository

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
  champu-pr branch
  champu-pr branch cleanup
  champu-pr --base develop
  champu-pr --pr 123
  champu-pr --base main --ready
  champu-pr --pr 123 --ready
  champu-pr --dry-run
  champu-pr review 123
  champu-pr review https://github.com/org/repo/pull/123 --depth deep
  champu-pr review 123 --instructions .champu-pr/review-instructions.md
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
