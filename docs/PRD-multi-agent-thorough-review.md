# PRD: Deep-by-Default Review and a Multi-Agent `thorough` Tier

## Objective

Drop `standard` entirely, make `deep` the only single-pass depth and the
default, and introduce `thorough` as the new top tier. Under `thorough`,
review runs as a multi-agent pipeline (parallel specialist reviewers,
deduplication, adversarial verification, synthesis) instead of one
`codex exec` call, closing the gap with Codex's own multi-pass PR review.

## Current behavior

```text
champu review <number-or-url> [--depth standard|deep] [--instructions path]
```

- Default depth: `standard`.
- `standard`: 8 MB diff-capture budget, `medium` Codex reasoning effort, one
  `codex exec` call covering every review dimension in one prompt.
- `deep`: 32 MB diff-capture budget, `high` reasoning effort, still one
  `codex exec` call.

## Proposed depth tiers

| Depth      | Default | Diff budget | Execution                                   |
|------------|---------|--------------|----------------------------------------------|
| `deep`     | **yes** | 32 MB        | unchanged single-pass behavior, now the only single-pass tier and the default |
| `thorough` | no      | 32 MB        | new: multi-agent pipeline (below)            |

- `standard` is removed. `champu review <target> --depth standard` becomes an
  invalid invocation (see Compatibility).
- `deep`'s internal behavior does not change — it simply becomes the default
  and the CLI's only single-pass option.
- `champu review <number-or-url> --depth thorough` opts into the multi-agent
  pipeline. Everything else about the CLI surface (target parsing,
  instructions file, output rendering) stays the same.

## Compatibility

Removing `standard` is a breaking CLI change: any existing script or habit
that passes `--depth standard` starts failing validation instead of running
a cheaper review. No compatibility alias is kept — `standard` maps to
nothing, not to `deep` — because the point is that every default and every
explicit invocation now gets the more rigorous evidence budget. This forces
the major-version bump below.

## `thorough`: multi-agent pipeline

1. **Specialist lenses (parallel).** Five focused `codex exec` calls replace
   the one general-purpose prompt, each scoped to one review dimension and
   given the same evidence payload:
   - correctness and logic
   - security
   - performance, concurrency, and N+1/loop-bound external calls
   - error handling
   - test coverage
   Each lens prompt carries the same rigor already added to `reviewPrompt`
   (flag-only-if-all-conditions-hold checklist, continue through the whole
   diff, no speculation) narrowed to its one dimension.
2. **Fan-out.** The five lens calls run concurrently, bounded by a
   concurrency cap of 5 (one slot per lens). A lens that fails or times out is
   dropped; the review continues with the remaining lenses and the omission
   is surfaced the same way `EvidenceOmitted` is surfaced today (a notice
   plus which lens dimensions are missing).
3. **Dedup (pure Go, no LLM).** Before spending any more tokens, findings
   from different lenses that share the same file and an overlapping line
   range are merged into one candidate finding.
4. **Verify (one `codex exec` call per surviving candidate).** Each candidate
   is re-checked against the evidence and either confirmed or dropped. This
   is the step that removes speculative or marginal findings that a
   single-pass review would otherwise ship.
5. **Synthesize.** Confirmed findings are placed into the existing three
   report sections (`code_quality_and_style`, `specific_suggestions`,
   `potential_issues_and_risks`) and given a severity, using the section and
   severity rubrics already in `reviewPrompt`. The final `review.schema.json`
   shape does not change, so rendering in `output.go` needs no changes.

## Progress and usage reporting

- Replace the current linear `reviewStages` progression with a `thorough`-
  specific view: lens completion count (`3/5 lenses complete`), then
  candidate count, then verification count, then synthesis — mirroring the
  existing detailed-progress mechanism rather than introducing a new one.
- Token usage and review-credit accounting must sum across all Codex calls
  (5 lenses + 1 verify call per candidate + synthesis) instead of assuming a
  single call, since `calculateUsageReport` currently reads one usage value.

## Cost and latency impact

- Every review now runs at `deep`'s evidence budget by default: 4x the diff
  capture and `high` instead of `medium` reasoning effort compared to
  today's default. There is no cheaper opt-out depth anymore.
- `thorough` costs roughly `5 + (number of surviving candidates)` Codex
  calls per review, substantially more than `deep`. It is opt-in only; the
  default stays `deep`.

## Documentation and tests

- `champu --help`, README, and `internal/cli/options.go` depth validation
  and help text: remove `standard`, add `thorough`, update the default-depth
  description.
- `internal/review`: fan-out/dedup/verify/synthesize pipeline, per-lens
  prompts, aggregated usage accounting, partial-lens-failure handling.
- `internal/review` tests: remove `standard`-specific cases, add a fake
  `command.Runner` that can respond to concurrent invocations, and add
  coverage for partial lens failure, dedup correctness, and a candidate that
  fails verification.
- `AGENTS.md` project map entry for `internal/review` stays accurate (still
  "bounded-evidence pull-request review," now across two execution modes).

## Version

Latest published tag is `v0.3.0`. Dropping `standard` is a breaking CLI
change (an existing `--depth standard` invocation now fails), so this is a
major release regardless of `thorough` being purely additive. Target
release: `v1.0.0` (`major:` or `feat!:`).

## Explicit design decisions (flag if wrong)

- `standard` is removed outright, with no compatibility alias to `deep`.
- Five lenses, as listed above, for v1. Boundaries can be re-tuned after
  real usage without a schema change.
- Verification is a single confirm/refute call per candidate, not a
  multi-vote majority — keeps `thorough`'s cost bound linear in candidate
  count. Multi-vote verification for `critical`/`high` candidates is a
  possible v2 enhancement, out of scope here.
- No new schema fields (e.g., which lens found a finding) in v1 — output
  shape is unchanged from today's single-pass review.

## Acceptance criteria

- `champu review <target>` with no `--depth` runs at `deep` budget and
  reasoning effort.
- `champu review <target> --depth standard` is rejected as an invalid depth.
- `champu review <target> --depth deep` behaves exactly as today's `deep`.
- `champu review <target> --depth thorough` runs five lens reviews in
  parallel, deduplicates, verifies each candidate, and renders one report in
  the existing three-section format.
- A single lens failure does not fail the whole `thorough` review; the
  report notes which dimension is missing.
- Usage/credit totals reported for `thorough` reflect the sum of every Codex
  call made during that review.
- `champu --help`, README, and CLI validation document only `deep` and
  `thorough`, with `deep` as the default.
- `go test ./...`, `go vet ./...`, and `go test -race ./...` pass.
