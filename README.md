# champu-pr

`champu-pr` collects the Git changes on your current branch, asks Codex to
produce an evidence-backed pull-request description, lets you refine the
preview, and updates GitHub only after you type `apply`.

## Prerequisites

- Go
- Git
- [GitHub CLI](https://cli.github.com/) authenticated with `gh auth login`
- Codex CLI authenticated and available as `codex`

## Install

```bash
make install
```

Make sure Go's binary directory is on your `PATH`. You can inspect it with:

```bash
go env GOBIN GOPATH
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

The preview loop accepts commands such as:

```text
exclude F2
include F2.C1
combine F1.C1 F1.C2
separate F1.C1
make the summary shorter
focus the title on F3.C1
tests passed: go test ./...
reset
preview
```

Type the exact command `apply` to create or update the PR. Type `quit`, press
Ctrl+C, or send EOF to leave without changing GitHub. `--dry-run` prints one
preview and never calls a GitHub mutation.

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
