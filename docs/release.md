# Releasing Agoraform

Agoraform versions follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

| Place | Form | Example |
| --- | --- | --- |
| SemVer identifier | `MAJOR.MINOR.PATCH` with optional pre-release/build metadata | `0.3.0`, `0.3.0-rc.1` |
| Git tag | `v` + SemVer identifier (Go module convention) | `v0.3.0` |
| `agoraform --version` | SemVer identifier, no `v` prefix | `0.3.0` |
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

## What v0.3.0 ships

v0.3.0 keeps the provider-neutral commands `validate`, `plan`, `apply`, and
`import`, retains the v0.2 Matomo functionality, and adds Google Ads conversion
measurement:

- authenticated `googleads` provider access using developer-token + OAuth
  single-user credentials supplied at runtime;
- `googleads.conversion_action` for supported website (`WEBPAGE`) conversion
  actions such as `SIGNUP` / Trial Started;
- `googleads.customer_conversion_goal` for customer-level website goal
  `biddable` reconciliation;
- deterministic import/adoption of supported existing conversion actions and
  customer conversion-goal settings without mutation;
- computed provider-native identity and conversion metadata kept out of normal
  manifest configuration;
- the complete `examples/googleads-conversion/` quickstart.

v0.3.0 does **not** manage campaigns, budgets, ad groups, keywords, targeting,
ads, call/offline/app conversions, conversion-event uploads, website tags, or
application-side event emission. Service-account and interactive OAuth flows are
also not implemented by Agoraform v0.3.0.

Release archives contain the binary, license/notice files, README, changelog,
the v0.1 goal example, the v0.2 Matomo conversion example, and the complete
v0.3 Google Ads conversion example.

## Platform matrix

- Linux amd64, arm64
- macOS amd64, arm64
- Windows amd64

Windows arm64 is omitted. Archives are `.tar.gz` except Windows (`.zip`). Each
release includes `checksums.txt`.

## Before tagging v0.3.0

Start from an up-to-date `main` after the release-preparation pull request is
merged and required CI is green.

1. Confirm [CHANGELOG.md](../CHANGELOG.md) contains the dated `0.3.0` release
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
4. Configure a **non-production Google Ads account** and the runtime credentials
   documented in [Google Ads setup](google-ads-setup.md). Use a Google Ads test
   account when practical. Do not use a production advertiser for release
   mutation verification.
5. Validate and plan the shipped v0.3 example before creating the immutable tag:

   ```bash
   cp examples/googleads-conversion/agoraform.yaml ./agoraform.yaml
   agoraform validate
   agoraform plan
   ```

   `validate` must authenticate successfully without printing configured
   secrets. `plan` must succeed without mutation and show the expected
   conversion-action / conversion-goal changes when the account is not already
   equivalent.

## Tagging v0.3.0

Create an annotated tag on the verified `main` commit and push only the tag:

```bash
git tag -a v0.3.0 -m "Agoraform 0.3.0"
git push origin v0.3.0
```

The release workflow listens for `v*`, validates strict SemVer 2.0 syntax,
checks that the tagged commit is reachable from `main`, repeats formatting,
tests, Windows state-test compilation, vet, and version-injection checks, and
then runs pinned GoReleaser 2.17.1.

A successful run creates a **draft** GitHub release. Do not publish the draft
until the manual verification below succeeds.

## Manual verification of the draft release

Use a clean machine or environment that does not rely on a binary from the git
worktree. Perform remote mutation checks only against the designated
non-production Google Ads account.

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

   It must print exactly `0.3.0`, not `v0.3.0` and not `0.0.0-dev`.
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
   ```

### 2. Verify authentication and non-mutating planning

Start from a clean working directory with no existing `agoraform.state.json`.
Copy the shipped example and configure the non-production account:

```bash
cp examples/googleads-conversion/agoraform.yaml ./agoraform.yaml
agoraform validate
agoraform plan
```

Verify all of the following:

- authentication succeeds using the configured developer token and OAuth
  credentials;
- no developer token, client secret, refresh token, or access token appears in
  terminal output or provider diagnostics;
- `plan` performs no mutation;
- the plan is deterministic when repeated against unchanged state.

### 3. Verify create, goal reconciliation, and no-op

Review the plan, then run:

```bash
agoraform apply
agoraform plan
```

Verify in Google Ads that:

- **Trial Started** exists as a website `SIGNUP` conversion action with the
  example's supported settings;
- the `SIGNUP` / `WEBSITE` customer conversion goal is biddable;
- Agoraform did not attempt to create or delete the provider-created customer
  conversion-goal object;
- the post-apply plan reports `No changes.` with exit code `0`.

### 4. Verify an update and return to no-op

Temporarily change one supported mutable field in the test manifest, for
example `value: 0` to `value: 1`.

Run:

```bash
agoraform plan
agoraform apply
agoraform plan
```

Confirm the first plan shows an update, the remote conversion action changes,
and the final plan reports `No changes.`. Restore the example value and repeat
`plan -> apply -> plan` so the non-production account returns to the shipped
example state.

### 5. Verify import/adoption without mutation

Use an equivalent existing website conversion action in the non-production
account. This may be one created manually in Google Ads or the action created
above, provided the import test runs from a separate clean working directory
with no state binding.

Import the conversion action using its numeric ID or resource name:

```bash
agoraform import googleads.conversion_action.trial_started CONVERSION_ACTION_ID
```

Confirm that import:

- records the remote identity locally and prints canonical configurable YAML;
- does not mutate the Google Ads conversion action;
- omits credentials, resource names, tag snippets, and other computed metadata
  from normal configuration.

Reconcile the printed attributes with the shipped example, then immediately
run:

```bash
agoraform plan
```

The unchanged imported conversion action must produce no conversion-action
change.

Also verify the supported customer conversion goal when practical:

```bash
agoraform import googleads.customer_conversion_goal.signup SIGNUP~WEBSITE
agoraform plan
```

The import must not create/delete the goal, and equivalent unchanged state must
plan as no-op.

### 6. Publish the GitHub release

Only after the artifact/version, authentication/secret-safety, create, update,
customer-goal reconciliation, zero-change planning, and import-without-mutation
checks all pass should the draft GitHub release be published. Then close issue
#36 and milestone 0.3.0.

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
validates `examples/googleads-conversion/agoraform.yaml` and
`examples/googleads-search/agoraform.yaml` against the provider
schema. These checks catch cross-platform build, example-schema, and archive
configuration failures before an immutable tag is created.

## Out of scope for v0.3.0 distribution

Homebrew, Chocolatey, Scoop, OS package repositories, container images, hosted
services, new providers, Google Ads campaign-management resources, offline/call
/app conversion workflows, application instrumentation, rollback, scheduled
publication, approval workflows, and remote state are not part of this release.
