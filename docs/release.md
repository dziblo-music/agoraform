# Releasing Agoraform

Agoraform versions follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

| Place | Form | Example |
| --- | --- | --- |
| SemVer identifier | `MAJOR.MINOR.PATCH` with optional pre-release | `0.1.0`, `0.1.0-rc.1` |
| Git tag | `v` + SemVer identifier (Go module convention) | `v0.1.0` |
| `agoraform --version` | SemVer identifier, no `v` prefix | `0.1.0` |
| Untagged local/CI builds | pre-release of `0.0.0` | `0.0.0-dev` |

Until 1.0.0, a MINOR bump in the `0.x` series may include breaking CLI or
manifest changes. Those changes must still be intentional and documented in
[CHANGELOG.md](../CHANGELOG.md).

Do not hard-code release numbers in Go source. Release builds inject the
version with `-ldflags`.

## What 0.1.0 ships

0.1.0 is `validate`, `plan`, `apply`, and `import` for `matomo.goal` only,
with local identity state. It does not include other providers, Tag Manager
resources, destroy, or remote state. See [CHANGELOG.md](../CHANGELOG.md).

## Cutting a release

Do this from an up-to-date `main` after required CI is green.

1. Confirm [CHANGELOG.md](../CHANGELOG.md) describes the version you are tagging.
2. Run locally:

   ```bash
   gofmt -l .
   go test ./...
   go vet ./...
   go build -o agoraform ./cmd/agoraform
   ```

   `gofmt -l .` must print nothing.

3. Tag annotated SemVer and push **only the tag**:

   ```bash
   git tag -a v0.1.0 -m "Agoraform 0.1.0"
   git push origin v0.1.0
   ```

   Pushing a tag matching `vMAJOR.MINOR.PATCH` (optional pre-release suffix)
   runs [.github/workflows/release.yml](../.github/workflows/release.yml).
   That workflow runs the same Go checks as CI, builds with the tag version
   injected, then GoReleaser publishes **draft** artifacts.

4. Do **not** publish the GitHub release yet.

## Manual verification (required before publish)

Use a machine that does not rely on your git worktree binary.

1. Download the archive for your OS/arch from the draft GitHub release.
2. Verify SHA-256 against `checksums.txt`.
3. Place `agoraform` (or `agoraform.exe`) on `PATH`.
4. Confirm `agoraform --version` prints the SemVer identifier (`0.1.0`, not `v0.1.0` and not `0.0.0-dev`).
5. Against a **non-production** Matomo site, with
   `MATOMO_URL`, `MATOMO_TOKEN_AUTH`, and `MATOMO_SITE_ID` set, copy
   [examples/agoraform.yaml](../examples/agoraform.yaml) and run:

   ```text
   validate -> plan -> apply -> plan
   ```

   The second `plan` must report no changes while the remote goal and local
   state are unchanged (`plan` exit code `0`).
6. Import an existing Matomo goal id, add the printed YAML to the manifest,
   and confirm `plan` reports no changes.

`validate` and `plan` against the example **require live Matomo credentials**.
They are a manual gate, not a CI job.

After that verification succeeds, publish the draft GitHub release.

## Platform matrix (0.1.0)

- Linux amd64, arm64
- macOS amd64, arm64
- Windows amd64

Windows arm64 is omitted. Archives are `.tar.gz` except Windows (`.zip`).
Each release includes `checksums.txt`.

## Release config checks

CI runs `goreleaser check` so `.goreleaser.yaml` stays valid on pull
requests. A snapshot publish is not performed.

## Out of scope for this process

Homebrew, Chocolatey, Scoop, OS package repositories, container images,
and hosted services are not part of 0.1.0 distribution.
