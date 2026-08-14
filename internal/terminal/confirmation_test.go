package terminal

import "testing"

func TestParseConfirmation(t *testing.T) {
	tests := []struct {
		input string
		want  Confirmation
	}{
		{input: "y", want: ConfirmationAccepted},
		{input: " YES ", want: ConfirmationAccepted},
		{input: "n", want: ConfirmationDeclined},
		{input: "No", want: ConfirmationDeclined},
		{input: " \t", want: ConfirmationDeclined},
		{input: "later", want: ConfirmationInvalid},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			if got := ParseConfirmation(test.input); got != test.want {
				t.Fatalf("ParseConfirmation(%q) = %d, want %d", test.input, got, test.want)
			}
		})
	}
}
