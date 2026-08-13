package github

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
)

func TestNewResolver(t *testing.T) {
	t.Run("creates resolver with runner", func(t *testing.T) {
		runner := &resolverRunner{t: t}

		resolver, err := NewResolver(runner)
		if err != nil {
			t.Fatalf("NewResolver() unexpected error: %v", err)
		}
		if resolver.runner != runner {
			t.Fatal("NewResolver() did not retain runner")
		}
	})

	t.Run("requires runner", func(t *testing.T) {
		resolver, err := NewResolver(nil)
		if err == nil {
			t.Fatal("NewResolver() error = nil, want runner validation error")
		}
		if !strings.Contains(err.Error(), "runner is required") {
			t.Fatalf("NewResolver() error = %q, want runner validation error", err)
		}
		if resolver != (Resolver{}) {
			t.Fatalf("NewResolver() resolver = %+v, want zero value", resolver)
		}
	})
}

func TestResolverListOpenRequiresRunner(t *testing.T) {
	_, err := (Resolver{}).ListOpen(
		context.Background(),
		"/repo/gallery",
		"feature",
	)
	if err == nil || !strings.Contains(err.Error(), "runner is required") {
		t.Fatalf("ListOpen() error = %v, want runner validation", err)
	}
}

func TestParseOwnerRepositoryPath(t *testing.T) {
	for _, value := range []string{"acme/service", " /acme/service/ "} {
		got, err := ParseOwnerRepositoryPath(value)
		if err != nil || got != "acme/service" {
			t.Fatalf("ParseOwnerRepositoryPath(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"", "acme", "acme/team/service", "/service"} {
		if _, err := ParseOwnerRepositoryPath(value); err == nil {
			t.Fatalf("ParseOwnerRepositoryPath(%q) error = nil", value)
		}
	}
}

func TestResolverGetOpenByNumber(t *testing.T) {
	response := `{
		"number": 888,
		"state": "OPEN",
		"title": "Fix link pricesheet",
		"url": "https://github.com/aftershootco/gallery-go-backend/pull/888",
		"body": "PR body",
		"baseRefName": "main",
		"headRefName": "fix-link-pricesheet",
		"isDraft": false
	}`
	runner := &resolverRunner{
		t: t,
		want: command.Spec{
			Name: "gh",
			Args: []string{
				"pr", "view", "888",
				"--json",
				"number,state,title,url,body,baseRefName,headRefName,isDraft",
			},
			Dir: "/repo/gallery",
		},
		result: command.Result{Stdout: response},
	}
	resolver := mustNewResolver(t, runner)

	pullRequest, err := resolver.GetOpenByNumber(
		context.Background(),
		"  /repo/gallery  ",
		888,
	)
	if err != nil {
		t.Fatalf("GetOpenByNumber() unexpected error: %v", err)
	}
	want := PullRequest{
		Number:     888,
		State:      "OPEN",
		Title:      "Fix link pricesheet",
		URL:        "https://github.com/aftershootco/gallery-go-backend/pull/888",
		Body:       "PR body",
		BaseBranch: "main",
		HeadBranch: "fix-link-pricesheet",
	}
	if !reflect.DeepEqual(pullRequest, want) {
		t.Fatalf("GetOpenByNumber() = %+v, want %+v", pullRequest, want)
	}
	runner.assertCalledOnce()
}

func TestResolverGetReview(t *testing.T) {
	metadata := `{
		"number":123,
		"state":"OPEN",
		"title":"Add review",
		"url":"https://github.com/acme/service/pull/123",
		"body":"PR body",
		"baseRefName":"main",
		"headRefName":"review",
		"isDraft":false,
		"labels":[{"name":"backend"}],
		"files":[{"path":"review.go","additions":5,"deletions":1}]
	}`
	diff := "diff --git a/review.go b/review.go\n@@ -1 +1 @@\n-old\n+new\n"
	runner := &resolverSequenceRunner{
		t: t,
		runs: []resolverSequenceRun{
			{
				want: command.Spec{
					Name: "gh",
					Args: []string{
						"pr", "view", "123",
						"--json",
						"number,state,title,url,body,baseRefName,headRefName,isDraft,labels,files",
					},
					Dir: "/repo/service",
				},
				result: command.Result{Stdout: metadata},
			},
			{
				want: command.Spec{
					Name:        "gh",
					Args:        []string{"pr", "diff", "123", "--color=never"},
					Dir:         "/repo/service",
					StdoutLimit: 1024,
				},
				result: command.Result{Stdout: diff, StdoutTruncated: true},
			},
		},
	}
	resolver := mustNewResolver(t, runner)

	data, err := resolver.GetReview(context.Background(), ReviewRequest{
		RepositoryRoot:     " /repo/service ",
		Target:             " 123 ",
		ExpectedRepository: " ACME/SERVICE ",
		DiffByteLimit:      1024,
	})
	if err != nil {
		t.Fatalf("GetReview() unexpected error: %v", err)
	}
	want := ReviewData{
		PullRequest: PullRequest{
			Number:     123,
			State:      "OPEN",
			Title:      "Add review",
			URL:        "https://github.com/acme/service/pull/123",
			Body:       "PR body",
			BaseBranch: "main",
			HeadBranch: "review",
		},
		Labels:      []string{"backend"},
		Repository:  "acme/service",
		Files:       []ChangedFile{{Path: "review.go", Additions: 5, Deletions: 1}},
		Diff:        diff,
		DiffLimited: true,
	}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("GetReview() = %+v, want %+v", data, want)
	}
	runner.assertComplete()
}

func TestResolverGetReviewRejectsDifferentRepositoryBeforeDiff(t *testing.T) {
	runner := &resolverSequenceRunner{
		t: t,
		runs: []resolverSequenceRun{{
			want: command.Spec{
				Name: "gh",
				Args: []string{
					"pr", "view", "https://github.com/other/service/pull/9",
					"--json",
					"number,state,title,url,body,baseRefName,headRefName,isDraft,labels,files",
				},
				Dir: "/repo/service",
			},
			result: command.Result{Stdout: `{"number":9,"state":"OPEN","url":"https://github.com/other/service/pull/9"}`},
		}},
	}
	resolver := mustNewResolver(t, runner)

	_, err := resolver.GetReview(context.Background(), ReviewRequest{
		RepositoryRoot:     "/repo/service",
		Target:             "https://github.com/other/service/pull/9",
		ExpectedRepository: "acme/service",
		DiffByteLimit:      1024,
	})
	if err == nil || !strings.Contains(err.Error(), "belongs to other/service") {
		t.Fatalf("GetReview() error = %v, want repository mismatch", err)
	}
	runner.assertComplete()
}

func TestResolverGetReviewValidation(t *testing.T) {
	tests := []struct {
		name    string
		request ReviewRequest
		want    string
	}{
		{name: "root", request: ReviewRequest{Target: "1", ExpectedRepository: "acme/service", DiffByteLimit: 1}, want: "repository root"},
		{name: "target", request: ReviewRequest{RepositoryRoot: "/repo", ExpectedRepository: "acme/service", DiffByteLimit: 1}, want: "target"},
		{name: "repository", request: ReviewRequest{RepositoryRoot: "/repo", Target: "1", DiffByteLimit: 1}, want: "expected repository"},
		{name: "limit", request: ReviewRequest{RepositoryRoot: "/repo", Target: "1", ExpectedRepository: "acme/service"}, want: "byte limit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &resolverSequenceRunner{t: t}
			resolver := mustNewResolver(t, runner)
			if _, err := resolver.GetReview(context.Background(), test.request); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("GetReview() error = %v, want %q", err, test.want)
			}
			if runner.calls != 0 {
				t.Fatalf("runner calls = %d, want 0", runner.calls)
			}
		})
	}
}

func TestResolverGetReviewErrors(t *testing.T) {
	request := ReviewRequest{
		RepositoryRoot:     "/repo/service",
		Target:             "123",
		ExpectedRepository: "acme/service",
		DiffByteLimit:      1024,
	}
	viewSpec := command.Spec{
		Name: "gh",
		Args: []string{
			"pr", "view", "123",
			"--json",
			"number,state,title,url,body,baseRefName,headRefName,isDraft,labels,files",
		},
		Dir: "/repo/service",
	}
	diffSpec := command.Spec{
		Name:        "gh",
		Args:        []string{"pr", "diff", "123", "--color=never"},
		Dir:         "/repo/service",
		StdoutLimit: 1024,
	}
	openMetadata := `{"number":123,"state":"OPEN","url":"https://github.com/acme/service/pull/123","files":[]}`
	sentinel := errors.New("command failed")
	tests := []struct {
		name        string
		runs        []resolverSequenceRun
		want        string
		wantWrapped bool
	}{
		{
			name:        "metadata command",
			runs:        []resolverSequenceRun{{want: viewSpec, result: command.Result{Stderr: "authentication required"}, err: sentinel}},
			want:        "metadata",
			wantWrapped: true,
		},
		{
			name: "metadata JSON",
			runs: []resolverSequenceRun{{want: viewSpec, result: command.Result{Stdout: "{"}}},
			want: "decode gh response",
		},
		{
			name: "closed pull request",
			runs: []resolverSequenceRun{{want: viewSpec, result: command.Result{Stdout: `{"number":123,"state":"CLOSED","url":"https://github.com/acme/service/pull/123"}`}}},
			want: "is CLOSED",
		},
		{
			name: "diff command",
			runs: []resolverSequenceRun{
				{want: viewSpec, result: command.Result{Stdout: openMetadata}},
				{want: diffSpec, result: command.Result{Stderr: "diff unavailable"}, err: sentinel},
			},
			want:        "review diff",
			wantWrapped: true,
		},
		{
			name: "empty diff",
			runs: []resolverSequenceRun{
				{want: viewSpec, result: command.Result{Stdout: openMetadata}},
				{want: diffSpec, result: command.Result{Stdout: " \n"}},
			},
			want: "empty diff",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &resolverSequenceRunner{t: t, runs: test.runs}
			resolver := mustNewResolver(t, runner)
			_, err := resolver.GetReview(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("GetReview() error = %v, want %q", err, test.want)
			}
			if test.wantWrapped && !errors.Is(err, sentinel) {
				t.Fatalf("GetReview() error = %v, want wrapped sentinel", err)
			}
			runner.assertComplete()
		})
	}
}

func TestResolverGetOpenByNumberValidation(t *testing.T) {
	tests := []struct {
		name           string
		repositoryRoot string
		number         int
		wantError      string
	}{
		{
			name:      "requires repository root",
			number:    888,
			wantError: "repository root is required",
		},
		{
			name:           "requires positive number",
			repositoryRoot: "/repo/gallery",
			wantError:      "number must be positive",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &resolverRunner{t: t}
			resolver := mustNewResolver(t, runner)

			_, err := resolver.GetOpenByNumber(
				context.Background(),
				test.repositoryRoot,
				test.number,
			)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf(
					"GetOpenByNumber() error = %v, want %q",
					err,
					test.wantError,
				)
			}
			if runner.calls != 0 {
				t.Fatalf("runner calls = %d, want 0", runner.calls)
			}
		})
	}
}

func TestResolverGetOpenByNumberRejectsClosedPullRequest(t *testing.T) {
	runner := &resolverRunner{
		t: t,
		want: command.Spec{
			Name: "gh",
			Args: []string{
				"pr", "view", "888",
				"--json",
				"number,state,title,url,body,baseRefName,headRefName,isDraft",
			},
			Dir: "/repo/gallery",
		},
		result: command.Result{
			Stdout: `{"number":888,"state":"CLOSED"}`,
		},
	}
	resolver := mustNewResolver(t, runner)

	_, err := resolver.GetOpenByNumber(
		context.Background(),
		"/repo/gallery",
		888,
	)
	if err == nil || !strings.Contains(err.Error(), "is CLOSED, expected OPEN") {
		t.Fatalf("GetOpenByNumber() error = %v, want closed-state error", err)
	}
	runner.assertCalledOnce()
}

func TestResolverListOpen(t *testing.T) {
	response := `[
		{
			"number": 42,
			"title": "Add collection CRUD",
			"url": "https://github.com/aftershootco/gallery-go-backend/pull/42",
			"body": "PR body",
			"baseRefName": "main",
			"headRefName": "gal-1767-crud-for-collections",
			"isDraft": true
		},
		{
			"number": 43,
			"title": "Test collection CRUD",
			"url": "https://github.com/aftershootco/gallery-go-backend/pull/43",
			"body": "",
			"baseRefName": "develop",
			"headRefName": "gal-1767-crud-for-collections",
			"isDraft": false
		}
	]`
	runner := &resolverRunner{
		t: t,
		want: command.Spec{
			Name: "gh",
			Args: []string{
				"pr", "list",
				"--head", "gal-1767-crud-for-collections",
				"--state", "open",
				"--json",
				"number,title,url,body,baseRefName,headRefName,isDraft",
			},
			Dir: "/repo/gallery",
		},
		result: command.Result{Stdout: response},
	}
	resolver := mustNewResolver(t, runner)

	pullRequests, err := resolver.ListOpen(
		context.Background(),
		"  /repo/gallery  ",
		"  gal-1767-crud-for-collections  ",
	)
	if err != nil {
		t.Fatalf("ListOpen() unexpected error: %v", err)
	}

	want := []PullRequest{
		{
			Number:     42,
			Title:      "Add collection CRUD",
			URL:        "https://github.com/aftershootco/gallery-go-backend/pull/42",
			Body:       "PR body",
			BaseBranch: "main",
			HeadBranch: "gal-1767-crud-for-collections",
			Draft:      true,
		},
		{
			Number:     43,
			Title:      "Test collection CRUD",
			URL:        "https://github.com/aftershootco/gallery-go-backend/pull/43",
			BaseBranch: "develop",
			HeadBranch: "gal-1767-crud-for-collections",
		},
	}
	if !reflect.DeepEqual(pullRequests, want) {
		t.Fatalf("ListOpen() = %+v, want %+v", pullRequests, want)
	}
	runner.assertCalledOnce()
}

func TestResolverListOpenValidation(t *testing.T) {
	tests := []struct {
		name           string
		repositoryRoot string
		headBranch     string
		wantError      string
	}{
		{
			name:       "requires repository root",
			headBranch: "feature",
			wantError:  "repository root is required",
		},
		{
			name:           "rejects whitespace repository root",
			repositoryRoot: " \n\t ",
			headBranch:     "feature",
			wantError:      "repository root is required",
		},
		{
			name:           "requires head branch",
			repositoryRoot: "/repo/gallery",
			wantError:      "head branch is required",
		},
		{
			name:           "rejects whitespace head branch",
			repositoryRoot: "/repo/gallery",
			headBranch:     " \n\t ",
			wantError:      "head branch is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &resolverRunner{t: t}
			resolver := mustNewResolver(t, runner)

			pullRequests, err := resolver.ListOpen(
				context.Background(),
				test.repositoryRoot,
				test.headBranch,
			)
			if err == nil {
				t.Fatal("ListOpen() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("ListOpen() error = %q, want %q", err, test.wantError)
			}
			if pullRequests != nil {
				t.Fatalf("ListOpen() pull requests = %+v, want nil", pullRequests)
			}
			if runner.calls != 0 {
				t.Fatalf("runner calls = %d, want 0", runner.calls)
			}
		})
	}
}

func TestResolverListOpenErrors(t *testing.T) {
	t.Run("wraps gh command failure", func(t *testing.T) {
		sentinel := errors.New("gh failed")
		runner := &resolverRunner{
			t: t,
			want: command.Spec{
				Name: "gh",
				Args: []string{
					"pr", "list",
					"--head", "feature",
					"--state", "open",
					"--json",
					"number,title,url,body,baseRefName,headRefName,isDraft",
				},
				Dir: "/repo/gallery",
			},
			err: sentinel,
		}
		resolver := mustNewResolver(t, runner)

		pullRequests, err := resolver.ListOpen(
			context.Background(),
			"/repo/gallery",
			"feature",
		)
		if err == nil {
			t.Fatal("ListOpen() error = nil, want command error")
		}
		if !errors.Is(err, sentinel) {
			t.Fatalf("ListOpen() error = %v, want wrapped sentinel error", err)
		}
		if !strings.Contains(err.Error(), `head branch "feature"`) ||
			!strings.Contains(err.Error(), "gh failed") {
			t.Fatalf("ListOpen() error = %q, want command context", err)
		}
		if pullRequests != nil {
			t.Fatalf("ListOpen() pull requests = %+v, want nil", pullRequests)
		}
		runner.assertCalledOnce()
	})

	t.Run("wraps invalid gh response", func(t *testing.T) {
		runner := &resolverRunner{
			t: t,
			want: command.Spec{
				Name: "gh",
				Args: []string{
					"pr", "list",
					"--head", "feature",
					"--state", "open",
					"--json",
					"number,title,url,body,baseRefName,headRefName,isDraft",
				},
				Dir: "/repo/gallery",
			},
			result: command.Result{Stdout: `[{`},
		}
		resolver := mustNewResolver(t, runner)

		pullRequests, err := resolver.ListOpen(
			context.Background(),
			"/repo/gallery",
			"feature",
		)
		if err == nil {
			t.Fatal("ListOpen() error = nil, want JSON decoding error")
		}
		var syntaxError *json.SyntaxError
		if !errors.As(err, &syntaxError) {
			t.Fatalf("ListOpen() error = %v, want wrapped json.SyntaxError", err)
		}
		if !strings.Contains(err.Error(), `head branch "feature"`) ||
			!strings.Contains(err.Error(), "decode gh response") {
			t.Fatalf("ListOpen() error = %q, want decoding context", err)
		}
		if pullRequests != nil {
			t.Fatalf("ListOpen() pull requests = %+v, want nil", pullRequests)
		}
		runner.assertCalledOnce()
	})
}

type resolverRunner struct {
	t      *testing.T
	want   command.Spec
	result command.Result
	err    error
	calls  int
}

type resolverSequenceRun struct {
	want   command.Spec
	result command.Result
	err    error
}

type resolverSequenceRunner struct {
	t     *testing.T
	runs  []resolverSequenceRun
	calls int
}

func (runner *resolverSequenceRunner) Run(_ context.Context, spec command.Spec) (command.Result, error) {
	runner.t.Helper()
	if runner.calls >= len(runner.runs) {
		runner.t.Fatalf("unexpected command: %+v", spec)
	}
	run := runner.runs[runner.calls]
	runner.calls++
	if !reflect.DeepEqual(spec, run.want) {
		runner.t.Fatalf("command = %+v, want %+v", spec, run.want)
	}
	return run.result, run.err
}

func (runner *resolverSequenceRunner) assertComplete() {
	runner.t.Helper()
	if runner.calls != len(runner.runs) {
		runner.t.Fatalf("runner calls = %d, want %d", runner.calls, len(runner.runs))
	}
}

func (runner *resolverRunner) Run(
	_ context.Context,
	spec command.Spec,
) (command.Result, error) {
	runner.t.Helper()
	runner.calls++

	if spec.Name != runner.want.Name ||
		spec.Dir != runner.want.Dir ||
		spec.Stdin != runner.want.Stdin ||
		!slices.Equal(spec.Args, runner.want.Args) {
		runner.t.Fatalf("command = %+v, want %+v", spec, runner.want)
	}

	return runner.result, runner.err
}

func (runner *resolverRunner) assertCalledOnce() {
	runner.t.Helper()
	if runner.calls != 1 {
		runner.t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
}

func mustNewResolver(t *testing.T, runner command.Runner) Resolver {
	t.Helper()

	resolver, err := NewResolver(runner)
	if err != nil {
		t.Fatalf("NewResolver() unexpected error: %v", err)
	}

	return resolver
}
