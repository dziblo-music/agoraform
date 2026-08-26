# Examples

Example manifests live here. They are validated automatically by the Go test
suite and contain no credentials or private deployment values.

- [googleads-conversion](googleads-conversion/README.md) — primary v0.3.0
  quickstart: website `SIGNUP` / Trial Started conversion action, customer
  conversion-goal biddability, import guidance, and the boundary between
  Google Ads configuration and external website tags.
- [matomo-conversion](matomo-conversion/README.md) — primary v0.2.0 quickstart:
  complete Matomo Tag Manager conversion workflow with a Data Layer variable,
  Custom Event trigger, Matomo Analytics event tag, logical references, import
  guidance, and declarative publication.
- [agoraform.yaml](agoraform.yaml) — minimal `matomo.goal` example retained from
  v0.1.0.

Managed identities belong in `agoraform.state.json` beside the working
manifest, not in resource attributes. See [docs/state.md](../docs/state.md).

## v0.3.0 quickstart

`validate`, `plan`, `apply`, and `import` contact Google Ads. Set the required
runtime configuration first:

```bash
export GOOGLE_ADS_DEVELOPER_TOKEN=replace-me
export GOOGLE_ADS_CLIENT_ID=replace-me
export GOOGLE_ADS_CLIENT_SECRET=replace-me
export GOOGLE_ADS_REFRESH_TOKEN=replace-me
export GOOGLE_ADS_CUSTOMER_ID=1234567890

cp examples/googleads-conversion/agoraform.yaml agoraform.yaml

agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Review the first plan before applying. The conversion action is created; the
customer conversion goal is created by Google Ads and adopted or updated so
`SIGNUP` / `WEBSITE` is biddable. The final `plan` should report `No changes.`
when configuration and remote state are unchanged.

See [googleads-conversion/README.md](googleads-conversion/README.md) for the
Google Ads verification steps, conversion identifiers used by website tags,
and import guidance.

Provider credentials are supplied through `GOOGLE_ADS_*` environment
variables, never in the manifest. See
[providers/googleads/README.md](../providers/googleads/README.md).

## v0.2.0 quickstart

`validate`, `plan`, `apply`, and `import` contact Matomo. Set the required
runtime configuration first:

```bash
export MATOMO_URL=https://matomo.example.com
export MATOMO_TOKEN_AUTH=replace-me
export MATOMO_SITE_ID=1
export MATOMO_CONTAINER_ID=replace-with-your-container-id

cp examples/matomo-conversion/agoraform.yaml agoraform.yaml

agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Review the first plan before applying. The example enables declarative
publication, so `plan` makes the potential publication visible and `apply`
publishes only after draft resources converge. The final `plan` should report
`No changes.` when configuration and remote state are unchanged.

See [matomo-conversion/README.md](matomo-conversion/README.md) for the
application-side event contract, Matomo verification, disabled-publication
workflow, and import guidance.

## Minimal goal example

For the smaller v0.1-style goal workflow:

```bash
cp examples/agoraform.yaml agoraform.yaml
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Provider credentials are supplied through `MATOMO_*` environment variables,
never in the manifest. See [providers/matomo/README.md](../providers/matomo/README.md).
