package github

import (
	"errors"
	"strings"
)

// ParseOwnerRepositoryPath validates a trimmed GitHub path and returns its
// canonical owner/repository form.
func ParseOwnerRepositoryPath(value string) (string, error) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(value), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", errors.New("GitHub path must identify one owner and repository")
	}
	return parts[0] + "/" + parts[1], nil
}
