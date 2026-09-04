# Meta Ads provider

The `meta` provider is the Meta Marketing API provider for Agoraform v0.6.0.
It is registered with the same provider-neutral lifecycle used by the existing
providers. Website conversion measurement and campaign management are
and ad-set management are implemented here. Creative and ad resources remain
later v0.6.0 issues.

## `meta.ad_set`

Ad sets connect a managed campaign to budget, schedule, targeting, placements,
and website conversion measurement. This fixed-duration example targets only
Instagram Feed, Stories, and Reels in the United States:

```yaml
- address: meta.ad_set.instagram_acquisition
  attributes:
    name: Instagram Acquisition
    status: PAUSED
    campaign:
      $ref: meta.campaign.acquisition
    lifetimeBudget: 50000
    startTime: "2026-09-01T05:00:00Z"
    endTime: "2026-10-01T05:00:00Z"
    billingEvent: IMPRESSIONS
    optimizationGoal: OFFSITE_CONVERSIONS
    bidStrategy: LOWEST_COST_WITHOUT_CAP
    destinationType: WEBSITE
    pixel:
      $ref: meta.pixel.website
    customConversion:
      $ref: meta.custom_conversion.trial_started
    targeting:
      countries: [US]
      ageMin: 18
      ageMax: 65
      publisherPlatforms: [INSTAGRAM]
      instagramPositions: [FEED, STORIES, REELS]
      devicePlatforms: [MOBILE]
```

`status` defaults to `PAUSED`; enabling delivery requires an explicit
`ACTIVE` configuration and therefore appears in `plan`. `billingEvent`
currently supports `IMPRESSIONS`. The initial optimization subset is
`OFFSITE_CONVERSIONS` and `LINK_CLICKS`, both with `destinationType: WEBSITE`.
Website conversions require logical `pixel` and `customConversion` references.
The referenced Custom Conversion must use the same pixel, and
`OFFSITE_CONVERSIONS` requires an `OUTCOME_SALES` campaign. Link-click ad sets
do not accept conversion references and require an `OUTCOME_TRAFFIC` or
`OUTCOME_SALES` campaign.

Budgets use the ad-account currency's smallest unit, exactly like campaign
budgets. `dailyBudget` and `lifetimeBudget` are mutually exclusive. A campaign
with a campaign-level budget forbids an ad-set budget; a campaign without one
requires each ad set to declare a budget. A lifetime budget requires both
`startTime` and `endTime`. Timestamps must be RFC3339 and are canonicalized to
UTC. The supported bid strategies are `LOWEST_COST_WITHOUT_CAP`,
`LOWEST_COST_WITH_BID_CAP`, and `COST_CAP`; the latter two require a positive
whole-number `bidAmount`, while lowest cost without a cap forbids one.

The intentionally bounded targeting object supports:

- ISO two-letter `countries` and numeric Meta `regions` identifiers;
- `ageMin`/`ageMax` from 18 through 65 (defaulting to 18/65);
- `genders` values `MALE` and `FEMALE` (omission means all genders);
- positive numeric Meta `locales` identifiers;
- `publisherPlatforms: [INSTAGRAM]` and the Instagram positions `FEED`,
  `STORIES`, and `REELS`;
- `devicePlatforms` values `MOBILE` and `DESKTOP`.

At least one country or region is required. Instagram positions require the
Instagram publisher platform. Omitting both placement fields leaves placement
selection to Meta. Arbitrary targeting JSON, interests, custom audiences,
lookalikes, and additional publishers are rejected rather than silently
discarded. Attribution settings are not managed by this initial schema and
remain provider-owned. In the API payload, Agoraform maps the three documented
position names to Meta's `stream`, `story`, and `reels` values.

Updates support name, serving status, the existing budget value, end time,
targeting, and compatible bid values. Campaign, billing/optimization goal,
destination, promoted conversion object, start time, and budget ownership/type
are treated as immutable or unsafe; Agoraform fails planning rather than
performing a hidden replacement.

Import uses the numeric ad-set id and reconstructs campaign, pixel, and Custom
Conversion references only when each remote id has one unique binding in local
state. Import dependencies first. Unsupported targeting or ambiguous/unbound
relationships fail without persisting the ad-set identity. The declared output
is `adSetId`. Destroy calls `DELETE /{ad_set_id}` and treats `DELETED`,
`ARCHIVED`, or absence as terminal and idempotent.

## `meta.campaign`

Declare an Outcome-Driven Ad Experiences (ODAX) campaign:

```yaml
- address: meta.campaign.acquisition
  attributes:
    name: Website Acquisition
    objective: OUTCOME_SALES
    status: PAUSED
    specialAdCategories: []
    buyingType: AUCTION
    lifetimeBudget: 50000
    bidStrategy: LOWEST_COST_WITHOUT_CAP
```

`objective` accepts the six current outcome values:
`OUTCOME_APP_PROMOTION`, `OUTCOME_AWARENESS`, `OUTCOME_ENGAGEMENT`,
`OUTCOME_LEADS`, `OUTCOME_SALES`, and `OUTCOME_TRAFFIC`. Legacy objective
names are rejected. `specialAdCategories` is required; use an empty list when
none apply. Supported categories are `CREDIT`, `EMPLOYMENT`,
`FINANCIAL_PRODUCTS_SERVICES`, `HOUSING`, `ISSUES_ELECTIONS_POLITICS`, and
`ONLINE_GAMBLING_AND_GAMING`. `NONE` is accepted as a single value and
canonicalized to an empty list.

`status` defaults to `PAUSED`. `ACTIVE` must be declared explicitly and its
before/after value is shown by `plan`. Import preserves the remote `ACTIVE` or
`PAUSED` configured status; it never pauses an existing campaign.

`buyingType` defaults to `AUCTION`. `RESERVED` requires a materially different
schema and is not supported. `dailyBudget` and `lifetimeBudget` are mutually
exclusive positive integers in the ad account currency's smallest unit (for
example, `5000` means USD 50.00 and means JPY 5,000). Using the API-native
smallest unit avoids rounding and makes campaign and future ad-set budget
normalization deterministic. A campaign without either field uses ad-set
budget ownership. `bidStrategy` is optional and is valid only with a
campaign-level budget.

For campaigns whose budgets live on ad sets, `adSetBudgetSharingEnabled`
defaults to `false` and is sent explicitly as required by current Graph API
versions. It may be set to `true` only when neither campaign-level budget field
is present; the parameter is omitted for campaign-budget campaigns.

Agoraform updates `name`, `status`, the existing budget value,
`specialAdCategories`, and a declared `bidStrategy`. Objective, buying type,
and campaign budget ownership/type cannot change in place; `plan` fails with
guidance instead of hiding a replacement or an unsafe migration. A configured
bid strategy cannot be cleared through the current schema.

Import uses the numeric campaign id and emits canonical YAML. The declared
output is `campaignId`. Destroy calls `DELETE /{campaign_id}` and treats
`DELETED`, `ARCHIVED`, or absence as terminal, including idempotent repeated
destroy runs.

## Website conversion measurement

Agoraform manages Meta **configuration** for website acquisition campaigns.
It does not install browser Pixel code, emit `fbq` events, send Conversions
API server events, manage application SDKs, or generate application code.

### `meta.pixel`

A website Pixel/Dataset is a Business Manager / Events Manager object.
Marketing API `POST /act_{ad-account-id}/adspixels` exists, but creation is
not a stable, deterministic ad-account operation: many accounts already have
a pixel, ownership lives on the business, and documented creation failures
include API codes 6200 and 6202. Agoraform therefore **does not create or
delete** pixels.

Declare the existing event source by name and bind it:

```yaml
- address: meta.pixel.website
  attributes:
    name: Website
```

- `agoraform import meta.pixel.website <PIXEL_ID>` binds a specific pixel
  without mutation.
- An unbound pixel whose `name` uniquely matches one account `adspixels`
  row is adopted on apply. Missing and ambiguous names are errors.
- Bound reads use the persisted numeric id and never rebind by name.
- `name` is not updated through Agoraform.
- Destroy reports the pixel as provider-owned and leaves the remote object.

The declared output `pixelId` is the numeric Pixel/Dataset id used by
application-side instrumentation (`fbq('init', pixelId)` or an equivalent
external tag). Agoraform never reads or stores the Pixel JavaScript snippet.

### `meta.custom_conversion`

A website Custom Conversion references a managed pixel with a logical `$ref`,
a Meta-native `rule`, and a `custom_event_type` category:

```yaml
- address: meta.custom_conversion.trial_started
  attributes:
    name: Trial Started
    eventType: START_TRIAL
    pixel:
      $ref: meta.pixel.website
    rule:
      and:
        - event:
            eq: StartTrial
    defaultValue: 0
```

`eventType` is the Marketing API `custom_event_type` enum (for example
`START_TRIAL`, `PURCHASE`, `LEAD`, `COMPLETE_REGISTRATION`, `OTHER`).
`rule` is the documented Meta rule object. The common website-event form
matches `event` with `eq`. URL rules use `url` with operators such as
`i_contains`. Agoraform sends `action_source_type=website` and
`event_source_id` from the referenced pixel.

Optional `defaultValue` maps to `default_conversion_value`. The API has no
stable currency field on Custom Conversion, so currency is not configurable.

Create, read, import, and destroy follow the v26.0 Custom Conversion
contract. Update may change `name` and `defaultValue` only. Changing `rule`,
`pixel`, or `eventType` is rejected rather than emulated. Destroy issues
`DELETE /{custom_conversion_id}` and treats a subsequent `is_archived=true`
response or object absence as the terminal state.

An unbound `meta.custom_conversion` is created. Agoraform does not discover
or adopt existing Custom Conversions by name because an equivalent object
would not acquire a persisted identity. Import an existing object explicitly.

Import a Custom Conversion after the pixel is bound:

```bash
agoraform import meta.pixel.website 111222333444555
agoraform import meta.custom_conversion.trial_started 998877665544332
```

Import reconstructs `pixel: { $ref: meta.pixel.NAME }` only when that pixel
id is uniquely bound. Otherwise import fails instead of writing a remote id
into YAML. Offline, app, and other non-pixel event sources are rejected.

The declared output `customConversionId` is the numeric Custom Conversion
id. Application event names used in `rule` remain an external contract;
Agoraform does not emit those events.

## Runtime configuration

Supply both required values through environment variables or a local
`.agoraform.env` file:

```dotenv
META_ACCESS_TOKEN=replace-with-access-token
META_AD_ACCOUNT_ID=act_123456789012345
```

`META_AD_ACCOUNT_ID` accepts either `act_123456789012345` or
`123456789012345`; Agoraform normalizes both to the `act_` form. Other forms
are rejected before an API call.

For unattended automation, prefer a Meta Business system-user access token
with access to the selected ad account and the `ads_management` permission.
Agoraform remains compatible with other Meta token types that Meta supports
for the same API operations. Agoraform consumes an existing token; it does
not create Meta apps, Business Manager accounts, system users, or tokens.

The access token must not be placed in `agoraform.yaml`, command arguments,
state, examples, or source control. The ad-account selection is runtime
configuration as well, which makes one manifest reusable across environments.

Declare the provider with an empty configuration block:

```yaml
apiVersion: agoraform.io/v1alpha1
providers:
  meta: {}
resources: []
```

## Connection validation

`agoraform validate` and `agoraform plan` perform read-only checks when the
manifest declares `meta` or contains a Meta resource. Validation:

1. checks that the token reports a granted `ads_management` permission;
2. reads the configured ad account and confirms its identity.

Authentication failures, insufficient permissions, and inaccessible accounts
are reported separately. Both checks use versioned GET requests and do not
create, update, or delete any Meta object.

## API version policy

Agoraform v0.6.0 pins all Meta Graph and Marketing API requests to **v26.0**.
It never calls an unversioned endpoint or an implicit `latest` API.

The version is centralized in `providers/meta/client/version.go`. Upgrading it
requires a reviewed code change, review of Meta's version changelog and
migration guidance, and successful provider client/resource tests against the
new version before release. Patch releases do not silently switch API
versions.

## Client behavior

The reusable client provides versioned GET, form-encoded POST and DELETE,
cursor pagination, per-request timeouts, context cancellation, bounded JSON
responses, and Meta error mapping. API code/subcode, transient classification,
request ID, and trace ID are retained when available.

The client classifies transient and rate-limit failures but does not
automatically retry requests. Resource implementations must decide whether an
operation is safe to retry, particularly for mutations. Diagnostics redact
the configured token and common credential-bearing headers/query parameters.

Automated tests use local HTTP servers only and never call Meta production
services.
