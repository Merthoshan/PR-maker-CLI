package terminal

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestNewReporter(t *testing.T) {
	if _, err := NewReporter(nil); err == nil {
		t.Fatal("NewReporter() error = nil, want output validation error")
	}

	reporter, err := NewReporter(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("NewReporter() unexpected error: %v", err)
	}
	if reporter.interactive {
		t.Fatal("buffer reporter is interactive, want static reporting")
	}
}

func TestReporterStartWritesStaticStatusForNonTerminal(t *testing.T) {
	var output bytes.Buffer
	reporter := newReporter(&output, false, time.Second, time.Now)

	stop := reporter.Start("  Collecting Git evidence  ")
	stop()
	stop()

	if got := output.String(); got != "Collecting Git evidence...\n" {
		t.Fatalf("reporter output = %q, want static status line", got)
	}
}

func TestReporterStartAnimatesAndClearsTerminalLine(t *testing.T) {
	var output bytes.Buffer
	now := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	reporter := newReporter(
		&output,
		true,
		time.Hour,
		func() time.Time { return now },
	)

	stop := reporter.Start("Generating PR description with Codex")
	stop()
	stop()

	got := output.String()
	if !strings.Contains(got, "⠋ Generating PR description with Codex...") {
		t.Fatalf("reporter output missing initial frame: %q", got)
	}
	if !strings.HasSuffix(got, clearLine) {
		t.Fatalf("reporter output does not clear final line: %q", got)
	}
}

func TestReporterRenderIncludesElapsedTime(t *testing.T) {
	var output bytes.Buffer
	startedAt := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	reporter := newReporter(
		&output,
		true,
		time.Second,
		func() time.Time { return startedAt.Add(8 * time.Second) },
	)

	reporter.render("⠹", "Refining PR description with Codex", startedAt)

	if !strings.Contains(output.String(), "8s") {
		t.Fatalf("reporter output missing elapsed time: %q", output.String())
	}
}

func TestReporterDetailedProgressIsDeterministicWhenRedirected(t *testing.T) {
	var output bytes.Buffer
	reporter := newReporter(&output, false, time.Second, time.Now)

	update, stop := reporter.StartDetailed(Status{
		Message: "Inspecting Git repository",
		Percent: 5,
	})
	update(Status{
		Message: "Starting Codex review",
		Percent: 50,
		Details: []string{
			"Evidence estimate: 18,420 / 32,000 tokens",
			"Account credit balance: unavailable",
		},
	})
	update(Status{
		Message: "Starting Codex review",
		Percent: 50,
		Details: []string{"Review usage: now available"},
	})
	stop()

	want := "[  5%] Inspecting Git repository...\n" +
		"[ 50%] Starting Codex review...\n" +
		"  Evidence estimate: 18,420 / 32,000 tokens\n" +
		"  Account credit balance: unavailable\n"
	if output.String() != want {
		t.Fatalf("detailed output = %q, want %q", output.String(), want)
	}
}

func TestReporterDetailedProgressAnimatesWithElapsedTime(t *testing.T) {
	var output bytes.Buffer
	startedAt := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	nowCalls := 0
	reporter := newReporter(
		&output,
		true,
		time.Hour,
		func() time.Time {
			nowCalls++
			if nowCalls == 1 {
				return startedAt
			}
			return startedAt.Add(8 * time.Second)
		},
	)

	_, stop := reporter.StartDetailed(Status{
		Message: "Streaming Codex events",
		Percent: 58,
		Details: []string{"Review usage: pending"},
	})
	stop()

	got := output.String()
	for _, expected := range []string{"[██████░░░░]", "58%", "Elapsed: 00:08", "Review usage: pending"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("detailed output missing %q: %q", expected, got)
		}
	}
	if !strings.HasSuffix(got, clearLine) {
		t.Fatalf("detailed output does not clear final line: %q", got)
	}
}
