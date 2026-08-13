package review

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const creditReconciliationTolerance = 0.000001

// TotalTokens returns the non-overlapping total used by the review turn.
func (usage TokenUsage) TotalTokens() int64 {
	if !usage.Available {
		return 0
	}
	return usage.InputTokens + usage.OutputTokens
}

type unavailableAccountUsageSource struct{}

func (unavailableAccountUsageSource) Snapshot(context.Context) (AccountSnapshot, error) {
	return AccountSnapshot{}, nil
}

func (unavailableAccountUsageSource) CreditsForReview(
	context.Context,
	TokenUsage,
) (CreditUsage, error) {
	return CreditUsage{}, nil
}

func calculateUsageReport(
	usage TokenUsage,
	reviewCredits CreditUsage,
	before AccountSnapshot,
	after AccountSnapshot,
) UsageReport {
	report := UsageReport{
		Review:        usage,
		ReviewCredits: reviewCredits,
		AccountBefore: before,
		AccountAfter:  after,
	}
	if before.Available && before.RemainingCredits <= 0 {
		report.PreReviewNonPositive = true
	}
	if reviewCredits.Available && before.Available && before.RemainingCredits > 0 {
		percentage := reviewCredits.Credits / before.RemainingCredits * 100
		expectedRemaining := before.RemainingCredits - reviewCredits.Credits
		report.ConsumptionPercent = &percentage
		report.ExpectedRemaining = &expectedRemaining
	}
	if reviewCredits.Available && before.Available && after.Available {
		observed := before.RemainingCredits - after.RemainingCredits
		unattributed := observed - reviewCredits.Credits
		report.Reconciliation = &UsageReconciliation{
			ObservedAccountChange: observed,
			UnattributedChange:    unattributed,
			ConcurrentChange:      math.Abs(unattributed) > creditReconciliationTolerance,
		}
	}
	return report
}

// RenderUsageSummary renders usage separately from the canonical Markdown
// review so account credits, actual tokens, and evidence estimates cannot be
// confused.
func RenderUsageSummary(report UsageReport, evidenceEstimate int, evidenceBudget int) string {
	var builder strings.Builder
	builder.WriteString("Review usage\n")
	if report.Review.Available {
		fmt.Fprintf(&builder, "  Input tokens:              %s\n", formatTokens(report.Review.InputTokens))
		if report.Review.CachedInputAvailable {
			fmt.Fprintf(
				&builder,
				"  Cached input tokens:       %s (included in input)\n",
				formatTokens(report.Review.CachedInputTokens),
			)
		}
		if report.Review.CacheWriteInputAvailable {
			fmt.Fprintf(
				&builder,
				"  Cache-write input tokens:  %s (included in input)\n",
				formatTokens(report.Review.CacheWriteInputTokens),
			)
		}
		fmt.Fprintf(&builder, "  Output tokens:             %s\n", formatTokens(report.Review.OutputTokens))
		if report.Review.ReasoningOutputAvailable {
			fmt.Fprintf(
				&builder,
				"  Reasoning output tokens:   %s (included in output)\n",
				formatTokens(report.Review.ReasoningOutputTokens),
			)
		}
		fmt.Fprintf(
			&builder,
			"  Used by this review:       %s tokens\n",
			formatTokens(report.Review.TotalTokens()),
		)
	} else {
		builder.WriteString("  Used by this review:       unavailable\n")
	}

	if report.ReviewCredits.Available {
		fmt.Fprintf(
			&builder,
			"  Credits consumed:          %s credits\n",
			formatCredits(report.ReviewCredits.Credits),
		)
	} else {
		builder.WriteString("  Credits consumed:          unavailable\n")
	}

	if !report.AccountBefore.Available {
		builder.WriteString("  Account credit balance:    unavailable\n")
	} else {
		fmt.Fprintf(
			&builder,
			"  Available before review:   %s credits\n",
			formatCredits(report.AccountBefore.RemainingCredits),
		)
		if report.ConsumptionPercent != nil && report.ExpectedRemaining != nil {
			fmt.Fprintf(&builder, "  Consumed:                  %.1f%%\n", *report.ConsumptionPercent)
			fmt.Fprintf(
				&builder,
				"  Remaining afterward:       %s credits\n",
				formatCredits(*report.ExpectedRemaining),
			)
		} else if report.PreReviewNonPositive {
			builder.WriteString("  Consumed:                  unavailable (pre-review credit balance is not positive)\n")
			builder.WriteString("  Remaining afterward:       unavailable\n")
		} else {
			builder.WriteString("  Consumed:                  unavailable (review credit usage was not reported)\n")
		}
	}

	if report.AccountAfter.Available {
		fmt.Fprintf(
			&builder,
			"  Account snapshot after:    %s credits\n",
			formatCredits(report.AccountAfter.RemainingCredits),
		)
	}
	if report.Reconciliation != nil {
		if report.Reconciliation.ConcurrentChange {
			fmt.Fprintf(
				&builder,
				"  Reconciliation:            %s credits beyond this review; concurrent activity, a reset, or accounting delay may be involved\n",
				formatSignedCredits(report.Reconciliation.UnattributedChange),
			)
		} else {
			builder.WriteString("  Reconciliation:            account credit change matches this review\n")
		}
	}

	builder.WriteString("\nEvidence budget\n")
	fmt.Fprintf(
		&builder,
		"  Estimated review evidence: %s / %s tokens\n",
		formatTokens(int64(evidenceEstimate)),
		formatTokens(int64(evidenceBudget)),
	)
	return strings.TrimSpace(builder.String())
}

func validCreditUsage(usage CreditUsage) bool {
	return usage.Available && !math.IsNaN(usage.Credits) && !math.IsInf(usage.Credits, 0) && usage.Credits >= 0
}

func validAccountSnapshot(snapshot AccountSnapshot) bool {
	return snapshot.Available && !math.IsNaN(snapshot.RemainingCredits) && !math.IsInf(snapshot.RemainingCredits, 0)
}

func formatSignedCredits(value float64) string {
	if value > 0 {
		return "+" + formatCredits(value)
	}
	return formatCredits(value)
}

func formatCredits(value float64) string {
	precision := 2
	absolute := math.Abs(value)
	if absolute > 0 && absolute < 1 {
		precision = 6
	}
	formatted := strconv.FormatFloat(value, 'f', precision, 64)
	for precision > 2 && strings.HasSuffix(formatted, "0") {
		formatted = strings.TrimSuffix(formatted, "0")
		precision--
	}
	return formatted
}

func formatTokens(value int64) string {
	negative := value < 0
	digits := strconv.FormatInt(value, 10)
	if negative {
		digits = strings.TrimPrefix(digits, "-")
	}
	for index := len(digits) - 3; index > 0; index -= 3 {
		digits = digits[:index] + "," + digits[index:]
	}
	if negative {
		return "-" + digits
	}
	return digits
}
