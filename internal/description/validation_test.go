package description

import (
	"strings"
	"testing"
)

func TestValidateRequest(t *testing.T) {
	t.Run("normalizes valid request", func(t *testing.T) {
		request := validDescriptionRequest()
		request.RepositoryRoot = "  /repo/gallery  "
		request.BaseBranch = " main "
		request.Evidence.BaseBranch = "\tmain\n"
		request.Evidence.MergeBaseSHA = " base123 "

		got, err := validateRequest(request)
		if err != nil {
			t.Fatalf("validateRequest() unexpected error: %v", err)
		}
		if got.RepositoryRoot != "/repo/gallery" {
			t.Fatalf("RepositoryRoot = %q, want /repo/gallery", got.RepositoryRoot)
		}
		if got.BaseBranch != "main" {
			t.Fatalf("BaseBranch = %q, want main", got.BaseBranch)
		}
		if got.Evidence.BaseBranch != "main" {
			t.Fatalf("Evidence.BaseBranch = %q, want main", got.Evidence.BaseBranch)
		}
		if got.Evidence.MergeBaseSHA != "base123" {
			t.Fatalf(
				"Evidence.MergeBaseSHA = %q, want base123",
				got.Evidence.MergeBaseSHA,
			)
		}
	})

	tests := []struct {
		name      string
		mutate    func(*Request)
		wantError string
	}{
		{
			name: "requires repository root",
			mutate: func(request *Request) {
				request.RepositoryRoot = " \n\t "
			},
			wantError: "repository root is required",
		},
		{
			name: "requires base branch",
			mutate: func(request *Request) {
				request.BaseBranch = ""
			},
			wantError: "base branch is required",
		},
		{
			name: "requires evidence base",
			mutate: func(request *Request) {
				request.Evidence.BaseBranch = ""
			},
			wantError: "evidence base branch is required",
		},
		{
			name: "rejects base mismatch",
			mutate: func(request *Request) {
				request.Evidence.BaseBranch = "develop"
			},
			wantError: `requested base "main" does not match evidence base "develop"`,
		},
		{
			name: "requires merge base SHA",
			mutate: func(request *Request) {
				request.Evidence.MergeBaseSHA = " "
			},
			wantError: "evidence merge-base SHA is required",
		},
		{
			name: "requires useful evidence",
			mutate: func(request *Request) {
				request.Evidence.CommitLog = ""
				request.Evidence.ChangedFiles = ""
				request.Evidence.Diff = ""
			},
			wantError: "Git evidence contains no changes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validDescriptionRequest()
			test.mutate(&request)

			got, err := validateRequest(request)
			if err == nil {
				t.Fatal("validateRequest() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateRequest() error = %q, want %q", err, test.wantError)
			}
			if got != (Request{}) {
				t.Fatalf("validateRequest() = %+v, want zero request", got)
			}
		})
	}
}

func TestValidateRequestAcceptsEachEvidenceSource(t *testing.T) {
	tests := []struct {
		name         string
		commitLog    string
		changedFiles string
		diff         string
	}{
		{name: "commit log", commitLog: "abc\tChange"},
		{name: "changed files", changedFiles: "M\tfile.go"},
		{name: "diff", diff: "diff --git a/file.go b/file.go"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validDescriptionRequest()
			request.Evidence.CommitLog = test.commitLog
			request.Evidence.ChangedFiles = test.changedFiles
			request.Evidence.Diff = test.diff

			if _, err := validateRequest(request); err != nil {
				t.Fatalf("validateRequest() unexpected error: %v", err)
			}
		})
	}
}

func TestValidateDraft(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Draft)
		wantError string
	}{
		{
			name: "requires title",
			mutate: func(draft *Draft) {
				draft.Title = " "
			},
			wantError: "title is required",
		},
		{
			name: "limits title length",
			mutate: func(draft *Draft) {
				draft.Title = strings.Repeat("a", 73)
			},
			wantError: "title exceeds 72 characters",
		},
		{
			name: "requires summary",
			mutate: func(draft *Draft) {
				draft.Summary = nil
			},
			wantError: "summary must contain",
		},
		{
			name: "limits summary count",
			mutate: func(draft *Draft) {
				draft.Summary = []string{"1", "2", "3", "4", "5"}
			},
			wantError: "summary must contain",
		},
		{
			name: "rejects blank summary",
			mutate: func(draft *Draft) {
				draft.Summary = []string{" "}
			},
			wantError: "summary item 1 is blank",
		},
		{
			name: "requires changes",
			mutate: func(draft *Draft) {
				draft.Changes = nil
			},
			wantError: "at least one change is required",
		},
		{
			name: "validates change ID",
			mutate: func(draft *Draft) {
				draft.Changes[0].ID = "1.1"
			},
			wantError: `invalid ID "1.1"`,
		},
		{
			name: "rejects duplicate change IDs",
			mutate: func(draft *Draft) {
				draft.Changes = append(draft.Changes, draft.Changes[0])
			},
			wantError: "is duplicated",
		},
		{
			name: "requires change file",
			mutate: func(draft *Draft) {
				draft.Changes[0].File = ""
			},
			wantError: "has no file",
		},
		{
			name: "validates operation",
			mutate: func(draft *Draft) {
				draft.Changes[0].Operation = "updated"
			},
			wantError: `invalid operation "updated"`,
		},
		{
			name: "requires affected element",
			mutate: func(draft *Draft) {
				draft.Changes[0].Element = ""
			},
			wantError: "has no affected element",
		},
		{
			name: "requires change summary",
			mutate: func(draft *Draft) {
				draft.Changes[0].Summary = ""
			},
			wantError: "has no summary",
		},
		{
			name: "requires change details",
			mutate: func(draft *Draft) {
				draft.Changes[0].Details = nil
			},
			wantError: "has no details",
		},
		{
			name: "rejects blank change detail",
			mutate: func(draft *Draft) {
				draft.Changes[0].Details = []string{" "}
			},
			wantError: "details item 1 is blank",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			draft := validDescriptionDraft()
			test.mutate(&draft)

			err := validateDraft(draft)
			if err == nil {
				t.Fatal("validateDraft() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("validateDraft() error = %q, want %q", err, test.wantError)
			}
		})
	}
}

func TestValidateGeneratedDraftRequiresDeterministicTestingText(t *testing.T) {
	draft := validDescriptionDraft()
	draft.Testing = []string{"go test ./... passed"}

	err := validateGeneratedDraft(draft)
	if err == nil {
		t.Fatal("validateGeneratedDraft() error = nil, want testing error")
	}
	if !strings.Contains(err.Error(), "testing must contain exactly") {
		t.Fatalf("validateGeneratedDraft() error = %q, want testing context", err)
	}
}
