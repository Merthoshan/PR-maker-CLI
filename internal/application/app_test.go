package application

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"champu-pr/internal/cli"
	"champu-pr/internal/description"
	"champu-pr/internal/gitcontext"
	"champu-pr/internal/github"
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
	fixture := newAppFixture(t, "Apply\napply\n")
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
}

func TestRunUsesExistingPullRequestAndRefines(t *testing.T) {
	fixture := newAppFixture(t, "make the summary shorter\napply\n")
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
}

func TestRunCancelsOnQuitOrEOF(t *testing.T) {
	for _, input := range []string{"quit\n", ""} {
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

func TestRunRollsBackRefinementWhenRewriteFails(t *testing.T) {
	fixture := newAppFixture(t, "exclude F1.C1\nquit\n")
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
	if _, err := New(Dependencies{}, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("New() error = nil, want dependency error")
	}
}

type appFixture struct {
	app       *App
	output    *bytes.Buffer
	resolver  *fakeResolver
	drafts    *fakeDrafts
	publisher *fakePublisher
}

func newAppFixture(t *testing.T, input string) appFixture {
	t.Helper()
	output := &bytes.Buffer{}
	git := fakeGit{
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
	app, err := New(Dependencies{
		Git:          git,
		PullRequests: resolver,
		Drafts:       drafts,
		Publisher:    publisher,
		LoadTemplate: func(string) (string, error) { return "", nil },
		Render:       description.RenderMarkdown,
	}, strings.NewReader(input), output)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	return appFixture{
		app:       app,
		output:    output,
		resolver:  resolver,
		drafts:    drafts,
		publisher: publisher,
	}
}

type fakeGit struct {
	repository gitcontext.Repository
	evidence   gitcontext.Evidence
}

func (git fakeGit) Collect(
	context.Context,
	string,
) (gitcontext.Repository, error) {
	return git.repository, nil
}

func (git fakeGit) CollectEvidence(
	_ context.Context,
	_ string,
	base string,
) (gitcontext.Evidence, error) {
	evidence := git.evidence
	evidence.BaseBranch = base
	return evidence, nil
}

type fakeResolver struct {
	pullRequests []github.PullRequest
}

func (resolver *fakeResolver) ListOpen(
	context.Context,
	string,
	string,
) ([]github.PullRequest, error) {
	return resolver.pullRequests, nil
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
			Details:   []string{"Waits for approval before publishing."},
		}},
		Testing: []string{"Not run (no test results provided)."},
	}
}
