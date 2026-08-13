package github

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
)

// Publisher creates and updates pull requests through the gh CLI.
type Publisher struct {
	runner command.Runner
}

// NewPublisher creates a GitHub pull-request publisher.
func NewPublisher(runner command.Runner) (Publisher, error) {
	if runner == nil {
		return Publisher{}, errors.New(
			"create GitHub publisher: runner is required",
		)
	}
	return Publisher{runner: runner}, nil
}

// Publish creates a draft PR by default, or updates the selected PR.
func (publisher Publisher) Publish(
	ctx context.Context,
	request PublishRequest,
) (PublishResult, error) {
	if publisher.runner == nil {
		return PublishResult{}, errors.New(
			"publish pull request: runner is required",
		)
	}
	request.RepositoryRoot = strings.TrimSpace(request.RepositoryRoot)
	request.HeadBranch = strings.TrimSpace(request.HeadBranch)
	request.BaseBranch = strings.TrimSpace(request.BaseBranch)
	request.Title = strings.TrimSpace(request.Title)
	if err := validatePublishRequest(request); err != nil {
		return PublishResult{}, err
	}

	if request.PullRequest == nil {
		return publisher.create(ctx, request)
	}
	return publisher.update(ctx, request)
}

func (publisher Publisher) create(
	ctx context.Context,
	request PublishRequest,
) (PublishResult, error) {
	args := []string{
		"pr", "create",
		"--base", request.BaseBranch,
		"--head", request.HeadBranch,
		"--title", request.Title,
		"--body-file", "-",
	}
	if !request.Ready {
		args = append(args, "--draft")
	}

	result, err := publisher.runner.Run(ctx, command.Spec{
		Name:  "gh",
		Args:  args,
		Dir:   request.RepositoryRoot,
		Stdin: request.Body,
	})
	if err != nil {
		return PublishResult{}, command.WrapError(
			"create pull request",
			result,
			err,
		)
	}
	url := strings.TrimSpace(result.Stdout)
	if url == "" {
		return PublishResult{}, errors.New(
			"create pull request: gh returned an empty URL",
		)
	}
	return PublishResult{URL: url, Created: true}, nil
}

func (publisher Publisher) update(
	ctx context.Context,
	request PublishRequest,
) (PublishResult, error) {
	number := strconv.Itoa(request.PullRequest.Number)
	result, err := publisher.runner.Run(ctx, command.Spec{
		Name: "gh",
		Args: []string{
			"pr", "edit", number,
			"--title", request.Title,
			"--body-file", "-",
		},
		Dir:   request.RepositoryRoot,
		Stdin: request.Body,
	})
	if err != nil {
		return PublishResult{}, command.WrapError(
			fmt.Sprintf("update pull request #%d", request.PullRequest.Number),
			result,
			err,
		)
	}

	if request.Ready && request.PullRequest.Draft {
		readyResult, readyErr := publisher.runner.Run(ctx, command.Spec{
			Name: "gh",
			Args: []string{"pr", "ready", number},
			Dir:  request.RepositoryRoot,
		})
		if readyErr != nil {
			return PublishResult{}, command.WrapError(
				fmt.Sprintf(
					"mark pull request #%d ready",
					request.PullRequest.Number,
				),
				readyResult,
				readyErr,
			)
		}
	}

	return PublishResult{URL: request.PullRequest.URL}, nil
}

func validatePublishRequest(request PublishRequest) error {
	switch {
	case request.RepositoryRoot == "":
		return errors.New("publish pull request: repository root is required")
	case request.HeadBranch == "":
		return errors.New("publish pull request: head branch is required")
	case request.BaseBranch == "":
		return errors.New("publish pull request: base branch is required")
	case request.Title == "":
		return errors.New("publish pull request: title is required")
	case strings.TrimSpace(request.Body) == "":
		return errors.New("publish pull request: body is required")
	case request.PullRequest != nil && request.PullRequest.Number <= 0:
		return errors.New(
			"publish pull request: existing pull request number must be positive",
		)
	}
	return nil
}
