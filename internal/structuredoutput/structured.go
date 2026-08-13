// Package structuredoutput centralizes strict JSON response decoding and the
// temporary schemas supplied to structured-output commands.
package structuredoutput

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// WriteTempSchema writes contents to an owner-only temporary file and returns
// an idempotent cleanup function for removing it.
func WriteTempSchema(prefix string, contents []byte) (string, func(), error) {
	schemaFile, err := os.CreateTemp("", prefix)
	if err != nil {
		return "", func() {}, fmt.Errorf("create temporary schema: %w", err)
	}

	schemaPath := schemaFile.Name()
	cleanup := func() {
		_ = os.Remove(schemaPath)
	}
	if _, err := schemaFile.Write(contents); err != nil {
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

// DecodeSingleJSON decodes exactly one JSON value, rejects unknown struct
// fields, and rejects any additional JSON value after the first.
func DecodeSingleJSON[T any](output string) (T, error) {
	var value T
	decoder := json.NewDecoder(strings.NewReader(output))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("decode JSON object: %w", err)
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return value, errors.New("response contains multiple JSON values")
		}
		return value, fmt.Errorf("decode trailing response: %w", err)
	}
	return value, nil
}
