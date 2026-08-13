package review

import (
	"context"

	"github.com/Merthoshan/PR-maker-CLI/internal/github"
	"github.com/Merthoshan/PR-maker-CLI/internal/terminal"
)

type Progress interface{ Start(string) func() }

type Request struct {
	WorkingDirectory string
	Target           string
	Depth            string
	InstructionsPath string
}

type Outcome struct {
	Review                string
	PullRequest           github.PullRequest
	Omitted               bool
	Depth                 string
	Usage                 UsageReport
	EvidenceTokenEstimate int
	EvidenceTokenBudget   int
	SeverityLines         map[int]string
}

type payload struct {
	PullRequest            github.PullRequest   `json:"pull_request"`
	Labels                 []string             `json:"labels,omitempty"`
	Repository             string               `json:"repository"`
	ChangedFiles           []github.ChangedFile `json:"changed_files"`
	Diff                   string               `json:"selected_diff"`
	EvidenceOmitted        bool                 `json:"evidence_omitted"`
	OmittedFiles           []string             `json:"omitted_or_partial_files,omitempty"`
	SourceDiffLimited      bool                 `json:"source_diff_limited"`
	PullRequestBodyLimited bool                 `json:"pull_request_body_limited"`
	Depth                  string               `json:"depth"`
	ReviewInstructions     string               `json:"review_instructions,omitempty"`
	EvidenceTokenEstimate  int                  `json:"evidence_token_estimate"`
}

type localRepository struct {
	Root string
	Name string
}

type detailedProgress interface {
	StartDetailed(terminal.Status) (func(terminal.Status), func())
}

type diffSelection struct {
	Diff         string
	Omitted      bool
	OmittedFiles []string
}

type fileDiff struct {
	index    int
	path     string
	text     string
	complete bool
	score    int
}

type selectedFileDiff struct {
	index int
	text  string
	full  bool
	path  string
}

type structuredReview struct {
	Overview                string    `json:"overview"`
	CodeQualityAndStyle     []finding `json:"code_quality_and_style"`
	SpecificSuggestions     []finding `json:"specific_suggestions"`
	PotentialIssuesAndRisks []finding `json:"potential_issues_and_risks"`
}

type finding struct {
	Severity     string `json:"severity"`
	File         string `json:"file"`
	Line         *int   `json:"line"`
	Evidence     string `json:"evidence"`
	Impact       string `json:"impact"`
	SuggestedFix string `json:"suggested_fix"`
}

type eventEnvelope struct {
	Type string `json:"type"`
}

type itemCompletedEvent struct {
	Item struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
}

type turnCompletedEvent struct {
	Usage *rawTokenUsage `json:"usage"`
}

type rawTokenUsage struct {
	InputTokens           *int64 `json:"input_tokens"`
	CachedInputTokens     *int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens *int64 `json:"cache_write_input_tokens"`
	OutputTokens          *int64 `json:"output_tokens"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
}

// TokenUsage is the authoritative usage reported by one Codex invocation.
// Cached input is included in input tokens, and reasoning output is included
// in output tokens, so those breakdowns are not added to TotalTokens.
type TokenUsage struct {
	Available                bool
	InputTokens              int64
	CachedInputTokens        int64
	CachedInputAvailable     bool
	CacheWriteInputTokens    int64
	CacheWriteInputAvailable bool
	OutputTokens             int64
	ReasoningOutputTokens    int64
	ReasoningOutputAvailable bool
}

// CreditUsage is the authoritative credit charge for one Codex invocation.
// It is separate from TokenUsage because token categories and models can have
// different credit rates.
type CreditUsage struct {
	Available bool
	Credits   float64
}

// AccountSnapshot is a credit balance captured by a supported authenticated
// source. Available must be false when the source cannot provide that metric.
type AccountSnapshot struct {
	Available        bool
	RemainingCredits float64
}

// AccountUsageSource captures account credit allowance and reports the credit
// charge for the authoritative turn-token breakdown of that review.
// Implementations must not expose credentials or raw authorization responses.
type AccountUsageSource interface {
	Snapshot(ctx context.Context) (AccountSnapshot, error)
	CreditsForReview(ctx context.Context, usage TokenUsage) (CreditUsage, error)
}

// UsageReconciliation compares the review credit charge with the change
// between account snapshots. Any difference can be caused by concurrent
// account usage, a reset, or source-side accounting delay.
type UsageReconciliation struct {
	ObservedAccountChange float64
	UnattributedChange    float64
	ConcurrentChange      bool
}

// UsageReport combines per-review token and credit usage with optional account
// snapshots.
type UsageReport struct {
	Review               TokenUsage
	ReviewCredits        CreditUsage
	AccountBefore        AccountSnapshot
	AccountAfter         AccountSnapshot
	ConsumptionPercent   *float64
	ExpectedRemaining    *float64
	Reconciliation       *UsageReconciliation
	PreReviewNonPositive bool
}
