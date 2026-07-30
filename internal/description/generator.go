package description

import (
	"context"
	"fmt"

	"champu-pr/internal/command"
	"champu-pr/internal/gitcontext"
)

// Request contains the grounded inputs used to generate a PR description.
type Request struct {
	RepositoryRoot string
	BaseBranch     string
	ExistingTitle  string
	ExistingBody   string
	Evidence       gitcontext.Evidence
}

// Change describes one evidence-backed code change.
type Change struct {
	ID        string `json:"id"`
	File      string `json:"file"`
	Operation string `json:"operation"`
	Element   string `json:"element"`
	Summary   string `json:"summary"`
}

// Draft is the structured output produced before rendering a PR description.
type Draft struct {
	Title   string   `json:"title"`
	Summary []string `json:"summary"`
	Changes []Change `json:"changes"`
	Testing []string `json:"testing"`
}

// Generator creates structured PR-description drafts using the Codex CLI.
type Generator struct {
	runner command.Runner
}

// NewGenerator creates a PR-description generator.
func NewGenerator(runner command.Runner) (Generator, error) {
	if runner == nil {
		return Generator{}, fmt.Errorf(
			"create PR-description generator: runner is required",
		)
	}

	return Generator{runner: runner}, nil
}

// Generate builds an evidence-grounded structured PR-description draft.
func (generator Generator) Generate(ctx context.Context, request Request) (Draft, error) {
	request, err := validateRequest(request)
	if err != nil {
		return Draft{}, fmt.Errorf(
			"generate PR description: validate request: %w",
			err,
		)
	}

	payload, err := buildGenerationPayload(request)
	if err != nil {
		return Draft{}, fmt.Errorf(
			"generate PR description: encode Git evidence: %w",
			err,
		)
	}

	draft, err := generator.runDraft(
		ctx,
		request.RepositoryRoot,
		generationPrompt,
		payload,
	)
	if err != nil {
		return Draft{}, fmt.Errorf(
			"generate PR description: %w",
			err,
		)
	}
	if err := validateGeneratedDraft(draft); err != nil {
		return Draft{}, fmt.Errorf(
			"generate PR description: validate Codex response: %w",
			err,
		)
	}

	return draft, nil
}
