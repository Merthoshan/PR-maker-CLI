package review

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Merthoshan/PR-maker-CLI/internal/structuredoutput"
)

var allowedSeverities = map[string]bool{
	"critical": true,
	"high":     true,
	"medium":   true,
	"low":      true,
}

// decodeReview extracts one strict structured review response.
func decodeReview(output string) (structuredReview, error) {
	return structuredoutput.DecodeSingleJSON[structuredReview](output)
}

func validateReview(value structuredReview) error {
	if strings.TrimSpace(value.Overview) == "" {
		return errors.New("overview is required")
	}
	sections := []struct {
		name     string
		findings []finding
	}{
		{name: "code quality and style", findings: value.CodeQualityAndStyle},
		{name: "specific suggestions", findings: value.SpecificSuggestions},
		{name: "potential issues and risks", findings: value.PotentialIssuesAndRisks},
	}
	for _, section := range sections {
		for index, item := range section.findings {
			if !allowedSeverities[item.Severity] {
				return fmt.Errorf("%s finding %d has invalid severity %q", section.name, index+1, item.Severity)
			}
			if strings.TrimSpace(item.File) == "" {
				return fmt.Errorf("%s finding %d has no file", section.name, index+1)
			}
			if item.Line != nil && *item.Line <= 0 {
				return fmt.Errorf("%s finding %d has invalid line", section.name, index+1)
			}
			if strings.TrimSpace(item.Evidence) == "" || strings.TrimSpace(item.Impact) == "" || strings.TrimSpace(item.SuggestedFix) == "" {
				return fmt.Errorf("%s finding %d is incomplete", section.name, index+1)
			}
		}
	}
	return nil
}

func renderReview(value structuredReview) string {
	review, _ := renderReviewWithSeverities(value)
	return review
}

func renderReviewWithSeverities(value structuredReview) (string, map[int]string) {
	seen := make(map[string]bool)
	severityLines := make(map[int]string)
	var builder strings.Builder
	builder.WriteString("# Overview\n\n")
	builder.WriteString(strings.TrimSpace(value.Overview))
	builder.WriteString("\n\n")
	renderFindings(&builder, "Code Quality and Style", value.CodeQualityAndStyle, seen, severityLines)
	renderFindings(&builder, "Specific Suggestions", value.SpecificSuggestions, seen, severityLines)
	renderFindings(&builder, "Potential Issues and Risks", value.PotentialIssuesAndRisks, seen, severityLines)
	return strings.TrimSpace(builder.String()), severityLines
}

func addOverviewNotice(review string, notice string) string {
	const prefix = "# Overview\n\n"
	if !strings.HasPrefix(review, prefix) {
		return notice + "\n\n" + review
	}
	return prefix + notice + "\n\n" + strings.TrimPrefix(review, prefix)
}

func renderFindings(
	builder *strings.Builder,
	heading string,
	findings []finding,
	seen map[string]bool,
	severityLines map[int]string,
) {
	fmt.Fprintf(builder, "# %s\n\n", heading)
	written := 0
	for _, item := range findings {
		key := findingKey(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		location := strings.ReplaceAll(singleLine(item.File), "`", "\\`")
		if item.Line != nil {
			location = fmt.Sprintf("%s:%d", location, *item.Line)
		}
		severityLines[strings.Count(builder.String(), "\n")+1] = strings.ToUpper(item.Severity)
		fmt.Fprintf(
			builder,
			"- **%s** — `%s`: %s\n  - Impact: %s\n  - Suggested fix: %s\n",
			strings.ToUpper(item.Severity),
			location,
			singleLine(item.Evidence),
			singleLine(item.Impact),
			singleLine(item.SuggestedFix),
		)
		written++
	}
	if written == 0 {
		builder.WriteString("No actionable findings.\n")
	}
	builder.WriteByte('\n')
}

func findingKey(item finding) string {
	line := 0
	if item.Line != nil {
		line = *item.Line
	}
	return strings.ToLower(fmt.Sprintf(
		"%s\x00%d\x00%s",
		strings.TrimSpace(item.File),
		line,
		strings.TrimSpace(item.Evidence),
	))
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
