package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
)

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

// GetReview collects PR metadata and its diff without changing GitHub or the
// checked-out branch. target may be a number or a canonical GitHub PR URL.
func (resolver Resolver) GetReview(
	ctx context.Context,
	request ReviewRequest,
) (ReviewData, error) {
	if resolver.runner == nil {
		return ReviewData{}, errors.New("get pull request review: runner is required")
	}
	request.RepositoryRoot = strings.TrimSpace(request.RepositoryRoot)
	if request.RepositoryRoot == "" {
		return ReviewData{}, errors.New("get pull request review: repository root is required")
	}
	request.Target = strings.TrimSpace(request.Target)
	if request.Target == "" {
		return ReviewData{}, errors.New("get pull request review: target is required")
	}
	request.ExpectedRepository = strings.TrimSpace(request.ExpectedRepository)
	if request.ExpectedRepository == "" {
		return ReviewData{}, errors.New("get pull request review: expected repository is required")
	}
	if request.DiffByteLimit <= 0 {
		return ReviewData{}, errors.New("get pull request review: diff byte limit must be positive")
	}
	viewResult, err := resolver.runner.Run(ctx, command.Spec{
		Name: "gh",
		Args: []string{
			"pr", "view", request.Target,
			"--json",
			"number,state,title,url,body,baseRefName,headRefName,isDraft,labels,files",
		},
		Dir: request.RepositoryRoot,
	})
	if err != nil {
		return ReviewData{}, command.WrapError("get pull request review metadata", viewResult, err)
	}
	var view reviewView
	if err := json.Unmarshal([]byte(viewResult.Stdout), &view); err != nil {
		return ReviewData{}, fmt.Errorf("get pull request review metadata: decode gh response: %w", err)
	}
	if view.Number <= 0 {
		return ReviewData{}, errors.New(
			"get pull request review metadata: GitHub returned an invalid pull request number",
		)
	}
	repository, err := repositoryFromPullRequestURL(view.URL)
	if err != nil {
		return ReviewData{}, fmt.Errorf("get pull request review metadata: %w", err)
	}
	if !strings.EqualFold(repository, request.ExpectedRepository) {
		return ReviewData{}, fmt.Errorf(
			"get pull request review: pull request belongs to %s, current repository is %s",
			repository,
			request.ExpectedRepository,
		)
	}
	if !strings.EqualFold(view.State, "OPEN") {
		return ReviewData{}, fmt.Errorf(
			"get pull request review: pull request is %s, expected OPEN",
			strings.ToUpper(strings.TrimSpace(view.State)),
		)
	}
	if request.BeforeDiff != nil {
		request.BeforeDiff()
	}
	diffResult, err := resolver.runner.Run(ctx, command.Spec{
		Name:        "gh",
		Args:        []string{"pr", "diff", request.Target, "--color=never"},
		Dir:         request.RepositoryRoot,
		StdoutLimit: request.DiffByteLimit,
	})
	if err != nil {
		return ReviewData{}, command.WrapError("get pull request review diff", diffResult, err)
	}
	labels := make([]string, 0, len(view.Labels))
	for _, label := range view.Labels {
		labels = append(labels, label.Name)
	}
	if strings.TrimSpace(diffResult.Stdout) == "" {
		return ReviewData{}, errors.New("get pull request review diff: GitHub returned an empty diff")
	}
	return ReviewData{
		PullRequest: view.PullRequest,
		Labels:      labels,
		Repository:  repository,
		Files:       view.Files,
		Diff:        diffResult.Stdout,
		DiffLimited: diffResult.StdoutTruncated,
	}, nil
}

// repositoryFromPullRequestURL extracts the repository from a canonical GitHub
// pull-request URL while validating its pull-request number.
func repositoryFromPullRequestURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") {
		return "", errors.New("GitHub returned an invalid pull request URL")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] != "pull" {
		return "", errors.New("GitHub returned an invalid pull request URL path")
	}
	if number, err := strconv.Atoi(parts[3]); err != nil || number <= 0 {
		return "", errors.New("GitHub returned a pull request URL without a positive number")
	}
	repository, err := ParseOwnerRepositoryPath(parts[0] + "/" + parts[1])
	if err != nil {
		return "", errors.New("GitHub returned an invalid pull request repository path")
	}
	return repository, nil
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
