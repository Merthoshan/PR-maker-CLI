package application

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Merthoshan/PR-maker-CLI/internal/cli"
	"github.com/Merthoshan/PR-maker-CLI/internal/command"
	"github.com/Merthoshan/PR-maker-CLI/internal/description"
	"github.com/Merthoshan/PR-maker-CLI/internal/gitcontext"
	"github.com/Merthoshan/PR-maker-CLI/internal/github"
	"github.com/Merthoshan/PR-maker-CLI/internal/workflow"
)

// ErrCancelled reports that the user left without approving a GitHub change.
var ErrCancelled = errors.New("PR workflow cancelled")

const previewSeparator = "------------------------------------------------------------"

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

// App coordinates one PR-description workflow.
type App struct {
	dependencies Dependencies
	input        io.Reader
	output       io.Writer
	progress     progressReporter
}

// New creates an application from explicit dependencies.
func New(
	dependencies Dependencies,
	input io.Reader,
	output io.Writer,
	progress progressReporter,
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
	case progress == nil:
		return nil, errors.New("create application: progress reporter is required")
	}
	return &App{
		dependencies: dependencies,
		input:        input,
		output:       output,
		progress:     progress,
	}, nil
}

// NewDefault wires the production command-backed services.
func NewDefault(
	runner command.Runner,
	input io.Reader,
	output io.Writer,
	progress progressReporter,
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
	}, input, output, progress)
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
	stopProgress := app.progress.Start(
		"Generating PR description with Codex",
	)
	draft, err := app.dependencies.Drafts.Generate(ctx, description.Request{
		RepositoryRoot: repository.Root,
		BaseBranch:     target.BaseBranch,
		ExistingTitle:  existingTitle,
		ExistingBody:   existingBody,
		Evidence:       evidence,
	})
	stopProgress()
	if err != nil {
		return Outcome{}, err
	}
	state, err := description.NewRefinementState(draft)
	if err != nil {
		return Outcome{}, err
	}
	scanner := bufio.NewScanner(app.input)
	service := ""
	if !options.DryRun {
		service, err = app.selectService(scanner)
		if err != nil {
			return Outcome{}, err
		}
	}
	titleBranch := repository.Branch
	if target.PullRequest != nil {
		titleBranch = target.PullRequest.HeadBranch
	}
	state.Current.Title = titleWithMetadata(
		state.Current.Title,
		titleBranch,
		service,
	)
	if err := validateTitle(state.Current.Title); err != nil {
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
		scanner,
		service,
	)
}

func (app *App) collectInputs(
	ctx context.Context,
	options cli.Options,
	workingDirectory string,
) (gitcontext.Repository, workflow.Target, gitcontext.Evidence, error) {
	stopProgress := app.progress.Start("Inspecting Git repository")
	repository, err := app.dependencies.Git.Collect(ctx, workingDirectory)
	stopProgress()
	if err != nil {
		return gitcontext.Repository{}, workflow.Target{},
			gitcontext.Evidence{}, err
	}
	target, err := app.resolveTarget(ctx, options, repository)
	if err != nil {
		return gitcontext.Repository{}, workflow.Target{},
			gitcontext.Evidence{}, err
	}
	stopProgress = app.progress.Start("Collecting Git evidence")
	var evidence gitcontext.Evidence
	if options.PRNumber > 0 {
		evidence, err = app.dependencies.Git.CollectPullRequestEvidence(
			ctx,
			repository.Root,
			target.BaseBranch,
			options.PRNumber,
		)
	} else {
		evidence, err = app.dependencies.Git.CollectEvidence(
			ctx,
			repository.Root,
			target.BaseBranch,
		)
	}
	stopProgress()
	if err != nil {
		return gitcontext.Repository{}, workflow.Target{},
			gitcontext.Evidence{}, err
	}
	return repository, target, evidence, nil
}

func (app *App) resolveTarget(
	ctx context.Context,
	options cli.Options,
	repository gitcontext.Repository,
) (workflow.Target, error) {
	if options.PRNumber > 0 {
		stopProgress := app.progress.Start(
			fmt.Sprintf("Finding pull request #%d", options.PRNumber),
		)
		pullRequest, err := app.dependencies.PullRequests.GetOpenByNumber(
			ctx,
			repository.Root,
			options.PRNumber,
		)
		stopProgress()
		if err != nil {
			return workflow.Target{}, err
		}
		return workflow.ResolveTarget(
			options,
			[]github.PullRequest{pullRequest},
		)
	}

	stopProgress := app.progress.Start("Finding open pull requests")
	pullRequests, err := app.dependencies.PullRequests.ListOpen(
		ctx,
		repository.Root,
		repository.Branch,
	)
	stopProgress()
	if err != nil {
		return workflow.Target{}, err
	}
	return workflow.ResolveTarget(options, pullRequests)
}

func (app *App) editAndPublish(
	ctx context.Context,
	options cli.Options,
	repository gitcontext.Repository,
	target workflow.Target,
	evidence gitcontext.Evidence,
	template string,
	state *description.RefinementState,
	scanner *bufio.Scanner,
	service string,
) (Outcome, error) {
	titleBranch := repository.Branch
	if target.PullRequest != nil {
		titleBranch = target.PullRequest.HeadBranch
	}
	for {
		body, err := app.dependencies.Render(
			template,
			state.Current,
			state.Mode,
		)
		if err != nil {
			return Outcome{}, err
		}
		printPreview(
			app.output,
			state.Current.Title,
			body,
			state.Mode,
			options.DryRun,
		)

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
			if state.Mode != description.OutputModeDescription {
				fmt.Fprintln(
					app.output,
					"\nError: run `make description` before `apply`.",
				)
				continue
			}
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
			stopProgress := app.progress.Start(
				"Refining PR description with Codex",
			)
			rewritten, err := app.dependencies.Drafts.Refine(
				ctx,
				description.RefinementRequest{
					RepositoryRoot: repository.Root,
					Instruction:    result.Instruction,
					State:          candidate,
					Evidence:       evidence,
				},
			)
			stopProgress()
			if err != nil {
				fmt.Fprintf(app.output, "\nError: %v\n", err)
				continue
			}
			if err := candidate.ReplaceCurrent(rewritten); err != nil {
				fmt.Fprintf(app.output, "\nError: %v\n", err)
				continue
			}
		}
		candidate.Current.Title = titleWithMetadata(
			candidate.Current.Title,
			titleBranch,
			service,
		)
		if err := validateTitle(candidate.Current.Title); err != nil {
			fmt.Fprintf(app.output, "\nError: %v\n", err)
			continue
		}
		*state = candidate
	}
}

func (app *App) selectService(scanner *bufio.Scanner) (string, error) {
	fmt.Fprintln(app.output, "\nSelect affected service(s) for the PR title:")
	fmt.Fprintln(app.output, "1. api")
	fmt.Fprintln(app.output, "2. worker")
	fmt.Fprintln(app.output, "3. api, worker")
	fmt.Fprintln(app.output, "4. omit service")
	prompt := "Choice: "
	for {
		fmt.Fprint(app.output, prompt)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("read service choice: %w", err)
			}
			return "", ErrCancelled
		}
		service, err := serviceFromChoice(scanner.Text())
		if err == nil {
			return service, nil
		}
		fmt.Fprintf(app.output, "\nError: %v\n", err)
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
	message := "Creating pull request on GitHub"
	if target.PullRequest != nil {
		message = fmt.Sprintf(
			"Updating pull request #%d on GitHub",
			target.PullRequest.Number,
		)
	}
	headBranch := repository.Branch
	if target.PullRequest != nil {
		headBranch = target.PullRequest.HeadBranch
	}
	stopProgress := app.progress.Start(message)
	result, err := app.dependencies.Publisher.Publish(
		ctx,
		github.PublishRequest{
			RepositoryRoot: repository.Root,
			HeadBranch:     headBranch,
			BaseBranch:     target.BaseBranch,
			Title:          title,
			Body:           body,
			PullRequest:    target.PullRequest,
			Ready:          options.Ready,
		},
	)
	stopProgress()
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
	mode description.OutputMode,
	dryRun bool,
) {
	fmt.Fprintf(output, "\n%s\n", previewSeparator)

	label := "File-wise changelog"
	if mode == description.OutputModeDescription {
		label = "PR description"
	}
	fmt.Fprintf(output, "\nPR title:\n%s\n\n%s:\n%s", title, label, body)
	if dryRun {
		fmt.Fprintln(output, "\nDry run: GitHub was not changed.")
		return
	}
	if mode == description.OutputModeChangelog {
		fmt.Fprintln(
			output,
			"\nRefine the changelog, `make description` to continue, or `quit` to cancel:",
		)
		return
	}
	fmt.Fprintln(
		output,
		"\nType a refinement command, `apply` to publish, or `quit` to cancel:",
	)
}
