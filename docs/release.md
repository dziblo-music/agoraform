# Releasing Agoraform

Agoraform versions follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

| Place | Form | Example |
| --- | --- | --- |
| SemVer identifier | `MAJOR.MINOR.PATCH` with optional pre-release/build metadata | `0.4.0`, `0.4.0-rc.1` |
| Git tag | `v` + SemVer identifier (Go module convention) | `v0.4.0` |
| `agoraform --version` | SemVer identifier, no `v` prefix | `0.4.0` |
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

## What v0.4.0 ships

v0.4.0 keeps the provider-neutral commands `validate`, `plan`, `apply`, and
`import`, retains the v0.2 Matomo functionality and v0.3 Google Ads
conversion measurement, and adds complete Search campaign management:

- authenticated `googleads` provider access using developer-token + OAuth
  single-user credentials supplied at runtime;
- `googleads.campaign_budget` for daily Search budgets;
- `googleads.campaign` for paused Search campaigns with supported bidding
  and network settings;
- `googleads.campaign_conversion_goal` for campaign-level website goal
  `biddable` reconciliation;
- `googleads.ad_group` for Search standard ad groups;
- `googleads.keyword` for positive and negative Search keywords;
- `googleads.campaign_location` and `googleads.campaign_language` for
  campaign targeting;
- `googleads.responsive_search_ad` for paused Responsive Search Ads;
- deterministic import/adoption of each supported Search resource without
  mutation, with parents imported first so logical `$ref` values can be
  reconstructed;
- computed provider-native identity kept out of normal manifest
  configuration;
- the complete `examples/googleads-search/` quickstart.

v0.4.0 does **not** manage Performance Max, Display, Video, Shopping, App,
or Dynamic Search Ads campaigns. It also does not generate creative, upload
image or video assets, enable spend automatically, apply optimization
recommendations, manage call/offline/app conversions, upload conversion
events, install website tags, or emit application-side events.
Service-account and interactive OAuth flows are also not implemented.

Release archives contain the binary, license/notice files, README, changelog,
the v0.1 goal example, the v0.2 Matomo conversion example, the v0.3 Google
Ads conversion example, and the complete v0.4 Google Ads Search campaign
example.

## Platform matrix

- Linux amd64, arm64
- macOS amd64, arm64
- Windows amd64

Windows arm64 is omitted. Archives are `.tar.gz` except Windows (`.zip`). Each
release includes `checksums.txt`.

## Before tagging v0.4.0

Start from an up-to-date `main` after the release-preparation pull request is
merged and required CI is green.

1. Confirm [CHANGELOG.md](../CHANGELOG.md) contains the dated `0.4.0` release
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
5. Validate and plan the shipped v0.4 Search example before creating the
   immutable tag:

   ```bash
   cp examples/googleads-search/agoraform.yaml ./agoraform.yaml
   agoraform validate
   agoraform plan
   ```

   `validate` must authenticate successfully without printing configured
   secrets. `plan` must succeed without mutation and show the expected Search
   campaign graph, in dependency order, when the account is not already
   equivalent. New campaigns, ad groups, positive keywords, and Responsive
   Search Ads must remain `PAUSED`.

## Tagging v0.4.0

Create an annotated tag on the verified `main` commit and push only the tag:

```bash
git tag -a v0.4.0 -m "Agoraform 0.4.0"
git push origin v0.4.0
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

   It must print exactly `0.4.0`, not `v0.4.0` and not `0.0.0-dev`.
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
Copy the shipped Search example and configure the non-production account:

```bash
cp examples/googleads-search/agoraform.yaml ./agoraform.yaml
agoraform validate
agoraform plan
```

Verify all of the following:

- authentication succeeds using the configured developer token and OAuth
  credentials;
- no developer token, client secret, refresh token, or access token appears in
  terminal output or provider diagnostics;
- `plan` performs no mutation;
- the plan is deterministic when repeated against unchanged state;
- planned creates follow the `$ref` graph: conversion action and budget
  before campaign; campaign before targeting, conversion-goal reconciliation,
  and ad group; ad group before keywords and the Responsive Search Ad.

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
- the dedicated daily budget and paused **Search acquisition** campaign exist
  with Maximize conversions bidding;
- the campaign conversion goal keeps `SIGNUP` / `WEBSITE` biddable;
- United States location targeting and English language targeting exist;
- the paused **Trial** ad group, explicit match-type keywords, negatives, and
  paused Responsive Search Ad exist;
- Agoraform did not attempt to create or delete provider-created conversion
  goals;
- campaigns, ad groups, positive keywords, and the Responsive Search Ad remain
  `PAUSED`;
- the post-apply plan reports `No changes.` with exit code `0`.

### 4. Verify updates, replacement visibility, and return to no-op

Temporarily change supported mutable fields one at a time, then restore the
shipped example after each cycle with `plan -> apply -> plan`:

- campaign budget `amount`;
- campaign name or a supported bidding setting;
- campaign conversion-goal `biddable`;
- ad group name or optional `cpcBid`;
- positive keyword status;
- Responsive Search Ad status, then a creative list such as one headline.

Confirm each first plan shows an update, the remote object changes, and the
final plan reports `No changes.` Creative Responsive Search Ad changes must
appear as list replacements rather than hidden asset-ID churn.

Also confirm that an immutable identity change fails planning before mutation.
For example, changing a keyword `matchType` or a location value must error
instead of planning a destroy-and-recreate. Agoraform must not enable spend
unless `status` is changed on purpose.

### 5. Verify import/adoption without mutation

Use the equivalent Search campaign graph in the non-production account. This
may be the graph created above, provided the import test runs from a separate
clean working directory with no state binding.

Import parents first:

```bash
agoraform import googleads.conversion_action.trial_started CONVERSION_ACTION_ID
agoraform import googleads.customer_conversion_goal.signup SIGNUP~WEBSITE
agoraform import googleads.campaign_budget.search_acquisition CAMPAIGN_BUDGET_ID
agoraform import googleads.campaign.search_acquisition CAMPAIGN_ID
agoraform import googleads.campaign_conversion_goal.trial_signup CAMPAIGN_ID~SIGNUP~WEBSITE
agoraform import googleads.campaign_location.united_states CAMPAIGN_ID~LOCATION_CRITERION_ID
agoraform import googleads.campaign_language.english CAMPAIGN_ID~LANGUAGE_CRITERION_ID
agoraform import googleads.ad_group.trial AD_GROUP_ID
agoraform import googleads.keyword.saas_software_exact AD_GROUP_ID~CRITERION_ID
agoraform import googleads.responsive_search_ad.trial AD_GROUP_ID~AD_ID
```

Confirm that each import:

- records the remote identity locally and prints canonical configurable YAML;
- does not mutate Google Ads;
- omits credentials, resource names, micros, geo constants, asset IDs, tag
  snippets, and other computed metadata from normal configuration;
- reconstructs parent `$ref` values when the parent is already bound.

Reconcile the printed attributes with the shipped example, then immediately
run:

```bash
agoraform plan
```

The unchanged imported Search graph must produce no changes.

### 6. Publish the GitHub release

Only after the artifact/version, authentication/secret-safety, create, update,
conversion-goal reconciliation, replacement visibility, zero-change planning,
and import-without-mutation checks all pass should the draft GitHub release be
published. Then close issue #46 and milestone 0.4.0.

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

## Out of scope for v0.4.0 distribution

Homebrew, Chocolatey, Scoop, OS package repositories, container images, hosted
services, new providers, Performance Max/Display/Video/Shopping/App/Dynamic
Search Ads campaign families, automated optimization, creative generation,
image/video assets, offline/call/app conversion workflows, application
instrumentation, rollback, scheduled publication, approval workflows, and
remote state are not part of this release.
