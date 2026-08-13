# PRD: Local Branch Discovery and Cleanup

## Overview

Add a `champu-pr branch` command that makes recently active local branches easy
to find and removes safely merged local branches through one interactive,
preview-first cleanup flow. Remote branches must never be modified.

## Problem

When a repository has many local branches, ordinary `git branch` output makes
it difficult to identify recent work and remove local branches that are no
longer needed. Users need activity-aware listing, flexible search, selective
omission, and safe cleanup without risking remote branches.

## Goals

- List only local branches in latest-commit order.
- Always show relative activity time and a calendar date.
- Search local branch names with substring or full-name glob matching.
- Show every matched branch and its cleanup disposition.
- Let users omit eligible branches by numbered selection.
- Require an explicit preview and exact `apply` command.
- Revalidate every selected branch before deletion.
- Never delete or prune a remote branch.

## Non-goals

- Creating, renaming, merging, switching, or pushing branches.
- Deleting unmerged branches.
- Force-deleting branches.
- Deleting remote branches or pruning remote-tracking references.
- Supporting a separate argument-driven cleanup flow.

## Commands

```bash
champu-pr branch
champu-pr branch cleanup
```

`champu-pr branch` lists local branches. `champu-pr branch cleanup` is the only
cleanup entry point; it collects the base branch, search, omissions, and final
decision interactively.

## Listing requirements

Each branch row includes:

- A numbered index.
- The local branch name.
- Current, protected, merged, or unmerged status.
- Relative time since its latest commit.
- The latest commit's `YYYY-MM-DD` date.

Branches are sorted by latest commit time descending. Remote-tracking branches
must not be queried or displayed.

## Search rules

Input without `*` is a case-sensitive substring search over the complete branch
name. For example, `fix` matches `fix/login`, `feature/fix-login`,
`hotfix/payment`, and `release/fix`.

Input containing `*` is a case-sensitive glob matched against the complete
branch name. The asterisk may match any characters, including `/`. For example,
`*-dev` matches `xyz-dev`, `api-dev`, and `nested/api-dev`, while `fix/*`
matches `fix/login` and `fix/payment`.

Only local branch names participate in search.

## Interactive cleanup flow

1. Ask for a base branch, defaulting to `main`.
2. Ask for one substring or glob search.
3. Display every matching local branch with status and activity time.
4. Ask which eligible branches to omit.
5. Display a complete cleanup preview.
6. Accept `apply`, `modify`, or `exit`.

The omission prompt accepts comma- or space-separated indexes such as `2,3,5`,
and accepts `none` to omit nothing. Empty or invalid input is rejected. Branches
that are already protected or ineligible do not need to be omitted.

The final preview assigns every match one disposition:

- `DELETE`: eligible for local deletion.
- `OMIT`: eligible but explicitly omitted by the user.
- `PROTECTED`: current, base, or permanently protected branch.
- `KEEP`: not merged into the selected base.

At the final prompt:

- `apply` revalidates the candidates and performs safe local deletion.
- `modify` opens choices to change the search, omissions, or base branch, or to
  return to the preview.
- `exit` discards the plan and deletes nothing.

`exit` is also accepted at the interactive base, search, omission, and modify
prompts. It is a reserved interactive command, so a local branch literally
named `exit` cannot be selected as the base by entering its exact name.

## Protection and eligibility

A branch is eligible only when it:

- Exists under `refs/heads`.
- Matches the current search.
- Is merged into the selected local base branch.
- Is not the current branch.
- Is not the selected base branch.
- Is not named `main`, `master`, `dev`, or `develop`.
- Was not omitted.

The current branch, selected base branch, `main`, `master`, `dev`, and `develop`
are always protected.

## Git safety

Deletion uses an argument-safe invocation equivalent to:

```bash
git branch -d -- <branch-name>
```

The implementation must not invoke force deletion, remote deletion, `git push`,
or remote pruning. Deleting a local tracking branch only removes its local
`refs/heads` entry.

Immediately before deletion, the service reloads local branches, current-branch
state, and merge state. Changed or missing branches are skipped and reported.
Git refusal for one branch does not hide the results for other branches.

## Working-tree behavior

Listing and cleanup may inspect a dirty working tree. Cleanup warns that
uncommitted files exist and explicitly states that it will not modify them.

## Acceptance criteria

- `champu-pr branch` shows only local branches, newest first, with relative and
  calendar activity times.
- Plain `fix` and `xyz` searches match those strings anywhere in a branch name.
- `*-dev` and other searches containing `*` use full-name glob matching.
- The cleanup command has one interactive flow and no pattern arguments.
- Omission accepts `2,3,5`; `none` omits nothing.
- Every match appears in the final preview.
- `modify` can revisit search, omissions, and base selection.
- `exit` discards the plan.
- Only exact `apply` initiates deletion.
- Protected and unmerged branches are never deleted.
- Selected branches are revalidated before deletion.
- Only safe local deletion is used; remote branches are never affected.
