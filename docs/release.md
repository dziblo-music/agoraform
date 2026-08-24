# Releasing Agoraform

Agoraform versions follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

| Place | Form | Example |
| --- | --- | --- |
| SemVer identifier | `MAJOR.MINOR.PATCH` with optional pre-release/build metadata | `0.1.0`, `0.1.0-rc.1` |
| Git tag | `v` + SemVer identifier (Go module convention) | `v0.1.0` |
| `agoraform --version` | SemVer identifier, no `v` prefix | `0.1.0` |
| Untagged local/CI builds | pre-release of `0.0.0` | `0.0.0-dev` |

Until 1.0.0, a MINOR bump in the `0.x` series may include breaking CLI or
manifest changes. Those changes must still be intentional and documented in
[CHANGELOG.md](../CHANGELOG.md).

Do not hard-code release numbers in Go source. Release builds inject the
version with `-ldflags`.

## Release toolchain

The 0.1.0 release process intentionally pins the build and release toolchain:

- Go 1.26.7
- GoReleaser 2.17.1
- GitHub Actions are referenced by immutable full commit SHAs; comments beside
  each SHA record the reviewed action release version

Updating one of these pins should be a reviewed repository change rather than
an implicit change during a release run.

## What 0.1.0 ships

0.1.0 is `validate`, `plan`, `apply`, and `import` for `matomo.goal` only,
with local identity state. It does not include other providers, Tag Manager
resources, destroy, or remote state. See [CHANGELOG.md](../CHANGELOG.md).

Release archives contain the binary, license/notice files, README, changelog,
and `examples/agoraform.yaml` so the documented quickstart works from a clean
binary download.

## Cutting a release

Do this from an up-to-date `main` after required CI is green.

1. Confirm [CHANGELOG.md](../CHANGELOG.md) describes the version you are tagging.
2. Update local `main` and confirm the commit you intend to tag is on `main`:

   ```bash
   git switch main
   git pull --ff-only
   git status --short
   ```

   `git status --short` should be empty.

3. Run locally with Go 1.26.7:

   ```bash
   go version
   gofmt -l .
   go test ./...
   go vet ./...
   go build -o agoraform ./cmd/agoraform
   ```

   `go version` must report Go 1.26.7 or newer and `gofmt -l .` must print
   nothing.

4. Tag annotated SemVer and push **only the tag**:

   ```bash
   git tag -a v0.1.0 -m "Agoraform 0.1.0"
   git push origin v0.1.0
   ```

   The release workflow listens for `v*`, then performs strict SemVer 2.0
   validation before any release build. Invalid `v` tags fail verification.
   The workflow also fetches `main` and rejects a tag whose commit is not
   reachable from `main`.

5. The release workflow repeats formatting, tests, Windows state compilation,
   vet, and version-injection checks. Only after verification succeeds does
   pinned GoReleaser 2.17.1 create the multi-platform artifacts and a **draft**
   GitHub release.

6. Do **not** publish the GitHub release yet.

## Manual verification (required before publish)

Use a clean machine or environment that does not rely on your git worktree
binary.

1. Download the archive for your OS/arch and `checksums.txt` from the draft
   GitHub release.
2. Verify SHA-256 before extracting or running the binary. See the OS-specific
   commands in the [README](../README.md#verify-checksums).
3. Extract the archive and place `agoraform` (or `agoraform.exe`) on `PATH`.
4. Confirm `agoraform --version` prints the SemVer identifier (`0.1.0`, not
   `v0.1.0` and not `0.0.0-dev`).
5. Confirm the extracted archive contains `examples/agoraform.yaml`.
6. Against a **non-production** Matomo site, with `MATOMO_URL`,
   `MATOMO_TOKEN_AUTH`, and `MATOMO_SITE_ID` set, copy the included example
   manifest to `agoraform.yaml` and run:

   ```text
   validate -> plan -> apply -> plan
   ```

   The second `plan` must report no changes while the remote goal and local
   state are unchanged (`plan` exit code `0`).
7. Import an existing Matomo goal id, add the printed YAML to the manifest,
   and confirm `plan` reports no changes.

`validate`, `plan`, `apply`, and `import` require live Matomo configuration for
this end-to-end verification. Live-provider checks are a manual release gate,
not a CI job.

After that verification succeeds, publish the draft GitHub release and then
close issue #10 / milestone 0.1.0 as appropriate.

## Platform matrix (0.1.0)

- Linux amd64, arm64
- macOS amd64, arm64
- Windows amd64

Windows arm64 is omitted. Archives are `.tar.gz` except Windows (`.zip`).
Each release includes `checksums.txt`.

## Release config checks in CI

Every pull request and push to `main` runs:

```text
gofmt check
go test ./...
Windows state-test compile
go vet ./...
goreleaser check
goreleaser release --snapshot --clean
```

The snapshot build uses the same `.goreleaser.yaml` packaging matrix without
publishing a GitHub release. This catches cross-platform build or archive
configuration failures before an immutable release tag is created.

## Out of scope for this process

Homebrew, Chocolatey, Scoop, OS package repositories, container images,
and hosted services are not part of 0.1.0 distribution.
