package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"champu-pr/internal/application"
	"champu-pr/internal/cli"
	"champu-pr/internal/command"
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
	app, err := application.NewDefault(runner, input, output)
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
