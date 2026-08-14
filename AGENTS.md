# Repository Agent Instructions

## Scope

These instructions apply to the entire repository. A more deeply nested
`AGENTS.md` may add or override instructions for its directory tree.

## Required workflow

- Read this file and any applicable nested `AGENTS.md` before changing files.
- Read-only inspection is allowed when it is needed to understand or propose a
  change.
- For every feature request, first draft a product requirements document in the
  conversation. Do not begin implementation until the user explicitly approves
  the PRD.
- After approval, save the PRD as `docs/PRD-<feature-name>.md`, using a concise
  lowercase kebab-case feature name.
- Treat PRD approval and implementation approval as separate gates. After saving
  the approved PRD, show the proposed implementation behavior and affected areas
  and wait for the user to say `apply` before changing production code.
- Before modifying files, show the proposed behavior and affected areas, explain
  the important design decisions, and wait for the user to say `apply`.
- Treat only an explicit `apply` as authorization to modify files.
- After modification, summarize the resulting diff and verification results so
  the user can review them before any additional modification or Git action.
- Preserve all existing user changes. Never discard, overwrite, or reformat
  unrelated work in a dirty worktree.

## Git safety

- Never push code or tags.
- Do not stage, commit, tag, create a branch, or open a pull request unless the
  user explicitly requests that exact action after reviewing the current diff.
- Do not use destructive Git commands such as `git reset --hard` or forced
  checkout to remove local changes.
- For Champu branch cleanup, never delete or prune remote branches or
  remote-tracking references. Keep deletion local, revalidate eligibility
  immediately before deletion, and use safe `git branch -d` semantics.

## Editing and implementation

- Use `apply_patch` for hand-authored repository edits.
- Keep production command invocations argument-based; do not construct shell
  command strings from user input.
- Keep interactive destructive operations preview-first and require exact
  confirmation.
- Add or update tests for every behavior change, including relevant success,
  failure, boundary, and safety cases.
- Add comments that explain non-obvious intent, safety constraints, algorithms,
  and design decisions. Do not add comments that merely restate the code.
- Give every helper function a concise comment describing what it does and why
  it exists when the purpose is not already established by an interface.
- Comment struct fields only when their meaning, constraints, units, ownership,
  or lifecycle need explanation. Omit redundant field comments.
- The published Git semantic tag is the source of truth for release versions.
  Untagged local builds must continue to report a development build.

## Code reuse and maintainability

- Before writing a new helper, `grep` the repository for similar logic (Git
  invocation, temp-file JSON schema writing, single-object JSON decoding, URL or
  repository-reference parsing, interactive prompt handling). Reuse or extend an
  existing implementation instead of adding a parallel one.
- Route repository-root discovery and other repeated `git` invocations through
  `internal/gitcontext.Collector` (or extend it) rather than issuing a second raw
  `command.Spec{Name: "git", ...}` call from a new package with its own error
  wrapping.
- Route Codex `exec` invocations, temporary JSON-schema files, and "decode one
  JSON object with no trailing data" parsing through the existing helpers in
  `internal/description` (`runDraft`, `writeDraftSchema`) instead of recreating
  them for a new call site. If a new caller's needs genuinely differ, factor the
  shared part into a helper both callers use rather than copying the whole
  pattern.
- Use one convention per cross-cutting concern across the whole codebase. In
  particular, model "the user cancelled an interactive prompt" with a single
  sentinel error (see `application.ErrCancelled`) everywhere it applies; do not
  introduce a second `exited bool` return value in one package to mean the same
  thing a sentinel error already means in another.
- Keep functions single-purpose. When a function accumulates more than one
  responsibility (rendering output, reading input, dispatching commands, and
  calling an external service, for example), extract each responsibility into a
  named function instead of growing one long loop or switch body.
- When a new package solves a problem another package already solves (parsing
  and validating a GitHub owner/repository from a URL, reading a line from
  stdin and treating EOF or a cancel keyword consistently, and similar), call
  that existing function or promote it to a shared location. Do not write a
  second implementation with slightly different wording or edge-case handling.
- Before presenting a diff for approval, scan it for logic copy-pasted from
  elsewhere in the repository (temp-file handling, JSON decoding, Git plumbing,
  error-wrapping strings) and consolidate it first. Note any deliberate,
  unavoidable duplication and why it was kept.

## Versioning

- Plan a semantic-version update for every delivered change.
- Increment the minor version for a backward-compatible feature, the patch
  version for a backward-compatible bug fix, and the major version for a
  breaking change.
- Determine the next version from the latest published semantic Git tag and
  record it in the PRD and final change summary.
- Do not add a disconnected version file. Do not create or push a release tag
  unless the user explicitly approves that exact Git action after reviewing the
  final diff.

## Commit messages

- Every proposed or created commit message must begin with an approved release
  classification. Never provide an unclassified commit message.
- Use `fix:` or `patch:` for a patch release, `feat:` or `minor:` for a minor
  release, and `major:` for a major release.
- `feat!:` and `fix!:` also select a major release. A `BREAKING CHANGE:` footer
  selects a major release regardless of the subject prefix.
- Determine the classification from the complete diff. When several levels
  apply, use the highest one: `major > minor > patch`.
- Format every commit message with a classified, imperative, concise title,
  followed by a blank line and a body.
- Write the body as `-`-prefixed bullet points that summarize the material
  changes in the complete diff. Do not provide a title-only commit message.
- Always present the proposed message as a ready-to-run `git commit` command,
  with the title and bulleted body passed separately. Do not provide only the
  raw title and body. Presenting the command does not authorize executing it.

## CLI documentation

- Whenever a command, subcommand, flag, interactive command, prompt, or behavior
  changes, update `champu --help` in the same change.
- Update the README with the corresponding usage and behavior in the same
  change.
- Keep `champu --help` concise. Include command syntax, commands, options,
  workflow controls, and representative examples only.
- Put detailed explanations, edge cases, and design rationale in the README or
  the relevant PRD instead of expanding `--help` excessively.
- Add or update help-output tests so the documented CLI cannot silently drift
  from its implementation.

## Verification

After Go code changes, run:

```bash
go test ./...
go vet ./...
```

Run `go test -race ./...` for concurrency-sensitive or release-sized changes.
Run `git diff --check` before presenting the final diff. If the environment
blocks the default Go cache, use a task-scoped cache outside the repository.

## Project map

- `cmd/champu`: process entry point and exit-code handling.
- `internal/application`: PR drafting workflow orchestration.
- `internal/branch`: local branch listing and safe cleanup.
- `internal/description`: Codex-backed PR description generation and refinement.
- `internal/github`: repository and pull-request resolution and publishing.
- `internal/review`: bounded-evidence pull-request review and usage reporting.
- `internal/structuredoutput`: strict JSON decoding and temporary JSON schemas.
- `internal/terminal`: progress and terminal rendering.
- `docs/`: product requirements and design context.
