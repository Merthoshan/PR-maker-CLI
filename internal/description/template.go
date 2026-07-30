package description

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var templateCandidates = []string{
	".github/pull_request_template.md",
	".github/PULL_REQUEST_TEMPLATE.md",
	"pull_request_template.md",
}

// LoadTemplate returns the repository's pull-request template. A repository
// without a template is valid and returns an empty string.
func LoadTemplate(repositoryRoot string) (string, error) {
	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if repositoryRoot == "" {
		return "", errors.New("load pull-request template: repository root is required")
	}

	for _, candidate := range templateCandidates {
		path := filepath.Join(repositoryRoot, candidate)
		content, err := os.ReadFile(path)
		switch {
		case err == nil:
			return string(content), nil
		case errors.Is(err, os.ErrNotExist):
			continue
		default:
			return "", fmt.Errorf(
				"load pull-request template %q: %w",
				candidate,
				err,
			)
		}
	}
	return "", nil
}

// RenderMarkdown merges a structured draft into the repository template.
func RenderMarkdown(
	template string,
	draft Draft,
	mode OutputMode,
) (string, error) {
	if err := validateDraft(draft); err != nil {
		return "", fmt.Errorf("render PR description: %w", err)
	}
	if mode == OutputModeChangelog {
		return renderChangelog(draft) + "\n", nil
	}
	if mode != OutputModeDescription {
		return "", fmt.Errorf("render PR description: invalid output mode %q", mode)
	}

	description := renderBulletList(draft.Summary)
	testing := renderBulletList(draft.Testing)
	template = strings.ReplaceAll(
		template,
		"Please describe the tests that you ran to verify your changes.",
		"",
	)
	if strings.TrimSpace(template) == "" {
		return description + "\n\n## How Has This Been Tested?\n\n" +
			testing + "\n", nil
	}

	rendered, foundDescription := insertAfterHeading(
		template,
		"description",
		description,
	)
	if !foundDescription {
		rendered = "## Description\n\n" + description + "\n\n" + rendered
	}

	rendered, foundTesting := insertAfterHeading(
		rendered,
		"how has this been tested",
		testing,
	)
	if !foundTesting {
		rendered = strings.TrimRight(rendered, "\n") +
			"\n\n## How Has This Been Tested?\n\n" + testing + "\n"
	}
	return strings.TrimSpace(rendered) + "\n", nil
}

func renderChangelog(draft Draft) string {
	var output strings.Builder

	byFile := make(map[string][]Change)
	for _, change := range draft.Changes {
		byFile[change.File] = append(byFile[change.File], change)
	}
	files := make([]string, 0, len(byFile))
	for file := range byFile {
		files = append(files, file)
	}
	sort.Strings(files)

	for _, file := range files {
		fmt.Fprintf(&output, "### `%s`\n\n", file)
		for _, change := range byFile[file] {
			fmt.Fprintf(
				&output,
				"- **%s** — %s\n",
				change.ID,
				change.Summary,
			)
		}
		output.WriteString("\n")
	}
	return strings.TrimSpace(output.String())
}

func renderBulletList(items []string) string {
	var output strings.Builder
	for _, item := range items {
		fmt.Fprintf(&output, "- %s\n", item)
	}
	return strings.TrimRight(output.String(), "\n")
}

func insertAfterHeading(markdown string, headingText string, content string) (string, bool) {
	lines := strings.Split(markdown, "\n")
	for index, line := range lines {
		level, text, ok := parseHeading(line)
		if !ok || !strings.Contains(strings.ToLower(text), headingText) {
			continue
		}

		end := len(lines)
		for next := index + 1; next < len(lines); next++ {
			nextLevel, _, isHeading := parseHeading(lines[next])
			if isHeading && nextLevel <= level {
				end = next
				break
			}
		}
		before := strings.TrimRight(strings.Join(lines[:index+1], "\n"), "\n")
		existing := strings.Trim(strings.Join(lines[index+1:end], "\n"), "\n")
		after := strings.TrimLeft(strings.Join(lines[end:], "\n"), "\n")

		var section strings.Builder
		section.WriteString(before)
		section.WriteString("\n\n")
		section.WriteString(content)
		if existing != "" {
			section.WriteString("\n\n")
			section.WriteString(existing)
		}
		if after != "" {
			section.WriteString("\n\n")
			section.WriteString(after)
		}
		return section.String(), true
	}
	return markdown, false
}

func parseHeading(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || len(trimmed) == level ||
		trimmed[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimSpace(trimmed[level+1:]), true
}
