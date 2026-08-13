package gitcontext

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CollectEvidence gathers evidence for changes between baseBranch and HEAD.
func (collector Collector) CollectEvidence(
	ctx context.Context,
	repositoryRoot string,
	baseBranch string,
) (Evidence, error) {
	return collector.collectEvidence(
		ctx,
		repositoryRoot,
		baseBranch,
		"HEAD",
		"",
	)
}

// CollectPullRequestEvidence gathers evidence from GitHub's pull-request ref
// without checking out or modifying the pull request's branch.
func (collector Collector) CollectPullRequestEvidence(
	ctx context.Context,
	repositoryRoot string,
	baseBranch string,
	pullRequestNumber int,
) (Evidence, error) {
	if pullRequestNumber <= 0 {
		return Evidence{}, errors.New(
			"collect Git evidence: pull request number must be positive",
		)
	}

	headRef := fmt.Sprintf(
		"refs/champu-pr/pulls/%d/head",
		pullRequestNumber,
	)
	headRefspec := fmt.Sprintf(
		"+refs/pull/%d/head:%s",
		pullRequestNumber,
		headRef,
	)
	return collector.collectEvidence(
		ctx,
		repositoryRoot,
		baseBranch,
		headRef,
		headRefspec,
	)
}

func (collector Collector) collectEvidence(
	ctx context.Context,
	repositoryRoot string,
	baseBranch string,
	headRef string,
	headRefspec string,
) (Evidence, error) {
	if collector.runner == nil {
		return Evidence{}, errors.New(
			"collect Git evidence: runner is required",
		)
	}
	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if repositoryRoot == "" {
		return Evidence{}, errors.New("collect Git evidence: repository root is required")
	}

	baseBranch = strings.TrimSpace(baseBranch)
	if baseBranch == "" {
		return Evidence{}, errors.New("collect Git evidence: base branch is required")
	}
	headRef = strings.TrimSpace(headRef)
	if headRef == "" {
		return Evidence{}, errors.New("collect Git evidence: head ref is required")
	}

	baseRef := "refs/remotes/origin/" + baseBranch
	fetchRefspec := "refs/heads/" + baseBranch + ":" + baseRef

	if _, err := collector.gitOutput(ctx, repositoryRoot, "fetch", "--quiet", "origin", fetchRefspec); err != nil {
		return Evidence{}, fmt.Errorf("collect Git evidence: refresh base branch %q: %w", baseBranch, err)
	}

	if headRefspec != "" {
		if _, err := collector.gitOutput(
			ctx,
			repositoryRoot,
			"fetch",
			"--quiet",
			"origin",
			headRefspec,
		); err != nil {
			return Evidence{}, fmt.Errorf(
				"collect Git evidence: refresh pull request head: %w",
				err,
			)
		}
	}

	mergeBaseSHA, err := collector.gitOutput(
		ctx,
		repositoryRoot,
		"merge-base",
		headRef,
		baseRef,
	)
	if err != nil {
		return Evidence{}, fmt.Errorf("collect Git evidence: find merge base for %q: %w", baseBranch, err)
	}
	if mergeBaseSHA == "" {
		return Evidence{}, fmt.Errorf("collect Git evidence: merge base for %q is empty", baseBranch)
	}

	revisionRange := mergeBaseSHA + ".." + headRef
	commitLog, err := collector.gitOutput(ctx, repositoryRoot, "log", "--format=%H%x09%s", revisionRange)
	if err != nil {
		return Evidence{}, fmt.Errorf("collect Git evidence: collect commit log: %w", err)
	}

	changedFiles, err := collector.gitOutput(ctx, repositoryRoot, "diff", "--name-status", "--find-renames", revisionRange, "--")
	if err != nil {
		return Evidence{}, fmt.Errorf("collect Git evidence: collect changed files: %w", err)
	}

	diff, err := collector.gitOutput(ctx, repositoryRoot, "diff", "--no-ext-diff", "--no-color", "--find-renames", revisionRange, "--")
	if err != nil {
		return Evidence{}, fmt.Errorf("collect Git evidence: collect textual diff: %w", err)
	}

	return Evidence{
		BaseBranch:   baseBranch,
		BaseRef:      baseRef,
		MergeBaseSHA: mergeBaseSHA,
		CommitLog:    commitLog,
		ChangedFiles: changedFiles,
		Diff:         diff,
	}, nil
}
