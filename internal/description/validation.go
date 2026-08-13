package description

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Merthoshan/PR-maker-CLI/internal/structuredoutput"
)

const testsNotRun = "Not run (no test results provided)."

var changeIDPattern = regexp.MustCompile(`^F[1-9][0-9]*\.C[1-9][0-9]*$`)

var allowedOperations = map[string]bool{
	"added":     true,
	"removed":   true,
	"modified":  true,
	"renamed":   true,
	"moved":     true,
	"generated": true,
}

func validateRequest(request Request) (Request, error) {
	request.RepositoryRoot = strings.TrimSpace(request.RepositoryRoot)
	if request.RepositoryRoot == "" {
		return Request{}, errors.New("repository root is required")
	}

	request.BaseBranch = strings.TrimSpace(request.BaseBranch)
	if request.BaseBranch == "" {
		return Request{}, errors.New("base branch is required")
	}

	request.Evidence.BaseBranch = strings.TrimSpace(request.Evidence.BaseBranch)
	if request.Evidence.BaseBranch == "" {
		return Request{}, errors.New("evidence base branch is required")
	}
	if request.BaseBranch != request.Evidence.BaseBranch {
		return Request{}, fmt.Errorf(
			"requested base %q does not match evidence base %q",
			request.BaseBranch,
			request.Evidence.BaseBranch,
		)
	}

	request.Evidence.MergeBaseSHA = strings.TrimSpace(
		request.Evidence.MergeBaseSHA,
	)
	if request.Evidence.MergeBaseSHA == "" {
		return Request{}, errors.New("evidence merge-base SHA is required")
	}

	if strings.TrimSpace(request.Evidence.CommitLog) == "" &&
		strings.TrimSpace(request.Evidence.ChangedFiles) == "" &&
		strings.TrimSpace(request.Evidence.Diff) == "" {
		return Request{}, errors.New("Git evidence contains no changes")
	}

	return request, nil
}

// decodeDraft extracts one strict PR-draft response.
func decodeDraft(output string) (Draft, error) {
	return structuredoutput.DecodeSingleJSON[Draft](output)
}

func validateDraft(draft Draft) error {
	title := strings.TrimSpace(draft.Title)
	if title == "" {
		return errors.New("title is required")
	}
	if utf8.RuneCountInString(title) > 72 {
		return errors.New("title exceeds 72 characters")
	}

	if len(draft.Summary) == 0 || len(draft.Summary) > 4 {
		return errors.New("summary must contain between one and four items")
	}
	if err := validateNonBlankItems("summary", draft.Summary); err != nil {
		return err
	}

	if len(draft.Changes) == 0 {
		return errors.New("at least one change is required")
	}
	if err := validateChanges(draft.Changes); err != nil {
		return err
	}

	if len(draft.Testing) == 0 {
		return errors.New("at least one testing item is required")
	}
	if err := validateNonBlankItems("testing", draft.Testing); err != nil {
		return err
	}

	return nil
}

func validateGeneratedDraft(draft Draft) error {
	if err := validateDraft(draft); err != nil {
		return err
	}
	if len(draft.Testing) != 1 || draft.Testing[0] != testsNotRun {
		return fmt.Errorf("testing must contain exactly %q", testsNotRun)
	}
	return nil
}

func validateChanges(changes []Change) error {
	seenIDs := make(map[string]bool, len(changes))
	for index, change := range changes {
		if !changeIDPattern.MatchString(change.ID) {
			return fmt.Errorf("change %d has invalid ID %q", index+1, change.ID)
		}
		if seenIDs[change.ID] {
			return fmt.Errorf("change ID %q is duplicated", change.ID)
		}
		seenIDs[change.ID] = true

		if strings.TrimSpace(change.File) == "" {
			return fmt.Errorf("change %q has no file", change.ID)
		}
		if !allowedOperations[change.Operation] {
			return fmt.Errorf(
				"change %q has invalid operation %q",
				change.ID,
				change.Operation,
			)
		}
		if strings.TrimSpace(change.Element) == "" {
			return fmt.Errorf("change %q has no affected element", change.ID)
		}
		if strings.TrimSpace(change.Summary) == "" {
			return fmt.Errorf("change %q has no summary", change.ID)
		}
	}

	return nil
}

func validateNonBlankItems(name string, items []string) error {
	for index, item := range items {
		if strings.TrimSpace(item) == "" {
			return fmt.Errorf("%s item %d is blank", name, index+1)
		}
	}
	return nil
}
