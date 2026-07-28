package gitcontext

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Evidence contains the Git facts used to generate a pull-request description.
type Evidence struct {
	BaseBranch   string
	BaseRef      string
	MergeBaseSHA string
	CommitLog    string
	ChangedFiles string
	Diff         string
}

// CollectEvidence gathers evidence for changes between baseBranch and HEAD.
func (collector Collector) CollectEvidence(ctx context.Context, repositoryRoot string, baseBranch string) (Evidence, error) {
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

	baseRef := "refs/remotes/origin/" + baseBranch
	fetchRefspec := "refs/heads/" + baseBranch + ":" + baseRef

	if _, err := collector.gitOutput(ctx, repositoryRoot, "fetch", "--quiet", "origin", fetchRefspec); err != nil {
		return Evidence{}, fmt.Errorf("collect Git evidence: refresh base branch %q: %w", baseBranch, err)
	}

	mergeBaseSHA, err := collector.gitOutput(ctx, repositoryRoot, "merge-base", "HEAD", baseRef)
	if err != nil {
		return Evidence{}, fmt.Errorf("collect Git evidence: find merge base for %q: %w", baseBranch, err)
	}
	if mergeBaseSHA == "" {
		return Evidence{}, fmt.Errorf("collect Git evidence: merge base for %q is empty", baseBranch)
	}

	revisionRange := mergeBaseSHA + "..HEAD"
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
