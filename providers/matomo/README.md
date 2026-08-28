# Matomo provider

The Matomo provider is Agoraform's first production provider. v0.1.0
introduced analytics goals, v0.2.0 added Tag Manager variables, triggers,
tags, and declarative container publication, and the current line adds
`matomo.container` so a Tag Manager container can be declared and imported
instead of requiring a pre-created container id.

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
container. Apply creates/publishes a version only after all draft resource
changes succeed.

See [Matomo Tag Manager publication](../../docs/matomo-publishing.md).

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

Container deletion is not implemented. Import an existing container with:

```text
agoraform import matomo.container.main CONTAINER_ID
```

### `matomo.variable`

v0.2.0 Data Layer variable. In managed-container mode every child resource
must name the container:

```yaml
- address: matomo.variable.user_id
  attributes:
    container:
      $ref: matomo.container.main
    type: dataLayer
    key: userId
    name: User ID
```

In external-container mode omit `container` and set `MATOMO_CONTAINER_ID`.

`type: dataLayer` and `key` are required. `name` is optional and defaults to
`key`. `container` is required when a `matomo.container` resource is declared
and omitted when `MATOMO_CONTAINER_ID` selects an existing container.

Agoraform may encounter other, unmanaged Matomo variable types while reading a
container. Their scalar and structured parameter values are tolerated so they
do not prevent managed Data Layer variables from being planned or applied.

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

v0.2.0 Matomo Analytics event tag:

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
```

The target Tag Manager container must already contain a **Matomo Configuration**
variable. Matomo Analytics tags reference that variable for the Matomo URL,
site ID, and tracking configuration. v0.2 does not manage
`MatomoConfiguration` variables declaratively, so create one in Matomo before
managing `matomo.tag` resources if the container does not already have one.

Supported fields:

| Attribute | Required | Notes |
| --- | --- | --- |
| `type` | yes | v0.2 supports `matomoAnalytics`. |
| `trigger` | yes | `$ref` to managed `matomo.trigger`. |
| `eventCategory` | yes | Literal or supported variable `$ref`. |
| `eventAction` | yes | Literal or supported variable `$ref`. |
| `eventName` | no | Literal or supported variable `$ref`. |
| `eventValue` | no | Numeric literal/string or supported variable `$ref`. |
| `name` | no | Defaults from `eventAction` when possible. |
| `container` | managed mode | `$ref` to the declared `matomo.container`. |

Apply resolves logical references to provider-native identities at runtime.
Updates preserve unmanaged Matomo tag fields.

Import reconstructs logical trigger/variable `$ref`s when the related remote
objects are already represented in local state. Import prerequisites first.
When a `matomo.container` resource is already bound, child import also
reconstructs `container: { $ref: ... }`.

## Greenfield and existing-container workflows

**Greenfield (managed container).** Declare `matomo.container`, omit
`MATOMO_CONTAINER_ID`, and reference the container from every variable,
trigger, and tag. `apply` creates the container first, then child resources,
then publication if `publish: true`.

**Existing container.** Either import the container (`agoraform import
matomo.container.main CONTAINER_ID`) and switch children to `container: $ref`,
or keep `MATOMO_CONTAINER_ID` and omit the container resource and child
`container` attributes. The conversion-tracking example uses the external
`MATOMO_CONTAINER_ID` path.

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
