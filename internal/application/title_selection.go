package application

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Merthoshan/PR-maker-CLI/internal/description"
	"github.com/Merthoshan/PR-maker-CLI/internal/gitcontext"
)

const initialTitleSuggestionInstruction = `Create three shorter alternatives
that preserve the primary change described by the current title.`

func (app *App) resolveTitle(
	ctx context.Context,
	repositoryRoot string,
	branch string,
	service string,
	draft description.Draft,
	evidence gitcontext.Evidence,
	scanner *bufio.Scanner,
) (string, error) {
	draft.Title = titleWithoutMetadata(draft.Title)
	fullTitle := titleWithMetadata(draft.Title, branch, service)
	if err := validateTitle(fullTitle); err == nil {
		return fullTitle, nil
	}

	maxBodyLength := availableTitleLength(branch, service)
	if maxBodyLength < 1 {
		return "", errors.New("PR title metadata leaves no room for a title")
	}

	fmt.Fprintf(
		app.output,
		"\nGenerated PR title is %d characters; maximum is %d.\n",
		utf8.RuneCountInString(fullTitle),
		maxPRTitleLength,
	)

	instruction := initialTitleSuggestionInstruction
	var previousTitles []string
	for {
		stopProgress := app.progress.Start(
			"Generating shorter PR title options with Codex",
		)
		titles, err := app.dependencies.Drafts.SuggestTitles(
			ctx,
			description.TitleSuggestionRequest{
				RepositoryRoot: repositoryRoot,
				Instruction:    instruction,
				CurrentDraft:   draft,
				PreviousTitles: previousTitles,
				MaxTitleLength: maxBodyLength,
				Evidence:       evidence,
			},
		)
		stopProgress()
		if err != nil {
			return "", err
		}

		fullTitles := make([]string, len(titles))
		for index, title := range titles {
			fullTitles[index] = titleWithMetadata(title, branch, service)
			if err := validateTitle(fullTitles[index]); err != nil {
				return "", fmt.Errorf(
					"generated title option %d: %w",
					index+1,
					err,
				)
			}
		}

		fmt.Fprintln(app.output, "\nChoose a shorter PR title:")
		for index, title := range fullTitles {
			fmt.Fprintf(app.output, "%d. %s\n", index+1, title)
		}
		fmt.Fprintln(
			app.output,
			"\nEnter 1, 2, 3, or a title instruction",
		)
		fmt.Fprintln(
			app.output,
			"(example: combine 1 and 3, or omit validation):",
		)

		for {
			fmt.Fprint(app.output, "Choice or instruction: ")
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return "", fmt.Errorf("read title choice: %w", err)
				}
				return "", ErrCancelled
			}

			input := strings.TrimSpace(scanner.Text())
			if input == "quit" {
				return "", ErrCancelled
			}
			if choice, err := strconv.Atoi(input); err == nil {
				if choice >= 1 && choice <= len(fullTitles) {
					return fullTitles[choice-1], nil
				}
				fmt.Fprintln(
					app.output,
					"\nEnter 1, 2, 3, or a title instruction.",
				)
				continue
			}
			if input == "" {
				fmt.Fprintln(
					app.output,
					"\nEnter 1, 2, 3, or a title instruction.",
				)
				continue
			}

			instruction = input
			previousTitles = append([]string(nil), titles...)
			break
		}
	}
}
