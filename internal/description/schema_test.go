package description

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestDraftSchemaIsValidJSON(t *testing.T) {
	if !json.Valid(draftSchema) {
		t.Fatal("embedded draft schema is not valid JSON")
	}

	var schema map[string]any
	if err := json.Unmarshal(draftSchema, &schema); err != nil {
		t.Fatalf("decode embedded schema: %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("schema type = %v, want object", schema["type"])
	}
	if schema["additionalProperties"] != false {
		t.Fatalf(
			"schema additionalProperties = %v, want false",
			schema["additionalProperties"],
		)
	}
}

func TestWriteDraftSchema(t *testing.T) {
	schemaPath, cleanup, err := writeDraftSchema()
	if err != nil {
		t.Fatalf("writeDraftSchema() unexpected error: %v", err)
	}

	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read temporary schema: %v", err)
	}
	if string(schema) != string(draftSchema) {
		t.Fatal("temporary schema does not match embedded schema")
	}
	if !strings.Contains(schemaPath, "champu-pr-draft-schema-") {
		t.Fatalf("schema path = %q, want champu-pr prefix", schemaPath)
	}

	cleanup()
	if _, err := os.Stat(schemaPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary schema still exists or stat failed: %v", err)
	}
}
