package terminal

import (
	"io"
	"os"
	"strings"
)

const ansiReset = "\x1b[0m"

var severityColors = map[string]string{
	"CRITICAL": "\x1b[91m",
	"HIGH":     "\x1b[31m",
	"MEDIUM":   "\x1b[33m",
	"LOW":      "\x1b[34m",
}

// ColorizeValidatedSeverities applies color only at lines identified by the
// validated review renderer, and only when output is an interactive terminal
// that permits color.
func ColorizeValidatedSeverities(
	markdown string,
	severityLines map[int]string,
	output io.Writer,
) string {
	_, noColor := os.LookupEnv("NO_COLOR")
	return colorizeValidatedSeverities(markdown, severityLines, isTerminal(output), noColor)
}

func colorizeValidatedSeverities(
	markdown string,
	severityLines map[int]string,
	interactive bool,
	noColor bool,
) string {
	if !interactive || noColor {
		return markdown
	}
	lines := strings.Split(markdown, "\n")
	for lineNumber, severity := range severityLines {
		color, ok := severityColors[severity]
		if !ok || lineNumber <= 0 || lineNumber > len(lines) {
			continue
		}
		label := "**" + severity + "**"
		colored := "**" + color + severity + ansiReset + "**"
		lines[lineNumber-1] = strings.Replace(lines[lineNumber-1], label, colored, 1)
	}
	return strings.Join(lines, "\n")
}
