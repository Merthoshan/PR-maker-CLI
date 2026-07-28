package github

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"champu-pr/internal/command"
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
