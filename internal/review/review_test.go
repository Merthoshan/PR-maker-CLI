package review

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
	"github.com/Merthoshan/PR-maker-CLI/internal/github"
)

func TestRunnerRun(t *testing.T) {
	root := t.TempDir()
	subdirectory := filepath.Join(root, "internal", "service")
	if err := os.MkdirAll(subdirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	instructionsPath := filepath.Join(root, ".champu-pr", "review-instructions.md")
	if err := os.MkdirAll(filepath.Dir(instructionsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instructionsPath, []byte("Focus on transaction boundaries."), 0o600); err != nil {
		t.Fatal(err)
	}

	metadata := `{
		"number":123,
		"state":"OPEN",
		"title":"Add transaction handling",
		"url":"https://github.com/acme/service/pull/123",
		"body":"Updates writes.",
		"baseRefName":"main",
		"headRefName":"transactions",
		"isDraft":false,
		"labels":[{"name":"backend"}],
		"files":[{"path":"store.go","additions":2,"deletions":0}]
	}`
	diff := "diff --git a/store.go b/store.go\n--- a/store.go\n+++ b/store.go\n@@ -1 +1,2 @@\n existing\n+beginTransaction()\n"
	codexOutput := `{
		"overview":"One actionable issue was found.",
		"code_quality_and_style":[],
		"specific_suggestions":[],
		"potential_issues_and_risks":[{
			"severity":"high",
			"file":"store.go",
			"line":2,
			"evidence":"The transaction is started without a rollback path.",
			"impact":"A failed write can leave the transaction open.",
			"suggested_fix":"Defer a rollback immediately after beginning the transaction."
		}]
	}`
	var analysisDirectory string
	runner := &reviewCommandRunner{
		t: t,
		runs: []reviewCommandRun{
			{
				assert: exactReviewSpec(t, command.Spec{
					Name: "git",
					Args: []string{"rev-parse", "--show-toplevel"},
					Dir:  subdirectory,
				}),
				result: command.Result{Stdout: root + "\n"},
			},
			{
				assert: exactReviewSpec(t, command.Spec{
					Name: "git",
					Args: []string{"remote", "get-url", "origin"},
					Dir:  root,
				}),
				result: command.Result{Stdout: "git@github.com:acme/service.git\n"},
			},
			{
				assert: exactReviewSpec(t, command.Spec{
					Name: "gh",
					Args: []string{
						"pr", "view", "123",
						"--json",
						"number,state,title,url,body,baseRefName,headRefName,isDraft,labels,files",
					},
					Dir: root,
				}),
				result: command.Result{Stdout: metadata},
			},
			{
				assert: exactReviewSpec(t, command.Spec{
					Name:        "gh",
					Args:        []string{"pr", "diff", "123", "--color=never"},
					Dir:         root,
					StdoutLimit: standardDiffCaptureBytes,
				}),
				result: command.Result{Stdout: diff},
			},
			{
				assert: func(spec command.Spec) {
					if spec.Name != "codex" {
						t.Fatalf("command name = %q, want codex", spec.Name)
					}
					analysisDirectory = spec.Dir
					if spec.Dir == root || !strings.Contains(filepath.Base(spec.Dir), "champu-pr-review-") {
						t.Fatalf("Codex directory = %q, want isolated review directory", spec.Dir)
					}
					for _, required := range []string{"--json", "--ignore-user-config", "--skip-git-repo-check", `model_reasoning_effort="medium"`, "--output-schema", reviewPrompt} {
						if !slices.Contains(spec.Args, required) {
							t.Fatalf("Codex args missing %q: %q", required, spec.Args)
						}
					}
					schemaIndex := slices.Index(spec.Args, "--output-schema")
					if schemaIndex < 0 || schemaIndex+1 >= len(spec.Args) {
						t.Fatalf("Codex args missing schema path: %q", spec.Args)
					}
					if _, err := os.Stat(spec.Args[schemaIndex+1]); err != nil {
						t.Fatalf("schema file is unavailable during Codex call: %v", err)
					}
					var evidence payload
					if strings.Contains(spec.Stdin, `"author"`) || strings.Contains(spec.Stdin, `"reviewers"`) {
						t.Fatalf("identity data was sent to Codex: %s", spec.Stdin)
					}
					if err := json.Unmarshal([]byte(spec.Stdin), &evidence); err != nil {
						t.Fatalf("decode Codex stdin: %v", err)
					}
					if evidence.Repository != "acme/service" || evidence.ReviewInstructions != "Focus on transaction boundaries." {
						t.Fatalf("review payload = %+v", evidence)
					}
					if len(evidence.ChangedFiles) != 1 || evidence.ChangedFiles[0].Path != "store.go" {
						t.Fatalf("changed-file manifest = %+v", evidence.ChangedFiles)
					}
					if evidence.EvidenceOmitted || evidence.Diff == "" {
						t.Fatalf("selected evidence = %+v", evidence)
					}
				},
				result: command.Result{Stdout: codexEventStream(codexOutput)},
			},
		},
	}
	progress := &reviewProgress{}
	accountUsage := &sequenceAccountSource{
		snapshots: []AccountSnapshot{
			{Available: true, RemainingCredits: 100},
			{Available: true, RemainingCredits: 85},
		},
		reviewCredits: CreditUsage{Available: true, Credits: 15},
	}
	service, err := NewWithAccountUsageSource(runner, progress, accountUsage)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	outcome, err := service.Run(context.Background(), Request{
		WorkingDirectory: subdirectory,
		Target:           "123",
		InstructionsPath: ".champu-pr/review-instructions.md",
	})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if outcome.PullRequest.Number != 123 || outcome.Depth != Standard || outcome.Omitted {
		t.Fatalf("Run() outcome = %+v", outcome)
	}
	if !outcome.Usage.Review.Available || outcome.Usage.Review.TotalTokens() != 1500 {
		t.Fatalf("Run() usage = %+v", outcome.Usage)
	}
	if outcome.Usage.ConsumptionPercent == nil || *outcome.Usage.ConsumptionPercent != 15 ||
		outcome.Usage.Reconciliation == nil || outcome.Usage.Reconciliation.ConcurrentChange ||
		accountUsage.calls != 2 || accountUsage.creditCalls != 1 {
		t.Fatalf(
			"Run() account usage = %+v, snapshot calls = %d, credit calls = %d",
			outcome.Usage,
			accountUsage.calls,
			accountUsage.creditCalls,
		)
	}
	for _, expected := range []string{"# Overview", "# Code Quality and Style", "# Specific Suggestions", "# Potential Issues and Risks", "**HIGH**", "store.go:2"} {
		if !strings.Contains(outcome.Review, expected) {
			t.Fatalf("rendered review missing %q:\n%s", expected, outcome.Review)
		}
	}
	if _, err := os.Stat(analysisDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("isolated directory still exists after Run(): %v", err)
	}
	wantProgress := []string{
		"Inspecting Git repository",
		"Resolving pull-request metadata",
		"Collecting pull-request evidence",
		"Selecting evidence within the review budget",
		"Reading account credit balance before review",
		"Starting Codex review",
		"Streaming or processing Codex events",
		"Validating review findings",
		"Reconciling account usage after review",
		"Rendering the review report",
	}
	if !reflect.DeepEqual(progress.messages, wantProgress) || progress.stops != len(wantProgress) {
		t.Fatalf("progress = %v, stops = %d", progress.messages, progress.stops)
	}
	runner.assertComplete()
}

func TestNewRequiresDependencies(t *testing.T) {
	progress := &reviewProgress{}
	if _, err := New(nil, progress); err == nil || !strings.Contains(err.Error(), "runner") {
		t.Fatalf("New(nil) error = %v", err)
	}
	if _, err := New(&reviewCommandRunner{t: t}, nil); err == nil || !strings.Contains(err.Error(), "progress") {
		t.Fatalf("New(nil progress) error = %v", err)
	}
	if _, err := NewWithAccountUsageSource(
		&reviewCommandRunner{t: t},
		progress,
		nil,
	); err == nil || !strings.Contains(err.Error(), "account usage") {
		t.Fatalf("NewWithAccountUsageSource(nil source) error = %v", err)
	}
}

func TestRunnerRunValidatesBeforeCommands(t *testing.T) {
	runner := &reviewCommandRunner{t: t}
	service, err := New(runner, &reviewProgress{})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []Request{
		{},
		{WorkingDirectory: "/repo"},
		{WorkingDirectory: "/repo", Target: "not-a-pr"},
		{WorkingDirectory: "/repo", Target: "1", Depth: "wide"},
	} {
		if _, err := service.Run(context.Background(), request); err == nil {
			t.Fatalf("Run(%+v) error = nil", request)
		}
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr string
	}{
		{name: "number", target: "123"},
		{name: "github URL", target: "https://github.com/acme/service/pull/123"},
		{name: "trailing slash", target: "https://github.com/acme/service/pull/123/"},
		{name: "zero", target: "0", wantErr: "positive"},
		{name: "other host", target: "https://gitlab.com/acme/service/-/merge_requests/123", wantErr: "GitHub"},
		{name: "userinfo", target: "https://attacker@github.com/acme/service/pull/123", wantErr: "GitHub"},
		{name: "wrong path", target: "https://github.com/acme/service/issues/123", wantErr: "/owner/repository/pull/number"},
		{name: "query", target: "https://github.com/acme/service/pull/123?x=1", wantErr: "query"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateTarget(test.target)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateTarget() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidateTarget() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestRepositoryFromRemote(t *testing.T) {
	for _, remote := range []string{
		"git@github.com:acme/service.git",
		"https://github.com/acme/service.git",
		"ssh://git@github.com/acme/service.git",
		"ssh://git@ssh.github.com:443/acme/service.git",
	} {
		name, err := repositoryFromRemote(remote)
		if err != nil || name != "acme/service" {
			t.Fatalf("repositoryFromRemote(%q) = %q, %v", remote, name, err)
		}
	}
	for _, remote := range []string{"", "https://gitlab.com/acme/service.git", "https://github.com/acme/team/service.git"} {
		if _, err := repositoryFromRemote(remote); err == nil {
			t.Fatalf("repositoryFromRemote(%q) error = nil", remote)
		}
	}
}

func TestLoadInstructionsSecurity(t *testing.T) {
	root := t.TempDir()
	regular := filepath.Join(root, "review.md")
	if err := os.WriteFile(regular, []byte("  Focus on errors.  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadInstructions(root, "review.md")
	if err != nil || got != "Focus on errors." {
		t.Fatalf("loadInstructions() = %q, %v", got, err)
	}
	if got, err := loadInstructions(root, ""); err != nil || got != "" {
		t.Fatalf("loadInstructions(empty) = %q, %v", got, err)
	}

	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, "../outside.md", "linked.md"} {
		if _, err := loadInstructions(root, path); err == nil {
			t.Fatalf("loadInstructions(%q) error = nil", path)
		}
	}

	empty := filepath.Join(root, "empty.md")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadInstructions(root, "empty.md"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("loadInstructions(empty file) error = %v", err)
	}
	large := filepath.Join(root, "large.md")
	if err := os.WriteFile(large, []byte(strings.Repeat("x", maxInstructionsBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadInstructions(root, "large.md"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("loadInstructions(large file) error = %v", err)
	}
}

func TestSelectDiffUsesRealBoundariesAndCompleteHunks(t *testing.T) {
	first := "diff --git a/plain.go b/plain.go\n--- a/plain.go\n+++ b/plain.go\n@@ -1 +1 @@\n-const old = 1\n+const text = \"diff --git is data\"\n"
	second := "diff --git a/auth.go b/auth.go\n--- a/auth.go\n+++ b/auth.go\n@@ -1 +1 @@\n-old()\n+validatePassword()\n@@ -10 +10 @@\n-oldAgain()\n+checkPermission()\n"
	files := []github.ChangedFile{{Path: "plain.go"}, {Path: "auth.go"}}
	parts := parseFileDiffs(first+second, false)
	if len(parts) != 2 {
		t.Fatalf("parseFileDiffs() count = %d, want 2", len(parts))
	}

	budget := estimatedTokens(strings.Split(second, "@@ -10")[0]) + 5
	selection := selectDiff(first+second, files, budget, false)
	if !strings.Contains(selection.Diff, "auth.go") || strings.Contains(selection.Diff, "@@ -10") {
		t.Fatalf("selected diff did not preserve one complete risky hunk:\n%s", selection.Diff)
	}
	if !selection.Omitted || !slices.Contains(selection.OmittedFiles, "auth.go") || !slices.Contains(selection.OmittedFiles, "plain.go") {
		t.Fatalf("selection omission metadata = %+v", selection)
	}
}

func TestParseFileDiffsUsesDiffHeadersInsteadOfManifestOrder(t *testing.T) {
	first := "diff --git a/plain.go b/plain.go\n--- a/plain.go\n+++ b/plain.go\n@@ -1 +1 @@\n-old\n+new\n"
	second := "diff --git a/auth.go b/auth.go\n--- a/auth.go\n+++ b/auth.go\n@@ -1 +1 @@\n-old\n+validatePassword()\n"

	parts := parseFileDiffs(first+second, false)
	if len(parts) != 2 {
		t.Fatalf("parseFileDiffs() count = %d, want 2", len(parts))
	}
	if parts[0].path != "plain.go" || parts[1].path != "auth.go" {
		t.Fatalf(
			"parseFileDiffs() paths = [%q, %q], want diff-header order",
			parts[0].path,
			parts[1].path,
		)
	}

	filesInDifferentOrder := []github.ChangedFile{{Path: "auth.go"}, {Path: "plain.go"}}
	selection := selectDiff(first+second, filesInDifferentOrder, 1, false)
	if !slices.Contains(selection.OmittedFiles, "plain.go") ||
		!slices.Contains(selection.OmittedFiles, "auth.go") {
		t.Fatalf("selection omission metadata = %+v", selection)
	}
}

func TestSelectCompleteHunksDropsTruncatedTail(t *testing.T) {
	section := "diff --git a/auth.go b/auth.go\n--- a/auth.go\n+++ b/auth.go\n@@ -1 +1 @@\n-old\n+firstCompleteChange()\n@@ -10 +10 @@\n-oldAgain\n+truncatedChange("
	selected := selectCompleteHunks(section, 10_000, false)
	if !strings.Contains(selected, "firstCompleteChange") {
		t.Fatalf("complete hunk was omitted:\n%s", selected)
	}
	if strings.Contains(selected, "truncatedChange") || strings.Contains(selected, "@@ -10") {
		t.Fatalf("truncated tail was retained:\n%s", selected)
	}
}

func TestBuildPayloadReportsLimitedEvidence(t *testing.T) {
	data := github.ReviewData{
		PullRequest: github.PullRequest{Number: 1, Body: strings.Repeat("é", standardBodyTokenBudget*3)},
		Repository:  "acme/service",
		Files:       []github.ChangedFile{{Path: "large.go"}},
		Diff:        "diff --git a/large.go b/large.go\n@@ -1 +1 @@\n-old\n+new\n",
		DiffLimited: true,
	}
	result, err := buildPayload(data, Standard, "")
	if err != nil {
		t.Fatalf("buildPayload() error = %v", err)
	}
	if !result.EvidenceOmitted || !result.SourceDiffLimited || !result.PullRequestBodyLimited {
		t.Fatalf("buildPayload() omission state = %+v", result)
	}
	if result.EvidenceTokenEstimate > standardEvidenceTokenBudget {
		t.Fatalf("token estimate = %d, budget = %d", result.EvidenceTokenEstimate, standardEvidenceTokenBudget)
	}
	if !strings.Contains(omissionNotice(result), "not reviewed") {
		t.Fatalf("omission notice = %q", omissionNotice(result))
	}
}

func TestDepthUsesLargerBudgetsAndHigherReasoning(t *testing.T) {
	if diffCaptureLimit(Deep) <= diffCaptureLimit(Standard) {
		t.Fatal("deep diff capture limit is not larger than standard")
	}
	if evidenceTokenBudget(Deep) <= evidenceTokenBudget(Standard) {
		t.Fatal("deep evidence token budget is not larger than standard")
	}
	if reasoningEffort(Standard) != "medium" || reasoningEffort(Deep) != "high" {
		t.Fatalf("reasoning efforts = %q, %q", reasoningEffort(Standard), reasoningEffort(Deep))
	}
}

func TestSelectDiffWithoutValidPatchStillReportsManifest(t *testing.T) {
	files := []github.ChangedFile{{Path: "one.go"}, {Path: "two.go"}}
	selection := selectDiff("not a unified diff", files, 100, false)
	if !selection.Omitted || !reflect.DeepEqual(selection.OmittedFiles, []string{"one.go", "two.go"}) {
		t.Fatalf("selectDiff() = %+v", selection)
	}
}

func TestStructuredReviewValidationAndRendering(t *testing.T) {
	line := 8
	value := structuredReview{
		Overview: "One **HIGH** issue.",
		CodeQualityAndStyle: []finding{{
			Severity:     "medium",
			File:         "handler.go",
			Line:         &line,
			Evidence:     "  Error is ignored. ",
			Impact:       " Failure is hidden. ",
			SuggestedFix: " Return the error. ",
		}},
	}
	if err := validateReview(value); err != nil {
		t.Fatalf("validateReview() error = %v", err)
	}
	rendered, severityLines := renderReviewWithSeverities(value)
	if !strings.Contains(rendered, "handler.go:8") || !strings.Contains(rendered, "# Potential Issues and Risks") {
		t.Fatalf("renderReview() =\n%s", rendered)
	}
	lines := strings.Split(rendered, "\n")
	if len(severityLines) != 1 {
		t.Fatalf("severity lines = %v, want one validated finding", severityLines)
	}
	for lineNumber, severity := range severityLines {
		if severity != "MEDIUM" || lineNumber <= 0 || lineNumber > len(lines) ||
			!strings.Contains(lines[lineNumber-1], "**MEDIUM**") {
			t.Fatalf("severity line %d = %q (%s)", lineNumber, lines[lineNumber-1], severity)
		}
	}
	withNotice := addOverviewNotice(rendered, "Evidence notice")
	if !strings.HasPrefix(withNotice, "# Overview\n\nEvidence notice") || strings.Count(withNotice, "# Overview") != 1 {
		t.Fatalf("addOverviewNotice() =\n%s", withNotice)
	}
	if _, err := decodeReview(`{"overview":"x"} {}`); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("decodeReview(multiple) error = %v", err)
	}
	if _, err := decodeReview(`{"overview":"x","unknown":true}`); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("decodeReview(unknown field) error = %v", err)
	}
	value.Overview = ""
	if err := validateReview(value); err == nil {
		t.Fatal("validateReview(empty overview) error = nil")
	}
}

func codexEventStream(review string) string {
	encoded, _ := json.Marshal(review)
	return `{"type":"thread.started","thread_id":"test"}` + "\n" +
		`{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":` + string(encoded) + `}}` + "\n" +
		`{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":400,"output_tokens":300,"reasoning_output_tokens":100}}` + "\n"
}

type reviewCommandRun struct {
	assert func(command.Spec)
	result command.Result
	err    error
}

type sequenceAccountSource struct {
	snapshots     []AccountSnapshot
	reviewCredits CreditUsage
	calls         int
	creditCalls   int
}

func (source *sequenceAccountSource) Snapshot(context.Context) (AccountSnapshot, error) {
	if source.calls >= len(source.snapshots) {
		return AccountSnapshot{}, errors.New("unexpected account snapshot")
	}
	snapshot := source.snapshots[source.calls]
	source.calls++
	return snapshot, nil
}

func (source *sequenceAccountSource) CreditsForReview(
	context.Context,
	TokenUsage,
) (CreditUsage, error) {
	source.creditCalls++
	return source.reviewCredits, nil
}

type reviewCommandRunner struct {
	t     *testing.T
	runs  []reviewCommandRun
	calls int
}

func (runner *reviewCommandRunner) Run(_ context.Context, spec command.Spec) (command.Result, error) {
	runner.t.Helper()
	if runner.calls >= len(runner.runs) {
		runner.t.Fatalf("unexpected command: %+v", spec)
	}
	run := runner.runs[runner.calls]
	runner.calls++
	if run.assert != nil {
		run.assert(spec)
	}
	return run.result, run.err
}

func (runner *reviewCommandRunner) assertComplete() {
	runner.t.Helper()
	if runner.calls != len(runner.runs) {
		runner.t.Fatalf("runner calls = %d, want %d", runner.calls, len(runner.runs))
	}
}

func exactReviewSpec(t *testing.T, want command.Spec) func(command.Spec) {
	return func(got command.Spec) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("command = %+v, want %+v", got, want)
		}
	}
}

type reviewProgress struct {
	messages []string
	stops    int
}

func (progress *reviewProgress) Start(message string) func() {
	progress.messages = append(progress.messages, message)
	return func() { progress.stops++ }
}
