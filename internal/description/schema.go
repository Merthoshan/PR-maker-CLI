package description

import (
	_ "embed"
	"fmt"
	"os"
)

//go:embed draft.schema.json
var draftSchema []byte

func writeDraftSchema() (string, func(), error) {
	schemaFile, err := os.CreateTemp("", "champu-pr-draft-schema-*.json")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temporary schema: %w", err)
	}

	schemaPath := schemaFile.Name()
	cleanup := func() {
		_ = os.Remove(schemaPath)
	}

	if _, err := schemaFile.Write(draftSchema); err != nil {
		_ = schemaFile.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write temporary schema: %w", err)
	}
	if err := schemaFile.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close temporary schema: %w", err)
	}

	return schemaPath, cleanup, nil
}
