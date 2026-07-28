package workflow

import (
	"errors"
	"fmt"

	"champu-pr/internal/cli"
	"champu-pr/internal/github"
)

// Target describes the pull request and base branch selected for one workflow
// run.
type Target struct {
	PullRequest  *github.PullRequest
	BaseBranch   string
	ShouldCreate bool
}

// ResolveTarget converts validated CLI options and open pull requests into one
// workflow target.
func ResolveTarget(options cli.Options, pullRequests []github.PullRequest) (Target, error) {
	if options.PRNumber > 0 {
		pr, err := github.FindPullRequestByNumber(pullRequests, options.PRNumber)
		if err != nil {
			return Target{}, fmt.Errorf(
				"resolve workflow target by pull request number: %w",
				err,
			)
		}
		return Target{
			PullRequest: &pr,
			BaseBranch:  pr.BaseBranch,
		}, nil
	}

	selection, err := github.SelectPullRequestByBase(pullRequests, options.Base)
	if err != nil {
		return Target{}, fmt.Errorf(
			"resolve workflow target by base branch %q: %w",
			options.Base,
			err,
		)
	}

	if selection.ShouldCreate {
		return Target{
			BaseBranch:   options.Base,
			ShouldCreate: true,
		}, nil
	}

	if selection.PullRequest == nil {
		return Target{}, errors.New(
			"resolve workflow target: existing selection has no pull request",
		)
	}

	return Target{
		PullRequest: selection.PullRequest,
		BaseBranch:  selection.PullRequest.BaseBranch,
	}, nil
}
