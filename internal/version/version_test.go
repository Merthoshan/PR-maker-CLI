package version

import (
	"strings"
	"testing"
)

func TestIsRelease(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "v0.1.0", want: true},
		{value: "v12.34.56", want: true},
		{value: " v1.2.3 ", want: true},
		{value: "1.2.3"},
		{value: "v1.2"},
		{value: "v01.2.3"},
		{value: "v1.2.3-beta"},
		{value: Development},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			if got := IsRelease(test.value); got != test.want {
				t.Fatalf("IsRelease(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{name: "older major", left: "v1.9.9", right: "v2.0.0", want: -1},
		{name: "older minor", left: "v1.2.9", right: "v1.3.0", want: -1},
		{name: "older patch", left: "v1.2.3", right: "v1.2.4", want: -1},
		{name: "equal", left: "v1.2.3", right: "v1.2.3"},
		{name: "newer", left: "v2.0.0", right: "v1.9.9", want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Compare(test.left, test.right)
			if err != nil {
				t.Fatalf("Compare() unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("Compare() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestCompareRejectsInvalidVersion(t *testing.T) {
	_, err := Compare("development", "v1.0.0")
	if err == nil {
		t.Fatal("Compare() error = nil, want invalid version error")
	}
	if !strings.Contains(err.Error(), "vMAJOR.MINOR.PATCH") {
		t.Fatalf("Compare() error = %q, want format context", err)
	}
}

func TestCurrentReturnsKnownVersionForm(t *testing.T) {
	current := Current()
	if current != Development && !IsRelease(current) {
		t.Fatalf("Current() = %q, want development or semantic release", current)
	}
}
