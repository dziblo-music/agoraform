# Google Ads provider

The Google Ads provider registers as `googleads` and manages website
conversion actions, customer conversion-goal biddability, daily Search
campaign budgets, Search campaigns, campaign conversion-goal
biddability, and Search ad groups. Credentials come from the environment.
The Agoraform CLI remains provider-neutral; there is no Google Ads-specific
command.

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

`resourceName` and the `CATEGORY~ORIGIN` identity are computed. Custom
conversion goals are out of scope. Campaign-level conversion-goal
biddability is a separate `googleads.campaign_conversion_goal` resource.

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
strategies, accelerated delivery, and non-standard budget types are out of
scope.

```yaml
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      amount: 50
      deliveryMethod: STANDARD
      explicitlyShared: false
```

Search campaigns attach a budget with a logical `$ref` to this
address. Budget behavior stays on `googleads.campaign_budget`; do not embed
amount or sharing fields on the campaign resource.

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Budget name used for creation and unmanaged discovery. For a dedicated budget, Google keeps the live name synchronized with the attached campaign; Agoraform treats that provider-managed name as authoritative after creation. |
| `amount` | yes | Daily budget in account-currency units, for example `50` or `50.25`. Converted to Google Ads `amount_micros` (`1` unit = `1_000_000` micros) with at most six decimal places. |
| `explicitlyShared` | yes | `false` for a dedicated single-campaign budget; `true` for a shared budget. A shared budget can never become non-shared. A dedicated budget may become shared; Agoraform includes its name in the same mutation as required by Google Ads. |
| `deliveryMethod` | no | `STANDARD` (API default) for the supported Search workflow. `ACCELERATED` is rejected before mutation. Omitted values are not forced onto the remote resource. |

`period` is always `DAILY` and `type` is always `STANDARD`. Those fields,
provider-native IDs, resource names, `amountMicros`, `status`, and
`referenceCount` live in local state and on `RemoteResource.Computed`, not
in the manifest.

Equivalent live values, including `50` / `50.0` / `"50.00"` and enum case,
do not produce a plan diff. Amount comparison uses integer micros so
currency rounding stays deterministic. Updates use sparse field masks and
only send attributes that actually changed. For a dedicated budget, campaign-
driven name synchronization is ignored during planning and the name is not
resent on unrelated updates.

Import accepts the numeric campaign budget ID or the resource name
`customers/{customerId}/campaignBudgets/{id}` and stores the numeric ID:

```bash
agoraform import googleads.campaign_budget.brand 123456789
```

Lifetime, Smart, and other non-standard budgets fail import with guidance
instead of generating a lossy daily Search budget.

### `googleads.campaign`

Search campaigns. Agoraform creates and updates `SEARCH` campaigns only.
Performance Max, Display, Video, Shopping, App, and other channel types are
out of scope. Ad groups, ads, keywords, and targeting are separate
resources.

```yaml
resources:
  - address: googleads.campaign_budget.brand
    attributes:
      name: Brand daily budget
      amount: 50
      explicitlyShared: false
  - address: googleads.campaign.brand
    attributes:
      name: Brand
      status: PAUSED
      budget:
        $ref: googleads.campaign_budget.brand
      bidding:
        strategy: MANUAL_CPC
      network:
        googleSearch: true
        searchNetwork: true
        contentNetwork: false
        partnerSearchNetwork: false
```

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Campaign name. Must be unique in the customer. |
| `budget` | yes | `$ref` to a `googleads.campaign_budget`. Resolved to the provider-native budget at apply time. |
| `bidding` | yes | Search bidding strategy and optional settings. |
| `status` | no | `PAUSED` (default for new campaigns) or `ENABLED`. Omitted status is treated as `PAUSED`. |
| `advertisingChannelType` | no | Must be `SEARCH` when set. The provider always creates Search campaigns. |
| `network` | no | Search network / partner flags. When set, `googleSearch` must be `true`. Omitted network settings are not forced onto the remote resource. |
| `startDate` / `endDate` | no | Valid calendar dates in `YYYY-MM-DD` format. |
| `trackingUrlTemplate` / `finalUrlSuffix` | no | Optional URL tracking fields. Set an explicitly managed field to an empty string to clear its remote value; omission leaves it unmanaged. |

Supported `bidding.strategy` values:

| Strategy | Optional settings |
| --- | --- |
| `MANUAL_CPC` | `enhancedCpc` |
| `MAXIMIZE_CLICKS` | `cpcBidCeiling` in account-currency units; set `0` to clear an existing ceiling |
| `MAXIMIZE_CONVERSIONS` | `targetCpa` in account-currency units; set `0` to clear an existing optional target |
| `TARGET_CPA` | `targetCpa` (required) |
| `TARGET_ROAS` | `targetRoas` (required, `0.01`–`1000` inclusive) |
| `MAXIMIZE_CONVERSION_VALUE` | `targetRoas` (`0.01`–`1000` inclusive); set `0` to clear an existing optional target |

Omitting an optional bidding target leaves the current Google Ads value
unmanaged. Use the explicit `0` forms above when the desired state is to
remove a previously configured optional target.

New campaigns are created `PAUSED` unless configuration explicitly sets
`ENABLED`. Creates also send Search-safe network defaults when `network` is
omitted (`googleSearch` and `searchNetwork` true; Display and partner
networks false) and declare that the campaign does not contain EU political
advertising.

`advertisingChannelType` is always `SEARCH`. Provider-native IDs, resource
names, serving status, and related API defaults live in local state and on
`RemoteResource.Computed`, not in the manifest. Portfolio bidding
strategies, EU political advertising, and removed campaigns are rejected
with guidance.

Equivalent live values, including bidding enum aliases such as
`MANUAL_CPC` / `manual_cpc`, produce no plan diff. Updates use sparse field
masks.

Import accepts the numeric campaign ID or the resource name
`customers/{customerId}/campaigns/{id}` and stores the numeric ID.
Import reconstructs `budget` as a logical `$ref` when the campaign budget
is already bound in local state:

```bash
agoraform import googleads.campaign_budget.brand 123456789
agoraform import googleads.campaign.brand 987654321
```

Non-Search campaigns fail import with guidance instead of generating a
lossy Search configuration. Import the budget first, or apply it, then
re-import the campaign.

### `googleads.ad_group`

Search standard ad groups. Agoraform creates and updates `SEARCH_STANDARD`
ad groups only. Shopping, Display, Dynamic Search Ads, Performance Max
asset groups, keywords, targeting criteria, and ads are out of scope.

```yaml
resources:
  - address: googleads.campaign.brand
    attributes:
      name: Brand
      budget:
        $ref: googleads.campaign_budget.brand
      bidding:
        strategy: MANUAL_CPC
  - address: googleads.ad_group.brand
    attributes:
      name: Brand
      status: PAUSED
      campaign:
        $ref: googleads.campaign.brand
      type: SEARCH_STANDARD
      cpcBid: 1.5
```

Ad groups attach to a campaign with a logical `$ref` to this address.
Keywords and ads stay on separate resources; do not embed them on the
ad group.

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Ad group name. Must be unique within the campaign. |
| `campaign` | yes | `$ref` to a `googleads.campaign`. Resolved to the provider-native campaign at apply time. Immutable after create. |
| `status` | no | `PAUSED` (default for new ad groups) or `ENABLED`. Omitted status is treated as `PAUSED`. |
| `type` | no | Must be `SEARCH_STANDARD` when set. The provider always creates Search standard ad groups. |
| `cpcBid` | no | Max CPC bid in account-currency units, for example `1.5`. Converted to Google Ads `cpc_bid_micros` (`1` unit = `1_000_000` micros) with at most six decimal places. Effective when the campaign uses `MANUAL_CPC`; automated bidding strategies may ignore the ad-group bid. Omitted bids are not forced onto the remote resource. |

New ad groups are created `PAUSED` unless configuration explicitly sets
`ENABLED`. Creates always send `type: SEARCH_STANDARD`.

`type` is always `SEARCH_STANDARD`. Provider-native IDs, resource names,
and related API defaults live in local state and on
`RemoteResource.Computed`, not in the manifest. Shopping, DSA, and
removed ad groups are rejected with guidance.

Equivalent live values, including `1.5` / `"1.50"` and enum case, produce
no plan diff. Updates use sparse field masks. Campaign cannot be changed
after create.

Import accepts the numeric ad group ID or the resource name
`customers/{customerId}/adGroups/{id}` and stores the numeric ID.
Import reconstructs `campaign` as a logical `$ref` when the campaign is
already bound in local state:

```bash
agoraform import googleads.campaign.brand 987654321
agoraform import googleads.ad_group.brand 555666777
```

Non-Search ad group types fail import with guidance instead of generating
a lossy Search configuration. Import the campaign first, or apply it, then
re-import the ad group.

### `googleads.campaign_conversion_goal`

Campaign-level website conversion-goal biddability. Google Ads automatically
creates `CampaignConversionGoal` objects for campaign/category/origin
combinations. Agoraform reads those provider-created goals and reconciles
`biddable`; it never creates or deletes them.

Identify a goal by the managed campaign plus category and origin.
Provider-native identity is the computed `CAMPAIGN_ID~CATEGORY~ORIGIN` key
stored in local state.

```yaml
resources:
  - address: googleads.campaign.search_acquisition
    attributes:
      name: Search acquisition
      budget:
        $ref: googleads.campaign_budget.search_acquisition
      bidding:
        strategy: MAXIMIZE_CONVERSIONS
  - address: googleads.campaign_conversion_goal.trial_signup
    attributes:
      campaign:
        $ref: googleads.campaign.search_acquisition
      category: SIGNUP
      origin: WEBSITE
      biddable: true
      conversionAction:
        $ref: googleads.conversion_action.trial_started
```

| Attribute | Required | Description |
| --- | --- | --- |
| `campaign` | yes | `$ref` to a `googleads.campaign`. Resolved at apply time. |
| `category` | yes | Website category such as `SIGNUP`, `PURCHASE`, or `SUBSCRIBE_PAID`. |
| `origin` | yes | Must be `WEBSITE`. Other origins are out of scope. |
| `biddable` | yes | When true, Google Ads uses the goal as a campaign-level optimization goal. |
| `conversionAction` | no | `$ref` to a `googleads.conversion_action`. Use this when the matching conversion action is also managed so apply creates it before goal reconciliation. |

`resourceName` and the `CAMPAIGN_ID~CATEGORY~ORIGIN` identity are computed.
Customer-level goals remain `googleads.customer_conversion_goal`. Custom
conversion goals are out of scope.

If the expected provider-created goal is still missing after the campaign
and a matching conversion action exist, Agoraform reports that Google Ads
creates the object automatically and that Agoraform cannot create or delete
it.

Equivalent live values, including enum case, produce no plan diff.

Import accepts `CAMPAIGN_ID~CATEGORY~ORIGIN` or the resource name
`customers/{customerId}/campaignConversionGoals/{campaignId}~{category}~{origin}`.
Import the campaign first so Agoraform can reconstruct `campaign` as a
logical `$ref`:

```bash
agoraform import googleads.campaign.brand 987654321
agoraform import googleads.campaign_conversion_goal.trial_signup 987654321~SIGNUP~WEBSITE
```

Non-website origins fail import with guidance rather than reconstructing a
lossy `WEBSITE` goal. Import does not emit a `conversionAction` `$ref`; add
that optionally after import if the matching conversion action is also
managed.

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