# PRD: Rename the CLI Command to `champu`

## Objective

Make `champu` the canonical executable name instead of `champu-pr`, and replace
the required `make description` workflow command with a short confirmation
prompt.

## User-facing commands

The supported command examples become:

```text
champu
champu branch
champu branch cleanup
champu review 123
champu update
champu --version
```

## Description workflow

After showing the editable changelog, Champu asks:

```text
Do you want Champu to create the PR description? [y/N]:
```

The prompt accepts:

- `y` or `yes`, case-insensitively, to create and display the PR description.
- `n`, `no`, or an empty response to remain in changelog mode and request
  another refinement.
- `quit` to exit without changing GitHub.

After the description is created, the user can refine it, enter `apply`, or
quit. `apply` remains unavailable until the user answers `y` or `yes`.

The old `make description` command remains an undocumented compatibility alias
for one release. The proposed `describe` command is not introduced because the
confirmation requires less typing and makes the transition explicit.

## Installation

Install from a source checkout with:

```bash
go install ./cmd/champu
```

Install a published GitHub release with:

```bash
go install github.com/Merthoshan/PR-maker-CLI/cmd/champu@latest
```

The Makefile builds `bin/champu`.

## Compatibility

This is a clean executable rename:

- `cmd/champu-pr` becomes `cmd/champu`.
- Current documentation uses only `champu`.
- Existing `champu-pr` binaries are not automatically removed from user
  machines.
- Users upgrading from `v0.3.0` install `cmd/champu` once using the published
  GitHub command.

The following internal compatibility identifiers remain unchanged:

- `.champu-pr/` configuration directory.
- `refs/champu-pr/...` temporary Git references.
- Temporary-file prefixes.

Changing these identifiers would not improve the command experience and could
break existing configuration or cleanup behavior.

## Updater behavior

`champu update`:

- Queries the existing GitHub module.
- Installs `github.com/Merthoshan/PR-maker-CLI/cmd/champu@<version>`.
- Uses `champu` in user-facing messages and instructions.
- Continues refusing to overwrite untagged development builds.

## Documentation and tests

The implementation updates:

- `champu --help`.
- README installation, migration, workflow, and usage examples.
- Makefile build and install targets.
- Updater package paths and messages.
- GitHub pull-request selection guidance.
- `AGENTS.md` command and project-map references.
- CLI, application, updater, and integration tests.
- Interactive workflow tests for `y`, `yes`, capitalization, whitespace,
  negative responses, cancellation, premature `apply`, and the legacy alias.

Historical PRDs remain unchanged because they describe the command names used
when those features were designed.

## Version

The latest published semantic tag is `v0.3.0`. Changing the installed
executable path is a breaking change, so the target release is `v1.0.0`.

## Acceptance criteria

- `go install .../cmd/champu@v1.0.0` installs a `champu` executable.
- `champu --help` contains no current `champu-pr` examples.
- `y` and `yes` switch from changelog mode to the publishable description.
- Negative or empty answers return to refinement without publishing.
- `apply` still requires the description step.
- The hidden `make description` alias continues working for one release.
- `champu update` installs the new executable path.
- Existing configuration and internal Git references remain compatible.
- Tests, vet, race tests, build, and diff checks pass.
