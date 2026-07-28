package workflow

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"champu-pr/internal/cli"
	"champu-pr/internal/github"
)

func TestResolveTargetByPullRequestNumber(t *testing.T) {
	pullRequests := []github.PullRequest{
		{
			Number:     42,
			Title:      "Main PR",
			BaseBranch: "main",
			HeadBranch: "feature/collections",
		},
		{
			Number:     57,
			Title:      "Develop PR",
			BaseBranch: "develop",
			HeadBranch: "feature/collections",
		},
	}

	t.Run("resolves exact pull request and its base", func(t *testing.T) {
		target, err := ResolveTarget(
			cli.Options{PRNumber: 57},
			pullRequests,
		)
		if err != nil {
			t.Fatalf("ResolveTarget() unexpected error: %v", err)
		}
		if target.PullRequest == nil {
			t.Fatal("ResolveTarget() PullRequest = nil, want existing PR")
		}
		if *target.PullRequest != pullRequests[1] {
			t.Fatalf(
				"ResolveTarget() PR = %+v, want %+v",
				*target.PullRequest,
				pullRequests[1],
			)
		}
		if target.BaseBranch != "develop" {
			t.Fatalf(
				"ResolveTarget() BaseBranch = %q, want develop",
				target.BaseBranch,
			)
		}
		if target.ShouldCreate {
			t.Fatal("ResolveTarget() ShouldCreate = true, want false")
		}
	})

	t.Run("wraps missing pull request error", func(t *testing.T) {
		target, err := ResolveTarget(
			cli.Options{PRNumber: 99},
			pullRequests,
		)
		if err == nil {
			t.Fatal("ResolveTarget() error = nil, want not-found error")
		}
		if !strings.Contains(err.Error(), "resolve workflow target by pull request number") ||
			!strings.Contains(err.Error(), "#99 was not found") {
			t.Fatalf("ResolveTarget() error = %q, want workflow and PR context", err)
		}
		if target != (Target{}) {
			t.Fatalf("ResolveTarget() = %+v, want zero target", target)
		}
	})

	t.Run("preserves numbered ambiguity details", func(t *testing.T) {
		duplicates := []github.PullRequest{
			pullRequests[0],
			{
				Number:     42,
				Title:      "Duplicate PR",
				BaseBranch: "develop",
				HeadBranch: "feature/collections",
			},
		}

		target, err := ResolveTarget(
			cli.Options{PRNumber: 42},
			duplicates,
		)
		if err == nil {
			t.Fatal("ResolveTarget() error = nil, want ambiguity error")
		}
		var ambiguity github.AmbiguousPullRequestsError
		if !errors.As(err, &ambiguity) {
			t.Fatalf(
				"ResolveTarget() error = %T, want wrapped AmbiguousPullRequestsError",
				err,
			)
		}
		if !reflect.DeepEqual(ambiguity.Matches, duplicates) {
			t.Fatalf(
				"ambiguity matches = %+v, want %+v",
				ambiguity.Matches,
				duplicates,
			)
		}
		if !strings.Contains(err.Error(), "#42") ||
			!strings.Contains(err.Error(), "feature/collections -> main") {
			t.Fatalf("ResolveTarget() error = %q, want formatted PR details", err)
		}
		if target != (Target{}) {
			t.Fatalf("ResolveTarget() = %+v, want zero target", target)
		}
	})
}

func TestResolveTargetByBase(t *testing.T) {
	pullRequests := []github.PullRequest{
		{
			Number:     42,
			Title:      "Main PR",
			BaseBranch: "main",
			HeadBranch: "feature/collections",
		},
		{
			Number:     57,
			Title:      "Develop PR",
			BaseBranch: "develop",
			HeadBranch: "feature/collections",
		},
	}

	t.Run("resolves existing pull request", func(t *testing.T) {
		target, err := ResolveTarget(
			cli.Options{Base: "  develop  "},
			pullRequests,
		)
		if err != nil {
			t.Fatalf("ResolveTarget() unexpected error: %v", err)
		}
		if target.PullRequest == nil {
			t.Fatal("ResolveTarget() PullRequest = nil, want existing PR")
		}
		if *target.PullRequest != pullRequests[1] {
			t.Fatalf(
				"ResolveTarget() PR = %+v, want %+v",
				*target.PullRequest,
				pullRequests[1],
			)
		}
		if target.BaseBranch != pullRequests[1].BaseBranch {
			t.Fatalf(
				"ResolveTarget() BaseBranch = %q, want %q",
				target.BaseBranch,
				pullRequests[1].BaseBranch,
			)
		}
		if target.ShouldCreate {
			t.Fatal("ResolveTarget() ShouldCreate = true, want false")
		}
	})

	t.Run("requests creation when base has no pull request", func(t *testing.T) {
		target, err := ResolveTarget(
			cli.Options{Base: "release"},
			pullRequests,
		)
		if err != nil {
			t.Fatalf("ResolveTarget() unexpected error: %v", err)
		}
		if target.PullRequest != nil {
			t.Fatalf(
				"ResolveTarget() PullRequest = %+v, want nil",
				target.PullRequest,
			)
		}
		if target.BaseBranch != "release" {
			t.Fatalf(
				"ResolveTarget() BaseBranch = %q, want release",
				target.BaseBranch,
			)
		}
		if !target.ShouldCreate {
			t.Fatal("ResolveTarget() ShouldCreate = false, want true")
		}
	})

	t.Run("wraps base validation error", func(t *testing.T) {
		target, err := ResolveTarget(cli.Options{}, pullRequests)
		if err == nil {
			t.Fatal("ResolveTarget() error = nil, want base validation error")
		}
		if !strings.Contains(err.Error(), `base branch ""`) ||
			!strings.Contains(err.Error(), "base branch is required") {
			t.Fatalf("ResolveTarget() error = %q, want workflow and base context", err)
		}
		if target != (Target{}) {
			t.Fatalf("ResolveTarget() = %+v, want zero target", target)
		}
	})

	t.Run("preserves base ambiguity details", func(t *testing.T) {
		mainMatches := []github.PullRequest{
			pullRequests[0],
			{
				Number:     58,
				Title:      "Second main PR",
				BaseBranch: "main",
				HeadBranch: "feature/collections",
			},
		}

		target, err := ResolveTarget(
			cli.Options{Base: "main"},
			mainMatches,
		)
		if err == nil {
			t.Fatal("ResolveTarget() error = nil, want ambiguity error")
		}
		var ambiguity github.AmbiguousPullRequestsError
		if !errors.As(err, &ambiguity) {
			t.Fatalf(
				"ResolveTarget() error = %T, want wrapped AmbiguousPullRequestsError",
				err,
			)
		}
		if !reflect.DeepEqual(ambiguity.Matches, mainMatches) {
			t.Fatalf(
				"ambiguity matches = %+v, want %+v",
				ambiguity.Matches,
				mainMatches,
			)
		}
		if !strings.Contains(err.Error(), "#42") ||
			!strings.Contains(err.Error(), "#58") ||
			!strings.Contains(err.Error(), "feature/collections -> main") {
			t.Fatalf("ResolveTarget() error = %q, want formatted PR details", err)
		}
		if target != (Target{}) {
			t.Fatalf("ResolveTarget() = %+v, want zero target", target)
		}
	})
}
