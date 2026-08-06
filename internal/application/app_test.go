package application

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Merthoshan/PR-maker-CLI/internal/cli"
	"github.com/Merthoshan/PR-maker-CLI/internal/description"
	"github.com/Merthoshan/PR-maker-CLI/internal/gitcontext"
	"github.com/Merthoshan/PR-maker-CLI/internal/github"
)

func TestRunDryRunNeverPublishes(t *testing.T) {
	fixture := newAppFixture(t, "")
	outcome, err := fixture.app.Run(
		context.Background(),
		cli.Options{Base: "main", DryRun: true},
		"/working",
	)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !outcome.DryRun || fixture.publisher.calls != 0 {
		t.Fatalf("outcome = %+v, publish calls = %d", outcome, fixture.publisher.calls)
	}
	if !strings.Contains(fixture.output.String(), "GitHub was not changed") {
		t.Fatalf("output missing dry-run message:\n%s", fixture.output.String())
	}
}

func TestRunPublishesOnlyAfterExactApply(t *testing.T) {
	fixture := newAppFixture(t, "1\nApply\nmake description\napply\n")
	outcome, err := fixture.app.Run(
		context.Background(),
		cli.Options{Base: "main"},
		"/working",
	)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if fixture.publisher.calls != 1 || !outcome.Created {
		t.Fatalf("outcome = %+v, publish calls = %d", outcome, fixture.publisher.calls)
	}
	if fixture.drafts.refineCalls != 1 {
		t.Fatalf("refine calls = %d, want 1 for non-exact Apply", fixture.drafts.refineCalls)
	}
	for _, label := range []string{"File-wise changelog:", "PR description:"} {
		if !strings.Contains(fixture.output.String(), label) {
			t.Fatalf("output missing %q:\n%s", label, fixture.output.String())
		}
	}
	if count := strings.Count(
		fixture.output.String(),
		previewSeparator,
	); count < 3 {
		t.Fatalf(
			"preview separators = %d, want at least 3:\n%s",
			count,
			fixture.output.String(),
		)
	}
}

func TestRunUsesExistingPullRequestAndRefines(t *testing.T) {
	fixture := newAppFixture(
		t,
		"1\nmake the summary shorter\nmake description\napply\n",
	)
	fixture.resolver.pullRequests = []github.PullRequest{{
		Number:     12,
		Title:      "Old title",
		Body:       "Old body",
		URL:        "https://example.test/pr/12",
		BaseBranch: "main",
		HeadBranch: "feature",
		Draft:      true,
	}}
	outcome, err := fixture.app.Run(
		context.Background(),
		cli.Options{Base: "main", Ready: true},
		"/working",
	)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if fixture.drafts.generateRequest.ExistingTitle != "Old title" {
		t.Fatalf(
			"existing title = %q, want Old title",
			fixture.drafts.generateRequest.ExistingTitle,
		)
	}
	if fixture.drafts.refineCalls != 1 {
		t.Fatalf("refine calls = %d, want 1", fixture.drafts.refineCalls)
	}
	if fixture.publisher.request.PullRequest == nil ||
		fixture.publisher.request.PullRequest.Number != 12 ||
		!fixture.publisher.request.Ready {
		t.Fatalf("publish request = %+v", fixture.publisher.request)
	}
	if outcome.Created {
		t.Fatal("outcome Created = true, want updated PR")
	}
	fixture.progress.assertBalanced(t)
	for _, want := range []string{
		"Inspecting Git repository",
		"Finding open pull requests",
		"Collecting Git evidence",
		"Generating PR description with Codex",
		"Refining PR description with Codex",
		"Updating pull request #12 on GitHub",
	} {
		if !fixture.progress.startedMessage(want) {
			t.Fatalf(
				"progress messages = %q, want %q",
				fixture.progress.started,
				want,
			)
		}
	}
}

func TestRunResolvesPullRequestFromDifferentBranch(t *testing.T) {
	fixture := newAppFixture(t, "1\nmake description\napply\n")
	fixture.git.repository.Branch = "main"
	fixture.resolver.pullRequest = github.PullRequest{
		Number:     888,
		State:      "OPEN",
		Title:      "Fix link pricesheet",
		Body:       "Existing PR body",
		URL:        "https://example.test/pr/888",
		BaseBranch: "main",
		HeadBranch: "fix-link-pricesheet",
	}

	outcome, err := fixture.app.Run(
		context.Background(),
		cli.Options{PRNumber: 888},
		"/working",
	)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if outcome.Created {
		t.Fatal("outcome Created = true, want existing PR update")
	}
	if fixture.resolver.getByNumberCalls != 1 ||
		fixture.resolver.listOpenCalls != 0 {
		t.Fatalf(
			"resolver calls = get %d, list %d; want get 1, list 0",
			fixture.resolver.getByNumberCalls,
			fixture.resolver.listOpenCalls,
		)
	}
	if fixture.git.pullRequestEvidenceNumber != 888 ||
		fixture.git.collectEvidenceCalls != 0 {
		t.Fatalf(
			"evidence calls = PR #%d, HEAD %d; want PR #888 only",
			fixture.git.pullRequestEvidenceNumber,
			fixture.git.collectEvidenceCalls,
		)
	}
	if fixture.publisher.request.HeadBranch != "fix-link-pricesheet" {
		t.Fatalf(
			"publish head = %q, want PR head branch",
			fixture.publisher.request.HeadBranch,
		)
	}
	if fixture.drafts.generateRequest.ExistingTitle != "Fix link pricesheet" {
		t.Fatalf(
			"existing title = %q, want PR title",
			fixture.drafts.generateRequest.ExistingTitle,
		)
	}
}

func TestRunCancelsOnQuitOrEOF(t *testing.T) {
	for _, input := range []string{"4\nquit\n", "4\n"} {
		t.Run(input, func(t *testing.T) {
			fixture := newAppFixture(t, input)
			_, err := fixture.app.Run(
				context.Background(),
				cli.Options{Base: "main"},
				"/working",
			)
			if !errors.Is(err, ErrCancelled) {
				t.Fatalf("Run() error = %v, want ErrCancelled", err)
			}
			if fixture.publisher.calls != 0 {
				t.Fatalf("publish calls = %d, want zero", fixture.publisher.calls)
			}
		})
	}
}

func TestRunRequiresDescriptionBeforeApply(t *testing.T) {
	fixture := newAppFixture(t, "4\napply\nquit\n")

	_, err := fixture.app.Run(
		context.Background(),
		cli.Options{Base: "main"},
		"/working",
	)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Run() error = %v, want ErrCancelled", err)
	}
	if fixture.publisher.calls != 0 {
		t.Fatalf("publish calls = %d, want zero", fixture.publisher.calls)
	}
	if !strings.Contains(
		fixture.output.String(),
		"run `make description` before `apply`",
	) {
		t.Fatalf(
			"output missing description requirement:\n%s",
			fixture.output.String(),
		)
	}
}

func TestRunRollsBackRefinementWhenRewriteFails(t *testing.T) {
	fixture := newAppFixture(t, "4\nexclude F1.C1\nquit\n")
	fixture.drafts.refineErr = errors.New("Codex unavailable")

	_, err := fixture.app.Run(
		context.Background(),
		cli.Options{Base: "main"},
		"/working",
	)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("Run() error = %v, want ErrCancelled", err)
	}
	if count := strings.Count(
		fixture.output.String(),
		"F1.C1",
	); count != 2 {
		t.Fatalf(
			"F1.C1 preview count = %d, want original change in both previews:\n%s",
			count,
			fixture.output.String(),
		)
	}
}

func TestNewValidatesDependencies(t *testing.T) {
	if _, err := New(
		Dependencies{},
		strings.NewReader(""),
		&bytes.Buffer{},
		&fakeProgress{},
	); err == nil {
		t.Fatal("New() error = nil, want dependency error")
	}
}

type appFixture struct {
	app       *App
	output    *bytes.Buffer
	git       *fakeGit
	resolver  *fakeResolver
	drafts    *fakeDrafts
	publisher *fakePublisher
	progress  *fakeProgress
}

func newAppFixture(t *testing.T, input string) appFixture {
	t.Helper()
	output := &bytes.Buffer{}
	git := &fakeGit{
		repository: gitcontext.Repository{
			Root:   "/repo/gallery",
			Branch: "feature",
		},
		evidence: gitcontext.Evidence{
			BaseBranch:   "main",
			MergeBaseSHA: "abc123",
			Diff:         "diff --git a/file.go b/file.go",
		},
	}
	resolver := &fakeResolver{}
	drafts := &fakeDrafts{draft: appDraft()}
	publisher := &fakePublisher{}
	progress := &fakeProgress{}
	t.Cleanup(func() {
		progress.assertBalanced(t)
	})
	app, err := New(Dependencies{
		Git:          git,
		PullRequests: resolver,
		Drafts:       drafts,
		Publisher:    publisher,
		LoadTemplate: func(string) (string, error) { return "", nil },
		Render:       description.RenderMarkdown,
	}, strings.NewReader(input), output, progress)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	return appFixture{
		app:       app,
		output:    output,
		git:       git,
		resolver:  resolver,
		drafts:    drafts,
		publisher: publisher,
		progress:  progress,
	}
}

type fakeGit struct {
	repository                gitcontext.Repository
	evidence                  gitcontext.Evidence
	collectEvidenceCalls      int
	pullRequestEvidenceNumber int
}

func (git *fakeGit) Collect(
	context.Context,
	string,
) (gitcontext.Repository, error) {
	return git.repository, nil
}

func (git *fakeGit) CollectEvidence(
	_ context.Context,
	_ string,
	base string,
) (gitcontext.Evidence, error) {
	git.collectEvidenceCalls++
	evidence := git.evidence
	evidence.BaseBranch = base
	return evidence, nil
}

func (git *fakeGit) CollectPullRequestEvidence(
	_ context.Context,
	_ string,
	base string,
	number int,
) (gitcontext.Evidence, error) {
	git.pullRequestEvidenceNumber = number
	evidence := git.evidence
	evidence.BaseBranch = base
	return evidence, nil
}

type fakeResolver struct {
	pullRequests     []github.PullRequest
	pullRequest      github.PullRequest
	listOpenCalls    int
	getByNumberCalls int
}

func (resolver *fakeResolver) ListOpen(
	context.Context,
	string,
	string,
) ([]github.PullRequest, error) {
	resolver.listOpenCalls++
	return resolver.pullRequests, nil
}

func (resolver *fakeResolver) GetOpenByNumber(
	_ context.Context,
	_ string,
	_ int,
) (github.PullRequest, error) {
	resolver.getByNumberCalls++
	return resolver.pullRequest, nil
}

type fakeDrafts struct {
	draft           description.Draft
	generateRequest description.Request
	refineCalls     int
	refineErr       error
}

func (drafts *fakeDrafts) Generate(
	_ context.Context,
	request description.Request,
) (description.Draft, error) {
	drafts.generateRequest = request
	return drafts.draft, nil
}

func (drafts *fakeDrafts) Refine(
	_ context.Context,
	request description.RefinementRequest,
) (description.Draft, error) {
	drafts.refineCalls++
	if drafts.refineErr != nil {
		return description.Draft{}, drafts.refineErr
	}
	draft := request.State.Current
	draft.Title = "Refined PR title"
	return draft, nil
}

type fakePublisher struct {
	request github.PublishRequest
	calls   int
}

type fakeProgress struct {
	started []string
	stopped int
	active  int
}

func (progress *fakeProgress) Start(message string) func() {
	progress.started = append(progress.started, message)
	progress.active++
	stopped := false
	return func() {
		if stopped {
			return
		}
		stopped = true
		progress.stopped++
		progress.active--
	}
}

func (progress *fakeProgress) startedMessage(message string) bool {
	for _, started := range progress.started {
		if started == message {
			return true
		}
	}
	return false
}

func (progress *fakeProgress) assertBalanced(t *testing.T) {
	t.Helper()
	if progress.active != 0 || progress.stopped != len(progress.started) {
		t.Fatalf(
			"progress started = %d, stopped = %d, active = %d",
			len(progress.started),
			progress.stopped,
			progress.active,
		)
	}
}

func (publisher *fakePublisher) Publish(
	_ context.Context,
	request github.PublishRequest,
) (github.PublishResult, error) {
	publisher.request = request
	publisher.calls++
	return github.PublishResult{
		URL:     "https://example.test/pr/1",
		Created: request.PullRequest == nil,
	}, nil
}

func appDraft() description.Draft {
	return description.Draft{
		Title:   "Add PR workflow",
		Summary: []string{"Generate and publish a PR description."},
		Changes: []description.Change{{
			ID:        "F1.C1",
			File:      "internal/application/app.go",
			Operation: "added",
			Element:   "App.Run",
			Summary:   "Coordinate the PR workflow.",
		}},
		Testing: []string{"Not run (no test results provided)."},
	}
}
