# Releasing Agoraform

Agoraform versions follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

| Place | Form | Example |
| --- | --- | --- |
| SemVer identifier | `MAJOR.MINOR.PATCH` with optional pre-release/build metadata | `0.5.0`, `0.5.0-rc.1` |
| Git tag | `v` + SemVer identifier (Go module convention) | `v0.5.0` |
| `agoraform --version` | SemVer identifier, no `v` prefix | `0.5.0` |
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

## What v0.5.0 ships

v0.5.0 is a maturity and completeness release, not a new-provider release. It
keeps Matomo and Google Ads and completes the supported lifecycle for
resources present in the manifest and bound in state:

```text
validate -> plan -> apply -> plan -> import/adopt -> destroy
```

v0.5.0 ships:

- provider-neutral `agoraform destroy` with interactive confirmation or
  `--auto-approve`;
- Agoraform-managed `matomo.container` greenfield bootstrap, while
  `MATOMO_CONTAINER_ID` continues to select an externally managed container
  that destroy never deletes;
- `matomo.variable` `type: matomoConfiguration`;
- Matomo destroy for goals, Data Layer and Matomo Configuration variables,
  triggers, tags, and managed containers, including publication interaction;
- Google Ads destroy/remove for every v0.3/v0.4 resource type, with
  customer and campaign conversion goals remaining provider-owned;
- cross-provider `{ $ref, output }` selectors and import reconstruction of
  unique bound outputs;
- `matomo.tag` `type: googleAdsConversion` consuming Google Ads conversion
  ID and label;
- the complete `examples/matomo-googleads/` lifecycle example, plus the
  retained v0.1–v0.4 examples.

`agoraform destroy` is explicit. Removing a resource from the manifest does
not destroy or prune it. State-only identities are preserved. Destroy may
complete supported teardown while preserving provider-owned Google Ads
bindings; that run must report the remnants and return non-zero.

v0.5.0 does **not** add Meta Ads or other new providers, provision Matomo
websites, manage Google Ads billing or customer accounts, emit application
events, implement cross-provider transactional rollback, or add a drift
command.

Release archives contain the binary, license/notice files, README, changelog,
the v0.1 goal example, the v0.2 Matomo conversion example, the v0.3 Google
Ads conversion example, the v0.4 Google Ads Search campaign example, and the
complete v0.5 Matomo + Google Ads lifecycle example (including the
external-container variant).

## Platform matrix

- Linux amd64, arm64
- macOS amd64, arm64
- Windows amd64

Windows arm64 is omitted. Archives are `.tar.gz` except Windows (`.zip`). Each
release includes `checksums.txt`.

## Before tagging v0.5.0

Start from an up-to-date `main` after the release-preparation pull request is
merged and required CI is green.

1. Confirm [CHANGELOG.md](../CHANGELOG.md) contains the dated `0.5.0` release
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
4. Confirm the example test suite still validates every shipped example,
   including `examples/matomo-googleads/agoraform.yaml` and
   `examples/matomo-googleads/external/agoraform.yaml`.
5. Configure a **dedicated non-production Matomo site** and a
   **non-production Google Ads account** using the runtime credentials
   documented in [local configuration](local-configuration.md) and
   [Google Ads setup](google-ads-setup.md). Use a Google Ads test account
   when practical. Do not use production Matomo or advertiser accounts for
   release mutation verification.
6. Validate and plan the shipped v0.5 lifecycle example before creating the
   immutable tag:

   ```bash
   cp examples/matomo-googleads/agoraform.yaml ./agoraform.yaml
   agoraform validate
   agoraform plan
   ```

   Omit `MATOMO_CONTAINER_ID` for the managed-container manifest. `validate`
   must authenticate successfully without printing configured secrets.
   `plan` must succeed without mutation.

## Tagging v0.5.0

Create an annotated tag on the verified `main` commit and push only the tag:

```bash
git tag -a v0.5.0 -m "Agoraform 0.5.0"
git push origin v0.5.0
```

The release workflow listens for `v*`, validates strict SemVer 2.0 syntax,
checks that the tagged commit is reachable from `main`, repeats formatting,
tests, Windows state-test compilation, vet, and version-injection checks, and
then runs pinned GoReleaser 2.17.1.

A successful run creates a **draft** GitHub release. Do not publish the draft
until the manual verification below succeeds. A release binary built from
tag `v0.5.0` must report exactly `0.5.0`.

## Manual verification of the draft release

Use a clean machine or environment that does not rely on a binary from the git
worktree. Perform remote mutation checks only against the designated
non-production Matomo site and Google Ads account.

### 1. Verify the artifact and version

1. Download the archive for your OS/architecture plus `checksums.txt` from the
   draft GitHub release.
2. Verify the archive SHA-256 checksum before extracting it.
3. Extract the archive and place `agoraform` (`agoraform.exe` on Windows) on
   `PATH`.
4. Run:

   ```bash
   agoraform --version
   ```

   It must print exactly `0.5.0`, not `v0.5.0` and not `0.0.0-dev`.
5. Confirm the archive contains:

   ```text
   examples/agoraform.yaml
   examples/README.md
   examples/matomo-conversion/agoraform.yaml
   examples/matomo-conversion/README.md
   examples/googleads-conversion/agoraform.yaml
   examples/googleads-conversion/README.md
   examples/googleads-search/agoraform.yaml
   examples/googleads-search/README.md
   examples/matomo-googleads/agoraform.yaml
   examples/matomo-googleads/README.md
   examples/matomo-googleads/external/agoraform.yaml
   ```

### 2. Verify Matomo greenfield lifecycle

Start from a clean working directory with no existing `agoraform.state.json`
and no `MATOMO_CONTAINER_ID`. Copy the shipped managed-container example:

```bash
cp examples/matomo-googleads/agoraform.yaml ./agoraform.yaml
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Verify all of the following against the dedicated non-production Matomo site:

- Agoraform creates a Tag Manager container without a pre-existing
  `MATOMO_CONTAINER_ID`;
- it creates and manages a Matomo Configuration variable;
- it creates the dependent Data Layer variable, trigger, and Google Ads
  conversion tag;
- publication is visible in the plan and executes only when the converged
  draft differs from `live`;
- the post-apply plan reports `No changes.` with exit code `0`;
- repeating unchanged apply does not create a duplicate container version;
- no Matomo token or Google Ads secret appears in terminal output.

Then import equivalent existing resources from a separate clean working
directory with no state binding. Import the Google Ads conversion action
before the Matomo conversion tag. The unchanged imported graph must produce
no mutations and a zero-change plan.

Destroy the managed graph:

```bash
agoraform destroy
```

Confirm children are deleted in reverse dependency order, the managed
container is deleted, and an externally selected container is never part of
this run. Re-copy the [external-container variant](../examples/matomo-googleads/README.md#external-container-variant)
and confirm destroy removes children but protects the container selected by
`MATOMO_CONTAINER_ID`.

### 3. Verify Google Ads lifecycle

Against the same non-production Google Ads setup, confirm that existing
v0.3/v0.4 create/update/no-op/import behavior remains intact. The v0.4 Search
example remains the Search-campaign check:

```bash
cp examples/googleads-search/agoraform.yaml ./agoraform.yaml
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Then destroy that graph and verify:

- each supported resource has explicit destroy/remove/provider-owned
  semantics in the destroy plan;
- removable resources reach Google Ads `status=REMOVED`;
- customer and campaign conversion goals receive no delete mutation, remain
  in state, and cause a non-zero destroy result after supported teardown;
- reverse dependency ordering is respected (ads/keywords before ad group,
  campaign criteria before campaign, campaign before budget);
- destroy never enables serving or spend;
- paused, hidden, and enabled objects are removed; already-`REMOVED` objects
  converge idempotently.

Existing Search serving objects must remain `PAUSED` until `status` is
changed on purpose.

### 4. Verify cross-provider conversion tracking

Use the shipped v0.5 example to prove the Matomo + Google Ads workflow:

- `googleads.conversion_action` exposes the documented non-secret
  `conversionId` and `conversionLabel` outputs;
- the Matomo Google Ads conversion tag consumes those outputs through
  `{ $ref, output }`;
- the dependency graph spans providers deterministically (conversion action
  before the Matomo tag);
- apply converges to a zero-change plan;
- import/adoption reconstructs `{ $ref, output }` only on a unique
  bound-output match;
- teardown follows each provider's lifecycle semantics, including the
  provider-owned customer conversion goal remnant.

### 5. Verify destroy/state recovery

Manually exercise at least one partial-destroy failure (for example, revoke
credentials or interrupt after some resources have already reached a terminal
remote state) and verify:

- already-completed resources are removed from state only after a confirmed
  terminal or already-absent result;
- failed and unattempted resources remain in state;
- retry resumes safely from the remaining bindings;
- already-absent/removed resources converge idempotently;
- unsupported and provider-owned identities are not dropped from state.

Removing a resource from the copied manifest and running `plan` or `apply`
must not destroy that remote object. Destroy still requires the resource to
be present in both the manifest and local state.

### 6. Publish the GitHub release

Only after the artifact/version, Matomo greenfield, Google Ads lifecycle,
cross-provider conversion, zero-change planning, import-without-mutation,
destroy remnant reporting, and partial-destroy retry checks all pass should
the draft GitHub release be published. Then close issue #72 and milestone
0.5.0.

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
publishing a GitHub release. The example test suite automatically parses and
validates shipped example manifests, including
`examples/googleads-conversion/agoraform.yaml`,
`examples/googleads-search/agoraform.yaml`,
`examples/matomo-googleads/agoraform.yaml`, and
`examples/matomo-googleads/external/agoraform.yaml`, against the provider
schema. These checks catch cross-platform build, example-schema, and archive
configuration failures before an immutable tag is created.

## Out of scope for v0.5.0 distribution

Homebrew, Chocolatey, Scoop, OS package repositories, container images, hosted
services, new providers including Meta Ads, Matomo website/site provisioning,
Google Ads billing/customer-account lifecycle, Performance Max/Display/Video/
Shopping/App/Dynamic Search Ads campaign families, automated optimization,
creative generation, image/video assets, offline/call/app conversion
workflows, application instrumentation, drift detection, automatic
manifest-prune destruction, cross-provider transactional rollback, scheduled
publication, approval workflows, and remote state are not part of this
release.
