# Agoraform

Marketing Infrastructure as Code.

Agoraform is an open-source Go CLI for declaring marketing-platform
configuration in YAML, reviewing changes with a non-mutating plan, and
reconciling those changes through provider APIs.

The provider-neutral workflow is:

```text
validate -> plan -> apply -> plan
```

`import` adopts supported existing remote objects without mutating them.
`destroy` tears down managed resources in reverse dependency order after an
explicit confirmation (or `--auto-approve`).

## v0.4.0

Agoraform 0.4.0 adds complete Google Ads Search campaign management while
retaining Matomo from v0.2.0 and Google Ads conversion measurement from
v0.3.0.

| Area | v0.4.0 |
| --- | --- |
| Commands | `validate`, `plan`, `apply`, `import` |
| Providers | Matomo, Google Ads (`googleads`) |
| Matomo resources | `matomo.goal`, `matomo.variable`, `matomo.trigger`, `matomo.tag` |
| Google Ads conversion | `googleads.conversion_action`, `googleads.customer_conversion_goal` |
| Google Ads Search | `googleads.campaign_budget`, `googleads.campaign`, `googleads.campaign_conversion_goal`, `googleads.ad_group`, `googleads.keyword`, `googleads.responsive_search_ad`, `googleads.campaign_location`, `googleads.campaign_language` |
| References | Logical `$ref` dependencies |
| Import | Supported Matomo resources plus supported Google Ads conversion measurement and Search campaign resources |
| Matomo publication | Declarative through provider configuration, visible in `plan`, executed during `apply` |
| State | local `agoraform.state.json` |
| Mutations | create and update; new Search serving objects default to `PAUSED` |

v0.3.0 added website conversion actions and customer conversion-goal
biddability. v0.2.0 introduced the dependency-aware Matomo Tag Manager
workflow. v0.1.0 was the first public release and managed `matomo.goal`
only.

## Install

Official release artifacts require no runtime besides the binary. They are
built with Go 1.26.7 and `CGO_ENABLED=0`.

### GitHub Releases

1. Download the `v0.4.0` archive for your OS and architecture from GitHub
   Releases.
2. Download `checksums.txt` from the same release and verify the archive.
3. Extract `agoraform` (`agoraform.exe` on Windows) and place it on `PATH`.
4. Confirm the version:

```bash
agoraform --version
```

A v0.4.0 binary prints `0.4.0`. Git tags use the Go convention with a `v`
prefix. Untagged local builds print `0.0.0-dev`.

Release archives:

| File | Platform |
| --- | --- |
| `agoraform_0.4.0_linux_amd64.tar.gz` | Linux amd64 |
| `agoraform_0.4.0_linux_arm64.tar.gz` | Linux arm64 |
| `agoraform_0.4.0_darwin_amd64.tar.gz` | macOS amd64 |
| `agoraform_0.4.0_darwin_arm64.tar.gz` | macOS arm64 |
| `agoraform_0.4.0_windows_amd64.zip` | Windows amd64 |

The archives also contain the README, changelog, license files, the v0.1 goal
example, the complete v0.2 Matomo conversion example, the complete v0.3
Google Ads conversion example, and the complete v0.4 Google Ads Search
campaign example.

### go install

Requires Go 1.26.7 or newer:

```bash
go install github.com/dziblo-music/agoraform/cmd/agoraform@v0.4.0
```

### Build current source

```bash
git clone https://github.com/dziblo-music/agoraform.git
cd agoraform
go build -o agoraform ./cmd/agoraform
./agoraform --version
```

On Windows:

```powershell
go build -o agoraform.exe ./cmd/agoraform
```

## Runtime configuration

Matomo credentials and connection details stay outside the manifest:

```text
MATOMO_URL            Matomo base URL, for example https://matomo.example.com
MATOMO_TOKEN_AUTH     API token
MATOMO_SITE_ID        numeric site id
MATOMO_CONTAINER_ID   existing Tag Manager container id when no matomo.container resource is declared
```

Credentials never belong in YAML, plan output, logs, or local state.

Google Ads credentials stay outside the manifest as well:

```text
GOOGLE_ADS_DEVELOPER_TOKEN     Google Ads API developer token
GOOGLE_ADS_CLIENT_ID           OAuth 2.0 client ID
GOOGLE_ADS_CLIENT_SECRET       OAuth 2.0 client secret
GOOGLE_ADS_REFRESH_TOKEN       OAuth 2.0 refresh token
GOOGLE_ADS_CUSTOMER_ID         10-digit customer ID (hyphens optional)
GOOGLE_ADS_LOGIN_CUSTOMER_ID   optional manager-account customer ID
```

The `googleads` provider manages supported website conversion actions,
customer conversion-goal biddability, daily Search campaign budgets, Search
campaigns, campaign conversion-goal biddability, Search ad groups, Search
keywords, Responsive Search Ads, and campaign location and language
targeting. See the [Google Ads setup guide](docs/google-ads-setup.md),
[provider reference](providers/googleads/README.md),
[v0.4 Search campaign example](examples/googleads-search/README.md), and
[v0.3 conversion example](examples/googleads-conversion/README.md).

Non-secret publication desired state belongs in the Matomo provider manifest:

```yaml
providers:
  matomo:
    publish: true
    environment: live
```

`publish` defaults to `false`; `environment` defaults to `live` when
publication is enabled.

## v0.4 Google Ads Search campaign

The primary v0.4 example manages a paused SaaS paid-acquisition Search
campaign in Google Ads:

- website `Trial Started` conversion measurement and account-default
  `SIGNUP` / `WEBSITE` goal biddability;
- a dedicated daily Search budget and paused Search campaign that
  maximizes conversions;
- campaign-level conversion-goal selection, United States location
  targeting, and English language targeting;
- a paused Search ad group with explicit `EXACT`, `PHRASE`, and `BROAD`
  keywords plus negative-keyword coverage;
- a paused Responsive Search Ad with placeholder copy and
  `https://example.com/` landing URLs;
- logical `$ref` values so Agoraform applies the graph in dependency
  order.

The example stays paused until you verify the campaign in Google Ads and
intentionally change `status`. Agoraform does not generate creative or
enable spend automatically.

Set Google Ads runtime configuration, then copy the included example. Load
secret values from your normal secret manager; for an interactive Bash session,
`read -s` avoids placing typed secrets in shell command history:

```bash
export GOOGLE_ADS_CLIENT_ID=replace-with-your-oauth-client-id
export GOOGLE_ADS_CUSTOMER_ID=1234567890

read -rsp "Google Ads developer token: " GOOGLE_ADS_DEVELOPER_TOKEN; echo
export GOOGLE_ADS_DEVELOPER_TOKEN
read -rsp "Google Ads OAuth client secret: " GOOGLE_ADS_CLIENT_SECRET; echo
export GOOGLE_ADS_CLIENT_SECRET
read -rsp "Google Ads refresh token: " GOOGLE_ADS_REFRESH_TOKEN; echo
export GOOGLE_ADS_REFRESH_TOKEN

cp examples/googleads-search/agoraform.yaml agoraform.yaml
```

Do not substitute literal secret values into those prompt commands. On
automated systems, inject the `GOOGLE_ADS_*` secrets from your secret manager.

Run:

```bash
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Review the first plan before applying. Customer and campaign conversion
goals are created by Google Ads; Agoraform adopts or updates them and never
attempts unsupported create or delete operations. The final plan should
report `No changes.` when desired configuration, local state, and remote
state are unchanged.

See [the Google Ads Search campaign example](examples/googleads-search/README.md)
for Google Ads verification before enabling spend, and import of an
equivalent manually configured campaign.

## Safe serving state and replacement

v0.4.0 Search resources stay paused until you review them in Google Ads and
change `status` in the manifest:

- new campaigns, ad groups, positive keywords, and Responsive Search Ads
  default to `PAUSED`;
- negative keywords default to `ENABLED` because Google Ads does not allow
  paused negative ad-group criteria;
- enabling spend is a reviewed `status` update, not an automatic side
  effect of apply.

Agoraform never deletes remote Google Ads objects. Immutable identity
fields fail planning instead of hiding a destroy-and-recreate:

- keyword `text`, `matchType`, `negative`, and parent ad group;
- campaign location, language, and whether a location is excluded;
- Responsive Search Ad parent ad group;
- ad group parent campaign.

Create a new resource address for those identity changes. Responsive Search
Ad **status** updates the ad-group-ad relationship in place. Creative
changes (headlines, descriptions, final URLs, display paths, pinning)
replace the underlying ad lists and appear as list diffs in `plan`.

Customer and campaign conversion goals are created by Google Ads. Agoraform
adopts or updates `biddable` and never attempts unsupported create or delete
operations.

## v0.3 Google Ads conversion measurement

The primary v0.3 example manages a website `Trial Started` conversion in Google
Ads:

- `googleads.conversion_action.trial_started` as a `SIGNUP` website conversion;
- `googleads.customer_conversion_goal.signup` so `SIGNUP` / `WEBSITE` is
  biddable as an account-default optimization goal;
- a logical `$ref` so the conversion action exists before goal reconciliation.

Agoraform manages Google Ads configuration only. Website tags, Google Tag
Manager, and application event emission stay outside the provider. After apply,
use the conversion ID and conversion label from Google Ads when configuring
those external tools.

Set Google Ads runtime configuration, then copy the included example. Load
secret values from your normal secret manager; for an interactive Bash session,
`read -s` avoids placing typed secrets in shell command history:

```bash
export GOOGLE_ADS_CLIENT_ID=replace-with-your-oauth-client-id
export GOOGLE_ADS_CUSTOMER_ID=1234567890

read -rsp "Google Ads developer token: " GOOGLE_ADS_DEVELOPER_TOKEN; echo
export GOOGLE_ADS_DEVELOPER_TOKEN
read -rsp "Google Ads OAuth client secret: " GOOGLE_ADS_CLIENT_SECRET; echo
export GOOGLE_ADS_CLIENT_SECRET
read -rsp "Google Ads refresh token: " GOOGLE_ADS_REFRESH_TOKEN; echo
export GOOGLE_ADS_REFRESH_TOKEN

cp examples/googleads-conversion/agoraform.yaml agoraform.yaml
```

Do not substitute literal secret values into those prompt commands. On
automated systems, inject the `GOOGLE_ADS_*` secrets from your secret manager.

Run:

```bash
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Review the first plan before applying. The conversion action is created. The
customer conversion goal is created by Google Ads; Agoraform adopts or updates
it and never attempts unsupported create or delete operations. The final plan
should report `No changes.` when desired configuration, local state, and remote
state are unchanged.

See [the Google Ads conversion example](examples/googleads-conversion/README.md)
for Google Ads verification, conversion identifiers for website tags, and
import of equivalent manually configured conversion measurement.

## v0.2 Matomo conversion tracking

The primary v0.2 example manages a complete `trialStarted` conversion flow:

- a Data Layer variable that reads `userId`;
- a Custom Event trigger for `trialStarted`;
- a Matomo Analytics event tag;
- logical references that force the variable/trigger to exist before the tag;
- declarative publication to a Matomo Tag Manager environment.

Set Matomo runtime configuration, then copy the included example:

```bash
export MATOMO_URL=https://matomo.example.com
export MATOMO_TOKEN_AUTH=replace-with-your-api-token
export MATOMO_SITE_ID=1
export MATOMO_CONTAINER_ID=replace-with-your-container-id

cp examples/matomo-conversion/agoraform.yaml agoraform.yaml
```

Run:

```bash
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Review the first plan before applying. With publication enabled, the plan makes
the potential container publication visible before mutation. `apply` performs
draft resource changes in dependency order and only then creates/publishes a
container version when the converged draft still differs from the published
environment.

The final plan should report `No changes.` when desired configuration, local
state, and remote state are unchanged. Repeated unchanged apply must not create
duplicate container versions.

See [the complete Matomo conversion example](examples/matomo-conversion/README.md)
for Matomo verification and the application-side data-layer event contract.

## Resource references and dependency ordering

Dependencies are expressed with a single-key `$ref` object containing a logical
Agoraform address:

```yaml
resources:
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStarted

  - address: matomo.tag.trial_started
    attributes:
      type: matomoAnalytics
      trigger:
        $ref: matomo.trigger.trial_started
      eventCategory: trial
      eventAction: started
```

Agoraform validates references before remote mutation, rejects missing
references/self-references/cycles, and plans/applies prerequisites first.
Provider-native IDs are resolved at runtime and remain in local state rather
than leaking into configuration.

## Supported Matomo Tag Manager resources

### `matomo.variable`

v0.2.0 supports Data Layer variables. v0.5.0 also supports Matomo
Configuration variables:

```yaml
- address: matomo.variable.user_id
  attributes:
    type: dataLayer
    key: userId
    name: User ID

- address: matomo.variable.config
  attributes:
    type: matomoConfiguration
    name: Matomo Configuration
    matomoUrl: https://matomo.example.com
    siteId: 1
    enableLinkTracking: true
```

### `matomo.trigger`

v0.2.0 supports Custom Event triggers:

```yaml
- address: matomo.trigger.trial_started
  attributes:
    type: customEvent
    event: trialStarted
```

### `matomo.tag`

v0.2.0 supports Matomo Analytics event tags. The fire trigger is a logical
`$ref`; supported event fields can use literals or supported managed-variable
references where documented. Tags may reference a managed Matomo
Configuration variable with `matomoConfiguration: { $ref: matomo.variable.* }`.

See the [Matomo provider reference](providers/matomo/README.md) for the complete
supported schemas.

## Import existing resources

`agoraform import` binds a supported existing remote resource to its logical
Agoraform address, records the remote identity in `agoraform.state.json`, and
prints canonical YAML. It does not mutate the remote platform or edit your
manifest.

```bash
agoraform import matomo.variable.config VARIABLE_ID
agoraform import matomo.variable.user_id VARIABLE_ID
agoraform import matomo.trigger.trial_started TRIGGER_ID
agoraform import matomo.tag.trial_started TAG_ID
agoraform import googleads.conversion_action.trial_started 123456789
agoraform import googleads.customer_conversion_goal.signup SIGNUP~WEBSITE
agoraform import googleads.campaign_budget.brand 123456789
agoraform import googleads.campaign.brand 987654321
agoraform import googleads.ad_group.brand 555666777
agoraform import googleads.keyword.brand_exact 555666777~888999000
agoraform import googleads.responsive_search_ad.brand 555666777~888999000
agoraform import googleads.campaign_location.united_states 987654321~888999000
agoraform import googleads.campaign_language.english 987654321~888999001
agoraform import googleads.campaign_conversion_goal.trial_signup 987654321~SIGNUP~WEBSITE
```

For a Tag Manager tag, import its dependencies first so Agoraform can
reconstruct logical references. Review the printed YAML, update your manifest
if necessary, and run `agoraform plan`. An equivalent imported configuration
should produce no changes.

Google Ads import accepts supported website conversion actions, supported
`WEBSITE` customer conversion goals, daily Search campaign budgets, Search
campaigns, Search ad groups, Search keywords including negatives,
Responsive Search Ads, campaign location and language criteria, and
campaign conversion goals. Import a campaign budget before the campaign,
the campaign before an ad group, campaign conversion goal, location, or
language, and the ad group before a keyword or Responsive Search Ad, so
Agoraform can reconstruct logical `$ref`s.
Unsupported conversion types, origins, channel types, Dynamic Search Ads
settings, ad group types, ad types, criterion types, keyword-level URL
or tracking settings, and budget periods fail with actionable diagnostics
instead of producing a lossy manifest.

## Declarative Matomo Tag Manager publication

Agoraform deliberately does not add a provider-specific `publish` command.
Publication is desired state:

```yaml
providers:
  matomo:
    publish: true
    environment: live
```

When publication is required, `plan` shows a provider action such as:

```text
> matomo.container.main: publish -> live [conditional]
```

With `MATOMO_CONTAINER_ID` and no `matomo.container` resource, that action
is addressed as `matomo.container.external`.

`apply` first reconciles all planned draft resources. Only after those
mutations succeed does it recheck the converged draft, create a container
version if needed, and publish it to the configured environment. The recheck
prevents duplicate publication after convergence.

Set `publish: false` or omit the provider publication configuration to manage
Tag Manager draft resources without publishing them.

Before version creation, Agoraform verifies that the credentials can publish
to the configured environment. Unknown environments or insufficient publish
capability fail before the version-creation mutation.

See [Matomo Tag Manager publication](docs/matomo-publishing.md).

## Commands

Default manifest path: `agoraform.yaml`.

```bash
agoraform validate [-f path/to/manifest.yaml]
agoraform plan [-f path/to/manifest.yaml]
agoraform apply [-f path/to/manifest.yaml]
agoraform import [-f path/to/manifest.yaml] ADDRESS REMOTE-ID
```

| Command | Purpose |
| --- | --- |
| `validate` | Validate manifest, provider configuration, dependencies, connectivity, and resource schemas |
| `plan` | Read remote state and show all resource/provider actions; never mutate |
| `apply` | Execute the reviewed resource changes and configured provider finalization |
| `import` | Bind an existing remote identity and print configurable YAML; no remote mutation |

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success; for `plan`, no changes |
| `1` | Runtime/validation/provider failure |
| `2` | `plan` succeeded and changes are present |
| `3` | Invalid CLI usage |

## Current development limitations

- v0.4.0 supports Matomo plus Google Ads website conversion measurement and
  the complete Search campaign graph. Meta Ads is not implemented.
- Google Ads campaign support is Search only. Performance Max, Display,
  Video, Shopping, App, Dynamic Search Ads, and other campaign families
  are not implemented.
- Agoraform does not generate creative, upload image or video assets,
  enable spend automatically, or apply Google Ads optimization
  recommendations.
- Offline/call/app conversion workflows and conversion-event uploads are
  not implemented.
- Agoraform Google Ads authentication uses developer-token + single-user
  OAuth refresh-token credentials. Service-account, multi-user, and
  interactive OAuth flows are not implemented.
- One Matomo Tag Manager container is selected at a time, either by a
  `matomo.container` resource or by `MATOMO_CONTAINER_ID`.
- Tag Manager support includes `matomo.container` plus the variable, trigger,
  and tag types documented for v0.2.0. Agoraform-managed containers can be
  destroyed after their children; containers selected only by
  `MATOMO_CONTAINER_ID` are never deleted.
- There is no provider-specific `agoraform publish` command; publication is
  declarative through normal `plan`/`apply`/`destroy`.
- Rollback, scheduled publication, approval workflows, generalized deployment
  pipelines, and multi-container deployment orchestration are not implemented.
- Remote state, workspaces, locking, and encryption are not implemented.
- `apply` does not delete remote resources. Use `agoraform destroy` to tear
  down managed resources. Immutable Google Ads identity fields fail planning
  rather than planning a replacement delete. Google Ads destroy/removal is
  not implemented yet; those resources remain in state as unsupported.
- Pre-1.0 releases may intentionally introduce documented breaking changes.

## Documentation

- [Manifest format](docs/manifest.md)
- [Plan engine](docs/plan.md)
- [Apply execution](docs/apply.md)
- [Destroy](docs/destroy.md)
- [Import](docs/import.md)
- [Local provider configuration](docs/local-configuration.md)
- [Google Ads setup](docs/google-ads-setup.md)
- [Matomo Tag Manager publication](docs/matomo-publishing.md)
- [Local state](docs/state.md)
- [Matomo provider](providers/matomo/README.md)
- [Google Ads provider](providers/googleads/README.md)
- [v0.2 Matomo conversion example](examples/matomo-conversion/README.md)
- [v0.3 Google Ads conversion example](examples/googleads-conversion/README.md)
- [v0.4 Google Ads Search campaign example](examples/googleads-search/README.md)
- [Release process](docs/release.md)
- [Changelog](CHANGELOG.md)

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for the build, test, branch, and pull
request workflow.

Agoraform is licensed under the [Apache License 2.0](LICENSE).
