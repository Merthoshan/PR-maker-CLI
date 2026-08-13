package description

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// RefinementState preserves the original generated draft and every user edit.
type RefinementState struct {
	Original          Draft
	Current           Draft
	Mode              OutputMode
	ExcludedChangeIDs map[string]bool
	CombinedGroups    []CombinedGroup
	TitleFocus        string
	SummaryPreference SummaryPreference
}

// NewRefinementState creates editable state from an initial generated draft.
func NewRefinementState(draft Draft) (RefinementState, error) {
	if err := validateDraft(draft); err != nil {
		return RefinementState{}, fmt.Errorf(
			"create PR-description refinement state: %w",
			err,
		)
	}

	return RefinementState{
		Original:          cloneDraft(draft),
		Current:           cloneDraft(draft),
		Mode:              OutputModeChangelog,
		ExcludedChangeIDs: make(map[string]bool),
		SummaryPreference: SummaryConcise,
	}, nil
}

// Clone returns an independent copy suitable for transactional edits.
func (state RefinementState) Clone() RefinementState {
	return cloneRefinementState(state)
}

// Apply parses and transactionally applies one or more editing commands.
func (state *RefinementState) Apply(input string) (ApplyResult, error) {
	commands, err := ParseCommands(input)
	if err != nil {
		return ApplyResult{}, err
	}

	next := cloneRefinementState(*state)
	result := ApplyResult{Instruction: strings.TrimSpace(input)}
	for _, command := range commands {
		commandResult, err := next.applyCommand(command)
		if err != nil {
			return ApplyResult{}, err
		}
		result.NeedsRewrite = result.NeedsRewrite ||
			commandResult.NeedsRewrite
		result.rebuild = result.rebuild || commandResult.rebuild
	}

	if result.rebuild {
		if err := next.rebuildCurrent(); err != nil {
			return ApplyResult{}, err
		}
	}
	*state = next
	return result, nil
}

// ReplaceCurrent accepts wording changes and an ordered subset of the original
// evidence-backed changes.
func (state *RefinementState) ReplaceCurrent(draft Draft) error {
	if err := validateDraft(draft); err != nil {
		return fmt.Errorf("replace refined draft: %w", err)
	}

	originalIndexes := make(map[string]int, len(state.Original.Changes))
	for index, change := range state.Original.Changes {
		originalIndexes[change.ID] = index
	}

	activeChangeIDs := make(map[string]bool, len(draft.Changes))
	previousIndex := -1
	for _, replacement := range draft.Changes {
		originalIndex, ok := originalIndexes[replacement.ID]
		if !ok {
			return fmt.Errorf(
				"replace refined draft: unknown change ID %q",
				replacement.ID,
			)
		}
		original := state.Original.Changes[originalIndex]
		if replacement.File != original.File ||
			replacement.Operation != original.Operation ||
			replacement.Element != original.Element {
			return fmt.Errorf(
				"replace refined draft: structure changed for %q",
				replacement.ID,
			)
		}
		if originalIndex <= previousIndex {
			return errors.New(
				"replace refined draft: changes are not in their original order",
			)
		}
		activeChangeIDs[replacement.ID] = true
		previousIndex = originalIndex
	}

	for _, original := range state.Original.Changes {
		if activeChangeIDs[original.ID] {
			delete(state.ExcludedChangeIDs, original.ID)
		} else {
			state.ExcludedChangeIDs[original.ID] = true
		}
	}

	state.Current = cloneDraft(draft)
	state.pruneCombinedGroups(state.Current.Changes)
	return nil
}

func (state *RefinementState) applyCommand(
	command EditCommand,
) (ApplyResult, error) {
	switch command.Kind {
	case CommandMakeDescription:
		state.Mode = OutputModeDescription
		return ApplyResult{}, nil

	case CommandExclude:
		changeIDs, err := state.resolveTargets(command.Targets)
		if err != nil {
			return ApplyResult{}, err
		}
		for _, changeID := range changeIDs {
			state.ExcludedChangeIDs[changeID] = true
		}
		return ApplyResult{NeedsRewrite: true, rebuild: true}, nil

	case CommandInclude:
		changeIDs, err := state.resolveTargets(command.Targets)
		if err != nil {
			return ApplyResult{}, err
		}
		for _, changeID := range changeIDs {
			delete(state.ExcludedChangeIDs, changeID)
		}
		return ApplyResult{NeedsRewrite: true, rebuild: true}, nil

	case CommandCombine:
		if err := state.combine(command.Targets); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{NeedsRewrite: true, rebuild: true}, nil

	case CommandSeparate:
		if err := state.separate(command.Targets); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{NeedsRewrite: true, rebuild: true}, nil

	case CommandSummaryConcise:
		state.SummaryPreference = SummaryConcise
		return ApplyResult{NeedsRewrite: true}, nil

	case CommandSummaryDetailed:
		state.SummaryPreference = SummaryDetailed
		return ApplyResult{NeedsRewrite: true}, nil

	case CommandTitleFocus:
		if _, err := state.findFocus(command.Value); err != nil {
			return ApplyResult{}, err
		}
		state.TitleFocus = command.Value
		return ApplyResult{NeedsRewrite: true}, nil

	case CommandTests:
		state.Current.Testing = []string{command.Value}
		return ApplyResult{}, nil

	case CommandReset:
		reset, err := NewRefinementState(state.Original)
		if err != nil {
			return ApplyResult{}, err
		}
		*state = reset
		return ApplyResult{}, nil

	case CommandPreview:
		return ApplyResult{}, nil

	case CommandRewrite:
		return ApplyResult{NeedsRewrite: true}, nil

	default:
		return ApplyResult{}, fmt.Errorf(
			"apply refinement command: unsupported command %q",
			command.Raw,
		)
	}
}

func (state *RefinementState) rebuildCurrent() error {
	testing := slices.Clone(state.Current.Testing)
	current := cloneDraft(state.Current)
	current.Testing = testing
	current.Changes = current.Changes[:0]
	activeChanges := make(map[string]Change, len(state.Current.Changes))
	for _, change := range state.Current.Changes {
		activeChanges[change.ID] = change
	}
	for _, change := range state.Original.Changes {
		if state.ExcludedChangeIDs[change.ID] {
			continue
		}
		if active, ok := activeChanges[change.ID]; ok {
			current.Changes = append(current.Changes, cloneChange(active))
		} else {
			current.Changes = append(current.Changes, cloneChange(change))
		}
	}
	if len(current.Changes) == 0 {
		return errors.New("apply refinement command: all changes are excluded")
	}

	state.pruneCombinedGroups(current.Changes)
	state.Current = current
	return nil
}

func (state *RefinementState) resolveTargets(
	targets []string,
) ([]string, error) {
	var resolved []string
	for _, target := range targets {
		target = normalizeChangeID(target)
		if changeIDPattern.MatchString(target) {
			if !state.hasChange(target) {
				return nil, fmt.Errorf("unknown change ID %q", target)
			}
			resolved = append(resolved, target)
			continue
		}
		if !fileIDPattern.MatchString(target) {
			return nil, fmt.Errorf("invalid change or file ID %q", target)
		}

		found := false
		prefix := target + "."
		for _, change := range state.Original.Changes {
			if strings.HasPrefix(change.ID, prefix) {
				resolved = append(resolved, change.ID)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown file ID %q", target)
		}
	}
	return uniqueStrings(resolved), nil
}

func (state *RefinementState) combine(changeIDs []string) error {
	changeIDs = normalizeChangeIDs(changeIDs)
	if len(changeIDs) < 2 {
		return errors.New("combine requires at least two change IDs")
	}
	for _, changeID := range changeIDs {
		if !changeIDPattern.MatchString(changeID) || !state.hasChange(changeID) {
			return fmt.Errorf("unknown change ID %q", changeID)
		}
	}

	state.removeFromGroups(changeIDs)
	state.CombinedGroups = append(state.CombinedGroups, CombinedGroup{
		ChangeIDs: uniqueStrings(changeIDs),
	})
	return nil
}

func (state *RefinementState) separate(changeIDs []string) error {
	changeIDs = normalizeChangeIDs(changeIDs)
	if len(changeIDs) == 0 {
		return errors.New("separate requires at least one change ID")
	}
	for _, changeID := range changeIDs {
		if !changeIDPattern.MatchString(changeID) || !state.hasChange(changeID) {
			return fmt.Errorf("unknown change ID %q", changeID)
		}
	}
	state.removeFromGroups(changeIDs)
	return nil
}

func (state *RefinementState) removeFromGroups(changeIDs []string) {
	remove := make(map[string]bool, len(changeIDs))
	for _, changeID := range changeIDs {
		remove[changeID] = true
	}

	groups := state.CombinedGroups[:0]
	for _, group := range state.CombinedGroups {
		remaining := group.ChangeIDs[:0]
		for _, changeID := range group.ChangeIDs {
			if !remove[changeID] {
				remaining = append(remaining, changeID)
			}
		}
		if len(remaining) >= 2 {
			group.ChangeIDs = remaining
			groups = append(groups, group)
		}
	}
	state.CombinedGroups = groups
}

func (state *RefinementState) pruneCombinedGroups(changes []Change) {
	active := make(map[string]bool, len(changes))
	for _, change := range changes {
		active[change.ID] = true
	}
	groups := state.CombinedGroups[:0]
	for _, group := range state.CombinedGroups {
		remaining := make([]string, 0, len(group.ChangeIDs))
		for _, changeID := range group.ChangeIDs {
			if active[changeID] {
				remaining = append(remaining, changeID)
			}
		}
		if len(remaining) >= 2 {
			groups = append(groups, CombinedGroup{ChangeIDs: remaining})
		}
	}
	state.CombinedGroups = groups
}

func (state *RefinementState) findFocus(value string) (Change, error) {
	needle := strings.ToLower(strings.TrimSpace(value))
	for _, change := range state.Current.Changes {
		if strings.EqualFold(change.ID, needle) ||
			strings.Contains(strings.ToLower(change.File), needle) ||
			strings.Contains(strings.ToLower(change.Element), needle) ||
			strings.Contains(strings.ToLower(change.Summary), needle) {
			return change, nil
		}
	}
	return Change{}, fmt.Errorf("title focus %q did not match a change", value)
}

func (state *RefinementState) hasChange(changeID string) bool {
	for _, change := range state.Original.Changes {
		if change.ID == changeID {
			return true
		}
	}
	return false
}

func cloneRefinementState(state RefinementState) RefinementState {
	clone := state
	clone.Original = cloneDraft(state.Original)
	clone.Current = cloneDraft(state.Current)
	clone.ExcludedChangeIDs = make(map[string]bool, len(state.ExcludedChangeIDs))
	for changeID, excluded := range state.ExcludedChangeIDs {
		clone.ExcludedChangeIDs[changeID] = excluded
	}
	clone.CombinedGroups = make([]CombinedGroup, len(state.CombinedGroups))
	for index, group := range state.CombinedGroups {
		clone.CombinedGroups[index] = CombinedGroup{
			ChangeIDs: slices.Clone(group.ChangeIDs),
		}
	}
	return clone
}

func cloneDraft(draft Draft) Draft {
	clone := draft
	clone.Summary = slices.Clone(draft.Summary)
	clone.Testing = slices.Clone(draft.Testing)
	clone.Changes = make([]Change, len(draft.Changes))
	for index, change := range draft.Changes {
		clone.Changes[index] = cloneChange(change)
	}
	return clone
}

func cloneChange(change Change) Change {
	return change
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func normalizeChangeIDs(changeIDs []string) []string {
	normalized := make([]string, len(changeIDs))
	for index, changeID := range changeIDs {
		normalized[index] = normalizeChangeID(changeID)
	}
	return normalized
}

func normalizeChangeID(changeID string) string {
	return strings.ToUpper(strings.TrimSpace(changeID))
}
