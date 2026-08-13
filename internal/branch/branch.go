// Package branch lists and safely cleans local Git branches.
package branch

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
	"github.com/Merthoshan/PR-maker-CLI/internal/gitcontext"
)

const defaultBase = "main"

var permanentlyProtected = map[string]struct{}{
	"main":    {},
	"master":  {},
	"dev":     {},
	"develop": {},
}

var omissionListPattern = regexp.MustCompile(
	`^[0-9]+(?:(?:[ \t]*,[ \t]*|[ \t]+)[0-9]+)*$`,
)

var errExitRequested = errors.New("branch cleanup exit requested")

// Service coordinates local branch inspection and cleanup.
type Service struct {
	runner      command.Runner
	git         gitcontext.Collector
	input       *bufio.Scanner
	output      io.Writer
	errorOutput io.Writer
	now         func() time.Time
}

// New creates a local branch service.
func New(
	runner command.Runner,
	input io.Reader,
	output io.Writer,
	errorOutput io.Writer,
) (*Service, error) {
	if runner == nil {
		return nil, errors.New("create branch service: runner is required")
	}
	if input == nil || output == nil || errorOutput == nil {
		return nil, errors.New("create branch service: input and outputs are required")
	}
	git, err := gitcontext.NewCollector(runner)
	if err != nil {
		return nil, fmt.Errorf("create branch service: %w", err)
	}
	return &Service{
		runner:      runner,
		git:         git,
		input:       bufio.NewScanner(input),
		output:      output,
		errorOutput: errorOutput,
		now:         time.Now,
	}, nil
}

// Run lists local branches or starts the interactive cleanup workflow.
func (service *Service) Run(ctx context.Context, request Request) error {
	root, err := service.git.Root(ctx, request.WorkingDirectory)
	if err != nil {
		return err
	}
	if request.Cleanup {
		err := service.runCleanup(ctx, root)
		if errors.Is(err, errExitRequested) {
			service.writeCancelled()
			return nil
		}
		return err
	}
	return service.runList(ctx, root)
}

type baseNotFoundError struct {
	base string
}

func (err *baseNotFoundError) Error() string {
	return fmt.Sprintf("local base branch %q was not found", err.base)
}

func (service *Service) inspect(
	ctx context.Context,
	root string,
	base string,
) (snapshot, error) {
	currentResult, err := service.runner.Run(ctx, command.Spec{
		Name: "git",
		Args: []string{"branch", "--show-current"},
		Dir:  root,
	})
	if err != nil {
		return snapshot{}, command.WrapError(
			"find current local branch",
			currentResult,
			err,
		)
	}
	current := strings.TrimSpace(currentResult.Stdout)

	branchesResult, err := service.runner.Run(ctx, command.Spec{
		Name: "git",
		Args: []string{
			"for-each-ref",
			"--sort=-committerdate",
			"--format=%(refname:short)%00%(committerdate:unix)%00%(committerdate:short)",
			"refs/heads",
		},
		Dir: root,
	})
	if err != nil {
		return snapshot{}, command.WrapError(
			"list local branches",
			branchesResult,
			err,
		)
	}
	branches, err := parseBranches(branchesResult.Stdout, current)
	if err != nil {
		return snapshot{}, err
	}
	if len(branches) == 0 {
		return snapshot{}, errors.New("list local branches: repository has no local branches")
	}
	if !containsBranch(branches, base) {
		return snapshot{}, &baseNotFoundError{base: base}
	}

	mergedResult, err := service.runner.Run(ctx, command.Spec{
		Name: "git",
		Args: []string{
			"for-each-ref",
			"--format=%(refname:short)",
			"--merged=refs/heads/" + base,
			"refs/heads",
		},
		Dir: root,
	})
	if err != nil {
		return snapshot{}, command.WrapError(
			fmt.Sprintf("find local branches merged into %s", base),
			mergedResult,
			err,
		)
	}
	merged := lineSet(mergedResult.Stdout)
	for index := range branches {
		_, branches[index].Merged = merged[branches[index].Name]
	}
	return snapshot{branches: branches, current: current, base: base}, nil
}

func parseBranches(value string, current string) ([]localBranch, error) {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	branches := make([]localBranch, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) != 3 || strings.TrimSpace(fields[0]) == "" {
			return nil, fmt.Errorf("parse local branches: unexpected Git output %q", line)
		}
		unixSeconds, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf(
				"parse activity time for local branch %q: %w",
				fields[0],
				err,
			)
		}
		branches = append(branches, localBranch{
			Name:       fields[0],
			CommitTime: time.Unix(unixSeconds, 0),
			CommitDate: fields[2],
			Current:    fields[0] == current,
		})
	}
	return branches, nil
}

func containsBranch(branches []localBranch, name string) bool {
	for _, branch := range branches {
		if branch.Name == name {
			return true
		}
	}
	return false
}

func lineSet(value string) map[string]bool {
	set := make(map[string]bool)
	for _, line := range strings.Split(value, "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			set[name] = true
		}
	}
	return set
}

func (service *Service) runList(ctx context.Context, root string) error {
	state, err := service.inspect(ctx, root, defaultBase)
	if err != nil {
		return fmt.Errorf("list branches: %w", err)
	}
	if state.current == "" {
		fmt.Fprintln(service.output, "Current branch: detached HEAD")
	} else {
		fmt.Fprintf(service.output, "Current branch: %s\n", state.current)
	}
	service.writeMatches("Local branches", state.branches, state, nil)
	return nil
}

// runCleanup collects one cleanup plan and applies, modifies, or exits it.
func (service *Service) runCleanup(ctx context.Context, root string) error {
	dirty, err := service.workingTreeDirty(ctx, root)
	if err != nil {
		return err
	}
	if dirty {
		fmt.Fprintln(
			service.errorOutput,
			"Warning: the working tree contains uncommitted changes.",
		)
		fmt.Fprintln(
			service.errorOutput,
			"Branch cleanup will not modify these files.",
		)
	}

	state, err := service.askForBase(ctx, root)
	if err != nil {
		return err
	}
	pattern, matches, err := service.askForSearch(state)
	if err != nil {
		return err
	}
	omitted, err := service.askForOmissions(matches, state)
	if err != nil {
		return err
	}

	for {
		service.writePreview(matches, state, omitted)
		choice, err := service.readLine("Enter apply, modify, or exit: ")
		if err != nil {
			return err
		}
		switch choice {
		case "apply":
			return service.apply(ctx, root, matches, state, omitted)
		case "exit":
			return errExitRequested
		case "modify":
			selection, err := service.askForModification()
			if err != nil {
				return err
			}
			switch selection {
			case 1:
				pattern, matches, err = service.askForSearch(state)
				if err != nil {
					return err
				}
				omitted = make(map[string]bool)
			case 2:
				service.writeMatches("Matching local branches", matches, state, nil)
				omitted, err = service.askForOmissions(matches, state)
				if err != nil {
					return err
				}
			case 3:
				state, err = service.askForBase(ctx, root)
				if err != nil {
					return err
				}
				matches, err = matchBranches(state.branches, pattern)
				if err != nil {
					return err
				}
			case 4:
			}
		default:
			fmt.Fprintln(service.output, "Enter apply, modify, or exit.")
		}
	}
}

func (service *Service) workingTreeDirty(ctx context.Context, root string) (bool, error) {
	result, err := service.runner.Run(ctx, command.Spec{
		Name: "git",
		Args: []string{"status", "--porcelain", "--untracked-files=normal"},
		Dir:  root,
	})
	if err != nil {
		return false, command.WrapError("inspect working tree", result, err)
	}
	return strings.TrimSpace(result.Stdout) != "", nil
}

// askForBase prompts until the user selects an existing local base or exits.
func (service *Service) askForBase(
	ctx context.Context,
	root string,
) (snapshot, error) {
	for {
		value, err := service.readLine("Base branch [main]: ")
		if err != nil {
			return snapshot{}, err
		}
		if value == "exit" {
			return snapshot{}, errExitRequested
		}
		if value == "" {
			value = defaultBase
		}
		state, err := service.inspect(ctx, root, value)
		if err == nil {
			return state, nil
		}
		var missingBase *baseNotFoundError
		if errors.As(err, &missingBase) {
			fmt.Fprintf(service.output, "%v. Enter another base or exit.\n", err)
			continue
		}
		return snapshot{}, err
	}
}

// askForSearch prompts until a branch pattern has at least one local match or
// the user exits.
func (service *Service) askForSearch(
	state snapshot,
) (string, []localBranch, error) {
	for {
		pattern, err := service.readLine(
			"Branch search (examples: fix, xyz, *-dev, feature/*): ",
		)
		if err != nil {
			return "", nil, err
		}
		if pattern == "exit" {
			return "", nil, errExitRequested
		}
		if pattern == "" {
			fmt.Fprintln(service.output, "Branch search cannot be empty.")
			continue
		}
		matches, err := matchBranches(state.branches, pattern)
		if err != nil {
			fmt.Fprintf(service.output, "Invalid branch search: %v\n", err)
			continue
		}
		if len(matches) == 0 {
			fmt.Fprintf(
				service.output,
				"No local branches match %q. Enter another search or exit.\n",
				pattern,
			)
			continue
		}
		service.writeMatches("Matching local branches", matches, state, nil)
		return pattern, matches, nil
	}
}

func matchBranches(branches []localBranch, pattern string) ([]localBranch, error) {
	if pattern == "" {
		return nil, errors.New("pattern cannot be empty")
	}
	var matches []localBranch
	if !strings.Contains(pattern, "*") {
		for _, branch := range branches {
			if strings.Contains(branch.Name, pattern) {
				matches = append(matches, branch)
			}
		}
		return matches, nil
	}

	parts := strings.Split(pattern, "*")
	var expression strings.Builder
	expression.WriteString("^")
	for index, part := range parts {
		if index > 0 {
			expression.WriteString(".*")
		}
		expression.WriteString(regexp.QuoteMeta(part))
	}
	expression.WriteString("$")
	compiled, err := regexp.Compile(expression.String())
	if err != nil {
		return nil, fmt.Errorf("compile glob: %w", err)
	}
	for _, branch := range branches {
		if compiled.MatchString(branch.Name) {
			matches = append(matches, branch)
		}
	}
	return matches, nil
}

// askForOmissions parses the eligible branch indexes the user wants to keep.
func (service *Service) askForOmissions(
	matches []localBranch,
	state snapshot,
) (map[string]bool, error) {
	for {
		value, err := service.readLine(
			"Branches to omit (for example: 2,3,5; type none to omit nothing): ",
		)
		if err != nil {
			return nil, err
		}
		if value == "exit" {
			return nil, errExitRequested
		}
		indexes, err := parseOmissions(value, len(matches))
		if err != nil {
			fmt.Fprintf(service.output, "Invalid omissions: %v\n", err)
			continue
		}
		omitted := make(map[string]bool, len(indexes))
		valid := true
		for _, index := range indexes {
			branch := matches[index-1]
			disposition, reason := classify(branch, state, nil)
			if disposition != "DELETE" {
				fmt.Fprintf(
					service.output,
					"Branch %d (%s) is already safe: %s.\n",
					index,
					branch.Name,
					reason,
				)
				valid = false
				continue
			}
			omitted[branch.Name] = true
		}
		if valid {
			return omitted, nil
		}
	}
}

func parseOmissions(value string, maximum int) ([]int, error) {
	if value == "none" {
		return nil, nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("enter branch numbers, none, or exit")
	}
	if !omissionListPattern.MatchString(value) {
		return nil, errors.New("use comma- or space-separated branch numbers")
	}
	parts := strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == ' ' || character == '\t'
	})
	if len(parts) == 0 {
		return nil, errors.New("enter branch numbers, none, or exit")
	}
	seen := make(map[int]bool)
	indexes := make([]int, 0, len(parts))
	for _, part := range parts {
		index, err := strconv.Atoi(part)
		if err != nil || index < 1 || index > maximum {
			return nil, fmt.Errorf("%q is not a branch number from 1 to %d", part, maximum)
		}
		if !seen[index] {
			seen[index] = true
			indexes = append(indexes, index)
		}
	}
	sort.Ints(indexes)
	return indexes, nil
}

// askForModification returns the cleanup-plan section the user wants to edit.
func (service *Service) askForModification() (int, error) {
	fmt.Fprintln(service.output, "What would you like to modify?")
	fmt.Fprintln(service.output, "  1. Search pattern")
	fmt.Fprintln(service.output, "  2. Omitted branches")
	fmt.Fprintln(service.output, "  3. Base branch")
	fmt.Fprintln(service.output, "  4. Return to preview")
	for {
		value, err := service.readLine("Selection: ")
		if err != nil {
			return 0, err
		}
		if value == "exit" {
			return 0, errExitRequested
		}
		selection, err := strconv.Atoi(value)
		if err == nil && selection >= 1 && selection <= 4 {
			return selection, nil
		}
		fmt.Fprintln(service.output, "Enter 1, 2, 3, 4, or exit.")
	}
}

func (service *Service) writeMatches(
	label string,
	branches []localBranch,
	state snapshot,
	omitted map[string]bool,
) {
	fmt.Fprintf(service.output, "\n%s:\n", label)
	for index, branch := range branches {
		status := ""
		disposition, _ := classify(branch, state, omitted)
		if branch.Name == state.current || branch.Current {
			status = "current"
		} else if disposition == "DELETE" {
			status = "merged"
		} else if disposition == "KEEP" {
			status = "unmerged"
		} else {
			status = strings.ToLower(disposition)
		}
		fmt.Fprintf(
			service.output,
			"  %d. %-30s %-10s %s (%s)\n",
			index+1,
			branch.Name,
			status,
			relativeTime(service.now(), branch.CommitTime),
			branch.CommitDate,
		)
	}
	fmt.Fprintln(service.output)
}

func (service *Service) writePreview(
	branches []localBranch,
	state snapshot,
	omitted map[string]bool,
) {
	fmt.Fprintln(service.output, "\nCleanup preview:")
	selected := 0
	for _, branch := range branches {
		disposition, reason := classify(branch, state, omitted)
		if disposition == "DELETE" {
			selected++
		}
		fmt.Fprintf(
			service.output,
			"  %-9s %-30s %s (%s) — %s\n",
			disposition,
			branch.Name,
			relativeTime(service.now(), branch.CommitTime),
			branch.CommitDate,
			reason,
		)
	}
	fmt.Fprintf(service.output, "\nLocal branches selected: %d\n", selected)
	fmt.Fprintln(service.output, "Remote branches affected: none")
	fmt.Fprintln(service.output)
}

func classify(
	branch localBranch,
	state snapshot,
	omitted map[string]bool,
) (string, string) {
	if branch.Name == state.current || branch.Current {
		return "PROTECTED", "current branch"
	}
	if branch.Name == state.base {
		return "PROTECTED", "selected base branch"
	}
	if _, protected := permanentlyProtected[branch.Name]; protected {
		return "PROTECTED", "protected branch"
	}
	if !branch.Merged {
		return "KEEP", "not merged into " + state.base
	}
	if omitted[branch.Name] {
		return "OMIT", "omitted by user"
	}
	return "DELETE", "merged into " + state.base
}

func (service *Service) apply(
	ctx context.Context,
	root string,
	matches []localBranch,
	state snapshot,
	omitted map[string]bool,
) error {
	selected := make([]string, 0, len(matches))
	for _, branch := range matches {
		if disposition, _ := classify(branch, state, omitted); disposition == "DELETE" {
			selected = append(selected, branch.Name)
		}
	}
	if len(selected) == 0 {
		fmt.Fprintln(service.output, "No eligible local branches were selected.")
		fmt.Fprintln(service.output, "Remote branches affected: none")
		return nil
	}

	refreshed, err := service.inspect(ctx, root, state.base)
	if err != nil {
		return fmt.Errorf("revalidate local branches: %w", err)
	}
	byName := make(map[string]localBranch, len(refreshed.branches))
	for _, branch := range refreshed.branches {
		byName[branch.Name] = branch
	}

	deleted := make([]string, 0, len(selected))
	skipped := make([]skippedBranch, 0)
	for _, name := range selected {
		branch, exists := byName[name]
		if !exists {
			skipped = append(skipped, skippedBranch{name: name, reason: "no longer exists"})
			continue
		}
		if disposition, reason := classify(branch, refreshed, nil); disposition != "DELETE" {
			skipped = append(skipped, skippedBranch{name: name, reason: reason})
			continue
		}
		result, deleteErr := service.runner.Run(ctx, command.Spec{
			Name: "git",
			Args: []string{"branch", "-d", "--", name},
			Dir:  root,
			Env: []string{
				"GIT_CONFIG_COUNT=2",
				"GIT_CONFIG_KEY_0=branch." + name + ".remote",
				"GIT_CONFIG_VALUE_0=.",
				"GIT_CONFIG_KEY_1=branch." + name + ".merge",
				"GIT_CONFIG_VALUE_1=refs/heads/" + state.base,
			},
		})
		if deleteErr != nil {
			skipped = append(skipped, skippedBranch{
				name:   name,
				reason: command.WrapError("Git refused safe deletion", result, deleteErr).Error(),
			})
			continue
		}
		deleted = append(deleted, name)
	}

	if len(deleted) == 0 {
		fmt.Fprintln(service.output, "No local branches were deleted.")
	} else {
		fmt.Fprintf(service.output, "Deleted %d local branches:\n", len(deleted))
		for _, name := range deleted {
			fmt.Fprintf(service.output, "  %s\n", name)
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintf(service.output, "\nSkipped %d branches:\n", len(skipped))
		for _, branch := range skipped {
			fmt.Fprintf(service.output, "  %s — %s\n", branch.name, branch.reason)
		}
	}
	fmt.Fprintln(service.output, "\nRemote branches affected: none")
	return nil
}

func (service *Service) readLine(prompt string) (string, error) {
	fmt.Fprint(service.output, prompt)
	if !service.input.Scan() {
		if err := service.input.Err(); err != nil {
			return "", fmt.Errorf("read branch cleanup input: %w", err)
		}
		return "", io.EOF
	}
	return strings.TrimSpace(service.input.Text()), nil
}

func (service *Service) writeCancelled() {
	fmt.Fprintln(service.output, "Cleanup exited. No branches were deleted.")
}

func relativeTime(now time.Time, activity time.Time) string {
	duration := now.Sub(activity)
	if duration < time.Minute {
		return "just now"
	}
	minutes := int(duration / time.Minute)
	if minutes < 60 {
		return plural(minutes, "minute") + " ago"
	}
	hours := int(duration / time.Hour)
	if hours < 24 {
		return plural(hours, "hour") + " ago"
	}
	days := int(duration / (24 * time.Hour))
	if days < 14 {
		return plural(days, "day") + " ago"
	}
	if days < 60 {
		return plural(days/7, "week") + " ago"
	}
	if days < 730 {
		return plural(days/30, "month") + " ago"
	}
	return plural(days/365, "year") + " ago"
}

func plural(value int, unit string) string {
	if value == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", value, unit)
}
