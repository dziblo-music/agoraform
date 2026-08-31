# Examples

Example manifests live here. They are validated automatically by the Go test
suite and contain no credentials or private deployment values.

- [matomo-googleads](matomo-googleads/README.md) — primary v0.5.0 lifecycle
  example: managed Matomo Tag Manager container, Matomo Configuration and
  Data Layer variables, `trialStarted` trigger, Google Ads website
  conversion action, cross-provider Google Ads conversion tag, publication,
  import/adoption, and destroy, plus an external-container variant.
- [googleads-search](googleads-search/README.md) — primary v0.4.0
  quickstart: paused SaaS Search campaign from conversion measurement
  through budget, campaign, conversion-goal biddability, targeting,
  ad group, keywords, and Responsive Search Ad, plus import/adoption.
- [googleads-conversion](googleads-conversion/README.md) — primary v0.3.0
  quickstart: website `SIGNUP` / Trial Started conversion action, customer
  conversion-goal biddability, import guidance, and the boundary between
  Google Ads configuration and external website tags.
- [matomo-conversion](matomo-conversion/README.md) — primary v0.2.0 quickstart:
  complete Matomo Tag Manager conversion workflow in an existing container
  selected by `MATOMO_CONTAINER_ID`, with a Data Layer variable, Custom Event
  trigger, Matomo Analytics event tag, optional managed Matomo Configuration
  variable, logical references, import guidance, and declarative publication.
- [agoraform.yaml](agoraform.yaml) — minimal `matomo.goal` example retained from
  v0.1.0.

Managed identities belong in `agoraform.state.json` beside the working
manifest, not in resource attributes. See [docs/state.md](../docs/state.md).

## v0.5.0 quickstart

`validate`, `plan`, `apply`, `import`, and `destroy` contact Matomo and
Google Ads. Omit `MATOMO_CONTAINER_ID` for the primary managed-container
manifest. Set the required runtime configuration first. Load secret values
from your normal secret manager; for an interactive Bash session, `read -s`
avoids placing them in shell command history:

```bash
export MATOMO_URL=https://matomo.example.com
export MATOMO_SITE_ID=1
export GOOGLE_ADS_CLIENT_ID=replace-with-your-oauth-client-id
export GOOGLE_ADS_CUSTOMER_ID=1234567890

read -rsp "Matomo API token: " MATOMO_TOKEN_AUTH; echo
export MATOMO_TOKEN_AUTH
read -rsp "Google Ads developer token: " GOOGLE_ADS_DEVELOPER_TOKEN; echo
export GOOGLE_ADS_DEVELOPER_TOKEN
read -rsp "Google Ads OAuth client secret: " GOOGLE_ADS_CLIENT_SECRET; echo
export GOOGLE_ADS_CLIENT_SECRET
read -rsp "Google Ads refresh token: " GOOGLE_ADS_REFRESH_TOKEN; echo
export GOOGLE_ADS_REFRESH_TOKEN

cp examples/matomo-googleads/agoraform.yaml agoraform.yaml

agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Do not substitute literal secret values into those prompt commands. On
automated systems, inject the secrets from your secret manager.

Review the first plan before applying. Agoraform creates the Matomo
container, configuration variable, data-layer variable, trigger, Google Ads
conversion action, and Matomo Google Ads conversion tag. The customer
conversion goal is created by Google Ads and adopted or updated so
`SIGNUP` / `WEBSITE` is biddable. The example publishes to `live` only when
the converged draft differs from that environment. The final `plan` should
report `No changes.` when configuration and remote state are unchanged.

See [matomo-googleads/README.md](matomo-googleads/README.md) for the
application-side event contract, exact apply and destroy ordering, the
external-container variant, import guidance, and verification in Matomo and
Google Ads.

## v0.4.0 quickstart

`validate`, `plan`, `apply`, and `import` contact Google Ads. Set the required
runtime configuration first. Load secret values from your normal secret
manager; for an interactive Bash session, `read -s` avoids placing them in
shell command history:

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

agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Do not substitute literal secret values into those prompt commands. On
automated systems, inject the `GOOGLE_ADS_*` secrets from your secret manager.

Review the first plan before applying. The conversion action, dedicated
budget, paused Search campaign, targeting, ad group, keywords, and
Responsive Search Ad are created. Customer and campaign conversion goals
are created by Google Ads and adopted or updated so `SIGNUP` / `WEBSITE`
is biddable. The example stays paused until you change `status` after
verifying the campaign in Google Ads. The final `plan` should report
`No changes.` when configuration and remote state are unchanged.

See [googleads-search/README.md](googleads-search/README.md) for the Google
Ads verification steps before enabling spend, and import guidance for an
equivalent manually configured campaign.

Provider credentials are supplied through `GOOGLE_ADS_*` environment
variables, never in the manifest. See
[providers/googleads/README.md](../providers/googleads/README.md).

## v0.3.0 quickstart

`validate`, `plan`, `apply`, and `import` contact Google Ads. Set the required
runtime configuration first. Load secret values from your normal secret
manager; for an interactive Bash session, `read -s` avoids placing them in
shell command history:

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

agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Do not substitute literal secret values into those prompt commands. On
automated systems, inject the `GOOGLE_ADS_*` secrets from your secret manager.

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
