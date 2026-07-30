# champu-pr

`champu-pr` collects the Git changes on your current branch, asks Codex to
produce an evidence-backed pull-request description, lets you refine the
preview, and updates GitHub only after you type `apply`.

## Prerequisites

- Go
- Git
- [GitHub CLI](https://cli.github.com/) authenticated with `gh auth login`
- Codex CLI authenticated and available as `codex`

## Setup

Install `champu-pr`:

```bash
make install
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

## Use

Run the command from any directory inside your repository:

```bash
champu-pr
```

View every option and interactive command in the terminal:

```bash
champu-pr --help
```

By default, the command targets `main` and creates a draft pull request. Common
variants are:

```bash
champu-pr --base develop
champu-pr --pr 123
champu-pr --ready
champu-pr --dry-run
```

`--pr` selects one existing pull request by number. `--base` selects an
existing PR against that base branch or creates one when none exists. They
cannot be used together.

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

The first preview is a file-wise changelog containing only major changes.
There is no mode flag to pass: changelog review starts automatically. Refine
the entries as needed, then run `make description` to produce concise PR
description bullets that correlate related changes across files.

The preview loop accepts commands such as:

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
