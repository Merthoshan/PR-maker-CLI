package branch

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Merthoshan/PR-maker-CLI/internal/command"
)

func TestMatchBranchesUsesSubstringWithoutAsterisk(t *testing.T) {
	branches := namedBranches(
		"fix/login",
		"feature/fix-login",
		"hotfix/payment",
		"release/fix",
		"feature/payment",
	)

	matches, err := matchBranches(branches, "fix")
	if err != nil {
		t.Fatalf("matchBranches() unexpected error: %v", err)
	}
	if got, want := branchNames(matches), []string{
		"fix/login",
		"feature/fix-login",
		"hotfix/payment",
		"release/fix",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("matchBranches() = %v, want %v", got, want)
	}
}

func TestMatchBranchesUsesFullNameGlobWithCrossSlashAsterisk(t *testing.T) {
	branches := namedBranches(
		"xyz-dev",
		"api-dev",
		"feature-dev",
		"nested/api-dev",
		"dev-api",
		"xyz-development",
	)

	matches, err := matchBranches(branches, "*-dev")
	if err != nil {
		t.Fatalf("matchBranches() unexpected error: %v", err)
	}
	if got, want := branchNames(matches), []string{
		"xyz-dev",
		"api-dev",
		"feature-dev",
		"nested/api-dev",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("matchBranches() = %v, want %v", got, want)
	}
}

func TestParseOmissions(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []int
		wantErr bool
	}{
		{name: "none", value: "none"},
		{name: "commas and spaces", value: "5, 2 3", want: []int{2, 3, 5}},
		{name: "duplicates", value: "2,2,3", want: []int{2, 3}},
		{name: "empty", value: "", wantErr: true},
		{name: "zero", value: "0", wantErr: true},
		{name: "too large", value: "6", wantErr: true},
		{name: "word", value: "two", wantErr: true},
		{name: "repeated comma", value: "2,,3", wantErr: true},
		{name: "trailing comma", value: "2,", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseOmissions(test.value, 5)
			if test.wantErr {
				if err == nil {
					t.Fatal("parseOmissions() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOmissions() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseOmissions() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestClassifyProtectsAndFiltersBranches(t *testing.T) {
	state := snapshot{current: "feature/current", base: "release"}
	tests := []struct {
		name       string
		branch     localBranch
		omitted    map[string]bool
		wantStatus string
	}{
		{name: "current", branch: localBranch{Name: "feature/current", Merged: true}, wantStatus: "PROTECTED"},
		{name: "base", branch: localBranch{Name: "release", Merged: true}, wantStatus: "PROTECTED"},
		{name: "permanent", branch: localBranch{Name: "develop", Merged: true}, wantStatus: "PROTECTED"},
		{name: "unmerged", branch: localBranch{Name: "feature/open"}, wantStatus: "KEEP"},
		{
			name:       "omitted",
			branch:     localBranch{Name: "fix/keep", Merged: true},
			omitted:    map[string]bool{"fix/keep": true},
			wantStatus: "OMIT",
		},
		{name: "eligible", branch: localBranch{Name: "fix/delete", Merged: true}, wantStatus: "DELETE"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _ := classify(test.branch, state, test.omitted)
			if got != test.wantStatus {
				t.Fatalf("classify() = %q, want %q", got, test.wantStatus)
			}
		})
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		ago  time.Duration
		want string
	}{
		{name: "recent", ago: 30 * time.Second, want: "just now"},
		{name: "minute", ago: time.Minute, want: "1 minute ago"},
		{name: "hours", ago: 3 * time.Hour, want: "3 hours ago"},
		{name: "days", ago: 2 * 24 * time.Hour, want: "2 days ago"},
		{name: "weeks", ago: 21 * 24 * time.Hour, want: "3 weeks ago"},
		{name: "months", ago: 90 * 24 * time.Hour, want: "3 months ago"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := relativeTime(now, now.Add(-test.ago)); got != test.want {
				t.Fatalf("relativeTime() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCleanupPreviewsAndDeletesOnlyEligibleLocalBranches(t *testing.T) {
	runner := newBranchRunner(t)
	var output bytes.Buffer
	var errorOutput bytes.Buffer
	service, err := New(
		runner,
		strings.NewReader("\nfix\n3\napply\n"),
		&output,
		&errorOutput,
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	service.now = func() time.Time {
		return time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	}

	err = service.Run(context.Background(), Request{
		WorkingDirectory: "/work",
		Cleanup:          true,
	})
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	for _, want := range []string{
		"Warning: the working tree contains uncommitted changes.",
		"Cleanup preview:",
		"PROTECTED feature/fix-current",
		"DELETE    fix/delete",
		"OMIT      feature/fix-omit",
		"KEEP      hotfix/unmerged",
		"2 days ago (2026-08-12)",
		"Deleted 1 local branches:",
		"Remote branches affected: none",
	} {
		combined := output.String() + errorOutput.String()
		if !strings.Contains(combined, want) {
			t.Fatalf("cleanup output missing %q:\n%s", want, combined)
		}
	}
	if got, want := runner.deleted, []string{"fix/delete"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("deleted branches = %v, want %v", got, want)
	}
	runner.assertLocalOnly()
}

func TestCleanupRevalidatesBeforeDeletion(t *testing.T) {
	runner := newBranchRunner(t)
	runner.changeMergeStatusOnRevalidation = true
	var output bytes.Buffer
	service, err := New(
		runner,
		strings.NewReader("\nfix/delete\nnone\napply\n"),
		&output,
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	if err := service.Run(context.Background(), Request{
		WorkingDirectory: "/work",
		Cleanup:          true,
	}); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if len(runner.deleted) != 0 {
		t.Fatalf("deleted branches = %v, want none", runner.deleted)
	}
	if !strings.Contains(output.String(), "fix/delete — not merged into main") {
		t.Fatalf("output missing revalidation skip:\n%s", output.String())
	}
}

func TestCleanupReportsSafeDeletionFailure(t *testing.T) {
	runner := newBranchRunner(t)
	runner.deleteError = true
	var output bytes.Buffer
	service, err := New(
		runner,
		strings.NewReader("\nfix/delete\nnone\napply\n"),
		&output,
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	if err := service.Run(context.Background(), Request{
		WorkingDirectory: "/work",
		Cleanup:          true,
	}); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !strings.Contains(output.String(), "No local branches were deleted.") ||
		!strings.Contains(output.String(), "Git refused safe deletion") ||
		!strings.Contains(output.String(), "branch is checked out elsewhere") {
		t.Fatalf("output missing deletion failure:\n%s", output.String())
	}
}

func TestCleanupModifySearchAndExit(t *testing.T) {
	runner := newBranchRunner(t)
	var output bytes.Buffer
	service, err := New(
		runner,
		strings.NewReader("\nfix/delete\nnone\nmodify\n1\nxyz\nnone\nexit\n"),
		&output,
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	if err := service.Run(context.Background(), Request{
		WorkingDirectory: "/work",
		Cleanup:          true,
	}); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !strings.Contains(output.String(), "What would you like to modify?") ||
		!strings.Contains(output.String(), "xyz-dev") ||
		!strings.Contains(output.String(), "Cleanup exited. No branches were deleted.") {
		t.Fatalf("output missing modify flow:\n%s", output.String())
	}
	if len(runner.deleted) != 0 {
		t.Fatalf("deleted branches = %v, want none", runner.deleted)
	}
}

func TestCleanupExitFromEveryInteractiveStage(t *testing.T) {
	inputs := map[string]string{
		"base":          "exit\n",
		"search":        "\nexit\n",
		"omissions":     "\nfix/delete\nexit\n",
		"modify choice": "\nfix/delete\nnone\nmodify\nexit\n",
		"preview":       "\nfix/delete\nnone\nexit\n",
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			runner := newBranchRunner(t)
			var output bytes.Buffer
			service, err := New(
				runner,
				strings.NewReader(input),
				&output,
				&bytes.Buffer{},
			)
			if err != nil {
				t.Fatalf("New() unexpected error: %v", err)
			}
			if err := service.Run(context.Background(), Request{
				WorkingDirectory: "/work",
				Cleanup:          true,
			}); err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}
			if count := strings.Count(
				output.String(),
				"Cleanup exited. No branches were deleted.",
			); count != 1 {
				t.Fatalf("cancellation message count = %d, want 1:\n%s", count, output.String())
			}
			if len(runner.deleted) != 0 {
				t.Fatalf("deleted branches = %v, want none", runner.deleted)
			}
		})
	}
}

func TestListShowsOnlyLocalBranchesWithActivity(t *testing.T) {
	runner := newBranchRunner(t)
	var output bytes.Buffer
	service, err := New(runner, strings.NewReader(""), &output, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	service.now = func() time.Time {
		return time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	}

	if err := service.Run(context.Background(), Request{WorkingDirectory: "/work"}); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if !strings.Contains(output.String(), "Current branch: feature/fix-current") ||
		!strings.Contains(output.String(), "feature/fix-current") ||
		!strings.Contains(output.String(), "2026-08-14") {
		t.Fatalf("list output missing branch details:\n%s", output.String())
	}
	if strings.Contains(output.String(), "origin/") {
		t.Fatalf("list output contains remote branch:\n%s", output.String())
	}
	runner.assertLocalOnly()
}

func TestCleanupWithRealGitLeavesRemoteTrackingReferenceUntouched(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.name", "Champu Test")
	runGit(t, root, "config", "user.email", "champu@example.test")
	runGit(t, root, "commit", "--allow-empty", "-m", "initial")
	initial := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "switch", "-c", "fix/merged")
	runGit(t, root, "commit", "--allow-empty", "-m", "fix")
	mergedHead := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "switch", "main")
	runGit(t, root, "merge", "--no-ff", "fix/merged", "-m", "merge fix")
	runGit(t, root, "switch", "-c", "feature/current", initial)
	runGit(t, root, "commit", "--allow-empty", "-m", "current work")
	runGit(t, root, "update-ref", "refs/remotes/origin/fix/merged", mergedHead)

	var output bytes.Buffer
	service, err := New(
		command.ExecRunner{},
		strings.NewReader("\nfix/merged\nnone\napply\n"),
		&output,
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	if err := service.Run(context.Background(), Request{
		WorkingDirectory: root,
		Cleanup:          true,
	}); err != nil {
		t.Fatalf("Run() unexpected error: %v\n%s", err, output.String())
	}

	localCheck := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/fix/merged")
	localCheck.Dir = root
	if err := localCheck.Run(); err == nil {
		t.Fatal("local branch still exists after confirmed cleanup")
	}
	runGit(t, root, "show-ref", "--verify", "refs/remotes/origin/fix/merged")
	if !strings.Contains(output.String(), "Remote branches affected: none") {
		t.Fatalf("output missing remote safety result:\n%s", output.String())
	}
}

func namedBranches(names ...string) []localBranch {
	branches := make([]localBranch, 0, len(names))
	for _, name := range names {
		branches = append(branches, localBranch{Name: name})
	}
	return branches
}

func branchNames(branches []localBranch) []string {
	names := make([]string, 0, len(branches))
	for _, branch := range branches {
		names = append(names, branch.Name)
	}
	return names
}

func runGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

type branchRunner struct {
	t                               *testing.T
	calls                           []command.Spec
	deleted                         []string
	mergedCalls                     int
	changeMergeStatusOnRevalidation bool
	deleteError                     bool
}

func newBranchRunner(t *testing.T) *branchRunner {
	return &branchRunner{t: t}
}

func (runner *branchRunner) Run(
	_ context.Context,
	spec command.Spec,
) (command.Result, error) {
	runner.t.Helper()
	runner.calls = append(runner.calls, spec)
	key := spec.Name + " " + strings.Join(spec.Args, " ")
	switch key {
	case "git rev-parse --show-toplevel":
		return command.Result{Stdout: "/repo\n"}, nil
	case "git status --porcelain --untracked-files=normal":
		return command.Result{Stdout: " M README.md\n"}, nil
	case "git branch --show-current":
		return command.Result{Stdout: "feature/fix-current\n"}, nil
	case "git for-each-ref --sort=-committerdate --format=%(refname:short)%00%(committerdate:unix)%00%(committerdate:short) refs/heads":
		return command.Result{Stdout: strings.Join([]string{
			"feature/fix-current\x001786705200\x002026-08-14",
			"fix/delete\x001786536000\x002026-08-12",
			"feature/fix-omit\x001786449600\x002026-08-11",
			"hotfix/unmerged\x001786363200\x002026-08-10",
			"xyz-dev\x001786276800\x002026-08-09",
			"main\x001786190400\x002026-08-08",
		}, "\n") + "\n"}, nil
	case "git for-each-ref --format=%(refname:short) --merged=refs/heads/main refs/heads":
		runner.mergedCalls++
		if runner.changeMergeStatusOnRevalidation && runner.mergedCalls > 1 {
			return command.Result{Stdout: "main\nfeature/fix-current\nfeature/fix-omit\n"}, nil
		}
		return command.Result{Stdout: "main\nfeature/fix-current\nfix/delete\nfeature/fix-omit\nxyz-dev\n"}, nil
	}
	if spec.Name == "git" && len(spec.Args) == 4 &&
		reflect.DeepEqual(spec.Args[:3], []string{"branch", "-d", "--"}) {
		wantEnvironment := []string{
			"GIT_CONFIG_COUNT=2",
			"GIT_CONFIG_KEY_0=branch." + spec.Args[3] + ".remote",
			"GIT_CONFIG_VALUE_0=.",
			"GIT_CONFIG_KEY_1=branch." + spec.Args[3] + ".merge",
			"GIT_CONFIG_VALUE_1=refs/heads/main",
		}
		if !reflect.DeepEqual(spec.Env, wantEnvironment) {
			runner.t.Fatalf("delete environment = %v, want %v", spec.Env, wantEnvironment)
		}
		if runner.deleteError {
			return command.Result{Stderr: "branch is checked out elsewhere"},
				errors.New("exit status 1")
		}
		runner.deleted = append(runner.deleted, spec.Args[3])
		return command.Result{Stdout: "Deleted branch " + spec.Args[3]}, nil
	}
	return command.Result{}, errors.New("unexpected command: " + key)
}

func (runner *branchRunner) assertLocalOnly() {
	runner.t.Helper()
	for _, spec := range runner.calls {
		joined := strings.Join(spec.Args, " ")
		if strings.Contains(joined, "push") ||
			strings.Contains(joined, "prune") ||
			strings.Contains(joined, "refs/remotes") {
			runner.t.Fatalf("remote-affecting Git call: git %s", joined)
		}
	}
}
