package description

import "github.com/Merthoshan/PR-maker-CLI/internal/gitcontext"

// Request contains the grounded inputs used to generate a PR description.
type Request struct {
	RepositoryRoot string
	BaseBranch     string
	ExistingTitle  string
	ExistingBody   string
	Evidence       gitcontext.Evidence
}

// Change describes one evidence-backed code change.
type Change struct {
	ID        string `json:"id"`
	File      string `json:"file"`
	Operation string `json:"operation"`
	Element   string `json:"element"`
	Summary   string `json:"summary"`
}

// Draft is the structured output produced before rendering a PR description.
type Draft struct {
	Title   string   `json:"title"`
	Summary []string `json:"summary"`
	Changes []Change `json:"changes"`
	Testing []string `json:"testing"`
}

// SummaryPreference controls how much detail appears in the active summary.
type SummaryPreference string

const (
	SummaryConcise  SummaryPreference = "concise"
	SummaryDetailed SummaryPreference = "detailed"
)

// OutputMode controls whether the preview shows the editable changelog or the
// publishable PR description.
type OutputMode string

const (
	OutputModeChangelog   OutputMode = "changelog"
	OutputModeDescription OutputMode = "description"
)

// CombinedGroup records changes that should be described as one logical unit.
type CombinedGroup struct {
	ChangeIDs []string `json:"change_ids"`
}

// ApplyResult describes what the caller should do after applying commands.
type ApplyResult struct {
	NeedsRewrite bool
	Instruction  string
	rebuild      bool
}

// RefinementRequest contains the current state and evidence for one wording
// refinement.
type RefinementRequest struct {
	RepositoryRoot string
	Instruction    string
	State          RefinementState
	Evidence       gitcontext.Evidence
}

type refinementPayload struct {
	CurrentDraft      Draft               `json:"current_draft"`
	AvailableChanges  []Change            `json:"available_changes"`
	ExcludedChangeIDs []string            `json:"excluded_change_ids,omitempty"`
	CombinedGroups    []CombinedGroup     `json:"combined_groups,omitempty"`
	TitleFocus        string              `json:"title_focus,omitempty"`
	SummaryPreference SummaryPreference   `json:"summary_preference"`
	Evidence          gitcontext.Evidence `json:"git_evidence"`
}

// CommandKind identifies one parsed refinement command.
type CommandKind string

const (
	CommandMakeDescription CommandKind = "make_description"
	CommandExclude         CommandKind = "exclude"
	CommandInclude         CommandKind = "include"
	CommandCombine         CommandKind = "combine"
	CommandSeparate        CommandKind = "separate"
	CommandSummaryConcise  CommandKind = "summary_concise"
	CommandSummaryDetailed CommandKind = "summary_detailed"
	CommandTitleFocus      CommandKind = "title_focus"
	CommandTests           CommandKind = "tests"
	CommandReset           CommandKind = "reset"
	CommandPreview         CommandKind = "preview"
	CommandRewrite         CommandKind = "rewrite"
)

// EditCommand is one parsed refinement instruction line.
type EditCommand struct {
	Kind    CommandKind
	Targets []string
	Value   string
	Raw     string
}

// TitleSuggestionRequest contains the context and hard length constraint for
// one set of shorter title options.
type TitleSuggestionRequest struct {
	RepositoryRoot string
	Instruction    string
	CurrentDraft   Draft
	PreviousTitles []string
	MaxTitleLength int
	Evidence       gitcontext.Evidence
}

type titleSuggestionPayload struct {
	CurrentDraft   Draft               `json:"current_draft"`
	PreviousTitles []string            `json:"previous_titles,omitempty"`
	MaxTitleLength int                 `json:"max_title_length"`
	Evidence       gitcontext.Evidence `json:"git_evidence"`
}

type titleSuggestionResponse struct {
	Titles []string `json:"titles"`
}

type generationPayload struct {
	BaseBranch    string `json:"base_branch"`
	BaseRef       string `json:"base_ref,omitempty"`
	MergeBaseSHA  string `json:"merge_base_sha"`
	CommitLog     string `json:"commit_log,omitempty"`
	ChangedFiles  string `json:"changed_files,omitempty"`
	Diff          string `json:"diff,omitempty"`
	ExistingTitle string `json:"existing_title,omitempty"`
	ExistingBody  string `json:"existing_body,omitempty"`
}
