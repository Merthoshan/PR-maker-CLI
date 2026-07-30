package description

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"champu-pr/internal/command"
)

func TestRefinementStateAppliesDeterministicCommands(t *testing.T) {
	draft := refinementDraft()
	state, err := NewRefinementState(draft)
	if err != nil {
		t.Fatalf("NewRefinementState() unexpected error: %v", err)
	}

	result, err := state.Apply("exclude f1.c1\ntests passed: go test ./...")
	if err != nil {
		t.Fatalf("Apply() unexpected error: %v", err)
	}
	if !result.NeedsRewrite {
		t.Fatal("Apply() NeedsRewrite = false, want true")
	}
	if len(state.Current.Changes) != 1 ||
		state.Current.Changes[0].ID != "F1.C2" {
		t.Fatalf("current changes = %+v, want only F1.C2", state.Current.Changes)
	}
	if got := state.Current.Testing[0]; got != "go test ./... — passed." {
		t.Fatalf("testing = %q, want recorded passing test", got)
	}
}

func TestRefinementStateSwitchesToDescriptionMode(t *testing.T) {
	state, err := NewRefinementState(refinementDraft())
	if err != nil {
		t.Fatalf("NewRefinementState() unexpected error: %v", err)
	}
	if state.Mode != OutputModeChangelog {
		t.Fatalf("initial mode = %q, want changelog", state.Mode)
	}

	result, err := state.Apply("make description")
	if err != nil {
		t.Fatalf("Apply() unexpected error: %v", err)
	}
	if result.NeedsRewrite {
		t.Fatal("Apply() NeedsRewrite = true, want deterministic mode switch")
	}
	if state.Mode != OutputModeDescription {
		t.Fatalf("mode = %q, want description", state.Mode)
	}
}

func TestRefinementStateRollsBackInvalidBatch(t *testing.T) {
	state, err := NewRefinementState(refinementDraft())
	if err != nil {
		t.Fatalf("NewRefinementState() unexpected error: %v", err)
	}

	_, err = state.Apply("exclude F1.C1\ncombine F1.C2 F9.C1")
	if err == nil {
		t.Fatal("Apply() error = nil, want unknown ID error")
	}
	if len(state.Current.Changes) != 2 {
		t.Fatalf("current changes = %d, want rollback to 2", len(state.Current.Changes))
	}
	if len(state.ExcludedChangeIDs) != 0 {
		t.Fatalf("excluded IDs = %v, want rollback", state.ExcludedChangeIDs)
	}
}

func TestRefinementStatePreservesRewrittenText(t *testing.T) {
	state, err := NewRefinementState(refinementDraft())
	if err != nil {
		t.Fatalf("NewRefinementState() unexpected error: %v", err)
	}
	rewritten := cloneDraft(state.Current)
	rewritten.Title = "Improve the refined title"
	rewritten.Summary = []string{"A rewritten summary."}
	if err := state.ReplaceCurrent(rewritten); err != nil {
		t.Fatalf("ReplaceCurrent() unexpected error: %v", err)
	}

	if _, err := state.Apply("tests: not run"); err != nil {
		t.Fatalf("Apply() unexpected error: %v", err)
	}
	if state.Current.Title != rewritten.Title {
		t.Fatalf("title = %q, want %q", state.Current.Title, rewritten.Title)
	}
}

func TestRefinementStateResetAndStructuralValidation(t *testing.T) {
	state, err := NewRefinementState(refinementDraft())
	if err != nil {
		t.Fatalf("NewRefinementState() unexpected error: %v", err)
	}
	if _, err := state.Apply("exclude F1.C1"); err != nil {
		t.Fatalf("exclude unexpected error: %v", err)
	}
	if _, err := state.Apply("make description"); err != nil {
		t.Fatalf("make description unexpected error: %v", err)
	}
	if _, err := state.Apply("reset"); err != nil {
		t.Fatalf("reset unexpected error: %v", err)
	}
	if len(state.Current.Changes) != 2 || len(state.ExcludedChangeIDs) != 0 {
		t.Fatalf("reset state = %+v, want original state", state)
	}
	if state.Mode != OutputModeChangelog {
		t.Fatalf("reset mode = %q, want changelog", state.Mode)
	}

	changed := cloneDraft(state.Current)
	changed.Changes[0].File = "other.go"
	if err := state.ReplaceCurrent(changed); err == nil {
		t.Fatal("ReplaceCurrent() error = nil, want structural drift error")
	}
}

func TestRefinementStatePrunesInvalidCombinedGroups(t *testing.T) {
	state, err := NewRefinementState(refinementDraft())
	if err != nil {
		t.Fatalf("NewRefinementState() unexpected error: %v", err)
	}
	if _, err := state.Apply("combine F1.C1 F1.C2"); err != nil {
		t.Fatalf("combine unexpected error: %v", err)
	}
	if len(state.CombinedGroups) != 1 {
		t.Fatalf("combined groups = %+v, want one group", state.CombinedGroups)
	}
	if _, err := state.Apply("exclude F1.C1"); err != nil {
		t.Fatalf("exclude unexpected error: %v", err)
	}
	if len(state.CombinedGroups) != 0 {
		t.Fatalf(
			"combined groups = %+v, want invalid group pruned",
			state.CombinedGroups,
		)
	}
}

func TestGeneratorRefineSeparatesInstructionFromEvidence(t *testing.T) {
	state, err := NewRefinementState(refinementDraft())
	if err != nil {
		t.Fatalf("NewRefinementState() unexpected error: %v", err)
	}
	response, _ := json.Marshal(state.Current)
	runner := &generatorRunner{
		t: t,
		run: func(spec command.Spec) (command.Result, error) {
			if !strings.Contains(spec.Args[len(spec.Args)-1], "Make it concise") {
				t.Fatal("trusted instruction is missing from prompt")
			}
			if strings.Contains(spec.Args[len(spec.Args)-1], "malicious diff") {
				t.Fatal("untrusted evidence was embedded in prompt")
			}
			if !strings.Contains(spec.Stdin, "malicious diff") {
				t.Fatal("evidence is missing from stdin")
			}
			return command.Result{Stdout: string(response)}, nil
		},
	}
	generator := mustNewDescriptionGenerator(t, runner)
	evidence := validDescriptionRequest().Evidence
	evidence.Diff = "malicious diff"

	draft, err := generator.Refine(context.Background(), RefinementRequest{
		RepositoryRoot: "/repo/gallery",
		Instruction:    "Make it concise",
		State:          state,
		Evidence:       evidence,
	})
	if err != nil {
		t.Fatalf("Refine() unexpected error: %v", err)
	}
	if len(draft.Changes) != 2 {
		t.Fatalf("changes = %d, want 2", len(draft.Changes))
	}
}

func refinementDraft() Draft {
	draft := validDescriptionDraft()
	draft.Changes = append(draft.Changes, Change{
		ID:        "F1.C2",
		File:      "internal/workflow/target.go",
		Operation: "added",
		Element:   "Target",
		Summary:   "Represent the selected target.",
	})
	return draft
}
