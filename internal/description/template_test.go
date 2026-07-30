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

	got, err := RenderMarkdown(template, draft, OutputModeDescription)
	if err != nil {
		t.Fatalf("RenderMarkdown() unexpected error: %v", err)
	}
	for _, want := range []string{
		"Resolve PR targets by number or base.",
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

func TestRenderMarkdownRendersFileWiseChangelog(t *testing.T) {
	got, err := RenderMarkdown(
		"## Description\n\nTemplate content",
		refinementDraft(),
		OutputModeChangelog,
	)
	if err != nil {
		t.Fatalf("RenderMarkdown() unexpected error: %v", err)
	}
	for _, want := range []string{
		"### `internal/workflow/target.go`",
		"**F1.C1** — Resolve workflow targets.",
		"**F1.C2** — Represent the selected target.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered changelog missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"Resolve PR targets by number or base.",
		"Template content",
		"How Has This Been Tested",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("rendered changelog contains %q:\n%s", unwanted, got)
		}
	}
}

func TestRenderMarkdownWithoutTemplate(t *testing.T) {
	got, err := RenderMarkdown(
		"",
		validDescriptionDraft(),
		OutputModeDescription,
	)
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
	if _, err := RenderMarkdown("", Draft{}, OutputModeDescription); err == nil {
		t.Fatal("RenderMarkdown() error = nil, want validation error")
	}
}

func TestRenderMarkdownRejectsInvalidMode(t *testing.T) {
	if _, err := RenderMarkdown(
		"",
		validDescriptionDraft(),
		OutputMode("invalid"),
	); err == nil {
		t.Fatal("RenderMarkdown() error = nil, want output mode error")
	}
}
