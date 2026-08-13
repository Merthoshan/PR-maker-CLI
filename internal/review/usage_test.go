package review

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
)

func TestUsageCalculationUsesPreReviewRemainingAllowance(t *testing.T) {
	usage := TokenUsage{
		Available:            true,
		InputTokens:          80,
		CachedInputTokens:    30,
		CachedInputAvailable: true,
		OutputTokens:         20,
	}
	report := calculateUsageReport(
		usage,
		CreditUsage{Available: true, Credits: 10},
		AccountSnapshot{Available: true, RemainingCredits: 80},
		AccountSnapshot{Available: true, RemainingCredits: 70},
	)

	if usage.TotalTokens() != 100 {
		t.Fatalf("TotalTokens() = %d, want cached input not to be double-counted", usage.TotalTokens())
	}
	if report.ConsumptionPercent == nil || math.Abs(*report.ConsumptionPercent-12.5) > 0.0001 {
		t.Fatalf("ConsumptionPercent = %v, want 12.5", report.ConsumptionPercent)
	}
	if report.ExpectedRemaining == nil || math.Abs(*report.ExpectedRemaining-70) > 0.0001 {
		t.Fatalf("ExpectedRemaining = %v, want 70", report.ExpectedRemaining)
	}
	if report.Reconciliation == nil || report.Reconciliation.ConcurrentChange {
		t.Fatalf("Reconciliation = %+v, want matching snapshots", report.Reconciliation)
	}

	rendered := RenderUsageSummary(report, 18_420, 32_000)
	for _, expected := range []string{
		"Cached input tokens:       30 (included in input)",
		"Used by this review:       100 tokens",
		"Credits consumed:          10.00 credits",
		"Consumed:                  12.5%",
		"Remaining afterward:       70.00 credits",
		"Estimated review evidence: 18,420 / 32,000 tokens",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("usage summary missing %q:\n%s", expected, rendered)
		}
	}
}

func TestUsageCalculationRequiresReviewCreditsForPercentage(t *testing.T) {
	report := calculateUsageReport(
		TokenUsage{Available: true, InputTokens: 800, OutputTokens: 200},
		CreditUsage{},
		AccountSnapshot{Available: true, RemainingCredits: 80},
		AccountSnapshot{Available: true, RemainingCredits: 70},
	)
	if report.ConsumptionPercent != nil || report.ExpectedRemaining != nil || report.Reconciliation != nil {
		t.Fatalf("missing review credits produced account calculations: %+v", report)
	}
	rendered := RenderUsageSummary(report, 1, 10)
	for _, expected := range []string{
		"Used by this review:       1,000 tokens",
		"Credits consumed:          unavailable",
		"Available before review:   80.00 credits",
		"Consumed:                  unavailable (review credit usage was not reported)",
		"Account snapshot after:    70.00 credits",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("usage summary missing %q:\n%s", expected, rendered)
		}
	}
}

func TestUsageCalculationHandlesUnknownAndNonPositiveAllowance(t *testing.T) {
	usage := TokenUsage{Available: true, InputTokens: 8, OutputTokens: 2}
	tests := []struct {
		name     string
		before   AccountSnapshot
		contains string
	}{
		{name: "unknown", contains: "Account credit balance:    unavailable"},
		{
			name:     "zero",
			before:   AccountSnapshot{Available: true, RemainingCredits: 0},
			contains: "pre-review credit balance is not positive",
		},
		{
			name:     "negative",
			before:   AccountSnapshot{Available: true, RemainingCredits: -4},
			contains: "pre-review credit balance is not positive",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := calculateUsageReport(
				usage,
				CreditUsage{Available: true, Credits: 10},
				test.before,
				AccountSnapshot{},
			)
			if report.ConsumptionPercent != nil || report.ExpectedRemaining != nil {
				t.Fatalf("non-positive/unknown allowance produced a percentage: %+v", report)
			}
			rendered := RenderUsageSummary(report, 1, 10)
			if !strings.Contains(rendered, test.contains) || strings.Contains(rendered, "NaN") || strings.Contains(rendered, "Inf") {
				t.Fatalf("usage summary =\n%s", rendered)
			}
		})
	}
}

func TestUsageCalculationRoundsAndSurfacesConcurrentChange(t *testing.T) {
	usage := TokenUsage{Available: true, InputTokens: 1, OutputTokens: 0}
	report := calculateUsageReport(
		usage,
		CreditUsage{Available: true, Credits: 1},
		AccountSnapshot{Available: true, RemainingCredits: 6},
		AccountSnapshot{Available: true, RemainingCredits: 3},
	)
	if report.Reconciliation == nil || !report.Reconciliation.ConcurrentChange ||
		report.Reconciliation.UnattributedChange != 2 {
		t.Fatalf("Reconciliation = %+v", report.Reconciliation)
	}
	rendered := RenderUsageSummary(report, 1, 10)
	if !strings.Contains(rendered, "Consumed:                  16.7%") ||
		!strings.Contains(rendered, "+2.00 credits beyond this review") ||
		!strings.Contains(rendered, "concurrent activity") {
		t.Fatalf("usage summary =\n%s", rendered)
	}
}

func TestFormatCreditsPreservesSmallNonZeroCharges(t *testing.T) {
	for value, want := range map[float64]string{
		10:       "10.00",
		0.5:      "0.50",
		0.065625: "0.065625",
		0.004:    "0.004",
	} {
		if got := formatCredits(value); got != want {
			t.Fatalf("formatCredits(%v) = %q, want %q", value, got, want)
		}
	}
}

func TestCodexEventParser(t *testing.T) {
	t.Run("extracts final response and token breakdown", func(t *testing.T) {
		var streamed TokenUsage
		parser := &codexEventParser{onUsage: func(usage TokenUsage) { streamed = usage }}
		parser.consume(`{"type":"thread.started","thread_id":"test"}`)
		parser.consume(`{"type":"item.completed","item":{"id":"1","type":"reasoning","text":"checking"}}`)
		parser.consume(`{"type":"item.completed","item":{"id":"2","type":"agent_message","text":"{\"overview\":\"done\"}"}}`)
		parser.consume(`{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":400,"cache_write_input_tokens":50,"output_tokens":300,"reasoning_output_tokens":100}}`)

		final, usage, err := parser.result()
		if err != nil || final != `{"overview":"done"}` {
			t.Fatalf("result() = %q, %+v, %v", final, usage, err)
		}
		if usage.TotalTokens() != 1500 || !usage.CachedInputAvailable ||
			!usage.CacheWriteInputAvailable || !usage.ReasoningOutputAvailable {
			t.Fatalf("usage = %+v", usage)
		}
		if streamed.TotalTokens() != usage.TotalTokens() {
			t.Fatalf("streamed usage = %+v, final usage = %+v", streamed, usage)
		}
	})

	t.Run("missing usage fields remains explicitly unavailable", func(t *testing.T) {
		parser := &codexEventParser{}
		parser.consume(`{"type":"item.completed","item":{"id":"2","type":"agent_message","text":"{}"}}`)
		parser.consume(`{"type":"turn.completed","usage":{"input_tokens":12}}`)
		_, usage, err := parser.result()
		if err != nil || usage.Available {
			t.Fatalf("result() usage = %+v, error = %v", usage, err)
		}
	})

	for name, event := range map[string]string{
		"malformed JSON": `{"type":`,
		"negative usage": `{"type":"turn.completed","usage":{"input_tokens":-1,"output_tokens":2}}`,
		"turn failure":   `{"type":"turn.failed","error":{"message":"private raw response"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			parser := &codexEventParser{}
			parser.consume(event)
			_, _, err := parser.result()
			if err == nil {
				t.Fatal("result() error = nil")
			}
			if strings.Contains(err.Error(), "private raw response") {
				t.Fatalf("result() leaked raw event data: %v", err)
			}
		})
	}
}

type streamingEventRunner struct {
	lines       []string
	streamCalls int
	limit       int64
}

func (runner *streamingEventRunner) Run(context.Context, command.Spec) (command.Result, error) {
	return command.Result{}, errors.New("buffered execution should not be used")
}

func (runner *streamingEventRunner) RunStreaming(
	_ context.Context,
	spec command.Spec,
	onLine func(string),
) (command.Result, error) {
	runner.streamCalls++
	runner.limit = spec.StdoutLimit
	for _, line := range runner.lines {
		onLine(line)
	}
	return command.Result{}, nil
}

func TestRunCodexUsesStreamingRunner(t *testing.T) {
	streaming := &streamingEventRunner{lines: []string{
		`{"type":"item.completed","item":{"id":"2","type":"agent_message","text":"{}"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":12,"output_tokens":3}}`,
	}}
	runner := &Runner{runner: streaming}
	parser := &codexEventParser{}

	if err := runner.runCodex(context.Background(), command.Spec{Name: "codex"}, parser); err != nil {
		t.Fatalf("runCodex() error = %v", err)
	}
	final, usage, err := parser.result()
	if err != nil || final != "{}" || usage.TotalTokens() != 15 {
		t.Fatalf("streamed result = %q, %+v, %v", final, usage, err)
	}
	if streaming.streamCalls != 1 || streaming.limit != maxCodexEventBytes {
		t.Fatalf("streaming calls = %d, limit = %d", streaming.streamCalls, streaming.limit)
	}
}

type failingAccountSource struct{}

func (failingAccountSource) Snapshot(context.Context) (AccountSnapshot, error) {
	return AccountSnapshot{}, errors.New("secret-token raw authorization response")
}

func (failingAccountSource) CreditsForReview(
	context.Context,
	TokenUsage,
) (CreditUsage, error) {
	return CreditUsage{}, errors.New("secret-credit raw authorization response")
}

func TestAccountSourceErrorsAreNotLeaked(t *testing.T) {
	runner := &Runner{accountUsage: failingAccountSource{}}
	snapshot := runner.accountSnapshot(context.Background())
	if snapshot.Available {
		t.Fatalf("account snapshot = %+v, want unavailable", snapshot)
	}
	credits := runner.reviewCredits(
		context.Background(),
		TokenUsage{Available: true, InputTokens: 1},
	)
	if credits.Available {
		t.Fatalf("review credits = %+v, want unavailable", credits)
	}
	rendered := RenderUsageSummary(
		calculateUsageReport(TokenUsage{}, credits, snapshot, AccountSnapshot{}),
		1,
		10,
	)
	if strings.Contains(rendered, "secret-token") || strings.Contains(rendered, "authorization response") {
		t.Fatalf("usage summary leaked source error: %s", rendered)
	}
}
