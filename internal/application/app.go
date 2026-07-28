package application

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"champu-pr/internal/cli"
	"champu-pr/internal/command"
	"champu-pr/internal/description"
	"champu-pr/internal/gitcontext"
	"champu-pr/internal/github"
	"champu-pr/internal/workflow"
)

// ErrCancelled reports that the user left without approving a GitHub change.
var ErrCancelled = errors.New("PR workflow cancelled")

type gitService interface {
	Collect(context.Context, string) (gitcontext.Repository, error)
	CollectEvidence(context.Context, string, string) (gitcontext.Evidence, error)
}

type pullRequestResolver interface {
	ListOpen(context.Context, string, string) ([]github.PullRequest, error)
}

type draftService interface {
	Generate(context.Context, description.Request) (description.Draft, error)
	Refine(context.Context, description.RefinementRequest) (description.Draft, error)
}

type pullRequestPublisher interface {
	Publish(context.Context, github.PublishRequest) (github.PublishResult, error)
}

// Dependencies contains the workflow boundaries replaced by tests.
type Dependencies struct {
	Git          gitService
	PullRequests pullRequestResolver
	Drafts       draftService
	Publisher    pullRequestPublisher
	LoadTemplate func(string) (string, error)
	Render       func(string, description.Draft) (string, error)
}

// Outcome summarizes a completed or dry-run workflow.
type Outcome struct {
	URL     string
	Title   string
	Body    string
	Created bool
	DryRun  bool
}

// App coordinates one PR-description workflow.
type App struct {
	dependencies Dependencies
	input        io.Reader
	output       io.Writer
}

// New creates an application from explicit dependencies.
func New(
	dependencies Dependencies,
	input io.Reader,
	output io.Writer,
) (*App, error) {
	switch {
	case dependencies.Git == nil:
		return nil, errors.New("create application: Git service is required")
	case dependencies.PullRequests == nil:
		return nil, errors.New("create application: PR resolver is required")
	case dependencies.Drafts == nil:
		return nil, errors.New("create application: draft service is required")
	case dependencies.Publisher == nil:
		return nil, errors.New("create application: publisher is required")
	case dependencies.LoadTemplate == nil:
		return nil, errors.New("create application: template loader is required")
	case dependencies.Render == nil:
		return nil, errors.New("create application: renderer is required")
	case input == nil:
		return nil, errors.New("create application: input is required")
	case output == nil:
		return nil, errors.New("create application: output is required")
	}
	return &App{
		dependencies: dependencies,
		input:        input,
		output:       output,
	}, nil
}

// NewDefault wires the production command-backed services.
func NewDefault(
	runner command.Runner,
	input io.Reader,
	output io.Writer,
) (*App, error) {
	gitCollector, err := gitcontext.NewCollector(runner)
	if err != nil {
		return nil, err
	}
	resolver, err := github.NewResolver(runner)
	if err != nil {
		return nil, err
	}
	generator, err := description.NewGenerator(runner)
	if err != nil {
		return nil, err
	}
	publisher, err := github.NewPublisher(runner)
	if err != nil {
		return nil, err
	}
	return New(Dependencies{
		Git:          gitCollector,
		PullRequests: resolver,
		Drafts:       generator,
		Publisher:    publisher,
		LoadTemplate: description.LoadTemplate,
		Render:       description.RenderMarkdown,
	}, input, output)
}

// Run collects evidence, generates an editable preview, and publishes only
// after exact user approval.
func (app *App) Run(
	ctx context.Context,
	options cli.Options,
	workingDirectory string,
) (Outcome, error) {
	repository, target, evidence, err := app.collectInputs(
		ctx,
		options,
		workingDirectory,
	)
	if err != nil {
		return Outcome{}, err
	}

	existingTitle, existingBody := existingDescription(target.PullRequest)
	draft, err := app.dependencies.Drafts.Generate(ctx, description.Request{
		RepositoryRoot: repository.Root,
		BaseBranch:     target.BaseBranch,
		ExistingTitle:  existingTitle,
		ExistingBody:   existingBody,
		Evidence:       evidence,
	})
	if err != nil {
		return Outcome{}, err
	}
	state, err := description.NewRefinementState(draft)
	if err != nil {
		return Outcome{}, err
	}
	template, err := app.dependencies.LoadTemplate(repository.Root)
	if err != nil {
		return Outcome{}, err
	}

	return app.editAndPublish(
		ctx,
		options,
		repository,
		target,
		evidence,
		template,
		&state,
	)
}

func (app *App) collectInputs(
	ctx context.Context,
	options cli.Options,
	workingDirectory string,
) (gitcontext.Repository, workflow.Target, gitcontext.Evidence, error) {
	repository, err := app.dependencies.Git.Collect(ctx, workingDirectory)
	if err != nil {
		return gitcontext.Repository{}, workflow.Target{},
			gitcontext.Evidence{}, err
	}
	pullRequests, err := app.dependencies.PullRequests.ListOpen(
		ctx,
		repository.Root,
		repository.Branch,
	)
	if err != nil {
		return gitcontext.Repository{}, workflow.Target{},
			gitcontext.Evidence{}, err
	}
	target, err := workflow.ResolveTarget(options, pullRequests)
	if err != nil {
		return gitcontext.Repository{}, workflow.Target{},
			gitcontext.Evidence{}, err
	}
	evidence, err := app.dependencies.Git.CollectEvidence(
		ctx,
		repository.Root,
		target.BaseBranch,
	)
	if err != nil {
		return gitcontext.Repository{}, workflow.Target{},
			gitcontext.Evidence{}, err
	}
	return repository, target, evidence, nil
}

func (app *App) editAndPublish(
	ctx context.Context,
	options cli.Options,
	repository gitcontext.Repository,
	target workflow.Target,
	evidence gitcontext.Evidence,
	template string,
	state *description.RefinementState,
) (Outcome, error) {
	scanner := bufio.NewScanner(app.input)
	for {
		body, err := app.dependencies.Render(template, state.Current)
		if err != nil {
			return Outcome{}, err
		}
		printPreview(app.output, state.Current.Title, body, options.DryRun)

		if options.DryRun {
			return Outcome{
				Title:  state.Current.Title,
				Body:   body,
				DryRun: true,
			}, nil
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return Outcome{}, fmt.Errorf("read refinement command: %w", err)
			}
			return Outcome{}, ErrCancelled
		}

		instruction := strings.TrimSpace(scanner.Text())
		switch instruction {
		case "apply":
			return app.publish(
				ctx,
				options,
				repository,
				target,
				state.Current.Title,
				body,
			)
		case "quit":
			return Outcome{}, ErrCancelled
		}

		candidate := state.Clone()
		result, err := candidate.Apply(instruction)
		if err != nil {
			fmt.Fprintf(app.output, "\nError: %v\n", err)
			continue
		}
		if result.NeedsRewrite {
			rewritten, err := app.dependencies.Drafts.Refine(
				ctx,
				description.RefinementRequest{
					RepositoryRoot: repository.Root,
					Instruction:    result.Instruction,
					State:          candidate,
					Evidence:       evidence,
				},
			)
			if err != nil {
				fmt.Fprintf(app.output, "\nError: %v\n", err)
				continue
			}
			if err := candidate.ReplaceCurrent(rewritten); err != nil {
				fmt.Fprintf(app.output, "\nError: %v\n", err)
				continue
			}
		}
		*state = candidate
	}
}

func (app *App) publish(
	ctx context.Context,
	options cli.Options,
	repository gitcontext.Repository,
	target workflow.Target,
	title string,
	body string,
) (Outcome, error) {
	result, err := app.dependencies.Publisher.Publish(
		ctx,
		github.PublishRequest{
			RepositoryRoot: repository.Root,
			HeadBranch:     repository.Branch,
			BaseBranch:     target.BaseBranch,
			Title:          title,
			Body:           body,
			PullRequest:    target.PullRequest,
			Ready:          options.Ready,
		},
	)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{
		URL:     result.URL,
		Title:   title,
		Body:    body,
		Created: result.Created,
	}, nil
}

func existingDescription(pullRequest *github.PullRequest) (string, string) {
	if pullRequest == nil {
		return "", ""
	}
	return pullRequest.Title, pullRequest.Body
}

func printPreview(
	output io.Writer,
	title string,
	body string,
	dryRun bool,
) {
	fmt.Fprintf(output, "\nPR title:\n%s\n\nPR description:\n%s", title, body)
	if dryRun {
		fmt.Fprintln(output, "\nDry run: GitHub was not changed.")
		return
	}
	fmt.Fprintln(
		output,
		"\nType a refinement command, `apply` to publish, or `quit` to cancel:",
	)
}
