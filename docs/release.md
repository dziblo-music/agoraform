# Releasing Agoraform

Agoraform versions follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

| Place | Form | Example |
| --- | --- | --- |
| SemVer identifier | `MAJOR.MINOR.PATCH` with optional pre-release/build metadata | `0.2.0`, `0.2.0-rc.1` |
| Git tag | `v` + SemVer identifier (Go module convention) | `v0.2.0` |
| `agoraform --version` | SemVer identifier, no `v` prefix | `0.2.0` |
| Untagged local/CI builds | pre-release of `0.0.0` | `0.0.0-dev` |

Until 1.0.0, a MINOR bump in the `0.x` series may include breaking CLI or
manifest changes. Those changes must still be intentional and documented in
[CHANGELOG.md](../CHANGELOG.md).

Do not hard-code release numbers in Go source. Release builds inject the
version with `-ldflags`.

## Release toolchain

The release process pins the build and release toolchain:

- Go 1.26.7
- GoReleaser 2.17.1
- GitHub Actions referenced by immutable full commit SHAs, with reviewed action
  versions recorded in comments

Updating one of these pins is a reviewed repository change, not an implicit
part of cutting a release.

## What v0.2.0 ships

v0.2.0 keeps the provider-neutral commands `validate`, `plan`, `apply`, and
`import` and expands the Matomo provider with:

- `matomo.goal` analytics goals;
- `matomo.variable` Data Layer variables;
- `matomo.trigger` Custom Event triggers;
- `matomo.tag` Matomo Analytics event tags;
- logical `$ref` dependencies with prerequisite-first planning and apply;
- import for the supported Tag Manager resources;
- declarative Tag Manager publication through `providers.matomo.publish` and
  `providers.matomo.environment`.

There is no provider-specific `agoraform publish` command. When publication is
enabled, `plan` exposes the provider action and `apply` performs publication
only after draft resource changes succeed and a final comparison confirms that
publication is still necessary.

Release archives contain the binary, license/notice files, README, changelog,
the v0.1 goal example, and the complete v0.2 conversion example under
`examples/matomo-conversion/`.

## Platform matrix

- Linux amd64, arm64
- macOS amd64, arm64
- Windows amd64

Windows arm64 is omitted. Archives are `.tar.gz` except Windows (`.zip`). Each
release includes `checksums.txt`.

## Before tagging v0.2.0

Start from an up-to-date `main` after the release-preparation pull request is
merged and required CI is green.

1. Confirm [CHANGELOG.md](../CHANGELOG.md) contains the dated `0.2.0` release
   notes and no release-blocking work remains open.
2. Update local `main` and confirm the intended tag commit is clean and on
   `main`:

   ```bash
   git switch main
   git pull --ff-only
   git status --short
   ```

   `git status --short` must be empty.
3. Run the local release checks with Go 1.26.7 or newer:

   ```bash
   go version
   gofmt -l .
   go test ./...
   go vet ./...
   go build -o agoraform ./cmd/agoraform
   ./agoraform --version
   ```

   `gofmt -l .` must print nothing. An untagged local build is expected to
   print `0.0.0-dev`; tag-version injection is verified by the release workflow.
4. With a non-production Matomo site/container configured, validate the v0.2
   example before creating the immutable tag:

   ```bash
   export MATOMO_URL=https://matomo.example.com
   export MATOMO_TOKEN_AUTH=replace-with-test-token
   export MATOMO_SITE_ID=1
   export MATOMO_CONTAINER_ID=replace-with-test-container

   cp examples/matomo-conversion/agoraform.yaml ./agoraform.yaml
   agoraform validate
   agoraform plan
   ```

   The plan must succeed and show the expected variable, trigger, tag, and
   publication action when those changes are required.

## Tagging v0.2.0

Create an annotated tag on the verified `main` commit and push only the tag:

```bash
git tag -a v0.2.0 -m "Agoraform 0.2.0"
git push origin v0.2.0
```

The release workflow listens for `v*`, validates strict SemVer 2.0 syntax,
checks that the tagged commit is reachable from `main`, repeats formatting,
tests, Windows state-test compilation, vet, and version-injection checks, and
then runs pinned GoReleaser 2.17.1.

A successful run creates a **draft** GitHub release. Do not publish the draft
until the manual verification below succeeds.

## Manual verification of the draft release

Use a clean machine or environment that does not rely on a binary from the git
worktree.

### 1. Verify the artifact

1. Download the archive for your OS/architecture plus `checksums.txt` from the
   draft GitHub release.
2. Verify the archive SHA-256 checksum before extracting it.
3. Extract the archive and place `agoraform` (`agoraform.exe` on Windows) on
   `PATH`.
4. Run:

   ```bash
   agoraform --version
   ```

   It must print exactly `0.2.0`, not `v0.2.0` and not `0.0.0-dev`.
5. Confirm the archive contains:

   ```text
   examples/agoraform.yaml
   examples/README.md
   examples/matomo-conversion/agoraform.yaml
   examples/matomo-conversion/README.md
   ```

### 2. Verify draft-only Tag Manager apply

Use a non-production Matomo site/container and credentials allowed to manage
Tag Manager resources. Start from a clean working directory/state file.

Copy the v0.2 example, then temporarily set:

```yaml
providers:
  matomo:
    publish: false
    environment: live
```

Run:

```text
validate -> plan -> apply -> plan
```

Verify in Matomo that the Data Layer variable, Custom Event trigger, and Matomo
Analytics event tag are created/updated in the draft in dependency order and
that no new container version is published.

### 3. Verify declarative publication

Set `providers.matomo.publish: true`, keep the intended test environment, then
run:

```text
plan -> apply -> plan
```

Verify all of the following:

- the pre-apply plan visibly includes the publication action when publication
  is required;
- draft resource mutations complete before version creation/publication;
- a new Tag Manager container version is created and published to the target
  environment;
- the final plan reports `No changes.` with exit code `0` when desired and
  remote state are unchanged;
- a repeated unchanged `apply` does not create another container version.

Use Matomo Tag Manager preview/debug mode if useful to confirm that the
`trialStarted` event fires the managed tag.

### 4. Verify import without mutation

Against existing supported Tag Manager resources, import dependencies before
the tag:

```bash
agoraform import matomo.variable.user_id VARIABLE_ID
agoraform import matomo.trigger.trial_started TRIGGER_ID
agoraform import matomo.tag.trial_started TAG_ID
```

Confirm the commands reconstruct usable logical configuration/state without
mutating Matomo, then reconcile the printed attributes with the example and
verify `agoraform plan` reports no changes.

### 5. Publish the GitHub release

Only after the artifact, draft-only apply, publication/idempotency, zero-change
plan, and import checks all pass should the draft GitHub release be published.
Then close issue #19 and milestone 0.2.0.

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

The snapshot uses the same `.goreleaser.yaml` platform/archive matrix without
publishing a GitHub release. This catches cross-platform build and archive
configuration failures before an immutable tag is created.

## Out of scope for v0.2.0 distribution

Homebrew, Chocolatey, Scoop, OS package repositories, container images, hosted
services, new providers, additional Tag Manager resource types, rollback,
scheduled publication, approval workflows, and multi-container deployment
orchestration are not part of this release.
