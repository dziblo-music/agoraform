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
Destructive `destroy` behavior is not implemented yet.

## v0.3.0

Agoraform 0.3.0 adds Google Ads conversion measurement while retaining the
Matomo analytics and Tag Manager functionality from v0.2.0.

| Area | v0.3.0 |
| --- | --- |
| Commands | `validate`, `plan`, `apply`, `import` |
| Providers | Matomo, Google Ads (`googleads`) |
| Matomo resources | `matomo.goal`, `matomo.variable`, `matomo.trigger`, `matomo.tag` |
| Google Ads resources | `googleads.conversion_action`, `googleads.customer_conversion_goal` |
| References | Logical `$ref` dependencies |
| Import | Supported Matomo resources plus supported Google Ads conversion measurement |
| Matomo publication | Declarative through provider configuration, visible in `plan`, executed during `apply` |
| State | local `agoraform.state.json` |
| Mutations | create and update |

v0.2.0 introduced the dependency-aware Matomo Tag Manager workflow; v0.1.0
was the first public release and managed `matomo.goal` only.

## Install

Official release artifacts require no runtime besides the binary. They are
built with Go 1.26.7 and `CGO_ENABLED=0`.

### GitHub Releases

1. Download the `v0.3.0` archive for your OS and architecture from GitHub
   Releases.
2. Download `checksums.txt` from the same release and verify the archive.
3. Extract `agoraform` (`agoraform.exe` on Windows) and place it on `PATH`.
4. Confirm the version:

```bash
agoraform --version
```

A v0.3.0 binary prints `0.3.0`. Git tags use the Go convention with a `v`
prefix. Untagged local builds print `0.0.0-dev`.

Release archives:

| File | Platform |
| --- | --- |
| `agoraform_0.3.0_linux_amd64.tar.gz` | Linux amd64 |
| `agoraform_0.3.0_linux_arm64.tar.gz` | Linux arm64 |
| `agoraform_0.3.0_darwin_amd64.tar.gz` | macOS amd64 |
| `agoraform_0.3.0_darwin_arm64.tar.gz` | macOS arm64 |
| `agoraform_0.3.0_windows_amd64.zip` | Windows amd64 |

The archives also contain the README, changelog, license files, the v0.1 goal
example, the complete v0.2 Matomo conversion example, and the complete v0.3
Google Ads conversion example.

### go install

Requires Go 1.26.7 or newer:

```bash
go install github.com/dziblo-music/agoraform/cmd/agoraform@v0.3.0
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
MATOMO_CONTAINER_ID   Tag Manager container id for v0.2 Tag Manager resources
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

The `googleads` provider manages supported website conversion actions and
customer conversion-goal biddability. See the
[Google Ads setup guide](docs/google-ads-setup.md),
[provider reference](providers/googleads/README.md), and
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

v0.2.0 supports Data Layer variables:

```yaml
- address: matomo.variable.user_id
  attributes:
    type: dataLayer
    key: userId
    name: User ID
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
references where documented.

See the [Matomo provider reference](providers/matomo/README.md) for the complete
supported schemas.

## Import existing resources

`agoraform import` binds a supported existing remote resource to its logical
Agoraform address, records the remote identity in `agoraform.state.json`, and
prints canonical YAML. It does not mutate the remote platform or edit your
manifest.

```bash
agoraform import matomo.variable.user_id VARIABLE_ID
agoraform import matomo.trigger.trial_started TRIGGER_ID
agoraform import matomo.tag.trial_started TAG_ID
agoraform import googleads.conversion_action.trial_started 123456789
agoraform import googleads.customer_conversion_goal.signup SIGNUP~WEBSITE
```

For a Tag Manager tag, import its dependencies first so Agoraform can
reconstruct logical references. Review the printed YAML, update your manifest
if necessary, and run `agoraform plan`. An equivalent imported configuration
should produce no changes.

Google Ads import accepts supported website conversion actions and supported
`WEBSITE` customer conversion goals only. Unsupported conversion types/origins
fail with actionable diagnostics instead of producing a lossy manifest.

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

- v0.3.0 supports Matomo plus Google Ads website conversion measurement. Meta
  Ads is not implemented.
- Google Ads support is limited to supported `WEBPAGE` conversion actions and
  `WEBSITE` customer conversion-goal biddability. Search campaigns, budgets,
  ad groups, keywords, targeting, ads, offline/call/app conversion workflows,
  and conversion-event uploads are not implemented.
- Agoraform v0.3.0 Google Ads authentication uses developer-token + single-user
  OAuth refresh-token credentials. Service-account, multi-user, and interactive
  OAuth flows are not implemented.
- One Matomo Tag Manager container is configured at a time through
  `MATOMO_CONTAINER_ID`.
- Tag Manager support is limited to the variable, trigger, and tag types
  documented for v0.2.0.
- There is no provider-specific `agoraform publish` command; publication is
  declarative through normal `plan`/`apply`.
- Rollback, scheduled publication, approval workflows, generalized deployment
  pipelines, and multi-container deployment orchestration are not implemented.
- Remote state, workspaces, locking, and encryption are not implemented.
- `apply` does not delete remote resources and there is no `destroy` command.
- Pre-1.0 releases may intentionally introduce documented breaking changes.

## Documentation

- [Manifest format](docs/manifest.md)
- [Plan engine](docs/plan.md)
- [Apply execution](docs/apply.md)
- [Import](docs/import.md)
- [Local provider configuration](docs/local-configuration.md)
- [Google Ads setup](docs/google-ads-setup.md)
- [Matomo Tag Manager publication](docs/matomo-publishing.md)
- [Local state](docs/state.md)
- [Matomo provider](providers/matomo/README.md)
- [Google Ads provider](providers/googleads/README.md)
- [v0.2 Matomo conversion example](examples/matomo-conversion/README.md)
- [v0.3 Google Ads conversion example](examples/googleads-conversion/README.md)
- [Release process](docs/release.md)
- [Changelog](CHANGELOG.md)

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for the build, test, branch, and pull
request workflow.

Agoraform is licensed under the [Apache License 2.0](LICENSE).
