# PRD: Automatic Semantic Release Tags

## Objective

Automatically calculate and create the next semantic Git tag whenever
classified commits reach `main`, without maintaining a separate version file.

## Commit-message classifications

Commit subjects use Conventional Commit-style prefixes with optional scopes:

| Prefix or marker | Version change |
| --- | --- |
| `fix:` or `patch:` | Patch |
| `feat:` or `minor:` | Minor |
| `major:` | Major |
| `feat!:` or `fix!:` | Major |
| `BREAKING CHANGE:` in the body | Major |

Examples:

```text
fix: prevent cancellation payload leaks
feat(branch): add local cleanup
major: rename the executable
feat!: replace the command interface
```

The required spelling is `patch`, not `path`.

## Commit-message agent policy

`AGENTS.md` requires every proposed or created commit message to:

- Use one of the recognized classifications above.
- Be selected from the complete diff rather than one isolated file.
- Use the highest applicable classification: `major > minor > patch`.
- Keep the subject imperative and concise.
- Never omit the release classification.

## Push classification

For a push containing several commits:

- Every non-merge commit must contain a recognized classification.
- Merge commits themselves are ignored.
- The highest required bump across the commit range wins.
- A push with no classifiable commits fails without creating a tag.

For example, two fixes and one feature produce a minor release.

## Release script

Add `scripts/release-tag.sh`. The script:

1. Confirms the target ref is exactly `refs/heads/main`.
2. Reads the latest reachable strict tag matching `vMAJOR.MINOR.PATCH`.
3. Inspects commits between the previous and pushed SHAs.
4. Rejects unclassified commits with their abbreviated SHAs and subjects.
5. Calculates the next version from the latest tag and highest bump.
6. Refuses to move, replace, or overwrite an existing tag.
7. Creates an annotated tag on the pushed commit.
8. Pushes only that exact tag when explicitly invoked with `--push`.
9. Supports `--dry-run` for mutation-free local verification.

The script uses argument-based Git commands and never force-pushes.

## GitHub automation

Add a GitHub Actions workflow triggered by pushes to `main`. It:

- Fetches complete Git history and tags.
- Serializes release jobs to prevent version-number races.
- Passes the GitHub push's before SHA, after SHA, and target ref to the script.
- Grants only the repository-content permission required to publish a tag.
- Never modifies or force-pushes `main`.
- Fails visibly without tagging when classification or version validation
  fails.

A push workflow runs after a commit reaches `main`; it cannot prevent the
original push. Blocking unclassified changes before merge requires branch
protection or a separate pull-request validation check.

## Documentation and tests

The implementation adds:

- Shell integration tests backed by temporary Git repositories.
- Coverage for patch, feature, explicit minor, breaking, precedence, missing
  classification, duplicate tags, wrong branch, and dry-run behavior.
- A Makefile target that runs the release-script tests.
- README release and commit-classification instructions.
- The commit-message rules in `AGENTS.md`.

## Version

The latest published semantic tag is `v0.3.0`. Independently, this
backward-compatible feature would target `v0.4.0`. When delivered with the
approved breaking executable rename, the combined target remains `v1.0.0`.

The combined change uses a classified commit message such as:

```text
major: rename the CLI and automate semantic release tags
```

## Acceptance criteria

- A qualifying push to `main` creates exactly one annotated semantic tag.
- Patch, minor, and major markers produce the expected next version.
- The highest marker in a multi-commit push wins.
- Unclassified commits, invalid refs, malformed tags, and duplicate target tags
  fail without mutation.
- Dry-run mode prints the decision without creating or pushing a tag.
- The workflow pushes only the calculated tag and never pushes branch code.
- Every AI-proposed commit message follows the same enforced classification.
- Script tests, Go tests, vet, race tests, build, and diff checks pass.
