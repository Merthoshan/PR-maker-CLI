package description

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
)

func TestGeneratorSuggestTitles(t *testing.T) {
	wantTitles := []string{
		"Add portfolio API validation",
		"Validate portfolio API requests",
		"Improve portfolio request validation",
	}
	response, err := json.Marshal(titleSuggestionResponse{Titles: wantTitles})
	if err != nil {
		t.Fatalf("marshal title suggestions: %v", err)
	}

	var schemaPath string
	runner := &generatorRunner{
		t: t,
		run: func(spec command.Spec) (command.Result, error) {
			if spec.Name != "codex" || spec.Dir != "/repo/gallery" {
				t.Fatalf("command = %+v", spec)
			}
			if len(spec.Args) != 9 || spec.Args[7] == "" {
				t.Fatalf("command args = %q", spec.Args)
			}
			schemaPath = spec.Args[7]
			if !strings.Contains(spec.Args[8], "combine 1 and 3") {
				t.Fatalf("prompt missing trusted instruction: %q", spec.Args[8])
			}

			schemaBytes, err := os.ReadFile(schemaPath)
			if err != nil {
				t.Fatalf("read title schema: %v", err)
			}
			var schema struct {
				Properties struct {
					Titles struct {
						Items struct {
							MaxLength int `json:"maxLength"`
						} `json:"items"`
					} `json:"titles"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(schemaBytes, &schema); err != nil {
				t.Fatalf("decode title schema: %v", err)
			}
			if schema.Properties.Titles.Items.MaxLength != 41 {
				t.Fatalf(
					"schema maxLength = %d, want 41",
					schema.Properties.Titles.Items.MaxLength,
				)
			}

			var payload titleSuggestionPayload
			if err := json.Unmarshal([]byte(spec.Stdin), &payload); err != nil {
				t.Fatalf("decode title payload: %v", err)
			}
			if payload.MaxTitleLength != 41 ||
				payload.CurrentDraft.Title != validDescriptionDraft().Title ||
				len(payload.PreviousTitles) != 3 {
				t.Fatalf("title payload = %+v", payload)
			}
			return command.Result{Stdout: string(response)}, nil
		},
	}
	generator := mustNewDescriptionGenerator(t, runner)

	titles, err := generator.SuggestTitles(
		context.Background(),
		TitleSuggestionRequest{
			RepositoryRoot: "/repo/gallery",
			Instruction:    "combine 1 and 3",
			CurrentDraft:   validDescriptionDraft(),
			PreviousTitles: []string{"One", "Two", "Three"},
			MaxTitleLength: 41,
			Evidence:       validDescriptionRequest().Evidence,
		},
	)
	if err != nil {
		t.Fatalf("SuggestTitles() unexpected error: %v", err)
	}
	if !slices.Equal(titles, wantTitles) {
		t.Fatalf("SuggestTitles() = %q, want %q", titles, wantTitles)
	}
	if _, err := os.Stat(schemaPath); !os.IsNotExist(err) {
		t.Fatalf("temporary title schema still exists or stat failed: %v", err)
	}
}

func TestValidateTitleSuggestions(t *testing.T) {
	tests := []struct {
		name   string
		titles []string
		want   string
	}{
		{
			name:   "wrong count",
			titles: []string{"One", "Two"},
			want:   "received 2 titles",
		},
		{
			name:   "duplicate",
			titles: []string{"One", "Two", "One"},
			want:   "duplicates another option",
		},
		{
			name:   "over limit",
			titles: []string{"One", "Two", "123456"},
			want:   "exceeds 5 characters",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTitleSuggestions(test.titles, 5)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateTitleSuggestions() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestGeneratorSuggestTitlesValidatesBeforeRun(t *testing.T) {
	runner := &generatorRunner{t: t}
	generator := mustNewDescriptionGenerator(t, runner)

	_, err := generator.SuggestTitles(
		context.Background(),
		TitleSuggestionRequest{
			RepositoryRoot: "/repo/gallery",
			Instruction:    "shorten it",
			CurrentDraft:   validDescriptionDraft(),
			MaxTitleLength: 0,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "outside 1..72") {
		t.Fatalf("SuggestTitles() error = %v", err)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}
