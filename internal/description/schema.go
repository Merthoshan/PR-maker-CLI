package description

import (
	_ "embed"

	"github.com/Merthoshan/PR-maker-CLI/internal/structuredoutput"
)

//go:embed draft.schema.json
var draftSchema []byte

// writeDraftSchema materializes the embedded draft schema for Codex.
func writeDraftSchema() (string, func(), error) {
	return structuredoutput.WriteTempSchema(
		"champu-pr-draft-schema-*.json",
		draftSchema,
	)
}
