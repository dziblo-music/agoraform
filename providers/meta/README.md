# Meta Ads provider

The `meta` provider is the Meta Marketing API provider for Agoraform v0.6.0.
It is registered with the same provider-neutral lifecycle used by the existing
providers. Website conversion measurement and campaign management are
implemented here together with local image/video upload, ad-set and
ad-creative management, including the final ad serving relationship.

The [website conversion campaign example](../../examples/meta-website-campaign/README.md)
runs the complete supported graph through validate, plan, apply, import, and
destroy while every serving resource stays paused.

## Lifecycle contract

The provider keeps one exhaustive lifecycle declaration for every registered
v0.6.0 resource. Adding a resource type without its identity, create, update,
import, destroy, terminal-state, and relationship semantics fails the provider
contract tests.

| Type | Remote identity | Create | Mutable fields | Immutable/replacement fields | Import | Destroy |
| --- | --- | --- | --- | --- | --- | --- |
| `meta.pixel` | numeric Pixel/Dataset id | external/import-only; apply may uniquely adopt by name | none | `name` | numeric id | provider-owned; remains bound |
| `meta.custom_conversion` | numeric custom conversion id | supported | `name`, `defaultValue` | `pixel`, `rule`, `eventType` | numeric id; requires Pixel ref | remove/archive with `DELETE` |
| `meta.campaign` | numeric campaign id | supported | `name`, `status`, categories, existing budget value, bid strategy, budget sharing | objective, buying type, budget ownership/type | numeric id; preserves campaign/ad-set budget ownership | remove with `DELETE` |
| `meta.ad_set` | numeric ad-set id | supported | `name`, `status`, existing budget value, end time, targeting, compatible bid values | campaign, billing/optimization/destination, conversion object, start time, budget ownership/type | numeric id; requires campaign and applicable conversion refs | remove with `DELETE` |
| `meta.image` | Meta image hash | supported | none | local source content | image hash; no fabricated `source.file` | provider-owned; remains bound |
| `meta.video` | numeric video id | supported | none | local source content | numeric id; no fabricated `source.file` | delete with `DELETE` |
| `meta.ad_creative` | numeric creative id | supported | `name` | page/account, destination, copy, CTA, media, URL tags | numeric id; external media ids remain lossless literals | delete with `DELETE` |
| `meta.ad` | numeric ad id | supported | `name`, `status`, `creative` | `adSet` | numeric id; requires ad-set and creative refs | remove with `DELETE` |

Import reconstructs managed relationships through the provider-neutral output
catalog and the declared id outputs (`pixelId`, `customConversionId`,
`campaignId`, `adSetId`, and `adCreativeId`). A unique output match emits the
logical `$ref`. A missing or ambiguous match fails before state is written;
none of these relationship fields permits an external literal. Import
dependencies first. Creative `imageHash` / `videoId` values remain lossless
external identifiers unless configuration already uses a managed `$ref`.

The manifest reference graph is also the destroy graph. Its reverse order
removes ads before ad sets and creatives, ad sets before campaigns and custom
conversions, custom conversions before their Pixel/Dataset, and videos before
dependents that referenced them. Images are provider-owned and stay bound.
Every remote destroy mutation is a `DELETE`; it never activates delivery or
increases a budget. Confirmed terminal resources are unbound immediately,
while a failed operation and all not-yet-attempted resources remain bound for
a deterministic retry. Provider-owned pixels and images remain bound and make
teardown explicitly incomplete.

## Money values

Every money attribute — `dailyBudget`, `lifetimeBudget`, and `bidAmount` — is
declared in ad account currency units. A `20` in a USD account is USD 20.00, a
`20` in a JPY account is 20 yen, and fractions such as `20.50` are supported
where the currency has a minor unit.

Meta's API instead expects the account currency's minimum denomination, and
Meta's per-currency [offset](https://developers.facebook.com/docs/marketing-api/currencies/)
decides how many of those make up one unit. That offset is 100 for USD and EUR
but 1 for JPY, KRW, HUF, ISK, IDR, and TWD, so the conversion cannot be
assumed. Agoraform reads the configured ad account's currency the first time an
amount has to cross the API boundary, caches it, and converts there; plan
output, state, and import always show the account-currency amount. Resources
that declare no money never trigger that read. An ad account in a currency Meta
does not list fails rather than falling back to a guessed conversion.

Amounts accept at most two decimal places. An amount the account currency
cannot pay — 20.50 in a JPY account, for instance — fails before the mutation
is sent, because the offset is only known once the account currency has been
read.

## `meta.ad`

Ads connect one managed ad set to one managed creative. Both relationships
must be logical references; provider-native ids are resolved only during
apply:

```yaml
- address: meta.ad.instagram_trial
  attributes:
    name: Instagram Trial Ad
    adSet:
      $ref: meta.ad_set.instagram_acquisition
    creative:
      $ref: meta.ad_creative.instagram_video
    status: PAUSED
```

`status` defaults to `PAUSED`. Setting it to `ACTIVE` must be explicit and is
shown as a normal before/after update in `plan`. This keeps creation safe even
when the referenced campaign and ad set are already active. Import preserves
the remote configured status.

Agoraform updates `name`, `status`, and `creative` in place. This makes the
documented workflow of declaring a new immutable creative and repointing an ad
explicit and reviewable. The parent `adSet` cannot change in place; planning
fails with guidance to create a new logical ad instead of performing a hidden
replacement. Tracking specifications remain provider-owned in this initial
schema because Agoraform cannot round-trip arbitrary Meta tracking objects
deterministically.

Import uses the numeric ad id and reconstructs both references only when the
remote ids are uniquely bound to `meta.ad_set` and `meta.ad_creative`
resources in local state. Import those dependencies first. A successful create
response always preserves the returned `adId` for the state write, even if the
immediate refresh is temporarily unavailable, preventing a blind retry from
creating a duplicate ad. Destroy calls `DELETE /{ad_id}` and treats `DELETED`,
`ARCHIVED`, or absence as terminal and idempotent. Dependency ordering creates
the ad after its ad set and creative and destroys it before either dependency.

## `meta.image`

`meta.image` uploads an already-produced local file through
`POST /act_{ad-account-id}/adimages` and exposes the provider-native image
hash as the `imageHash` output. Creative production stays outside Agoraform.

```yaml
assets:
  root: ./assets

resources:
  - address: meta.image.instagram_trial_hero
    attributes:
      source:
        file: hero.jpg
```

`source.file` uses the provider-neutral local-asset model. JPEG, PNG, and GIF
are accepted, up to 30 MB. Bytes are streamed at apply time and never appear
in YAML, plan output, logs, or state. Plans show the relative path and
`sha256:…` digest. Unchanged files are not uploaded again. Meta may
deduplicate identical image bytes and return an existing hash; Agoraform
treats that as a successful create.

Image content is immutable after upload. Changing the bytes at the same
logical address fails planning with guidance to declare a new `meta.image`
and repoint the creative. `agoraform destroy` unbinds local state only; Meta
does not get a delete because the same hash can be shared across creatives
Agoraform does not manage.

Import binds an existing account image by hash and does not invent a local
filename or content digest:

```bash
agoraform import meta.image.instagram_trial_hero 0123456789abcdef0123456789abcdef
```

## `meta.video`

`meta.video` uploads an already-produced local file through
`POST /act_{ad-account-id}/advideos` with multipart field `source`, then polls
`GET /{video-id}?fields=id,title,length,status` until
`status.video_status` is `ready`. Upload acceptance is not treated as
ready-to-serve. Creatives that `$ref` the video are created only after that
output exists.

```yaml
- address: meta.video.product_demo
  attributes:
    source:
      file: meta/product-demo.mp4
```

MP4 and MOV files are accepted, up to 4 GB. Bytes are streamed. Polling is
bounded (five minutes by default); a timeout after a successful upload reports
the returned video id so you can retry apply or `agoraform import` that id
instead of assuming the file is usable. A Meta `error` processing status fails
without exposing `videoId`.

Video content is immutable after create. Changed bytes at the same logical
address fail planning. Destroy calls `DELETE /{video_id}` and treats absence
as terminal. Meta may reject deletion while a creative still references the
video.

Import binds a numeric video id without fabricating `source.file`:

```bash
agoraform import meta.video.product_demo 345678901234567
```

## `meta.ad_creative`

The creative surface manages deterministic website/link creatives. Media may
be an existing external Meta identifier **or** a logical `$ref` to a managed
`meta.image` / `meta.video` resource. Use exactly one of `imageHash`, `image`,
`videoId`, or `video`:

```yaml
- address: meta.ad_creative.instagram_video
  attributes:
    name: Instagram Trial Video
    pageId: "123456789012345"
    instagramUserId: "234567890123456"
    destinationUrl: https://example.com/trial
    primaryText: Start organizing your catalog today.
    headline: Start Your Free Trial
    description: Keep every pitch and placement organized.
    callToAction: LEARN_MORE
    video:
      $ref: meta.video.product_demo
    urlTags: utm_source=meta&utm_medium=paid_social&utm_campaign={{campaign.name}}&utm_content={{ad.name}}
```

For a static creative, use `image: { $ref: meta.image.NAME }` or an existing
account `imageHash`. The supported v26.0 `object_story_spec` mapping is intentionally
narrow: `page_id`, optional `instagram_user_id`, and exactly one `link_data`
or `video_data` object. Existing posts, catalogs, dynamic creative, templates,
playables, arbitrary `object_story_spec` JSON, and binary uploads on the
creative itself are rejected.

`destinationUrl` must be an absolute HTTP(S) URL. `urlTags` uses Meta's native
query-string format without a leading `?`; Meta dynamic macros are preserved
as literals. The verified website CTA subset is `GET_STARTED`, `LEARN_MORE`,
and `SIGN_UP`. `START_TRIAL` is a conversion event/category, not a creative CTA
in the pinned v26.0 API; use `LEARN_MORE` or `SIGN_UP` for this workflow.

Meta permits updating an Ad Creative's name, so Agoraform reconciles `name` in
place. Page/Instagram identity, destination, copy, CTA, media mode/id, and URL
tags are immutable after creation. Changing any of those fields fails planning
with guidance to declare a new logical creative and repoint the future ad
resource. Agoraform never performs a hidden replacement.

Import uses the numeric creative id and emits only the canonical typed fields.
External `imageHash` / `videoId` values remain lossless literals; managed
`meta.image` / `meta.video` references are declared in configuration with
`$ref`. Equivalent imported configuration produces a no-op plan.
Destroy uses Meta's native Ad Creative delete operation; absence or the
`DELETED` terminal status is idempotent success. Meta can reject deletion while
a creative is still referenced by an ad, in which case Agoraform preserves the
state binding and returns the provider error.

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
    lifetimeBudget: 500
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
      interests:
        - id: "6003139266461"
          name: Music production
      excludedCustomAudiences:
        - id: "111000222333444"
          name: Existing customers
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

`dailyBudget` and `lifetimeBudget` are mutually exclusive positive amounts in
ad account currency units, so `50` means USD 50.00 in a USD account. See
[money values](#money-values). A campaign with a campaign-level budget forbids
an ad-set budget; a campaign without one requires each ad set to declare a
budget. A lifetime budget requires both `startTime` and `endTime`. Timestamps
must be RFC3339 and are canonicalized to UTC. The supported bid strategies are
`LOWEST_COST_WITHOUT_CAP`, `LOWEST_COST_WITH_BID_CAP`, and `COST_CAP`; the
latter two require a positive `bidAmount`, also in account currency units,
while lowest cost without a cap forbids one.

The intentionally bounded targeting object supports:

- ISO two-letter `countries` and numeric Meta `regions` identifiers;
- `ageMin`/`ageMax` from 18 through 65 (defaulting to 18/65);
- `genders` values `MALE` and `FEMALE` (omission means all genders);
- positive numeric Meta `locales` identifiers;
- `publisherPlatforms: [INSTAGRAM]` and the Instagram positions `FEED`,
  `STORIES`, and `REELS`;
- `devicePlatforms` values `MOBILE` and `DESKTOP`;
- `customAudiences` and `excludedCustomAudiences` lists of existing Custom
  Audience objects identified by numeric `id`, with optional `name` display
  metadata;
- `interests` as a single OR-group of detailed-targeting entities identified
  by numeric `id`, with optional `name` display metadata.

At least one country or region is required. Instagram positions require the
Instagram publisher platform. Omitting both placement fields leaves placement
selection to Meta. Audience references are validated against a non-mutating
Graph read before create or targeting update: missing IDs, inaccessible IDs,
authorization failures, and unsupported Custom Audience subtypes are distinct
errors. Name-only inputs are rejected rather than fuzzy-matched. Equivalent
ID sets, including omitted names and provider ordering, produce a no-op plan.
Arbitrary targeting JSON, behaviors, lookalike generation, Advantage+
audience models, and additional publishers are rejected rather than silently
discarded. Destroying an ad set never deletes referenced Custom Audiences.
Attribution settings are not managed by this initial schema and remain
provider-owned. In the API payload, Agoraform maps the three documented
position names to Meta's `stream`, `story`, and `reels` values, Custom
Audience references to `custom_audiences` / `excluded_custom_audiences`, and
interests to a single `flexible_spec` interests group.

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
    lifetimeBudget: 500
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
exclusive positive amounts in ad account currency units (see
[money values](#money-values)). A campaign without either field uses ad-set
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
