# champu-pr

`champu-pr` helps you find and safely clean local branches, collects Git changes
from the current branch or a selected pull request, asks Codex to produce an
evidence-backed pull-request description, lets you refine the preview, and
updates GitHub only after you type `apply`.

## Prerequisites

- Go
- Git
- Node.js and npm
- [GitHub CLI](https://cli.github.com/) authenticated with `gh auth login`
- Codex CLI authenticated and available as `codex`

## Setup

Install the Codex CLI:

```bash
npm install -g @openai/codex
codex login
codex login status
```

Install `champu-pr` from this source checkout:

```bash
go install ./cmd/champu-pr
```

Make sure Go's binary directory is on your `PATH`. You can inspect it with:

```bash
go env GOBIN GOPATH
```

Authenticate the GitHub CLI:

```bash
gh auth login --hostname github.com --git-protocol ssh --web
```

Complete the browser login, then verify the authentication:

```bash
gh auth status
```

GitHub CLI authentication normally needs to be completed only once per
machine. For automated environments, `gh` can alternatively read a token from
the `GH_TOKEN` environment variable. Never commit that token.

Verify that the Codex CLI and `champu-pr` are available:

```bash
codex --version
champu-pr --help
```

Once a semantic release has been published, a new installation can use:

```bash
go install github.com/Merthoshan/PR-maker-CLI/cmd/champu-pr@latest
```

## Use

Run the command from any directory inside your repository:

```bash
champu-pr
```

View every option and interactive command in the terminal:

```bash
champu-pr --help
```

By default, the command targets `main` and creates or updates a draft pull
request. Use `--ready` to create a new PR as ready for review, or to mark an
existing draft PR as ready:

```bash
champu-pr --base develop
champu-pr --pr 123
champu-pr --base main --ready
champu-pr --pr 123 --ready
champu-pr --dry-run
```

List local branches in latest-commit order so recently active work is easy to
find:

```bash
champu-pr branch
```

Start the interactive local cleanup flow:

```bash
champu-pr branch cleanup
```

Cleanup asks for a local base branch and one search. Plain text such as `fix`
matches anywhere in the branch name. A search containing `*`, such as `*-dev`
or `feature/*`, is matched as a full-name glob. Every result includes relative
activity time and its calendar date.

After reviewing the matches, enter branch numbers such as `2,3,5` to omit them,
or enter `none` to omit nothing. The final preview labels every result as
`DELETE`, `OMIT`, `PROTECTED`, or `KEEP`. At that preview:

- `apply` revalidates and safely deletes the selected local branches.
- `modify` lets you change the search, omissions, or base branch.
- `exit` discards the cleanup plan.

Cleanup protects the current branch, selected base, `main`, `master`, `dev`,
and `develop`. It deletes only branches merged into the selected local base and
uses `git branch -d`. It never deletes, prunes, or otherwise modifies remote
branches.

Review an existing pull request without changing GitHub:

```bash
champu-pr review 123
champu-pr review https://github.com/org/repo/pull/123
champu-pr review 123 --depth deep
champu-pr review 123 --instructions .champu-pr/review-instructions.md
```

The default `standard` review is optimized for daily use. It checks correctness,
security, performance, database or external calls inside loops, nested control
flow, error handling, and tests while reporting only actionable issues introduced
by the pull request. `--depth deep` uses a larger evidence budget and higher
reasoning effort, but it does not claim to review content outside the supplied
evidence.

The pull request must belong to the GitHub repository configured as the local
`origin`; this prevents a PR URL from being analyzed against an unrelated local
checkout. The command resolves the repository root, so it can be run from a
subdirectory.

The security and output instructions are built into `champu-pr` and cannot be
replaced by repository content. Additional review guidance is optional and must
be selected explicitly with `--instructions`. The selected path must be a
regular, non-symlinked file inside the repository. For example:

```bash
champu-pr review 123 --instructions .champu-pr/review-instructions.md
```

Large pull requests are reviewed with a bounded, approximate token budget. The
changed-file manifest is always supplied, patches are selected only at complete
file or hunk boundaries, and the output identifies when files or other evidence
were omitted.

Review progress is written to stderr as ten explicit stages. Interactive
terminals show an approximate workflow progress bar, elapsed time, the evidence
estimate, and Codex usage as soon as the structured event stream reports it;
redirected output and CI receive deterministic status lines instead.

The final report distinguishes the local evidence estimate from the actual
input, cached-input, and output tokens reported by Codex. Account-consumption
percentages use the credits consumed by the review divided by the credit balance
available immediately before the review. They are shown only when a supported
authenticated source supplies both values; otherwise the report explicitly
says that review credits or the account credit balance are unavailable. Finding
severity is color-coded only on interactive terminals, and `NO_COLOR` disables
it.

The standalone CLI does not currently configure an authenticated account-credit
source, so credit consumption and balance fields report `unavailable`. Actual
token usage from the Codex event stream is still reported. The account-credit
interfaces are retained for a future supported provider.

`--pr` selects one existing open pull request by number and works from any
checked-out local branch. It reads the pushed PR commits directly from
GitHub's pull-request ref without checking out or modifying the PR branch.
Uncommitted or unpushed local changes are not included in this mode.

`--base` selects an existing PR against that base branch or creates one when
none exists. `--pr` and `--base` cannot be used together.

## Progress indicators

While `champu-pr` performs a long-running operation, it displays the current
workflow stage:

```text
⠋ Inspecting Git repository...
⠹ Collecting Git evidence...
⠸ Generating PR description with Codex... 8s
```

Progress is shown for:

- Repository inspection
- Pull-request lookup
- Git-evidence collection
- Initial Codex generation
- Codex refinements
- GitHub creation or update

Interactive terminals receive an animated spinner and elapsed time. When output
is redirected or the command runs in CI, each operation produces one static
status line instead.

Progress is written to standard error, while the generated title and PR
description remain on standard output.

Press `Ctrl-C` to stop active processing. Champu terminates the active child
command, clears an interactive progress indicator, prints only `Cancelled.`,
and exits with status `130`. Cancellation does not print the request evidence,
raw diff, pull-request body, or captured child-process error, and an interrupted
refinement or title-generation operation is not retried.

## Versions and updates

Show the installed version:

```bash
champu-pr --version
```

A binary installed from the local source checkout reports:

```text
champu-pr development build
```

Refresh that development build after changing the local source:

```bash
cd /Users/aftershoot/champu-pr
go install ./cmd/champu-pr
rehash
```

Published builds use strict semantic versions such as `v0.1.0`. Check for a
newer published release with:

```bash
champu-pr update
```

When a newer release exists, `champu-pr` shows the installed and latest
versions and asks:

```text
Current version: v0.1.0
Latest version:  v0.2.0

A newer version of champu-pr is available.
Update now? [y/N]:
```

Only `y` or `yes` installs the displayed version. Any other response cancels
without changing the installed binary. After a successful update, run the
original `champu-pr` command again so it starts the new executable.

Development builds are never overwritten by `champu-pr update`. The update
command requires a published `vMAJOR.MINOR.PATCH` Git tag and installs that
exact version with `go install`.

The first preview is a file-wise changelog containing only major changes.
There is no mode flag to pass: changelog review starts automatically. Refine
the entries as needed, then run `make description` to produce concise PR
description bullets that correlate related changes across files.

The preview loop accepts normal-English refinement instructions. Keywords and
change IDs can help make a request more precise, but they are optional. For
example, all of these are valid:

```text
Leave out the documentation changes
Keep only the API and database changes
Bring the README change back
Combine the pricing changes into one point
```

Exact shortcuts are also available:

```text
exclude F2
include F2.C1
combine F1.C1 F1.C2
separate F1.C1
make the summary shorter
focus the title on F3.C1
tests passed: go test ./...
make description
reset
preview
```

After running `make description`, type the exact command `apply` to create or
update the PR. `apply` is rejected while the file-wise changelog is still being
reviewed. Type `quit`, press Ctrl+C, or send EOF to leave without changing
GitHub. `--dry-run` prints the file-wise changelog and never calls a GitHub
mutation.

Before the editable preview, the command asks which services are affected so
it can format the title as `[service][ticket] title`. Choose `api`, `worker`,
`api, worker`, or omit the service. The ticket is extracted from the current
branch name and normalized to uppercase; if no ticket is found, its brackets
remain empty.

## Develop

```bash
make test
make build
```

The workflow is divided into small boundaries:

1. `internal/gitcontext` collects repository facts and diff evidence.
2. `internal/github` discovers and publishes pull requests.
3. `internal/description` generates, refines, and renders the description.
4. `internal/application` coordinates preview, approval, and publishing.
5. `cmd/champu-pr` translates process input and errors into exit codes.
