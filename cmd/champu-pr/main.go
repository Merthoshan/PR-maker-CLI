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
	"github.com/Merthoshan/PR-maker-CLI/internal/cli"
	"github.com/Merthoshan/PR-maker-CLI/internal/command"
	"github.com/Merthoshan/PR-maker-CLI/internal/terminal"
	"github.com/Merthoshan/PR-maker-CLI/internal/updater"
	"github.com/Merthoshan/PR-maker-CLI/internal/version"
)

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
		fmt.Fprintln(errorOutput, err)
		return 1
	}
	return 0
}

func printVersion(output io.Writer, currentVersion string) {
	if currentVersion == version.Development {
		fmt.Fprintln(output, "champu-pr development build")
		return
	}
	fmt.Fprintf(output, "champu-pr %s\n", currentVersion)
}
