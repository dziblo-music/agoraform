# Matomo provider

The Matomo provider is Agoraform's first production provider. v0.1.0
introduced analytics goals, and v0.2.0 adds Matomo Tag Manager variables,
triggers, tags, and declarative container publication.

The Agoraform CLI remains provider-neutral. There is no Matomo-specific
`publish` command.

## Runtime configuration

Credentials and connection details come from environment variables:

```text
MATOMO_URL            required   Matomo base URL
MATOMO_TOKEN_AUTH     required   API token
MATOMO_SITE_ID        required for managed Matomo site resources
MATOMO_CONTAINER_ID   required for Tag Manager resources/publication
```

Tokens never belong in the manifest, logs, plan output, or local state.
`MATOMO_CONTAINER_ID` identifies the single Tag Manager container managed by
the v0.2.0 implementation.

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
makes publication visible as a provider action. Apply creates/publishes a
version only after all draft resource changes succeed.

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

### `matomo.variable`

v0.2.0 Data Layer variable:

```yaml
- address: matomo.variable.user_id
  attributes:
    type: dataLayer
    key: userId
    name: User ID
```

`type: dataLayer` and `key` are required. `name` is optional and defaults to
`key`. Provider-native IDs/status/version metadata are computed, not desired
attributes. Updates preserve unmanaged Matomo fields.

Agoraform may encounter other, unmanaged Matomo variable types while reading a
container. Their scalar and structured parameter values are tolerated so they
do not prevent managed Data Layer variables from being planned or applied.

### `matomo.trigger`

v0.2.0 Custom Event trigger:

```yaml
- address: matomo.trigger.trial_started
  attributes:
    type: customEvent
    event: trialStarted
```

`type: customEvent` and `event` are required. `name` is optional and defaults
to `event`. Updates preserve unmanaged description/conditions.

### `matomo.tag`

v0.2.0 Matomo Analytics event tag:

```yaml
- address: matomo.tag.trial_started
  attributes:
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

Apply resolves logical references to provider-native identities at runtime.
Updates preserve unmanaged Matomo tag fields.

Import reconstructs logical trigger/variable `$ref`s when the related remote
objects are already represented in local state. Import prerequisites first.

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
