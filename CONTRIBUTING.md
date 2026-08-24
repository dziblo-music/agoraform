# Contributing to Agoraform

Thanks for your interest in contributing to Agoraform.

Agoraform is an open-source Marketing Infrastructure as Code project written
in Go.

## Before starting

For significant changes, please work from an existing GitHub issue or open
one first so the proposed behavior can be discussed before substantial
implementation work begins.

Small documentation fixes and obvious bug fixes do not necessarily require a
separate issue.

## Development

### Prerequisites

- Go 1.23 or newer (`go version`)
- Git

Fork or clone the repository and create a branch from the latest `main`.

Branch names should follow:

    issue-<number>-<short-description>

For example:

    issue-12-google-conversion-action

### Build and run

From the repository root:

    go build -o agoraform ./cmd/agoraform
    ./agoraform --help
    ./agoraform --version

On Windows, the binary is typically `agoraform.exe`.

### Test and lint

    gofmt -w .
    go test ./...
    go vet ./...

CI also checks that `gofmt` would not change any files. Run `gofmt -l .` locally and ensure it prints nothing before opening a pull request.

Do not commit API tokens, credentials, personal data, or machine-local paths.

The v0.1 manifest format is documented in [docs/manifest.md](docs/manifest.md).
Local identity state is documented in [docs/state.md](docs/state.md).

## Pull requests

Open pull requests against `main`.

Keep each PR focused on one issue or logical change.

Include:

- the related issue;
- a summary of the implementation;
- tests performed;
- important design decisions;
- documentation changes where applicable.

Use:

    Closes #123

when the PR should automatically close an issue after merge.

All required CI checks must pass before merge.

## Design principles

Agoraform prioritizes:

- declarative configuration;
- deterministic plans;
- safe infrastructure changes;
- provider-independent core architecture;
- provider-native capabilities;
- reviewable Git-based workflows;
- secure handling of credentials.

`agoraform plan` must never mutate provider resources.

Destructive operations should not be introduced without explicit design and
safety consideration.

## Provider contributions

Provider implementations should keep provider-specific API behavior inside
their provider package.

Tests must not depend on production accounts or credentials.

Never commit API tokens, account secrets, or real customer data.

## Compatibility

Agoraform currently uses pre-1.0 semantic versioning.

The manifest format and CLI may evolve while the project is in the `0.x`
release series. Breaking changes should nevertheless be intentional and
documented.