package application

import "testing"

func TestTicketFromBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{branch: "oncl-11300-hey-team", want: "ONCL-11300"},
		{branch: "feature/abc-42-fix", want: "ABC-42"},
		{branch: "feature-without-ticket", want: ""},
	}
	for _, test := range tests {
		if got := ticketFromBranch(test.branch); got != test.want {
			t.Errorf("ticketFromBranch(%q) = %q, want %q", test.branch, got, test.want)
		}
	}
}

func TestServiceFromChoice(t *testing.T) {
	tests := []struct {
		choice string
		want   string
	}{
		{choice: "1", want: "api"},
		{choice: "2", want: "worker"},
		{choice: "3", want: "api, worker"},
		{choice: "4", want: ""},
	}
	for _, test := range tests {
		if got, err := serviceFromChoice(test.choice); err != nil || got != test.want {
			t.Errorf("serviceFromChoice(%q) = %q, %v; want %q", test.choice, got, err, test.want)
		}
	}
}

func TestTitleWithMetadata(t *testing.T) {
	tests := []struct {
		service string
		branch  string
		want    string
	}{
		{"api", "oncl-11300-fix", "[api][ONCL-11300] Fix pricesheet counts"},
		{"worker", "feature", "[worker][] Fix pricesheet counts"},
		{"", "oncl-11300-fix", "[][ONCL-11300] Fix pricesheet counts"},
	}
	for _, test := range tests {
		got := titleWithMetadata("Fix pricesheet counts", test.branch, test.service)
		if got != test.want {
			t.Errorf("titleWithMetadata() = %q, want %q", got, test.want)
		}
	}
	if got := titleWithMetadata(
		"[api][ONCL-11300] Fix pricesheet counts",
		"oncl-11300-fix",
		"worker",
	); got != "[worker][ONCL-11300] Fix pricesheet counts" {
		t.Errorf("titleWithMetadata() double-prefix result = %q", got)
	}
}

func TestAvailableTitleLengthIncludesMetadata(t *testing.T) {
	if got := availableTitleLength("gal-2281-portfolio-api", "api"); got != 56 {
		t.Fatalf("availableTitleLength() = %d, want 56", got)
	}
	if got := availableTitleLength("feature", ""); got != 67 {
		t.Fatalf("availableTitleLength() without metadata = %d, want 67", got)
	}
}

func TestValidateTitleCountsUnicodeCharacters(t *testing.T) {
	title := "修"
	for len([]rune(title)) < maxPRTitleLength {
		title += "修"
	}
	if err := validateTitle(title); err != nil {
		t.Fatalf("validateTitle() rejected %d Unicode characters: %v", maxPRTitleLength, err)
	}
	if err := validateTitle(title + "修"); err == nil {
		t.Fatal("validateTitle() accepted 73 Unicode characters")
	}
}
