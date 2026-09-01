# Matomo provider

The Matomo provider is Agoraform's first production provider. v0.1.0
introduced analytics goals, v0.2.0 added Tag Manager variables, triggers,
tags, and declarative container publication, and v0.5.0 adds
`matomo.container` so a Tag Manager container can be declared and imported
instead of requiring a pre-created container id. `matomo.tag` also supports
Matomo's Google Ads conversion template so a managed Google Ads conversion
action can supply conversion ID and label through `{ $ref, output }`.

The Agoraform CLI remains provider-neutral. There is no Matomo-specific
`publish` command.

## Runtime configuration

Credentials and connection details come from environment variables:

```text
MATOMO_URL            required   Matomo base URL
MATOMO_TOKEN_AUTH     required   API token
MATOMO_SITE_ID        required for managed Matomo site resources
MATOMO_CONTAINER_ID   required for Tag Manager resources/publication when no matomo.container resource is declared
```

Tokens never belong in the manifest, logs, plan output, or local state.

Agoraform supports two explicit Tag Manager container modes:

1. **Externally managed container** — omit `matomo.container` and set
   `MATOMO_CONTAINER_ID` to an existing container, as in v0.2.0.
2. **Agoraform-managed container** — declare one `matomo.container` resource
   and omit `MATOMO_CONTAINER_ID`. Child Tag Manager resources must reference
   that container with `container: { $ref: matomo.container.* }`.

Mixing a managed container with `MATOMO_CONTAINER_ID` is rejected before
mutation. v0.5.0 supports at most one managed Matomo container per manifest.

## Declarative provider configuration

Non-secret publication desired state belongs in `agoraform.yaml`:

```yaml
apiVersion: agoraform.io/v1alpha1
providers:
  matomo:
    publish: true
    environment: live
resources:
  # ...
```

| Field | Default | Meaning |
| --- | --- | --- |
| `publish` | `false` | Publish the reconciled Tag Manager draft through normal `plan`/`apply`. |
| `environment` | `live` | Matomo Tag Manager environment to converge. |

Unknown provider configuration fields are rejected.

When publication is enabled, plan performs capability-aware, non-mutating
preflight with `TagManager.getAvailableEnvironmentsWithPublishCapability` and
makes publication visible as a provider action. The publication address is
the managed `matomo.container` resource when one is declared, or
`matomo.container.external` when `MATOMO_CONTAINER_ID` selects an existing
container. Apply and destroy create/publish a version only after all draft
resource mutations succeed. Destroy does not publish an intermediate version
when the same plan will delete the managed container.

See [Matomo Tag Manager publication](../../docs/matomo-publishing.md) and
[Destroy](../../docs/destroy.md).

## Resources

### `matomo.goal`

Introduced in v0.1.0:

```yaml
- address: matomo.goal.trial_started
  attributes:
    name: Trial Started
    matchAttribute: event_action
    pattern: trialStarted
```

Supported configurable fields are `name`, `matchAttribute`, `pattern`, and
`patternType`. Agoraform preserves unmanaged Goal fields during updates.
Provider-native goal IDs are stored in local state.

### `matomo.container`

A Tag Manager container:

```yaml
- address: matomo.container.main
  attributes:
    name: Main Website
    context: web
    description: primary web container
```

| Attribute | Required | Notes |
| --- | --- | --- |
| `name` | yes | Container display name. |
| `context` | yes | `web`, `android`, or `ios`. Immutable after create. |
| `description` | no | Optional description. |

Provider-native container IDs, draft version IDs, publication state, and
unmanaged Matomo flags such as `ignoreGtmDataLayer` remain computed. Agoraform
preserves those flags on update.

Container deletion is implemented by `agoraform destroy` for containers that
are present in the manifest and bound in local state. Containers selected
only by `MATOMO_CONTAINER_ID` are never deleted. Import an existing
container with:

```text
agoraform import matomo.container.main CONTAINER_ID
```

### `matomo.variable`

v0.2.0 Data Layer variable and v0.5.0 Matomo Configuration variable. In
managed-container mode every child resource must name the container.

```yaml
- address: matomo.variable.user_id
  attributes:
    container:
      $ref: matomo.container.main
    type: dataLayer
    key: userId
    name: User ID

- address: matomo.variable.config
  attributes:
    container:
      $ref: matomo.container.main
    type: matomoConfiguration
    name: Matomo Configuration
    matomoUrl: https://matomo.example.com
    siteId: 1
    enableLinkTracking: true
```

In external-container mode omit `container` and set `MATOMO_CONTAINER_ID`.

Supported `type` values:

| `type` | Required fields | Notes |
| --- | --- | --- |
| `dataLayer` | `key` | `name` optional; defaults to `key`. |
| `matomoConfiguration` | `name`, `matomoUrl`, `siteId` | `enableLinkTracking` optional boolean. |

`container` is required when a `matomo.container` resource is declared and
omitted when `MATOMO_CONTAINER_ID` selects an existing container. `type` is
immutable after the resource is bound.

`matomoConfiguration` manages a stable initial subset: Matomo URL, site ID,
and optional link tracking. Cookie, consent, domain, cross-domain, and custom
dimension settings are unowned. Updates read the complete remote variable and
round-trip every unowned template parameter unchanged. If those parameters
cannot be represented without loss, apply fails before mutation.

Agoraform may encounter other, unmanaged Matomo variable types while reading a
container. Their scalar and structured parameter values are tolerated so they
do not prevent managed variables from being planned or applied.

Import an existing configuration variable with:

```text
agoraform import matomo.variable.config VARIABLE_ID
```

### `matomo.trigger`

v0.2.0 Custom Event trigger:

```yaml
- address: matomo.trigger.trial_started
  attributes:
    container:
      $ref: matomo.container.main
    type: customEvent
    event: trialStarted
```

`type: customEvent` and `event` are required. `name` is optional and defaults
to `event`. `container` follows the same managed-versus-external rule as
variables. Updates preserve unmanaged description/conditions.

### `matomo.tag`

v0.2.0 Matomo Analytics event tag, and Google Ads conversion tags that fire
from a managed Matomo trigger:

```yaml
- address: matomo.tag.trial_started
  attributes:
    container:
      $ref: matomo.container.main
    type: matomoAnalytics
    trigger:
      $ref: matomo.trigger.trial_started
    eventCategory: signup
    eventAction: trialStarted
    matomoConfiguration:
      $ref: matomo.variable.config

- address: matomo.tag.google_ads_trial_started
  attributes:
    container:
      $ref: matomo.container.main
    type: googleAdsConversion
    name: Google Ads trial started
    trigger:
      $ref: matomo.trigger.trial_started
    conversionId:
      $ref: googleads.conversion_action.trial_started
      output: conversionId
    conversionLabel:
      $ref: googleads.conversion_action.trial_started
      output: conversionLabel
```

Matomo Analytics tags need a Matomo Configuration variable for the Matomo URL,
site ID, and tracking settings. Fully managed setups declare that variable as
`type: matomoConfiguration` and reference it from the tag. Existing containers
can keep using a single pre-existing configuration variable: omit
`matomoConfiguration` and Agoraform locates it in the target container. Import
the variable when you want Agoraform to own it.

If several configuration variables exist and none is named
`Matomo Configuration`, implicit discovery fails until you add an explicit
`$ref` or keep a single configuration variable.

Supported fields for `type: matomoAnalytics`:

| Attribute | Required | Notes |
| --- | --- | --- |
| `type` | yes | `matomoAnalytics` or `googleAdsConversion`. Immutable after create. |
| `trigger` | yes | `$ref` to managed `matomo.trigger`. |
| `eventCategory` | yes for `matomoAnalytics` | Literal or supported variable `$ref`. |
| `eventAction` | yes for `matomoAnalytics` | Literal or supported variable `$ref`. |
| `eventName` | no | Literal or supported variable `$ref`. |
| `eventValue` | no | Numeric literal/string or supported variable `$ref`. |
| `name` | no | Defaults from `eventAction` when possible, otherwise the address name. |
| `container` | managed mode | `$ref` to the declared `matomo.container`. |
| `matomoConfiguration` | no | `$ref` to a managed `matomo.variable` with type `matomoConfiguration`. Omitted analytics tags keep implicit/external discovery. |

Supported fields for `type: googleAdsConversion`:

| Attribute | Required | Notes |
| --- | --- | --- |
| `conversionId` | yes | Literal, Matomo variable `$ref`, or `{ $ref, output: conversionId }` from `googleads.conversion_action`. |
| `conversionLabel` | yes | Literal, Matomo variable `$ref`, or `{ $ref, output: conversionLabel }` from `googleads.conversion_action`. |
| `conversionValue` | no | Optional template value. Omitted values are not forced onto the remote tag. |
| `conversionCurrency` | no | Optional currency code. Omitted values are not forced onto the remote tag. |
| `conversionTransactionId` | no | Optional order/transaction id. Omitted values are not forced onto the remote tag. |

Google Ads conversion tags do not use a Matomo Configuration variable.

#### Google Ads conversion template (API contract)

Verified against Matomo Tag Manager 5.2+ (`GoogleAdsConversionTag`) and a live
container compiled from that template. UI labels are not the API contract.

| Agoraform | Matomo `type` / parameter key | Required |
| --- | --- | --- |
| `type: googleAdsConversion` | `GoogleAdsConversion` | yes |
| `conversionId` | `parameters.googleAdsConversionId` | yes |
| `conversionLabel` | `parameters.googleAdsConversionLabel` | yes |
| `conversionValue` | `parameters.googleAdsConversionValue` | no |
| `conversionCurrency` | `parameters.googleAdsConversionCurrency` | no |
| `conversionTransactionId` | `parameters.googleAdsConversionTransactionId` | no |

The compiled template reads those keys, prefixes a conversion id with `AW-`
when the stored value does not already include it, and calls
`gtag('event', 'conversion', { send_to, value, currency, transaction_id })`.
Empty optional parameters are treated as omitted so they do not create plan
diffs. Equivalent conversion ids such as `9988776655` and `AW-9988776655`
compare equal. Unmanaged template parameters and omitted optional fields are
preserved on update.

Agoraform configures the Matomo tag and can consume outputs from a managed
Google Ads conversion action. It does not emit the application event. The
application remains responsible for pushing the configured event to the Tag
Manager data layer (for example `trialStarted`). Matomo Tag Manager fires the
conversion tag; Google Ads owns the conversion action. The Google Ads
conversion tag also depends on a Google Tag (`gtag.js`) in the same container;
Agoraform does not manage that Google Tag resource.

Apply resolves logical references and selected outputs to provider-native
values at runtime. Updates preserve unmanaged Matomo tag fields.

Import reconstructs logical trigger/variable `$ref`s when the related remote
objects are already represented in local state. Import prerequisites first.
When a `matomo.container` resource is already bound, child import also
reconstructs `container: { $ref: ... }`. For a Google Ads conversion tag,
import reconstructs `{ $ref, output }` only when a bound
`googleads.conversion_action` uniquely matches the remote conversion id or
label through its declared non-sensitive outputs. Ambiguous and absent
matches emit the supported literal values so the imported configuration can
still produce a zero-change plan. Import never guesses a relationship.

## Greenfield and existing-container workflows

**Greenfield (managed container).** Declare `matomo.container`, omit
`MATOMO_CONTAINER_ID`, and reference the container from every variable,
trigger, and tag. Declare a `matomo.variable` with `type: matomoConfiguration`
and reference it from `matomo.tag` so the container does not need a
manually created configuration variable. `apply` creates the container first,
then child resources, then publication if `publish: true`.

**Existing container.** Either import the container (`agoraform import
matomo.container.main CONTAINER_ID`) and switch children to `container: $ref`,
or keep `MATOMO_CONTAINER_ID` and omit the container resource and child
`container` attributes. The [v0.2 conversion-tracking example](../../examples/matomo-conversion/README.md)
uses the external `MATOMO_CONTAINER_ID` path. The
[v0.5 Matomo + Google Ads lifecycle example](../../examples/matomo-googleads/README.md)
uses a managed container as its primary manifest and includes the compact
external-container variant.

Do not mix the two modes in one manifest.

## Publication idempotency

When `publish: true`, Agoraform compares the draft with the currently published
version for the target environment. It ignores provider-native IDs and computed
version metadata that naturally change when Matomo snapshots a draft.

Behavioral tag status is part of the comparison. For example, a paused draft
tag and active published tag require publication even if all other fields are
equal.

Before version creation, Agoraform verifies that the current credentials can
publish to the requested environment. If permission is missing or the
environment is invalid, no version is created.

## HTTP client

`providers/matomo/client` centralizes Matomo API calls, including:

- token-safe POST request construction;
- context cancellation and timeout handling;
- API error mapping and secret redaction;
- Goals read/create/update helpers;
- Tag Manager container/draft helpers;
- variable, trigger, and tag CRUD helpers;
- available/publishable environment queries;
- container version creation and publication.

Provider resource code should use this client rather than issuing ad hoc HTTP
requests.

## Safety

- `agoraform plan` does not mutate Matomo.
- `apply` never publishes unless publication is present in declarative provider
  configuration and visible in the plan.
- Failed draft mutations prevent publication.
- Permission/environment failure prevents version creation.
- Authentication tokens are redacted from provider errors.
- Tests use local `httptest` servers only.
