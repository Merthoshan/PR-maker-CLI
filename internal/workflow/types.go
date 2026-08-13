package workflow

import "github.com/Merthoshan/PR-maker-CLI/internal/github"

// Target describes the pull request and base branch selected for one workflow
// run.
type Target struct {
	PullRequest  *github.PullRequest
	BaseBranch   string
	ShouldCreate bool
}
