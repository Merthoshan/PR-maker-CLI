package review

import (
	_ "embed"

	"github.com/Merthoshan/PR-maker-CLI/internal/structuredoutput"
)

//go:embed review.schema.json
var reviewSchema []byte

// writeReviewSchema materializes the embedded review schema for Codex.
func writeReviewSchema() (string, func(), error) {
	return structuredoutput.WriteTempSchema(
		"champu-pr-review-schema-*.json",
		reviewSchema,
	)
}
