# Google Ads provider

The Google Ads provider registers as `googleads` and manages website
conversion actions, customer conversion-goal biddability, and daily Search
campaign budgets. Credentials come from the environment. The Agoraform CLI
remains provider-neutral; there is no Google Ads-specific command.

See the [Google Ads account and OAuth setup guide](../../docs/google-ads-setup.md)
for the Manager Account, developer-token, Google Cloud, OAuth, refresh-token,
and customer-ID prerequisites.

See the [complete conversion-measurement example](../../examples/googleads-conversion/README.md)
for a reusable `Trial Started` / `SIGNUP` workflow, import, and Google Ads
verification.

## Runtime configuration

Credentials and connection details come from environment variables:

```text
GOOGLE_ADS_DEVELOPER_TOKEN     required   Google Ads API developer token
GOOGLE_ADS_CLIENT_ID           required   OAuth 2.0 client ID
GOOGLE_ADS_CLIENT_SECRET       required   OAuth 2.0 client secret
GOOGLE_ADS_REFRESH_TOKEN       required   OAuth 2.0 refresh token
GOOGLE_ADS_CUSTOMER_ID         required   10-digit customer ID (hyphens optional)
GOOGLE_ADS_LOGIN_CUSTOMER_ID   optional   manager account customer ID
```

Tokens never belong in the manifest, logs, plan output, or local state.
Interactive OAuth consent is not implemented; supply a previously issued
refresh token.

Customer IDs are normalized before API calls: hyphens, spaces, and a
`customers/` prefix are stripped so REST paths and the `login-customer-id`
header always receive a 10-digit identifier.

## Declarative provider configuration

There are no non-secret YAML fields yet. An empty `providers.googleads`
block is accepted and still validates environment credentials. Putting OAuth
secrets or the developer token in the manifest is rejected.

```yaml
apiVersion: agoraform.io/v1alpha1
providers:
  googleads: {}
resources: []
```

Unknown provider configuration fields are rejected.

## Resources

### `googleads.conversion_action`

Website conversion actions such as `Trial Started`. Agoraform creates and
updates `WEBPAGE` conversion actions only. Offline, call, app, and event
upload conversions are out of scope.

```yaml
resources:
  - address: googleads.conversion_action.trial_started
    attributes:
      name: Trial Started
      category: SIGNUP
      value: 0
      count: ONE
      primaryForGoal: true
```

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Conversion action name. Must be unique in the customer. |
| `category` | yes | Website category such as `SIGNUP`, `PURCHASE`, or `SUBSCRIBE_PAID`. |
| `status` | no | `ENABLED` (API default), `HIDDEN`, or `REMOVED`. |
| `value` | no | Default conversion value. When set, `alwaysUseDefaultValue` defaults to `true`. |
| `currency` | no | ISO 4217 currency code for the default value. |
| `alwaysUseDefaultValue` | no | When true, Google Ads always uses the default value. |
| `count` | no | `ONE` or `MANY`. Mapped to Google Ads `ONE_PER_CLICK` / `MANY_PER_CLICK`. The API default is `MANY`. |
| `primaryForGoal` | no | Whether this action is primary for its conversion goal. API default is `true`. |
| `clickThroughLookbackWindowDays` | no | Click-through window for `WEBPAGE` actions, 1–30 days. |
| `viewThroughLookbackWindowDays` | no | View-through window, 1–30 days. |

`type` is always `WEBPAGE` and is not configurable. Provider-native IDs and
resource names live in local state and on `RemoteResource.Computed`, not in
the manifest. Computed fields also include `origin`, `ownerCustomer`, tag
snippets, and, when present in those snippets, `conversionId` and
`conversionLabel` for downstream website tags.

Omitted optional fields are not forced onto the remote resource. Equivalent
live values, including Google Ads enum aliases and default windows, do not
produce a plan diff.

Import accepts the numeric conversion action ID or the resource name
`customers/{customerId}/conversionActions/{id}` and stores the numeric ID:

```bash
agoraform import googleads.conversion_action.trial_started 123456789
```

App, call, and other non-`WEBPAGE` conversion actions fail import with
actionable guidance instead of generating a lossy website configuration.

### `googleads.customer_conversion_goal`

Account-default website conversion-goal biddability. Google Ads automatically
creates `CustomerConversionGoal` objects for each conversion-action
category/origin combination. Agoraform reads those provider-created goals and
reconciles `biddable`; it never creates or deletes them.

Address goals by category and origin, not by opaque resource names.
Provider-native identity is the computed `CATEGORY~ORIGIN` key stored in
local state.

```yaml
resources:
  - address: googleads.conversion_action.trial_started
    attributes:
      name: Trial Started
      category: SIGNUP
  - address: googleads.customer_conversion_goal.signup
    attributes:
      category: SIGNUP
      origin: WEBSITE
      biddable: true
      conversionAction:
        $ref: googleads.conversion_action.trial_started
```

| Attribute | Required | Description |
| --- | --- | --- |
| `category` | yes | Website category such as `SIGNUP`, `PURCHASE`, or `SUBSCRIBE_PAID`. |
| `origin` | yes | Must be `WEBSITE`. Other origins are out of scope. |
| `biddable` | yes | When true, Google Ads uses the goal as an account-default optimization goal. |
| `conversionAction` | no | `$ref` to a `googleads.conversion_action`. Use this when the matching conversion action is also managed so apply creates it before goal reconciliation. |

`resourceName` and the `CATEGORY~ORIGIN` identity are computed. Campaign
conversion goals and custom conversion goals are out of scope.

If the expected provider-created goal is still missing after the matching
conversion action exists, Agoraform reports that Google Ads creates the
object automatically and that Agoraform cannot create or delete it.

Equivalent live values, including enum case, produce no plan diff.

Import accepts `CATEGORY~ORIGIN` or the resource name
`customers/{customerId}/customerConversionGoals/{category}~{origin}`:

```bash
agoraform import googleads.customer_conversion_goal.signup SIGNUP~WEBSITE
```

Non-website origins fail import with guidance rather than reconstructing a
lossy `WEBSITE` goal. Import does not emit a `conversionAction` `$ref`; add
that optionally after import if the matching conversion action is also
managed.

### `googleads.campaign_budget`

Daily Search campaign budgets. Agoraform creates and updates `STANDARD`
daily budgets only. Lifetime/`CUSTOM_PERIOD` budgets, portfolio bidding
strategies, and non-standard budget types are out of scope.

```yaml
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      amount: 50
      deliveryMethod: STANDARD
      explicitlyShared: false
```

Search campaigns will attach a budget with a logical `$ref` to this
address. Budget behavior stays on `googleads.campaign_budget`; do not embed
amount or sharing fields on the campaign resource.

```yaml
# Forthcoming googleads.campaign resource (not implemented yet):
#   budget:
#     $ref: googleads.campaign_budget.brand
```

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Budget name. Must be unique in the customer for unmanaged discovery. |
| `amount` | yes | Daily budget in account-currency units, for example `50` or `50.25`. Converted to Google Ads `amount_micros` (`1` unit = `1_000_000` micros) with at most six decimal places. |
| `explicitlyShared` | yes | `false` for a dedicated single-campaign budget; `true` for a shared budget. Google Ads cannot convert a shared budget back to non-shared. |
| `deliveryMethod` | no | `STANDARD` (API default) or `ACCELERATED`. Omitted values are not forced onto the remote resource. |

`period` is always `DAILY` and `type` is always `STANDARD`. Those fields,
provider-native IDs, resource names, `amountMicros`, `status`, and
`referenceCount` live in local state and on `RemoteResource.Computed`, not
in the manifest.

Equivalent live values, including `50` / `50.0` / `"50.00"` and enum case,
do not produce a plan diff. Amount comparison uses integer micros so
currency rounding stays deterministic.

Import accepts the numeric campaign budget ID or the resource name
`customers/{customerId}/campaignBudgets/{id}` and stores the numeric ID:

```bash
agoraform import googleads.campaign_budget.brand 123456789
```

Lifetime, Smart, and other non-standard budgets fail import with guidance
instead of generating a lossy daily Search budget.

## HTTP client

`providers/googleads/client` centralizes Google Ads REST calls, including:

- OAuth 2.0 refresh-token exchange and access-token caching;
- `developer-token` and optional `login-customer-id` headers;
- Google Ads Query Language search with pagination;
- resource mutate requests;
- API version selection (`v25`) so upgrades stay in one place;
- Google Ads / OAuth error mapping and secret redaction.

Provider resource code uses this client rather than issuing ad hoc HTTP
requests. Override `Config.BaseURL` and `Config.TokenURL` in tests.

## Safety

- `agoraform plan` does not mutate Google Ads.
- Authentication secrets are redacted from provider errors.
- Tests use local `httptest` servers only.
- Bound local-state identities are resolved by ID, never by renaming.
