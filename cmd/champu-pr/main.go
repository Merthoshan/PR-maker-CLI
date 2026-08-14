package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/Merthoshan/PR-maker-CLI/internal/application"
	"github.com/Merthoshan/PR-maker-CLI/internal/branch"
	"github.com/Merthoshan/PR-maker-CLI/internal/cli"
	"github.com/Merthoshan/PR-maker-CLI/internal/command"
	"github.com/Merthoshan/PR-maker-CLI/internal/review"
	"github.com/Merthoshan/PR-maker-CLI/internal/terminal"
	"github.com/Merthoshan/PR-maker-CLI/internal/updater"
	"github.com/Merthoshan/PR-maker-CLI/internal/version"
)

const interruptedExitCode = 130

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	workingDirectory, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "find working directory: %v\n", err)
		os.Exit(1)
	}
	os.Exit(run(
		ctx,
		os.Args[1:],
		workingDirectory,
		os.Stdin,
		os.Stdout,
		os.Stderr,
		command.ExecRunner{},
	))
}

func run(
	ctx context.Context,
	args []string,
	workingDirectory string,
	input io.Reader,
	output io.Writer,
	errorOutput io.Writer,
	runner command.Runner,
) int {
	return runWithVersion(
		ctx,
		args,
		workingDirectory,
		input,
		output,
		errorOutput,
		runner,
		version.Current(),
	)
}

func runWithVersion(
	ctx context.Context,
	args []string,
	workingDirectory string,
	input io.Reader,
	output io.Writer,
	errorOutput io.Writer,
	runner command.Runner,
	currentVersion string,
) int {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		printVersion(output, currentVersion)
		return 0
	}
	if len(args) > 0 && args[0] == "update" {
		if len(args) != 1 {
			fmt.Fprintln(errorOutput, "update command does not accept arguments")
			return 2
		}
		return runUpdate(
			ctx,
			input,
			output,
			errorOutput,
			runner,
			currentVersion,
		)
	}

	options, err := cli.ParseOptions(args)
	if errors.Is(err, flag.ErrHelp) {
		if err := cli.WriteHelp(output); err != nil {
			fmt.Fprintln(errorOutput, err)
			return 1
		}
		return 0
	}
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 2
	}
	if options.Branch {
		return runBranch(
			ctx,
			options,
			workingDirectory,
			input,
			output,
			errorOutput,
			runner,
		)
	}
	if options.Review {
		return runReview(ctx, options, workingDirectory, output, errorOutput, runner)
	}
	progress, err := terminal.NewReporter(errorOutput)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	app, err := application.NewDefault(runner, input, output, progress)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	outcome, err := app.Run(ctx, options, workingDirectory)
	if err != nil && reportCancellation(ctx, err, errorOutput) {
		return interruptedExitCode
	}
	if errors.Is(err, application.ErrCancelled) {
		fmt.Fprintln(output, "No GitHub changes were made.")
		return 0
	}
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	if outcome.DryRun {
		return 0
	}
	action := "Updated"
	if outcome.Created {
		action = "Created"
	}
	fmt.Fprintf(output, "%s pull request: %s\n", action, outcome.URL)
	return 0
}

func runBranch(
	ctx context.Context,
	options cli.Options,
	workingDirectory string,
	input io.Reader,
	output io.Writer,
	errorOutput io.Writer,
	runner command.Runner,
) int {
	service, err := branch.New(runner, input, output, errorOutput)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	if err := service.Run(ctx, branch.Request{
		WorkingDirectory: workingDirectory,
		Cleanup:          options.BranchCleanup,
	}); err != nil {
		if reportCancellation(ctx, err, errorOutput) {
			return interruptedExitCode
		}
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	return 0
}

func runReview(
	ctx context.Context,
	options cli.Options,
	workingDirectory string,
	output io.Writer,
	errorOutput io.Writer,
	runner command.Runner,
) int {
	progress, err := terminal.NewReporter(errorOutput)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	service, err := review.New(runner, progress)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	outcome, err := service.Run(ctx, review.Request{
		WorkingDirectory: workingDirectory,
		Target:           options.ReviewTarget,
		Depth:            options.ReviewDepth,
		InstructionsPath: options.ReviewInstructions,
	})
	if err != nil {
		if reportCancellation(ctx, err, errorOutput) {
			return interruptedExitCode
		}
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	reviewOutput := terminal.ColorizeValidatedSeverities(
		outcome.Review,
		outcome.SeverityLines,
		output,
	)
	usageOutput := review.RenderUsageSummary(
		outcome.Usage,
		outcome.EvidenceTokenEstimate,
		outcome.EvidenceTokenBudget,
	)
	fmt.Fprintf(
		output,
		"\nPR review #%d (%s):\n\n%s\n\n%s\n",
		outcome.PullRequest.Number,
		outcome.Depth,
		reviewOutput,
		usageOutput,
	)
	return 0
}

func runUpdate(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	errorOutput io.Writer,
	runner command.Runner,
	currentVersion string,
) int {
	progress, err := terminal.NewReporter(errorOutput)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	updateService, err := updater.New(
		runner,
		input,
		output,
		progress,
		currentVersion,
	)
	if err != nil {
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	if _, err := updateService.Run(ctx); err != nil {
		if reportCancellation(ctx, err, errorOutput) {
			return interruptedExitCode
		}
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	return 0
}

// reportCancellation renders the single safe message used when Ctrl-C stops a
// workflow. It intentionally ignores the wrapped error text, which may contain
// captured subprocess output derived from private repository evidence.
func reportCancellation(
	ctx context.Context,
	err error,
	output io.Writer,
) bool {
	if !errors.Is(err, context.Canceled) &&
		!errors.Is(ctx.Err(), context.Canceled) {
		return false
	}
	fmt.Fprintln(output, "Cancelled.")
	return true
}

func printVersion(output io.Writer, currentVersion string) {
	if currentVersion == version.Development {
		fmt.Fprintln(output, "champu-pr development build")
		return
	}
	fmt.Fprintf(output, "champu-pr %s\n", currentVersion)
}
