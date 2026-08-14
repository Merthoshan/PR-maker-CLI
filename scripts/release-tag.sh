#!/usr/bin/env bash

set -euo pipefail

usage() {
	cat <<'EOF'
Usage:
  scripts/release-tag.sh --before SHA --after SHA --ref refs/heads/main [options]

Options:
  --repo PATH    Git repository to inspect (default: current directory)
  --remote NAME  Remote used with --push (default: origin)
  --dry-run      Print the release decision without creating a tag
  --push         Create and push only the calculated annotated tag
  -h, --help     Show this help
EOF
}

fail() {
	printf 'release tag: %s\n' "$*" >&2
	exit 1
}

repository=.
remote=origin
before_sha=
after_sha=
target_ref=
dry_run=false
push_tag=false

while (($# > 0)); do
	case "$1" in
	--repo)
		(($# >= 2)) || fail "--repo requires a path"
		repository=$2
		shift 2
		;;
	--remote)
		(($# >= 2)) || fail "--remote requires a name"
		remote=$2
		shift 2
		;;
	--before)
		(($# >= 2)) || fail "--before requires a commit"
		before_sha=$2
		shift 2
		;;
	--after)
		(($# >= 2)) || fail "--after requires a commit"
		after_sha=$2
		shift 2
		;;
	--ref)
		(($# >= 2)) || fail "--ref requires a Git ref"
		target_ref=$2
		shift 2
		;;
	--dry-run)
		dry_run=true
		shift
		;;
	--push)
		push_tag=true
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		fail "unknown argument $1"
		;;
	esac
done

[[ -n $before_sha ]] || fail "--before is required"
[[ -n $after_sha ]] || fail "--after is required"
[[ -n $target_ref ]] || fail "--ref is required"
[[ $target_ref == refs/heads/main ]] ||
	fail "target ref must be refs/heads/main, received $target_ref"
if $dry_run && $push_tag; then
	fail "--dry-run and --push cannot be used together"
fi

repository=$(git -C "$repository" rev-parse --show-toplevel 2>/dev/null) ||
	fail "repository is not a Git worktree"

# git_repo keeps every Git operation scoped to the explicitly resolved
# repository instead of relying on the caller's current directory.
git_repo() {
	git -C "$repository" "$@"
}

git_repo cat-file -e "${after_sha}^{commit}" 2>/dev/null ||
	fail "after value is not a commit: $after_sha"

before_is_zero=false
if [[ $before_sha =~ ^0+$ ]]; then
	before_is_zero=true
else
	git_repo cat-file -e "${before_sha}^{commit}" 2>/dev/null ||
		fail "before value is not a commit: $before_sha"
	git_repo merge-base --is-ancestor "$before_sha" "$after_sha" ||
		fail "after commit is not descended from before commit"
fi

latest_tag=
while IFS= read -r candidate; do
	[[ -n $candidate ]] || continue
	if [[ ! $candidate =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
		fail "reachable version tag is not strict vMAJOR.MINOR.PATCH: $candidate"
	fi
	if [[ -z $latest_tag ]]; then
		latest_tag=$candidate
	fi
done < <(
	git_repo tag \
		--merged "$after_sha" \
		--list 'v[0-9]*' \
		--sort=-version:refname
)

current_version=${latest_tag:-v0.0.0}
if [[ ! $current_version =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
	fail "cannot parse current version $current_version"
fi
major=${BASH_REMATCH[1]}
minor=${BASH_REMATCH[2]}
patch=${BASH_REMATCH[3]}

if $before_is_zero; then
	commit_source=(git_repo rev-list --reverse --no-merges "$after_sha")
else
	commit_range="${before_sha}..${after_sha}"
	commit_source=(git_repo rev-list --reverse --no-merges "$commit_range")
fi

# classify_commit maps the approved commit markers to their semantic-version
# precedence. It reads the complete message so BREAKING CHANGE is authoritative.
classify_commit() {
	local commit=$1
	local subject
	local message
	subject=$(git_repo log -1 --format=%s "$commit")
	message=$(git_repo log -1 --format=%B "$commit")

	if grep -Eq '^BREAKING CHANGE:[[:space:]]' <<<"$message"; then
		printf 'major\n'
		return 0
	fi
	if grep -Eiq '^major(\([^)]*\))?!?:[[:space:]]' <<<"$subject"; then
		printf 'major\n'
		return 0
	fi
	if grep -Eiq '^(feat|fix)(\([^)]*\))?!:[[:space:]]' <<<"$subject"; then
		printf 'major\n'
		return 0
	fi
	if grep -Eiq '^(feat|minor)(\([^)]*\))?:[[:space:]]' <<<"$subject"; then
		printf 'minor\n'
		return 0
	fi
	if grep -Eiq '^(fix|patch)(\([^)]*\))?:[[:space:]]' <<<"$subject"; then
		printf 'patch\n'
		return 0
	fi
	return 1
}

bump_rank=0
commit_count=0
unclassified=()
while IFS= read -r commit; do
	[[ -n $commit ]] || continue
	((commit_count += 1))
	if classification=$(classify_commit "$commit"); then
		case "$classification" in
		major) rank=3 ;;
		minor) rank=2 ;;
		patch) rank=1 ;;
		esac
		if ((rank > bump_rank)); then
			bump_rank=$rank
		fi
else
		subject=$(git_repo log -1 --format=%s "$commit")
		short_sha=$(git_repo rev-parse --short "$commit")
		unclassified+=("$short_sha $subject")
	fi
done < <("${commit_source[@]}")

((commit_count > 0)) || fail "no untagged non-merge commits were found"
if ((${#unclassified[@]} > 0)); then
	printf 'release tag: every non-merge commit requires a release classification:\n' >&2
	for commit in "${unclassified[@]}"; do
		printf '  %s\n' "$commit" >&2
	done
	exit 1
fi

case "$bump_rank" in
3)
	bump=major
	((major += 1))
	minor=0
	patch=0
	;;
2)
	bump=minor
	((minor += 1))
	patch=0
	;;
1)
	bump=patch
	((patch += 1))
	;;
*)
	fail "no release classification was found"
	;;
esac

next_tag="v${major}.${minor}.${patch}"
if git_repo show-ref --verify --quiet "refs/tags/$next_tag"; then
	fail "tag already exists: $next_tag"
fi

printf 'Current tag: %s\n' "$current_version"
printf 'Release bump: %s\n' "$bump"
printf 'Next tag: %s\n' "$next_tag"
printf 'Target commit: %s\n' "$after_sha"

if $dry_run; then
	printf 'Dry run: no tag was created or pushed.\n'
	exit 0
fi

if $push_tag; then
	git_repo remote get-url "$remote" >/dev/null 2>&1 ||
		fail "remote does not exist: $remote"
	set +e
	git_repo ls-remote --exit-code --tags "$remote" \
		"refs/tags/$next_tag" >/dev/null 2>&1
	remote_status=$?
	set -e
	case "$remote_status" in
	0) fail "remote tag already exists: $next_tag" ;;
	2) ;;
	*) fail "could not inspect remote tag $next_tag on $remote" ;;
	esac
fi

git_repo tag -a "$next_tag" "$after_sha" -m "Release $next_tag"

if $push_tag; then
	# If publication fails, remove only the tag created by this invocation so a
	# retry starts from the same local state. Existing tags are never touched.
	if ! git_repo push "$remote" \
		"refs/tags/$next_tag:refs/tags/$next_tag"; then
		git_repo tag -d "$next_tag" >/dev/null
		fail "could not push tag $next_tag to $remote"
	fi
	printf 'Created and pushed %s.\n' "$next_tag"
	else
	printf 'Created local tag %s.\n' "$next_tag"
fi
