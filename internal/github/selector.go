package github

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
)

// AmbiguousPullRequestsError reports every pull request that matched a
// selection instead of silently choosing one.
type AmbiguousPullRequestsError struct {
	Matches []PullRequest
}

// Error formats the matching pull requests for display in the CLI.
func (selectionError AmbiguousPullRequestsError) Error() string {
	var message strings.Builder
	message.WriteString("multiple open pull requests matched:\n\n")

	table := tabwriter.NewWriter(&message, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "  NUMBER\tTITLE\tBRANCH")
	for _, pullRequest := range selectionError.Matches {
		fmt.Fprintf(
			table,
			"  #%d\t%s\t%s -> %s\n",
			pullRequest.Number,
			singleLine(pullRequest.Title),
			pullRequest.HeadBranch,
			pullRequest.BaseBranch,
		)
	}
	table.Flush()

	message.WriteString("\nchoose one explicitly:\n")
	message.WriteString("  champu --pr <number>")

	return message.String()
}

// FindPullRequestByNumber finds one existing pull request by its number.
func FindPullRequestByNumber(pullRequests []PullRequest, requestedNumber int) (PullRequest, error) {
	if requestedNumber <= 0 {
		return PullRequest{}, errors.New(
			"find pull request by number: requested number must be greater than zero",
		)
	}

	matches := []PullRequest{}
	for _, pullRequest := range pullRequests {
		if pullRequest.Number == requestedNumber {
			matches = append(matches, pullRequest)
		}
	}

	switch len(matches) {
	case 0:
		return PullRequest{}, fmt.Errorf(
			"find pull request by number: open pull request #%d was not found",
			requestedNumber,
		)
	case 1:
		return matches[0], nil
	default:
		return PullRequest{}, AmbiguousPullRequestsError{Matches: matches}
	}
}

// SelectPullRequestByBase selects an existing pull request for baseBranch or
// reports that a new pull request should be created.
func SelectPullRequestByBase(pullRequests []PullRequest, baseBranch string) (Selection, error) {
	baseBranch = strings.TrimSpace(baseBranch)
	if baseBranch == "" {
		return Selection{}, errors.New(
			"select pull request by base: base branch is required",
		)
	}

	matches := []PullRequest{}
	for _, pullRequest := range pullRequests {
		if pullRequest.BaseBranch == baseBranch {
			matches = append(matches, pullRequest)
		}
	}

	switch len(matches) {
	case 0:
		return Selection{ShouldCreate: true}, nil
	case 1:
		pullRequest := matches[0]
		return Selection{
			PullRequest: &pullRequest,
		}, nil
	default:
		return Selection{}, AmbiguousPullRequestsError{Matches: matches}
	}
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
