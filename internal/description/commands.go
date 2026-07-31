package description

import (
	"errors"
	"regexp"
	"strings"
)

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

var fileIDPattern = regexp.MustCompile(`^F[1-9][0-9]*$`)

type EditCommand struct {
	Kind    CommandKind
	Targets []string
	Value   string
	Raw     string
}

// ParseCommands parses one command per nonblank line.
func ParseCommands(input string) ([]EditCommand, error) {
	var commands []EditCommand
	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		command, err := parseCommand(line)
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	}
	if len(commands) == 0 {
		return nil, errors.New("refinement command is required")
	}
	return commands, nil
}

func parseCommand(line string) (EditCommand, error) {
	lower := strings.ToLower(line)
	switch lower {
	case "make description":
		return EditCommand{Kind: CommandMakeDescription, Raw: line}, nil
	case "preview":
		return EditCommand{Kind: CommandPreview, Raw: line}, nil
	case "reset":
		return EditCommand{Kind: CommandReset, Raw: line}, nil
	case "make the summary shorter":
		return EditCommand{Kind: CommandSummaryConcise, Raw: line}, nil
	case "make the summary more detailed":
		return EditCommand{Kind: CommandSummaryDetailed, Raw: line}, nil
	}

	for _, definition := range []struct {
		prefix string
		kind   CommandKind
	}{
		{prefix: "exclude ", kind: CommandExclude},
		{prefix: "include ", kind: CommandInclude},
		{prefix: "combine ", kind: CommandCombine},
		{prefix: "separate ", kind: CommandSeparate},
	} {
		if strings.HasPrefix(lower, definition.prefix) {
			targets := normalizeChangeIDs(
				strings.Fields(line[len(definition.prefix):]),
			)
			if !hasStructuredTargets(definition.kind, targets) {
				return EditCommand{
					Kind:  CommandRewrite,
					Value: line,
					Raw:   line,
				}, nil
			}
			return EditCommand{
				Kind:    definition.kind,
				Targets: targets,
				Raw:     line,
			}, nil
		}
	}

	if prefix, ok := matchingPrefix(lower, []string{
		"focus the title on ",
	}); ok {
		value := strings.TrimSpace(line[len(prefix):])
		if value == "" {
			return EditCommand{}, errors.New("title focus is required")
		}
		return EditCommand{
			Kind:  CommandTitleFocus,
			Value: value,
			Raw:   line,
		}, nil
	}

	for _, testCommand := range []struct {
		prefix string
		suffix string
	}{
		{prefix: "tests passed: ", suffix: " — passed."},
		{prefix: "tests failed: ", suffix: " — failed."},
		{prefix: "tests: ", suffix: " — result not provided."},
	} {
		if strings.HasPrefix(lower, testCommand.prefix) {
			value := strings.TrimSpace(line[len(testCommand.prefix):])
			if strings.EqualFold(value, "not run") {
				value = testsNotRun
			} else if value != "" {
				value += testCommand.suffix
			}
			if value == "" {
				return EditCommand{}, errors.New("test command is required")
			}
			return EditCommand{
				Kind:  CommandTests,
				Value: value,
				Raw:   line,
			}, nil
		}
	}

	return EditCommand{
		Kind:  CommandRewrite,
		Value: line,
		Raw:   line,
	}, nil
}

func hasStructuredTargets(kind CommandKind, targets []string) bool {
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		switch kind {
		case CommandExclude, CommandInclude:
			if !changeIDPattern.MatchString(target) &&
				!fileIDPattern.MatchString(target) {
				return false
			}
		case CommandCombine, CommandSeparate:
			if !changeIDPattern.MatchString(target) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func matchingPrefix(value string, prefixes []string) (string, bool) {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return prefix, true
		}
	}
	return "", false
}
