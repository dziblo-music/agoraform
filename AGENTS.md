# Agoraform Agent Instructions

## Project

Agoraform is an open-source Marketing Infrastructure as Code CLI written in Go.

The core workflow is:

    agoraform validate
    agoraform import
    agoraform plan
    agoraform apply

The project should favor predictable, reviewable infrastructure changes over convenience.

## Development workflow

When implementing a GitHub issue:

1. Read the entire issue, including dependencies and acceptance criteria.

2. Start from an up-to-date `main` branch.

3. Do not implement changes directly on `main`.

4. Create a branch using:

       issue-<number>-<short-description>

   Example:

       issue-1-scaffold-cli
       issue-4-matomo-client

5. Implement only the scope of the issue.

   Do not silently add unrelated features, refactors, providers, or abstractions.

6. If implementation reveals that the issue design is incorrect, prefer the
   simplest architecture that satisfies the issue and document the deviation
   in the pull request.

7. Add or update automated tests for changed behavior.

8. Before opening a pull request, run:

       gofmt
       go test ./...
       go vet ./...

   Run any additional repository checks configured by CI.

9. Update documentation when public CLI behavior, manifest syntax, provider
   behavior, or contributor workflow changes.

10. Open a pull request against `main`.

11. Link the issue using:

       Closes #<issue-number>

12. Do not merge the pull request until required CI checks pass.

## Pull requests

Pull requests should be focused and small enough to review.

The PR description should explain:

- what changed;
- why;
- how it was tested;
- any architecture or design decisions;
- any behavior intentionally left out of scope.

Avoid mixing unrelated cleanup with feature work.

## Go conventions

- Use standard Go formatting.
- Prefer standard-library functionality unless a dependency provides clear value.
- Keep packages focused.
- Avoid unnecessary abstractions.
- Return useful contextual errors.
- Pass `context.Context` through network/provider operations.
- Keep provider-specific API types out of the generic core.
- Keep secrets out of logs, manifests, plan output, test fixtures, and errors.

## Agoraform architecture

The core must remain provider-independent.

Provider-specific behavior belongs under provider packages.

Prefer provider-native resource models over premature lowest-common-denominator
abstractions.

The plan engine must never perform mutations.

Do not introduce destructive reconciliation unless explicitly required by an
approved issue.

v0.1 has local identity state in `agoraform.state.json`. Do not add remote
backends, workspaces, locking, encryption, or Terraform-compatible state
unless a later issue requires them.

## Provider development

Provider tests must not call production services.

Use mocks, fake transports, `httptest`, or equivalent test infrastructure.

API tokens and credentials must never be committed to the repository.

Provider errors should be sanitized so credentials cannot appear in logs or
terminal output.

## Scope discipline

Agoraform is currently pre-1.0.

Breaking internal changes are acceptable when they materially simplify the
design, but public CLI and manifest changes should be intentional and documented.

Do not implement speculative future features simply because the architecture
could support them.