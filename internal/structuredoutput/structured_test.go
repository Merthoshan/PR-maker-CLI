package structuredoutput

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestWriteTempSchemaCreatesOwnerOnlyFileAndCleansItUp(t *testing.T) {
	contents := []byte(`{"type":"object"}`)
	path, cleanup, err := WriteTempSchema("champu-pr-test-schema-*.json", contents)
	if err != nil {
		t.Fatalf("WriteTempSchema() unexpected error: %v", err)
	}
	t.Cleanup(cleanup)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temporary schema: %v", err)
	}
	if string(got) != string(contents) {
		t.Fatalf("temporary schema = %q, want %q", got, contents)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat temporary schema: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("temporary schema permissions = %o, want owner-only", info.Mode().Perm())
	}

	cleanup()
	cleanup()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary schema still exists or stat failed: %v", err)
	}
}

func TestDecodeSingleJSON(t *testing.T) {
	type response struct {
		Name string `json:"name"`
	}

	t.Run("decodes one object with trailing whitespace", func(t *testing.T) {
		got, err := DecodeSingleJSON[response]("{\"name\":\"champu\"}\n\t")
		if err != nil {
			t.Fatalf("DecodeSingleJSON() unexpected error: %v", err)
		}
		if got.Name != "champu" {
			t.Fatalf("DecodeSingleJSON() name = %q, want champu", got.Name)
		}
	})

	t.Run("rejects unknown fields", func(t *testing.T) {
		_, err := DecodeSingleJSON[response](`{"name":"champu","unknown":true}`)
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("DecodeSingleJSON() error = %v, want unknown-field error", err)
		}
	})

	t.Run("rejects multiple values", func(t *testing.T) {
		_, err := DecodeSingleJSON[response](`{"name":"champu"} {}`)
		if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
			t.Fatalf("DecodeSingleJSON() error = %v, want multiple-value error", err)
		}
	})

	t.Run("rejects malformed trailing data", func(t *testing.T) {
		_, err := DecodeSingleJSON[response](`{"name":"champu"} invalid`)
		if err == nil || !strings.Contains(err.Error(), "decode trailing response") {
			t.Fatalf("DecodeSingleJSON() error = %v, want trailing-data error", err)
		}
	})
}
