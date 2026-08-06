# champu-pr

`champu-pr` collects Git changes from the current branch or a selected pull
request, asks Codex to produce an evidence-backed pull-request description,
lets you refine the preview, and updates GitHub only after you type `apply`.

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
