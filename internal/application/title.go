package application

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var ticketPattern = regexp.MustCompile(`(?i)([a-z]+-[0-9]+)`)
var metadataTitlePattern = regexp.MustCompile(`^\[[^\]]*\]\[[^\]]*\]\s*`)

const (
	serviceAPI          = "api"
	serviceWorker       = "worker"
	serviceAPIAndWorker = "api, worker"
)

func ticketFromBranch(branch string) string {
	match := ticketPattern.FindStringSubmatch(branch)
	if len(match) < 2 {
		return ""
	}
	return strings.ToUpper(match[1])
}

func titleWithMetadata(title, branch, service string) string {
	baseTitle := metadataTitlePattern.ReplaceAllString(strings.TrimSpace(title), "")
	return fmt.Sprintf("[%s][%s] %s", service, ticketFromBranch(branch), baseTitle)
}

func validateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("PR title is required")
	}
	if utf8.RuneCountInString(title) > 72 {
		return fmt.Errorf("PR title exceeds 72 characters after service and ticket prefix")
	}
	return nil
}

func serviceFromChoice(choice string) (string, error) {
	switch strings.TrimSpace(choice) {
	case "1":
		return serviceAPI, nil
	case "2":
		return serviceWorker, nil
	case "3":
		return serviceAPIAndWorker, nil
	case "4":
		return "", nil
	default:
		return "", fmt.Errorf(
			"invalid service choice %q: enter 1, 2, 3, or 4",
			choice,
		)
	}
}
