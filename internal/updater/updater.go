package updater

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
	"github.com/Merthoshan/PR-maker-CLI/internal/terminal"
	"github.com/Merthoshan/PR-maker-CLI/internal/version"
)

const (
	modulePath  = "github.com/Merthoshan/PR-maker-CLI"
	packagePath = modulePath + "/cmd/champu"
)

// Updater checks and installs published Champu releases.
type Updater struct {
	runner         command.Runner
	input          io.Reader
	output         io.Writer
	progress       progressReporter
	currentVersion string
}

// New creates a version-aware updater.
func New(
	runner command.Runner,
	input io.Reader,
	output io.Writer,
	progress progressReporter,
	currentVersion string,
) (*Updater, error) {
	switch {
	case runner == nil:
		return nil, errors.New("create updater: runner is required")
	case input == nil:
		return nil, errors.New("create updater: input is required")
	case output == nil:
		return nil, errors.New("create updater: output is required")
	case progress == nil:
		return nil, errors.New("create updater: progress reporter is required")
	case strings.TrimSpace(currentVersion) == "":
		return nil, errors.New("create updater: current version is required")
	}
	return &Updater{
		runner:         runner,
		input:          input,
		output:         output,
		progress:       progress,
		currentVersion: strings.TrimSpace(currentVersion),
	}, nil
}

// Run checks the latest release and installs it only after confirmation.
func (updater *Updater) Run(ctx context.Context) (Outcome, error) {
	if updater.currentVersion == version.Development {
		fmt.Fprintln(
			updater.output,
			"champu is a development build and will not be overwritten.",
		)
		fmt.Fprintln(
			updater.output,
			"Refresh it from the source checkout with: go install ./cmd/champu",
		)
		return Outcome{CurrentVersion: version.Development}, nil
	}
	if !version.IsRelease(updater.currentVersion) {
		return Outcome{}, fmt.Errorf(
			"update champu: installed version %q is invalid",
			updater.currentVersion,
		)
	}

	latestVersion, err := updater.findLatest(ctx)
	if err != nil {
		return Outcome{}, err
	}
	outcome := Outcome{
		CurrentVersion: updater.currentVersion,
		LatestVersion:  latestVersion,
	}

	comparison, err := version.Compare(updater.currentVersion, latestVersion)
	if err != nil {
		return Outcome{}, fmt.Errorf("update champu: %w", err)
	}
	if comparison >= 0 {
		fmt.Fprintf(
			updater.output,
			"champu %s is already up to date.\n",
			updater.currentVersion,
		)
		return outcome, nil
	}

	fmt.Fprintf(
		updater.output,
		"Current version: %s\nLatest version:  %s\n\n",
		updater.currentVersion,
		latestVersion,
	)
	fmt.Fprint(
		updater.output,
		"A newer version of champu is available.\nUpdate now? [y/N]: ",
	)
	confirmed, err := updater.readConfirmation()
	if err != nil {
		return Outcome{}, err
	}
	if !confirmed {
		outcome.Cancelled = true
		fmt.Fprintf(
			updater.output,
			"Update cancelled. You are still using %s.\n",
			updater.currentVersion,
		)
		return outcome, nil
	}

	if err := updater.install(ctx, latestVersion); err != nil {
		return Outcome{}, err
	}
	outcome.Updated = true
	fmt.Fprintf(
		updater.output,
		"Successfully updated to %s.\n",
		latestVersion,
	)
	fmt.Fprintln(
		updater.output,
		"Run your champu command again to use the new version.",
	)
	return outcome, nil
}

func (updater *Updater) findLatest(ctx context.Context) (string, error) {
	directory, err := os.MkdirTemp("", "champu-pr-update-*")
	if err != nil {
		return "", fmt.Errorf(
			"check latest champu version: create temporary directory: %w",
			err,
		)
	}
	defer os.RemoveAll(directory)

	stopProgress := updater.progress.Start("Checking for champu updates")
	result, err := updater.runner.Run(ctx, command.Spec{
		Name: "go",
		Args: []string{
			"list",
			"-m",
			"-f", "{{.Version}}",
			modulePath + "@latest",
		},
		Dir: directory,
	})
	stopProgress()
	if err != nil {
		return "", command.WrapError(
			"check latest champu version",
			result,
			err,
		)
	}

	latestVersion := strings.TrimSpace(result.Stdout)
	if !version.IsRelease(latestVersion) {
		return "", fmt.Errorf(
			"check latest champu version: received invalid version %q; "+
				"publish a vMAJOR.MINOR.PATCH tag first",
			latestVersion,
		)
	}
	return latestVersion, nil
}

func (updater *Updater) readConfirmation() (bool, error) {
	scanner := bufio.NewScanner(updater.input)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read update confirmation: %w", err)
		}
		return false, nil
	}
	answer := terminal.ParseConfirmation(scanner.Text())
	return answer == terminal.ConfirmationAccepted, nil
}

func (updater *Updater) install(
	ctx context.Context,
	latestVersion string,
) error {
	stopProgress := updater.progress.Start(
		"Installing champu " + latestVersion,
	)
	result, err := updater.runner.Run(ctx, command.Spec{
		Name: "go",
		Args: []string{
			"install",
			packagePath + "@" + latestVersion,
		},
	})
	stopProgress()
	if err != nil {
		return command.WrapError(
			"install champu "+latestVersion,
			result,
			err,
		)
	}
	return nil
}
