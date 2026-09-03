# Meta Ads provider

The `meta` provider is the Meta Marketing API provider for Agoraform v0.6.0.
It is registered with the same provider-neutral lifecycle used by the existing
providers. Website conversion measurement is implemented here. Campaign,
ad-set, creative, ad, and targeting types remain later v0.6.0 issues.

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
