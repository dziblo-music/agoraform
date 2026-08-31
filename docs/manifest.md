# Manifest format (`v1alpha1`)

Agoraform configuration is a versioned YAML document containing declarative
provider configuration and desired resources.

```yaml
apiVersion: agoraform.io/v1alpha1
providers:
  matomo:
    publish: true
    environment: live
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
```

## Top-level fields

| Field | Required | Description |
| --- | --- | --- |
| `apiVersion` | yes | Must be `agoraform.io/v1alpha1`. |
| `providers` | no | Non-secret provider-specific desired state. |
| `resources` | no | Desired managed resources. Omitted/empty is valid. |

Provider credentials, tokens, passwords, and other secrets must never be put
in the manifest. They belong in runtime configuration such as environment
variables.

Provider configuration is intentionally separate from credentials. For
example, these are desired state and belong in Git:

```yaml
providers:
  matomo:
    publish: true
    environment: live
```

These do not:

```text
MATOMO_TOKEN_AUTH
MATOMO_URL
GOOGLE_ADS_DEVELOPER_TOKEN
GOOGLE_ADS_REFRESH_TOKEN
```

See [Matomo Tag Manager publication](matomo-publishing.md) and the
[Google Ads provider](../providers/googleads/README.md).

## Resource addresses

A resource address is three lowercase identifiers separated by dots:

```text
provider.type.name
```

Example:

```text
matomo.goal.trial_started
```

Each segment starts with a letter and may then contain lowercase letters,
digits, or underscores. Duplicate addresses are rejected.

## Resource references

Provider-neutral dependencies use a `$ref` object containing a logical
Agoraform address. Address-only references stay a single-key map:

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
      eventCategory: signup
      eventAction: trialStarted
```

The `$ref` value is always a logical Agoraform address, never a provider-native
ID. Ordinary strings remain provider-owned values even if they happen to look
like an address.

An optional `output` selector consumes one declared non-sensitive named
output from the referenced resource, including across providers:

```yaml
conversionId:
  $ref: googleads.conversion_action.trial_started
  output: conversionId
```

A reference without `output` still resolves to the full runtime binding
(provider-native identity plus computed outputs). A reference with `output`
resolves to that one value. Unknown, sensitive, unavailable, and wrong-kind
outputs fail before the dependent mutation. Arbitrary computed fields are not
automatically selectable.

Agoraform validates references and builds a directed dependency graph. Missing
references, self-references, and cycles fail before remote mutations.
Cross-provider output references create the same kind of dependency edge.

At apply time, logical references are resolved in dependency order. Those
provider-native values are not written back into the manifest.

## Resource attributes

`resources[].attributes` is provider-owned configuration. The core normalizes
YAML types and understands `$ref`; providers validate the actual schema.
Computed/read-only provider fields do not belong in configuration and must not
produce changes merely because provider-native IDs differ.

## Matomo resources

### `matomo.goal`

v0.1.0 introduced Matomo analytics goals. Common attributes:

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Goal name; immutable once local state binds the resource. |
| `matchAttribute` | yes | Goal matching mode such as `event_action`, `url`, or `manually`. |
| `pattern` | unless `manually` | Value to match. |
| `patternType` | no | `contains` (default), `exact`, or `regex`; numeric modes use provider defaults. |

Provider-native goal IDs live in local state, not attributes.

### `matomo.container`

A Tag Manager container can be declared instead of requiring
`MATOMO_CONTAINER_ID`:

```yaml
- address: matomo.container.main
  attributes:
    name: Main Website
    context: web
```

`name` and `context` (`web`, `android`, or `ios`) are required.
`description` is optional. Provider-native container IDs stay in local
state. v0.5.0 allows at most one `matomo.container` resource.

When that resource is present, Tag Manager children must reference it:

```yaml
container:
  $ref: matomo.container.main
```

When it is absent, omit `container` and set `MATOMO_CONTAINER_ID`. Mixing
the two modes is rejected. See the
[Matomo provider reference](../providers/matomo/README.md).

### `matomo.variable`

v0.2.0 supports Tag Manager Data Layer variables. v0.5.0 also supports
Matomo Configuration variables:

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

For `dataLayer`, `type` and `key` are required; `name` is optional and
defaults to `key`. For `matomoConfiguration`, `type`, `name`, `matomoUrl`,
and `siteId` are required; `enableLinkTracking` is optional.

### `matomo.trigger`

v0.2.0 supports Custom Event triggers:

```yaml
- address: matomo.trigger.trial_started
  attributes:
    type: customEvent
    event: trialStarted
```

`type` and `event` are required; `name` is optional and defaults to `event`.

### `matomo.tag`

v0.2.0 supports Matomo Analytics event tags. Google Ads conversion tags use
the Matomo Tag Manager `GoogleAdsConversion` template:

```yaml
- address: matomo.tag.trial_started
  attributes:
    type: matomoAnalytics
    trigger:
      $ref: matomo.trigger.trial_started
    eventCategory: signup
    eventAction: trialStarted
    matomoConfiguration:
      $ref: matomo.variable.config

- address: matomo.tag.google_ads_trial_started
  attributes:
    type: googleAdsConversion
    trigger:
      $ref: matomo.trigger.trial_started
    conversionId:
      $ref: googleads.conversion_action.trial_started
      output: conversionId
    conversionLabel:
      $ref: googleads.conversion_action.trial_started
      output: conversionLabel
```

Supported event fields may be literals or, where documented, references to
managed variables. `matomoConfiguration` is an optional `$ref` to a managed
`matomo.variable` of type `matomoConfiguration` and applies to Matomo
Analytics tags. Google Ads conversion tags consume `conversionId` and
`conversionLabel` as literals or as selected outputs from a managed
`googleads.conversion_action`. Agoraform does not emit the application event;
the application still pushes the configured data-layer event. See the
[Matomo provider reference](../providers/matomo/README.md)
for the complete resource-specific schema, template parameter mapping, and
preservation behavior. The
[v0.5 Matomo + Google Ads lifecycle example](../examples/matomo-googleads/README.md)
is a complete managed-container workflow that consumes those outputs.

## Google Ads resources

v0.4.0 Search resources form a dependency graph. Campaigns require a budget;
ad groups, targeting, and campaign conversion goals require a campaign;
keywords and Responsive Search Ads require an ad group:

```text
googleads.conversion_action
googleads.customer_conversion_goal
googleads.campaign_budget
  └── googleads.campaign
        ├── googleads.campaign_conversion_goal
        ├── googleads.campaign_location
        ├── googleads.campaign_language
        └── googleads.ad_group
              ├── googleads.keyword
              └── googleads.responsive_search_ad
```

New campaigns, ad groups, positive keywords, and Responsive Search Ads
default to `PAUSED`. See the
[complete Google Ads Search campaign example](../examples/googleads-search/README.md).

### `googleads.conversion_action`

Website conversion actions. Common attributes:

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Conversion action name. |
| `category` | yes | Website category such as `SIGNUP` or `PURCHASE`. |
| `value` | no | Default conversion value. |
| `count` | no | `ONE` or `MANY`. |
| `primaryForGoal` | no | Whether the action is primary for its conversion goal. |
| `status` | no | `ENABLED`, `HIDDEN`, or `REMOVED`. |

`type` is always website (`WEBPAGE`) and is computed. Provider-native IDs and
resource names live in local state, not attributes. Optional conversion
windows, currency, and `alwaysUseDefaultValue` are documented in the
[Google Ads provider reference](../providers/googleads/README.md).

```yaml
- address: googleads.conversion_action.trial_started
  attributes:
    name: Trial Started
    category: SIGNUP
    value: 0
    count: ONE
    primaryForGoal: true
```

See the [complete Google Ads conversion example](../examples/googleads-conversion/README.md)
and the [complete Google Ads Search campaign example](../examples/googleads-search/README.md).

### `googleads.customer_conversion_goal`

Account-default website conversion-goal biddability. Google Ads creates
these objects automatically; Agoraform only updates `biddable`.

| Attribute | Required | Description |
| --- | --- | --- |
| `category` | yes | Website category such as `SIGNUP` or `PURCHASE`. |
| `origin` | yes | `WEBSITE`. |
| `biddable` | yes | Whether the goal is an account-default optimization goal. |
| `conversionAction` | no | `$ref` to a managed `googleads.conversion_action` so apply creates that action first. |

```yaml
- address: googleads.customer_conversion_goal.signup
  attributes:
    category: SIGNUP
    origin: WEBSITE
    biddable: true
    conversionAction:
      $ref: googleads.conversion_action.trial_started
```

### `googleads.campaign_budget`

Daily Search campaign budgets. Amounts are declared in account-currency
units and normalized to Google Ads micros for API calls and plan
comparison.

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Budget name used for creation and unmanaged discovery. For dedicated budgets, Google synchronizes the live name with the attached campaign and Agoraform does not treat that synchronization as drift. |
| `amount` | yes | Daily budget in account-currency units. |
| `explicitlyShared` | yes | `false` for a dedicated budget; `true` for a shared budget. A shared budget cannot be changed back to dedicated. |
| `deliveryMethod` | no | `STANDARD` only for the supported Search workflow. `ACCELERATED` is rejected. |

```yaml
- address: googleads.campaign_budget.brand
  attributes:
    name: Brand daily budget
    amount: 50
    deliveryMethod: STANDARD
    explicitlyShared: false
```

Search campaigns reference the budget by logical address:

```yaml
- address: googleads.campaign.brand
  attributes:
    name: Brand
    budget:
      $ref: googleads.campaign_budget.brand
    bidding:
      strategy: MANUAL_CPC
```

Provider-native IDs, resource names, micros, period, and type are
computed. Updates use sparse field masks so unchanged sharing fields and
Google-managed dedicated-budget names are not resent. Converting a dedicated
budget to shared includes its name in the same mutation, as required by Google
Ads. See the [Google Ads provider reference](../providers/googleads/README.md).

### `googleads.campaign`

Search campaigns. The advertising channel is `SEARCH`. New campaigns default
to `PAUSED`. Campaigns must reference a `googleads.campaign_budget`.
Dynamic Search Ads settings are out of scope.

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Campaign name. |
| `budget` | yes | `$ref` to a `googleads.campaign_budget`. |
| `bidding` | yes | Search bidding strategy such as `MANUAL_CPC` or `MAXIMIZE_CONVERSIONS`. |
| `status` | no | `PAUSED` (default) or `ENABLED`. |
| `network` | no | Search network / partner flags. |
| `startDate` / `endDate` | no | `YYYY-MM-DD`. |
| `trackingUrlTemplate` / `finalUrlSuffix` | no | Optional URL tracking fields. |
| `locationTargeting` | no | Presence vs interest geotargeting: `positive` is `PRESENCE`, `PRESENCE_OR_INTEREST`, or `SEARCH_INTEREST`; `negative` is `PRESENCE` or `PRESENCE_OR_INTEREST`. |

```yaml
- address: googleads.campaign.brand
  attributes:
    name: Brand
    status: PAUSED
    budget:
      $ref: googleads.campaign_budget.brand
    bidding:
      strategy: MANUAL_CPC
```

Unsupported channel types fail validation before mutation. Ads, keywords,
and location/language criteria are separate resources. See the
[Google Ads provider reference](../providers/googleads/README.md).

### `googleads.ad_group`

Search standard ad groups. The type is `SEARCH_STANDARD`. New ad groups
default to `PAUSED`. Ad groups must reference a `googleads.campaign`.

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Ad group name. Unique within the campaign. |
| `campaign` | yes | `$ref` to a `googleads.campaign`. |
| `status` | no | `PAUSED` (default) or `ENABLED`. |
| `type` | no | Must be `SEARCH_STANDARD` when set. |
| `cpcBid` | no | Max CPC bid in account-currency units. |

```yaml
- address: googleads.ad_group.brand
  attributes:
    name: Brand
    status: PAUSED
    campaign:
      $ref: googleads.campaign.brand
    type: SEARCH_STANDARD
    cpcBid: 1.5
```

Unsupported types fail validation before mutation. Targeting and ads
remain separate resources. See the
[Google Ads provider reference](../providers/googleads/README.md).

### `googleads.keyword`

Search ad-group keyword criteria, including negative keywords. Keywords
must reference a `googleads.ad_group`. Match types are `EXACT`,
`PHRASE`, and `BROAD`. Keyword text is normalized to lowercase without
changing user intent. Text, match type, negative, and ad group are
immutable after create.

| Attribute | Required | Description |
| --- | --- | --- |
| `adGroup` | yes | `$ref` to a `googleads.ad_group`. |
| `text` | yes | Keyword text without match-type punctuation. |
| `matchType` | yes | `EXACT`, `PHRASE`, or `BROAD`. |
| `negative` | no | `true` for a negative keyword. Defaults to `false`. |
| `status` | no | Positive keywords default to `PAUSED` and may use `PAUSED` or `ENABLED`. Negative keywords default to `ENABLED`; their status is immutable after creation. |
| `cpcBid` | no | Optional max CPC bid override in account-currency units. Not allowed on negative keywords. |

```yaml
- address: googleads.keyword.brand_exact
  attributes:
    adGroup:
      $ref: googleads.ad_group.brand
    text: brand
    matchType: EXACT
    status: PAUSED
    cpcBid: 1.5
- address: googleads.keyword.competitor_neg
  attributes:
    adGroup:
      $ref: googleads.ad_group.brand
    text: competitor
    matchType: PHRASE
    negative: true
```

New negative keywords are created `ENABLED` when `status` is omitted. Creating
a new negative keyword as `PAUSED` is rejected because Google Ads does not
allow negative ad-group criteria to be updated later. For an existing negative
keyword, a requested status change fails during planning before any mutation.
Positive keyword status remains mutable.

Campaign-level negative keywords, Keyword Planner, audience or DSA
criteria, and keyword-level URL or tracking overrides are out of scope. See the
[Google Ads provider reference](../providers/googleads/README.md).

### `googleads.responsive_search_ad`

Responsive Search Ads attached to a Search ad group. Ads must reference
a `googleads.ad_group`. Agoraform manages serving copy, URLs, optional
display paths, optional pinning, and status. It does not generate
headlines or descriptions. Creative changes replace the underlying ad
lists; status updates the ad-group-ad relationship.

| Attribute | Required | Description |
| --- | --- | --- |
| `adGroup` | yes | `$ref` to a `googleads.ad_group`. |
| `finalUrls` | yes | One or more absolute `http` or `https` landing-page URLs. |
| `headlines` | yes | 3–15 headlines as strings or `{text, pin}` objects. |
| `descriptions` | yes | 2–4 descriptions as strings or `{text, pin}` objects. |
| `path1` | no | Optional display-path segment. Requires no `/`. |
| `path2` | no | Optional second path segment. Requires `path1`. |
| `status` | no | `PAUSED` (default) or `ENABLED`. |

```yaml
- address: googleads.responsive_search_ad.brand
  attributes:
    adGroup:
      $ref: googleads.ad_group.brand
    status: PAUSED
    finalUrls:
      - https://example.com/
    path1: shoes
    headlines:
      - text: Buy shoes online
        pin: HEADLINE_1
      - Free shipping today
      - Shop the collection
    descriptions:
      - Find shoes that fit your style.
      - Free returns on every order.
```

Dynamic Search Ads, Display, video, and Performance Max ads are out of
scope. See the
[Google Ads provider reference](../providers/googleads/README.md).

### `googleads.campaign_location`

Search campaign location criteria, including excluded locations. Locations
must reference a `googleads.campaign`. Prefer canonical names or country
codes; Agoraform resolves them to Google Ads geo target constants and
fails when a name is missing or ambiguous. Campaign, location, and
negative are immutable after create.

| Attribute | Required | Description |
| --- | --- | --- |
| `campaign` | yes | `$ref` to a `googleads.campaign`. |
| `location` | yes | Canonical name, ISO country code, or `geoTargetConstants/{id}`. |
| `negative` | no | `true` to exclude the location. Defaults to `false`. |

```yaml
- address: googleads.campaign_location.united_states
  attributes:
    campaign:
      $ref: googleads.campaign.brand
    location: United States
- address: googleads.campaign_location.exclude_canada
  attributes:
    campaign:
      $ref: googleads.campaign.brand
    location: Canada
    negative: true
```

Radius/proximity targeting and audience segments are out of scope.
Presence vs interest behavior is `locationTargeting` on
`googleads.campaign`. See the
[Google Ads provider reference](../providers/googleads/README.md).

### `googleads.campaign_language`

Search campaign language criteria. Languages must reference a
`googleads.campaign`. Prefer ISO codes such as `en`. Campaign and
language are immutable after create.

| Attribute | Required | Description |
| --- | --- | --- |
| `campaign` | yes | `$ref` to a `googleads.campaign`. |
| `language` | yes | ISO code, language name, or `languageConstants/{id}`. |

```yaml
- address: googleads.campaign_language.english
  attributes:
    campaign:
      $ref: googleads.campaign.brand
    language: en
```

See the [Google Ads provider reference](../providers/googleads/README.md).

### `googleads.campaign_conversion_goal`

Campaign-level website conversion-goal biddability. Google Ads creates
these objects automatically; Agoraform only updates `biddable`.

| Attribute | Required | Description |
| --- | --- | --- |
| `campaign` | yes | `$ref` to a managed `googleads.campaign`. |
| `category` | yes | Website category such as `SIGNUP` or `PURCHASE`. |
| `origin` | yes | `WEBSITE`. |
| `biddable` | yes | Whether the goal is a campaign-level optimization goal. |
| `conversionAction` | no | `$ref` to a managed `googleads.conversion_action` so apply creates that action first. |

```yaml
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

See the [complete Google Ads Search campaign example](../examples/googleads-search/README.md).

## Matomo provider desired state

v0.2.0 recognizes:

```yaml
providers:
  matomo:
    publish: true
    environment: live
```

| Field | Required | Description |
| --- | --- | --- |
| `publish` | no | Boolean; default `false`. When true, converge the configured Tag Manager container into the target environment through normal `plan`/`apply`. |
| `environment` | no | Tag Manager environment; default `live`. |

Unknown provider configuration fields are rejected.

## Validation

```bash
agoraform validate
agoraform validate -f path/to/manifest.yaml
agoraform validate path/to/manifest.yaml
```

Validation covers:

- YAML and `apiVersion`;
- provider names and provider-specific configuration;
- resource address syntax and duplicates;
- `$ref` validity and dependency cycles;
- registered provider/resource support;
- provider connectivity;
- provider-specific resource fields.

Matomo runtime connection settings come from `MATOMO_URL`,
`MATOMO_TOKEN_AUTH`, and `MATOMO_SITE_ID`. Tag Manager resources and
publication also require either a `matomo.container` resource or
`MATOMO_CONTAINER_ID`. Google Ads connection settings come
from `GOOGLE_ADS_DEVELOPER_TOKEN`, `GOOGLE_ADS_CLIENT_ID`,
`GOOGLE_ADS_CLIENT_SECRET`, `GOOGLE_ADS_REFRESH_TOKEN`, and
`GOOGLE_ADS_CUSTOMER_ID`.

See [plan.md](plan.md), [apply.md](apply.md), [import.md](import.md), and
[state.md](state.md).
