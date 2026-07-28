package description

import (
	"context"
	"errors"
	"fmt"

	"champu-pr/internal/command"
)

func (generator Generator) runDraft(
	ctx context.Context,
	repositoryRoot string,
	prompt string,
	payload string,
) (Draft, error) {
	if generator.runner == nil {
		return Draft{}, errors.New("run Codex: runner is required")
	}
	schemaPath, cleanupSchema, err := writeDraftSchema()
	if err != nil {
		return Draft{}, fmt.Errorf("prepare output schema: %w", err)
	}
	defer cleanupSchema()

	result, err := generator.runner.Run(ctx, command.Spec{
		Name: "codex",
		Args: []string{
			"exec",
			"--ephemeral",
			"--sandbox", "read-only",
			"--color", "never",
			"--output-schema", schemaPath,
			prompt,
		},
		Dir:   repositoryRoot,
		Stdin: payload,
	})
	if err != nil {
		return Draft{}, command.WrapError("run Codex", result, err)
	}

	draft, err := decodeDraft(result.Stdout)
	if err != nil {
		return Draft{}, fmt.Errorf("decode Codex response: %w", err)
	}
	if err := validateDraft(draft); err != nil {
		return Draft{}, fmt.Errorf("validate Codex response: %w", err)
	}

	return draft, nil
}
