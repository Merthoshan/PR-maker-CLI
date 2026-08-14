#!/usr/bin/env bash

set -uo pipefail

script_directory=$(
	cd "$(dirname "${BASH_SOURCE[0]}")" || exit 1
	pwd
)
release_script="$script_directory/release-tag.sh"
temporary_directories=()
passed=0
failed=0

cleanup() {
	for directory in "${temporary_directories[@]}"; do
		rm -rf "$directory"
	done
}
trap cleanup EXIT

# new_repository creates a repository at v1.2.3 and exposes its path and
# tagged commit through the REPOSITORY and BEFORE_SHA variables.
new_repository() {
	REPOSITORY=$(mktemp -d) || return 1
	temporary_directories+=("$REPOSITORY")
	git -C "$REPOSITORY" init -q -b main || return 1
	git -C "$REPOSITORY" config user.name "Champu Release Test" || return 1
	git -C "$REPOSITORY" config user.email "champu-release@example.test" || return 1
	git -C "$REPOSITORY" commit -q --allow-empty -m "fix: initialize release history" || return 1
	git -C "$REPOSITORY" tag -a v1.2.3 -m "Release v1.2.3" || return 1
	BEFORE_SHA=$(git -C "$REPOSITORY" rev-parse HEAD) || return 1
}

# add_commit adds one classified or deliberately unclassified test commit and
# exposes it through AFTER_SHA.
add_commit() {
	local subject=$1
	local body=${2:-}
	if [[ -n $body ]]; then
		git -C "$REPOSITORY" commit -q --allow-empty -m "$subject" -m "$body" ||
			return 1
	else
		git -C "$REPOSITORY" commit -q --allow-empty -m "$subject" || return 1
	fi
	AFTER_SHA=$(git -C "$REPOSITORY" rev-parse HEAD) || return 1
}

# dry_run calculates a tag for the current fixture without mutating it.
dry_run() {
	"$release_script" \
		--repo "$REPOSITORY" \
		--before "$BEFORE_SHA" \
		--after "$AFTER_SHA" \
		--ref refs/heads/main \
		--dry-run
}

assert_contains() {
	local value=$1
	local expected=$2
	if [[ $value != *"$expected"* ]]; then
		printf 'expected output to contain %q, received:\n%s\n' \
			"$expected" "$value" >&2
		return 1
	fi
}

test_patch_marker() {
	new_repository || return 1
	add_commit "fix(cli): stop leaking request evidence" || return 1
	local output
	output=$(dry_run 2>&1) || return 1
	assert_contains "$output" "Release bump: patch" || return 1
	assert_contains "$output" "Next tag: v1.2.4"
}

test_minor_markers() {
	local marker
	for marker in "feat: add branch cleanup" "minor(cli): rename help examples"; do
		new_repository || return 1
		add_commit "$marker" || return 1
		local output
		output=$(dry_run 2>&1) || return 1
		assert_contains "$output" "Release bump: minor" || return 1
		assert_contains "$output" "Next tag: v1.3.0" || return 1
	done
}

test_major_markers() {
	local subject
	local body
	for subject in \
		"major: rename the executable" \
		"feat!: replace the command interface" \
		"fix(core)!: remove compatibility"; do
		new_repository || return 1
		add_commit "$subject" || return 1
		local output
		output=$(dry_run 2>&1) || return 1
		assert_contains "$output" "Next tag: v2.0.0" || return 1
	done

	new_repository || return 1
	body=$'Migration is required.\n\nBREAKING CHANGE: rename the executable'
	add_commit "feat: prepare command rename" "$body" || return 1
	local output
	output=$(dry_run 2>&1) || return 1
	assert_contains "$output" "Next tag: v2.0.0"
}

test_highest_bump_wins() {
	new_repository || return 1
	add_commit "fix: repair updater output" || return 1
	add_commit "feat: add release automation" || return 1
	local output
	output=$(dry_run 2>&1) || return 1
	assert_contains "$output" "Release bump: minor" || return 1
	assert_contains "$output" "Next tag: v1.3.0"
}

test_rejects_unclassified_commit() {
	new_repository || return 1
	add_commit "docs: update release notes" || return 1
	local output
	if output=$(dry_run 2>&1); then
		printf 'unclassified commit unexpectedly succeeded\n' >&2
		return 1
	fi
	assert_contains "$output" "every non-merge commit requires" || return 1
	if git -C "$REPOSITORY" show-ref --verify --quiet refs/tags/v1.2.4; then
		printf 'unclassified commit created a tag\n' >&2
		return 1
	fi
}

test_rejects_non_main_ref() {
	new_repository || return 1
	add_commit "fix: repair updater output" || return 1
	local output
	if output=$(
		"$release_script" \
			--repo "$REPOSITORY" \
			--before "$BEFORE_SHA" \
			--after "$AFTER_SHA" \
			--ref refs/heads/develop \
			--dry-run 2>&1
	); then
		printf 'non-main ref unexpectedly succeeded\n' >&2
		return 1
	fi
	assert_contains "$output" "target ref must be refs/heads/main"
}

test_dry_run_does_not_create_tag() {
	new_repository || return 1
	add_commit "patch: update installation guidance" || return 1
	dry_run >/dev/null 2>&1 || return 1
	if git -C "$REPOSITORY" show-ref --verify --quiet refs/tags/v1.2.4; then
		printf 'dry run created a tag\n' >&2
		return 1
	fi
}

test_creates_annotated_local_tag() {
	new_repository || return 1
	add_commit "patch: update installation guidance" || return 1
	"$release_script" \
		--repo "$REPOSITORY" \
		--before "$BEFORE_SHA" \
		--after "$AFTER_SHA" \
		--ref refs/heads/main >/dev/null || return 1
	[[ $(git -C "$REPOSITORY" cat-file -t v1.2.4) == tag ]] || return 1
	[[ $(git -C "$REPOSITORY" rev-list -n 1 v1.2.4) == "$AFTER_SHA" ]]
}

test_rejects_malformed_reachable_tag() {
	new_repository || return 1
	git -C "$REPOSITORY" tag -a v1.3 -m "Malformed release" || return 1
	add_commit "fix: repair updater output" || return 1
	local output
	if output=$(dry_run 2>&1); then
		printf 'malformed tag unexpectedly succeeded\n' >&2
		return 1
	fi
	assert_contains "$output" "not strict vMAJOR.MINOR.PATCH"
}

test_pushes_only_the_calculated_tag() {
	new_repository || return 1
	add_commit "feat: add automatic release tagging" || return 1
	local remote_repository
	remote_repository=$(mktemp -d) || return 1
	temporary_directories+=("$remote_repository")
	git -C "$remote_repository" init -q --bare || return 1
	git -C "$REPOSITORY" remote add origin "$remote_repository" || return 1

	"$release_script" \
		--repo "$REPOSITORY" \
		--before "$BEFORE_SHA" \
		--after "$AFTER_SHA" \
		--ref refs/heads/main \
		--push >/dev/null || return 1

	git --git-dir="$remote_repository" show-ref --verify --quiet refs/tags/v1.3.0 ||
		return 1
	if git --git-dir="$remote_repository" show-ref --verify --quiet refs/heads/main; then
		printf 'tag publication unexpectedly pushed main\n' >&2
		return 1
	fi
}

test_rejects_existing_remote_tag() {
	new_repository || return 1
	add_commit "fix: repair updater output" || return 1
	local remote_repository
	remote_repository=$(mktemp -d) || return 1
	temporary_directories+=("$remote_repository")
	git -C "$remote_repository" init -q --bare || return 1
	git -C "$REPOSITORY" remote add origin "$remote_repository" || return 1
	git -C "$REPOSITORY" push -q origin \
		"$AFTER_SHA:refs/tags/v1.2.4" || return 1

	local output
	if output=$(
		"$release_script" \
			--repo "$REPOSITORY" \
			--before "$BEFORE_SHA" \
			--after "$AFTER_SHA" \
			--ref refs/heads/main \
			--push 2>&1
	); then
		printf 'existing remote tag unexpectedly succeeded\n' >&2
		return 1
	fi
	assert_contains "$output" "remote tag already exists: v1.2.4" || return 1
	if git -C "$REPOSITORY" show-ref --verify --quiet refs/tags/v1.2.4; then
		printf 'remote duplicate failure created a local tag\n' >&2
		return 1
	fi
}

# run_test reports all failures in one invocation so CI exposes every broken
# release-safety invariant instead of stopping after the first assertion.
run_test() {
	local name=$1
	local test_function=$2
	if "$test_function"; then
		printf 'ok - %s\n' "$name"
		((passed += 1))
	else
		printf 'not ok - %s\n' "$name" >&2
		((failed += 1))
	fi
}

run_test "patch marker" test_patch_marker
run_test "minor markers" test_minor_markers
run_test "major markers" test_major_markers
run_test "highest bump wins" test_highest_bump_wins
run_test "unclassified commit" test_rejects_unclassified_commit
run_test "non-main ref" test_rejects_non_main_ref
run_test "dry run" test_dry_run_does_not_create_tag
run_test "annotated local tag" test_creates_annotated_local_tag
run_test "malformed reachable tag" test_rejects_malformed_reachable_tag
run_test "pushes only tag" test_pushes_only_the_calculated_tag
run_test "existing remote tag" test_rejects_existing_remote_tag

printf '%d passed, %d failed\n' "$passed" "$failed"
((failed == 0))
