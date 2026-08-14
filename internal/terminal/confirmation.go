package terminal

import "strings"

// Confirmation classifies a conventional yes-or-no prompt response.
type Confirmation uint8

const (
	ConfirmationInvalid Confirmation = iota
	ConfirmationDeclined
	ConfirmationAccepted
)

// ParseConfirmation normalizes the y/yes and n/no forms shared by interactive
// workflows. An empty response follows the conventional default of no.
func ParseConfirmation(value string) Confirmation {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "y", "yes":
		return ConfirmationAccepted
	case "", "n", "no":
		return ConfirmationDeclined
	default:
		return ConfirmationInvalid
	}
}
