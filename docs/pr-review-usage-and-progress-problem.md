# Problem Statement: PR Review Usage and Progress Visibility

## Context

`champu-pr` reviews an existing GitHub pull request by collecting GitHub
evidence and sending a structured review request to Codex. The command is run
from a local checkout of the same repository as the pull request:

```text
champu-pr review <number-or-url>
```

The repository-origin validation is intentional. A review must not analyze a
pull request against an unrelated local checkout.

The current review implementation is primarily in:

- `internal/review/review.go`
- `internal/review/evidence.go`
- `internal/command/runner.go`
- `internal/terminal/reporter.go`
- `cmd/champu-pr/main.go`

## Current behavior

The review pipeline currently:

1. Inspects the local Git repository.
2. Resolves pull-request metadata and diff evidence through GitHub CLI.
3. Selects evidence using a bounded local estimate.
4. Invokes Codex with a structured output schema.
5. Validates and renders the returned findings.

The local evidence budgets are currently approximately:

- Standard review: `32,000` estimated tokens.
- Deep review: `96,000` estimated tokens.

The estimate uses a stable bytes-to-token approximation. It is useful for
controlling evidence size, but it is not the actual Codex usage charged to the
account.

The command runner buffers Codex output until the process exits. During a
review, the terminal therefore shows a generic status such as:

```text
Reviewing pull request with Codex...
```

The terminal reporter currently provides a spinner and elapsed time, but no
review-stage detail, token information, or account-usage information.

The installed Codex CLI exposes a structured `--json` event mode, but Champu
does not currently stream or parse those events.

## Required usage metric

The desired metric is the percentage of the account's available credit balance
that the current review consumes.

The denominator is the credit balance available immediately before this review
begins.

```text
credits_available_before_review = account_credit_balance_before_review

review_percentage =
    review_credits / credits_available_before_review * 100

credits_remaining_after_review =
    credits_available_before_review - review_credits
```

Example:

```text
Available before review:   80 credits
Review usage:              10 credits

Review consumption: 10 / 80 = 12.5%
Remaining afterward: 70 credits
```

The intended display is:

```text
Review usage
  Used by this review:       10 tokens
  Credits consumed:          10 credits
  Available before review:   80 credits
  Consumed:                  12.5%
  Remaining afterward:       70 credits
```

## Account-usage constraints

Champu must not treat its local evidence budget as the account quota. The
following values are different and must be labeled separately:

1. **Evidence estimate**: local estimate used to bound the review payload.
2. **Review usage**: actual input, cached-input, and output tokens consumed by
   this Codex invocation.
3. **Review credit usage**: the account credits charged for this invocation,
   using the applicable model and token-category rates.
4. **Account credit balance**: the credits available immediately before the
   review.

Both the review credit charge and account denominator must come from a
supported, authenticated account or usage source. Codex credit rates can depend
on model and token category, while balances can be affected by shared agentic
usage and concurrent activity. Champu must not guess either value or present a
fabricated percentage.

If the account usage source is unavailable, the fallback must be explicit:

```text
Used by this review: 10 tokens
Credits consumed: unavailable
Account credit balance: unavailable
```

In that case, Champu may still show the local evidence estimate, but it must be
labeled as an evidence-budget metric rather than account usage.

## Progress and status requirements

The review should expose detailed stages instead of only reporting that it is
reviewing the pull request:

1. Inspecting Git repository.
2. Resolving pull-request metadata.
3. Collecting pull-request evidence.
4. Selecting evidence within the review budget.
5. Reading account credit balance before review.
6. Starting Codex review.
7. Streaming or processing Codex events.
8. Validating review findings.
9. Reconciling account usage after review.
10. Rendering the review report.

Interactive terminals should show a spinner or progress bar, stage, elapsed
time, and usage details. For example:

```text
⠸ [██████░░░░] 58% Reviewing pull request with Codex
  Evidence estimate: 18,420 / 32,000
  Review usage:      24,812 tokens
  Review credits:    2.50 credits
  Account remaining: 50.00 credits before review
  Consumed:          5.0%
  Elapsed:           00:08
```

The overall stage percentage is approximate and must not be presented as token
usage. If Codex does not provide enough information for a reliable ETA, show
elapsed time and omit the ETA rather than inventing one.

Non-interactive terminals and CI should receive static status lines without
ANSI cursor manipulation.

## Severity color-coding

Interactive terminal output should color-code finding severity so urgent
problems are easy to scan:

- `CRITICAL`: bright red.
- `HIGH`: red.
- `MEDIUM`: yellow.
- `LOW`: blue or gray.

Example:

```text
# Potential Issues and Risks

- CRITICAL — `auth/middleware.go:42`
  Evidence: ...
  Impact: ...
  Suggested fix: ...

- HIGH — `payments/service.go:88`
  Evidence: ...
```

Only the severity label needs color. The textual label must remain visible so
the output remains accessible, searchable, and useful when copied as plain
text. Color must be applied at terminal presentation time rather than being
embedded in the canonical Markdown review.

Color behavior must satisfy these constraints:

- Enable colors automatically only when stdout is an interactive terminal.
- Emit no ANSI escape sequences for redirected output, CI, or tests.
- Respect the `NO_COLOR` environment variable.
- Color only validated severity values; never infer color from section names.
- Keep the plain Markdown renderer independent from terminal formatting.

## Evidence and correctness requirements

- Actual review usage must be sourced from Codex structured usage data when
  available, not inferred from character count.
- Input, cached-input, and output tokens should be reported separately when
  Codex provides them.
- The account credit balance should be captured immediately before the review
  starts.
- If account credit snapshots are available, a before/after reconciliation
  should be used to detect discrepancies with the review credit charge.
- Concurrent account activity must be surfaced as a possible source of change
  between the before and after snapshots.
- The percentage must be omitted when either the review credit charge or the
  pre-review credit balance is unavailable.
- A zero or negative pre-review credit balance must be handled explicitly and must
  never cause division by zero.
- The review must remain read-only with respect to GitHub.
- Repository-origin validation must remain intact.
- Evidence omissions must continue to be disclosed; progress reporting must not
  imply that omitted files were reviewed.
- Account credentials, tokens, and raw authorization responses must never be
  written to the repository or included in the review output.

## Acceptance criteria

### Usage

- A completed review shows the actual tokens consumed by that review when
  Codex reports them.
- The output distinguishes input, cached-input, and output tokens when present.
- When a supported account usage source is available, Champu shows:
  - credit balance available before the review;
  - review credits consumed;
  - percentage of the pre-review credit balance consumed;
  - expected credit balance after the review.
- The percentage matches:

```text
review_credits / credits_available_before_review * 100
```

- When account data is unavailable, Champu says so and does not fabricate a
  percentage.

### Progress

- The review reports the detailed stages listed above.
- Interactive output includes a progress indicator and elapsed time.
- Non-interactive output remains readable and deterministic.
- Progress output is written to stderr; the review report remains on stdout.
- A failed or cancelled review stops its progress indicator cleanly.

### Color output

- Interactive findings use the documented severity colors.
- `CRITICAL`, `HIGH`, `MEDIUM`, and `LOW` remain present as text labels.
- Redirected and non-interactive output contains no ANSI escape sequences.
- `NO_COLOR` disables severity colors.
- Tests cover color-enabled and color-disabled rendering.

### Verification

- Tests cover credit calculations, zero/unknown balance handling, rounding, and
  concurrent-usage reconciliation.
- Tests cover streamed Codex events, malformed events, missing usage fields,
  process failures, and cancellation.
- Tests verify that account credentials and raw usage responses are not leaked.
- Existing repository validation, evidence-boundary, schema-validation, and
  read-only behavior continue to pass.

## Non-goals

- Do not use the local `32,000` or `96,000` evidence budget as the account
  denominator.
- Do not estimate account quota from the pull-request size.
- Do not claim an exact percentage when the account source is unavailable.
- Do not bypass the repository-origin safety check.
- Do not write review comments or modify GitHub as part of usage tracking.
- Do not expose account secrets, access tokens, or private billing details.

## Open implementation questions

1. Which supported authenticated source will provide the account's credit
   balance and authoritative review credit charge to the local Champu process?
2. Which Codex JSON event contains authoritative per-review token usage, including
   cached-input and reasoning-token accounting where applicable?
3. Should a before/after account snapshot be mandatory, or only used when the
   account source supports it?
4. How should the UI represent multiple concurrent reviews using the same
   account credit balance?
5. What precision should be used for percentages: one decimal place or two?
6. Should the final usage summary be printed only on successful reviews, or also
   for cancelled and failed reviews when partial usage is known?
