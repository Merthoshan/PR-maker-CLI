package description

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTemplate(t *testing.T) {
	root := t.TempDir()
	if got, err := LoadTemplate(root); err != nil || got != "" {
		t.Fatalf("LoadTemplate() = %q, %v, want empty template", got, err)
	}

	templatePath := filepath.Join(root, ".github", "pull_request_template.md")
	if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
		t.Fatalf("create template directory: %v", err)
	}
	if err := os.WriteFile(templatePath, []byte("## Description\n"), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}

	got, err := LoadTemplate(root)
	if err != nil {
		t.Fatalf("LoadTemplate() unexpected error: %v", err)
	}
	if got != "## Description\n" {
		t.Fatalf("LoadTemplate() = %q, want template content", got)
	}
}

func TestRenderMarkdownPreservesTemplateAndAddsDraft(t *testing.T) {
	template := `## Description (what does this PR do?)

## Checklist
- [ ] Environment variables added

## Ticket
[#Ticket Number]

### How Has This Been Tested?

Please describe the tests that you ran to verify your changes.

- [ ] Manual Test
- [ ] Unit Tests
`
	draft := refinementDraft()
	draft.Testing = []string{"go test ./... — passed."}

	got, err := RenderMarkdown(template, draft)
	if err != nil {
		t.Fatalf("RenderMarkdown() unexpected error: %v", err)
	}
	for _, want := range []string{
		"Resolve PR targets by number or base.",
		"### Detailed changes",
		"`internal/workflow/target.go`",
		"F1.C1",
		"## Checklist",
		"- [ ] Environment variables added",
		"[#Ticket Number]",
		"go test ./... — passed.",
		"- [ ] Manual Test",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered Markdown missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Please describe the tests") {
		t.Fatalf("rendered Markdown retained testing placeholder:\n%s", got)
	}
}

func TestRenderMarkdownWithoutTemplate(t *testing.T) {
	got, err := RenderMarkdown("", validDescriptionDraft())
	if err != nil {
		t.Fatalf("RenderMarkdown() unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "- Resolve PR targets") {
		t.Fatalf("rendered Markdown = %q, want summary first", got)
	}
	if !strings.Contains(got, "## How Has This Been Tested?") {
		t.Fatalf("rendered Markdown missing testing heading:\n%s", got)
	}
}

func TestRenderMarkdownRejectsInvalidDraft(t *testing.T) {
	if _, err := RenderMarkdown("", Draft{}); err == nil {
		t.Fatal("RenderMarkdown() error = nil, want validation error")
	}
}
