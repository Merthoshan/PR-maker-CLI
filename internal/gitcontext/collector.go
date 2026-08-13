package gitcontext

import (
	"context"
	"errors"
	"strings"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
)

// Collector gathers repository information using Git commands.
type Collector struct {
	runner command.Runner
}

// NewCollector creates a Git context collector.
func NewCollector(runner command.Runner) (Collector, error) {
	if runner == nil {
		return Collector{}, errors.New("create Git collector: runner is required")
	}

	return Collector{
		runner: runner,
	}, nil
}

// Collect inspects the repository containing workingDirectory.
func (collector Collector) Collect(ctx context.Context, workingDirectory string) (Repository, error) {
	root, err := collector.Root(ctx, workingDirectory)
	if err != nil {
		return Repository{}, err
	}

	branch, err := collector.gitOutput(ctx, root, "branch", "--show-current")
	if err != nil {
		return Repository{}, err
	}
	if branch == "" {
		return Repository{}, errors.New("collect git context: detached HEAD is not supported")
	}
	headSHA, err := collector.gitOutput(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return Repository{}, err
	}

	remoteURL, err := collector.OriginURL(ctx, root)
	if err != nil {
		return Repository{}, err
	}

	status, err := collector.gitOutput(ctx, root, "status", "--porcelain")
	if err != nil {
		return Repository{}, err
	}

	dirty := status != ""

	return Repository{
		Root:      root,
		Branch:    branch,
		HeadSHA:   headSHA,
		RemoteURL: remoteURL,
		Dirty:     dirty,
	}, nil
}

// Root resolves the top-level directory for the repository containing
// workingDirectory without requiring a branch, remote, or clean working tree.
func (collector Collector) Root(ctx context.Context, workingDirectory string) (string, error) {
	if collector.runner == nil {
		return "", errors.New("collect git context: runner is required")
	}
	root, err := collector.gitOutput(ctx, workingDirectory, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if root == "" {
		return "", errors.New("collect git context: repository root is empty")
	}
	return root, nil
}

// OriginURL reads the configured origin URL for an already-resolved repository
// root without collecting unrelated branch or working-tree state.
func (collector Collector) OriginURL(ctx context.Context, root string) (string, error) {
	if collector.runner == nil {
		return "", errors.New("collect git context: runner is required")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("collect git context: repository root is required")
	}
	remoteURL, err := collector.gitOutput(ctx, root, "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	if remoteURL == "" {
		return "", errors.New("collect git context: origin remote URL is empty")
	}
	return remoteURL, nil
}

// gitOutput runs one Git query and returns trimmed stdout with command context
// preserved in any error.
func (collector Collector) gitOutput(ctx context.Context, directory string, args ...string) (string, error) {
	spec := command.Spec{
		Name: "git",
		Args: args,
		Dir:  directory,
	}

	result, err := collector.runner.Run(ctx, spec)
	if err != nil {
		commandName := "git " + strings.Join(args, " ")
		return "", command.WrapError(commandName, result, err)
	}

	return strings.TrimSpace(result.Stdout), nil
}
