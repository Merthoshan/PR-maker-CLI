package github

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
)

func TestPublisherCreatesDraftByDefault(t *testing.T) {
	var got command.Spec
	publisher := mustNewPublisher(t, &publisherRunner{
		t: t,
		run: func(spec command.Spec) (command.Result, error) {
			got = spec
			return command.Result{
				Stdout: "https://github.com/acme/repo/pull/42\n",
			}, nil
		},
	})

	result, err := publisher.Publish(context.Background(), validPublishRequest())
	if err != nil {
		t.Fatalf("Publish() unexpected error: %v", err)
	}
	wantArgs := []string{
		"pr", "create",
		"--base", "main",
		"--head", "feature",
		"--title", "Add PR workflow",
		"--body-file", "-",
		"--draft",
	}
	if got.Name != "gh" || !reflect.DeepEqual(got.Args, wantArgs) {
		t.Fatalf("command = %+v, want args %q", got, wantArgs)
	}
	if got.Stdin != "Rendered body" {
		t.Fatalf("stdin = %q, want rendered body", got.Stdin)
	}
	if !result.Created || result.URL == "" {
		t.Fatalf("result = %+v, want created URL", result)
	}
}

func TestPublisherCreatesReadyPullRequest(t *testing.T) {
	var args []string
	publisher := mustNewPublisher(t, &publisherRunner{
		t: t,
		run: func(spec command.Spec) (command.Result, error) {
			args = spec.Args
			return command.Result{Stdout: "https://example.test/pr/1"}, nil
		},
	})
	request := validPublishRequest()
	request.Ready = true

	if _, err := publisher.Publish(context.Background(), request); err != nil {
		t.Fatalf("Publish() unexpected error: %v", err)
	}
	for _, arg := range args {
		if arg == "--draft" {
			t.Fatalf("ready create args contain --draft: %q", args)
		}
	}
}

func TestPublisherUpdatesAndMarksDraftReady(t *testing.T) {
	var calls []command.Spec
	publisher := mustNewPublisher(t, &publisherRunner{
		t: t,
		run: func(spec command.Spec) (command.Result, error) {
			calls = append(calls, spec)
			return command.Result{}, nil
		},
	})
	request := validPublishRequest()
	request.Ready = true
	request.PullRequest = &PullRequest{
		Number: 7,
		URL:    "https://example.test/pr/7",
		Draft:  true,
	}

	result, err := publisher.Publish(context.Background(), request)
	if err != nil {
		t.Fatalf("Publish() unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want edit and ready", len(calls))
	}
	if !reflect.DeepEqual(calls[0].Args[:3], []string{"pr", "edit", "7"}) {
		t.Fatalf("edit args = %q", calls[0].Args)
	}
	if !reflect.DeepEqual(calls[1].Args, []string{"pr", "ready", "7"}) {
		t.Fatalf("ready args = %q", calls[1].Args)
	}
	if result.Created || result.URL != request.PullRequest.URL {
		t.Fatalf("result = %+v, want updated URL", result)
	}
}

func TestPublisherDoesNotMarkReadyPullRequestAgain(t *testing.T) {
	runner := &publisherRunner{
		t: t,
		run: func(command.Spec) (command.Result, error) {
			return command.Result{}, nil
		},
	}
	publisher := mustNewPublisher(t, runner)
	request := validPublishRequest()
	request.Ready = true
	request.PullRequest = &PullRequest{Number: 7, Draft: false}

	if _, err := publisher.Publish(context.Background(), request); err != nil {
		t.Fatalf("Publish() unexpected error: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("calls = %d, want only edit", runner.calls)
	}
}

func TestPublisherValidatesBeforeRunAndWrapsFailures(t *testing.T) {
	runner := &publisherRunner{t: t}
	publisher := mustNewPublisher(t, runner)
	if _, err := publisher.Publish(context.Background(), PublishRequest{}); err == nil {
		t.Fatal("Publish() error = nil, want validation error")
	}
	if runner.calls != 0 {
		t.Fatalf("calls = %d, want zero", runner.calls)
	}

	sentinel := errors.New("exit status 1")
	runner.run = func(command.Spec) (command.Result, error) {
		return command.Result{Stderr: "authentication required"}, sentinel
	}
	_, err := publisher.Publish(context.Background(), validPublishRequest())
	if !errors.Is(err, sentinel) ||
		!strings.Contains(err.Error(), "authentication required") {
		t.Fatalf("Publish() error = %v, want wrapped command failure", err)
	}
}

func TestPublisherRequiresRunner(t *testing.T) {
	_, err := (Publisher{}).Publish(
		context.Background(),
		validPublishRequest(),
	)
	if err == nil || !strings.Contains(err.Error(), "runner is required") {
		t.Fatalf("Publish() error = %v, want runner validation", err)
	}
}

type publisherRunner struct {
	t     *testing.T
	run   func(command.Spec) (command.Result, error)
	calls int
}

func (runner *publisherRunner) Run(
	_ context.Context,
	spec command.Spec,
) (command.Result, error) {
	runner.t.Helper()
	runner.calls++
	if runner.run == nil {
		runner.t.Fatalf("unexpected command: %+v", spec)
	}
	return runner.run(spec)
}

func mustNewPublisher(t *testing.T, runner command.Runner) Publisher {
	t.Helper()
	publisher, err := NewPublisher(runner)
	if err != nil {
		t.Fatalf("NewPublisher() unexpected error: %v", err)
	}
	return publisher
}

func validPublishRequest() PublishRequest {
	return PublishRequest{
		RepositoryRoot: "/repo/gallery",
		HeadBranch:     "feature",
		BaseBranch:     "main",
		Title:          "Add PR workflow",
		Body:           "Rendered body",
	}
}
