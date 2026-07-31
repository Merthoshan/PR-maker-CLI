package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
)

// PullRequest contains the GitHub fields needed to select and update a PR.
type PullRequest struct {
	Number     int    `json:"number"`
	State      string `json:"state"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	Body       string `json:"body"`
	BaseBranch string `json:"baseRefName"`
	HeadBranch string `json:"headRefName"`
	Draft      bool   `json:"isDraft"`
}

// Resolver discovers GitHub pull requests using the gh CLI.
type Resolver struct {
	runner command.Runner
}

// NewResolver creates a GitHub pull-request resolver.
func NewResolver(runner command.Runner) (Resolver, error) {
	if runner == nil {
		return Resolver{}, errors.New("create GitHub resolver: runner is required")
	}

	return Resolver{
		runner: runner,
	}, nil
}

// GetOpenByNumber resolves one open pull request without filtering by the
// currently checked-out branch.
func (resolver Resolver) GetOpenByNumber(
	ctx context.Context,
	repositoryRoot string,
	number int,
) (PullRequest, error) {
	if resolver.runner == nil {
		return PullRequest{}, errors.New(
			"get open pull request by number: runner is required",
		)
	}
	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if repositoryRoot == "" {
		return PullRequest{}, errors.New(
			"get open pull request by number: repository root is required",
		)
	}
	if number <= 0 {
		return PullRequest{}, errors.New(
			"get open pull request by number: number must be positive",
		)
	}

	result, err := resolver.runner.Run(ctx, command.Spec{
		Name: "gh",
		Args: []string{
			"pr", "view", strconv.Itoa(number),
			"--json",
			"number,state,title,url,body,baseRefName,headRefName,isDraft",
		},
		Dir: repositoryRoot,
	})
	if err != nil {
		return PullRequest{}, command.WrapError(
			fmt.Sprintf("get pull request #%d", number),
			result,
			err,
		)
	}

	var pullRequest PullRequest
	if err := json.Unmarshal([]byte(result.Stdout), &pullRequest); err != nil {
		return PullRequest{}, fmt.Errorf(
			"get pull request #%d: decode gh response: %w",
			number,
			err,
		)
	}
	if pullRequest.Number != number {
		return PullRequest{}, fmt.Errorf(
			"get pull request #%d: gh returned pull request #%d",
			number,
			pullRequest.Number,
		)
	}
	if !strings.EqualFold(pullRequest.State, "OPEN") {
		return PullRequest{}, fmt.Errorf(
			"get pull request #%d: pull request is %s, expected OPEN",
			number,
			strings.ToUpper(strings.TrimSpace(pullRequest.State)),
		)
	}

	return pullRequest, nil
}

// ListOpen returns every open pull request for headBranch.
func (resolver Resolver) ListOpen(ctx context.Context, repositoryRoot string, headBranch string) ([]PullRequest, error) {
	if resolver.runner == nil {
		return nil, errors.New("list open pull requests: runner is required")
	}
	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if repositoryRoot == "" {
		return nil, errors.New("list open pull requests: repository root is required")
	}

	headBranch = strings.TrimSpace(headBranch)
	if headBranch == "" {
		return nil, errors.New("list open pull requests: head branch is required")
	}

	result, err := resolver.runner.Run(ctx, command.Spec{
		Name: "gh",
		Args: []string{
			"pr", "list",
			"--head", headBranch,
			"--state", "open",
			"--json",
			"number,title,url,body,baseRefName,headRefName,isDraft",
		},
		Dir: repositoryRoot,
	})
	if err != nil {
		return nil, command.WrapError(
			fmt.Sprintf(
				"list open pull requests for head branch %q",
				headBranch,
			),
			result,
			err,
		)
	}

	var pullRequests []PullRequest
	if err := json.Unmarshal([]byte(result.Stdout), &pullRequests); err != nil {
		return nil, fmt.Errorf(
			"list open pull requests for head branch %q: decode gh response: %w",
			headBranch,
			err,
		)
	}

	return pullRequests, nil
}
