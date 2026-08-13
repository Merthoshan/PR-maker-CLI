package terminal

import (
	"strings"
	"testing"
)

func TestColorizeSeverities(t *testing.T) {
	markdown := "Overview says **HIGH**\n- **CRITICAL**\n- **HIGH**\n- **MEDIUM**\n- **LOW**\n- **UNKNOWN**"
	severityLines := map[int]string{
		2: "CRITICAL",
		3: "HIGH",
		4: "MEDIUM",
		5: "LOW",
		6: "UNKNOWN",
	}

	colored := colorizeValidatedSeverities(markdown, severityLines, true, false)
	for _, severity := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"} {
		if !strings.Contains(colored, severity) ||
			!strings.Contains(colored, severityColors[severity]+severity+ansiReset) {
			t.Fatalf("colored output missing %s: %q", severity, colored)
		}
	}
	if strings.Contains(colored, severityColors["HIGH"]+"UNKNOWN") {
		t.Fatalf("unvalidated severity was colored: %q", colored)
	}
	if strings.Contains(strings.Split(colored, "\n")[0], "\x1b[") {
		t.Fatalf("overview severity-like text was colored: %q", colored)
	}

	for name, got := range map[string]string{
		"redirected": colorizeValidatedSeverities(markdown, severityLines, false, false),
		"NO_COLOR":   colorizeValidatedSeverities(markdown, severityLines, true, true),
	} {
		if got != markdown || strings.Contains(got, "\x1b[") {
			t.Fatalf("%s output = %q, want plain markdown", name, got)
		}
	}
}
