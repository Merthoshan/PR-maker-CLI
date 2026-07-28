package github

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestAmbiguousPullRequestsError(t *testing.T) {
	selectionError := AmbiguousPullRequestsError{
		Matches: []PullRequest{
			{
				Number:     42,
				Title:      "Add collection CRUD",
				HeadBranch: "feature/collections",
				BaseBranch: "main",
			},
			{
				Number:     57,
				Title:      "Test\tcollection\nintegration",
				HeadBranch: "feature/collections",
				BaseBranch: "develop",
			},
		},
	}

	want := `multiple open pull requests matched:

  NUMBER  TITLE                        BRANCH
  #42     Add collection CRUD          feature/collections -> main
  #57     Test collection integration  feature/collections -> develop

choose one explicitly:
  champu-pr --pr <number>`

	if got := selectionError.Error(); got != want {
		t.Fatalf("Error() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFindPullRequestByNumber(t *testing.T) {
	pullRequests := []PullRequest{
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

	t.Run("finds exact pull request", func(t *testing.T) {
		pullRequest, err := FindPullRequestByNumber(pullRequests, 57)
		if err != nil {
			t.Fatalf("FindPullRequestByNumber() unexpected error: %v", err)
		}
		if pullRequest != pullRequests[1] {
			t.Fatalf(
				"FindPullRequestByNumber() = %+v, want %+v",
				pullRequest,
				pullRequests[1],
			)
		}
	})

	t.Run("requires positive number", func(t *testing.T) {
		for _, requestedNumber := range []int{-1, 0} {
			pullRequest, err := FindPullRequestByNumber(
				pullRequests,
				requestedNumber,
			)
			if err == nil {
				t.Fatalf(
					"FindPullRequestByNumber(%d) error = nil, want validation error",
					requestedNumber,
				)
			}
			if !strings.Contains(err.Error(), "must be greater than zero") {
				t.Fatalf(
					"FindPullRequestByNumber(%d) error = %q, want positive-number error",
					requestedNumber,
					err,
				)
			}
			if pullRequest != (PullRequest{}) {
				t.Fatalf(
					"FindPullRequestByNumber(%d) = %+v, want zero value",
					requestedNumber,
					pullRequest,
				)
			}
		}
	})

	t.Run("reports missing pull request", func(t *testing.T) {
		pullRequest, err := FindPullRequestByNumber(pullRequests, 99)
		if err == nil {
			t.Fatal("FindPullRequestByNumber() error = nil, want not-found error")
		}
		if !strings.Contains(err.Error(), "#99 was not found") {
			t.Fatalf(
				"FindPullRequestByNumber() error = %q, want PR number",
				err,
			)
		}
		if pullRequest != (PullRequest{}) {
			t.Fatalf(
				"FindPullRequestByNumber() = %+v, want zero value",
				pullRequest,
			)
		}
	})

	t.Run("reports every duplicate match", func(t *testing.T) {
		duplicates := []PullRequest{
			pullRequests[0],
			{
				Number:     42,
				Title:      "Duplicate PR",
				BaseBranch: "develop",
				HeadBranch: "feature/collections",
			},
		}

		pullRequest, err := FindPullRequestByNumber(duplicates, 42)
		if err == nil {
			t.Fatal("FindPullRequestByNumber() error = nil, want ambiguity error")
		}
		var ambiguity AmbiguousPullRequestsError
		if !errors.As(err, &ambiguity) {
			t.Fatalf(
				"FindPullRequestByNumber() error = %T, want AmbiguousPullRequestsError",
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
		if pullRequest != (PullRequest{}) {
			t.Fatalf(
				"FindPullRequestByNumber() = %+v, want zero value",
				pullRequest,
			)
		}
	})
}

func TestSelectPullRequestByBase(t *testing.T) {
	pullRequests := []PullRequest{
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

	t.Run("selects exact base", func(t *testing.T) {
		selection, err := SelectPullRequestByBase(
			pullRequests,
			"  develop  ",
		)
		if err != nil {
			t.Fatalf("SelectPullRequestByBase() unexpected error: %v", err)
		}
		if selection.ShouldCreate {
			t.Fatal("SelectPullRequestByBase() ShouldCreate = true, want false")
		}
		if selection.PullRequest == nil {
			t.Fatal("SelectPullRequestByBase() PullRequest = nil, want existing PR")
		}
		if *selection.PullRequest != pullRequests[1] {
			t.Fatalf(
				"SelectPullRequestByBase() PR = %+v, want %+v",
				*selection.PullRequest,
				pullRequests[1],
			)
		}
	})

	t.Run("requests creation when base has no pull request", func(t *testing.T) {
		selection, err := SelectPullRequestByBase(pullRequests, "release")
		if err != nil {
			t.Fatalf("SelectPullRequestByBase() unexpected error: %v", err)
		}
		if !selection.ShouldCreate {
			t.Fatal("SelectPullRequestByBase() ShouldCreate = false, want true")
		}
		if selection.PullRequest != nil {
			t.Fatalf(
				"SelectPullRequestByBase() PullRequest = %+v, want nil",
				selection.PullRequest,
			)
		}
	})

	t.Run("requires base branch", func(t *testing.T) {
		for _, baseBranch := range []string{"", " \n\t "} {
			selection, err := SelectPullRequestByBase(
				pullRequests,
				baseBranch,
			)
			if err == nil {
				t.Fatal("SelectPullRequestByBase() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), "base branch is required") {
				t.Fatalf(
					"SelectPullRequestByBase() error = %q, want base validation error",
					err,
				)
			}
			if selection != (Selection{}) {
				t.Fatalf(
					"SelectPullRequestByBase() = %+v, want zero value",
					selection,
				)
			}
		}
	})

	t.Run("reports every base match", func(t *testing.T) {
		mainMatches := []PullRequest{
			pullRequests[0],
			{
				Number:     58,
				Title:      "Second main PR",
				BaseBranch: "main",
				HeadBranch: "feature/collections",
			},
		}

		selection, err := SelectPullRequestByBase(mainMatches, "main")
		if err == nil {
			t.Fatal("SelectPullRequestByBase() error = nil, want ambiguity error")
		}
		var ambiguity AmbiguousPullRequestsError
		if !errors.As(err, &ambiguity) {
			t.Fatalf(
				"SelectPullRequestByBase() error = %T, want AmbiguousPullRequestsError",
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
		if selection != (Selection{}) {
			t.Fatalf(
				"SelectPullRequestByBase() = %+v, want zero value",
				selection,
			)
		}
	})
}
