package application

import (
	"context"

	"github.com/Merthoshan/PR-maker-CLI/internal/description"
	"github.com/Merthoshan/PR-maker-CLI/internal/gitcontext"
	"github.com/Merthoshan/PR-maker-CLI/internal/github"
)

type gitService interface {
	Collect(context.Context, string) (gitcontext.Repository, error)
	CollectEvidence(context.Context, string, string) (gitcontext.Evidence, error)
	CollectPullRequestEvidence(
		context.Context,
		string,
		string,
		int,
	) (gitcontext.Evidence, error)
}

type pullRequestResolver interface {
	ListOpen(context.Context, string, string) ([]github.PullRequest, error)
	GetOpenByNumber(
		context.Context,
		string,
		int,
	) (github.PullRequest, error)
}

type draftService interface {
	Generate(context.Context, description.Request) (description.Draft, error)
	Refine(context.Context, description.RefinementRequest) (description.Draft, error)
	SuggestTitles(
		context.Context,
		description.TitleSuggestionRequest,
	) ([]string, error)
}

type pullRequestPublisher interface {
	Publish(context.Context, github.PublishRequest) (github.PublishResult, error)
}

type progressReporter interface {
	Start(string) func()
}

// Dependencies contains the workflow boundaries replaced by tests.
type Dependencies struct {
	Git          gitService
	PullRequests pullRequestResolver
	Drafts       draftService
	Publisher    pullRequestPublisher
	LoadTemplate func(string) (string, error)
	Render       func(
		string,
		description.Draft,
		description.OutputMode,
	) (string, error)
}

// Outcome summarizes a completed or dry-run workflow.
type Outcome struct {
	URL     string
	Title   string
	Body    string
	Created bool
	DryRun  bool
}
