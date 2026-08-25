# Matomo provider

The Matomo provider is Agoraform's first production provider. It loads
credentials from the environment, talks to Matomo through
`providers/matomo/client`, and manages `matomo.goal` plus Tag Manager
`matomo.variable`, `matomo.trigger`, and `matomo.tag`.

`agoraform apply` writes Tag Manager resources to the configured container
draft. `agoraform publish` creates a container version from that draft and
publishes it to the configured environment. Apply never publishes.

## Configuration

Credentials are never read from manifests. Set them in the process
environment:

```text
MATOMO_URL            required   Matomo base URL, for example https://matomo.example.com
MATOMO_TOKEN_AUTH     required   API token
MATOMO_SITE_ID        required for goals and Tag Manager resources
MATOMO_CONTAINER_ID   required for Tag Manager resources and publish   container id such as 6OMh6taM
MATOMO_ENVIRONMENT    optional for publish   default live
```

`MATOMO_CONTAINER_ID` is the Tag Manager container Agoraform manages. Variable,
trigger, and tag create, read, and update operate on that container's draft
version. It is not used by `matomo.goal`. `agoraform publish` snapshots that
draft into a new version and publishes it to `MATOMO_ENVIRONMENT` (default
`live`). Re-running publish when the draft already matches the published
version is a no-op.

### Precedence

1. Values passed to `matomo.New` / `matomo.NewWithHTTPClient` (tests and
   programmatic construction)
2. `MATOMO_*` environment variables (`matomo.NewFromEnv`)

There is no manifest or Git-tracked file source for tokens. Do not put
`token_auth` in `MATOMO_URL`.

## Resources

### `matomo.goal`

Declares a Matomo analytics goal. Initial create/discovery looks up a goal by
`name`. Once Agoraform manages the resource, persist Matomo's goal ID in
[local state](../../docs/state.md), not in the manifest:

```yaml
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
```

```json
{
  "version": 1,
  "resources": {
    "matomo.goal.trial_started": {
      "provider": "matomo",
      "remoteId": "12"
    }
  }
}
```

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Goal name. Immutable once local state binds the resource. |
| `matchAttribute` | yes | `url`, `title`, `file`, `external_website`, `manually`, `visit_duration`, `visit_total_actions`, `visit_total_pageviews`, `event_action`, `event_category`, or `event_name`. |
| `pattern` | unless `manually` | Value to match. |
| `patternType` | no | `contains` (default), `exact`, or `regex`. Numeric match attributes default to `greater_than`. |

Lowercase `idgoal` from Matomo responses remains a computed field on the live
resource. Do not set `idGoal` in configuration; manifests that still contain
it are rejected. A stale persisted identity is an error, not a create
candidate, so Agoraform will not silently create a duplicate after a rename
or remote loss.

Other Matomo Goal API fields (`description`, `revenue`, `caseSensitive`,
`allowMultipleConversionsPerVisit`, and `useEventValueAsRevenue`) are not
managed in 0.1.0. They are nevertheless preserved during updates: Agoraform
re-reads the live goal immediately before mutation and sends those values back
to Matomo because `Goals.updateGoal` otherwise resets omitted parameters to
API defaults.

For `patternType: exact`, `url`, `file`, and `external_website` patterns must
start with `http://` or `https://`, matching Matomo's validation behavior.

A missing unbound remote goal plans as a create. A changed supported field
plans as an update. Goal deletion is out of scope. `agoraform apply`
executes those planned creates and updates and persists Matomo's goal ID
in local state.

`agoraform import matomo.goal.NAME ID` reads an existing goal by id, prints
configurable YAML, and stores that id in local state. It does not recreate
the goal or emit `idGoal` as a manifest attribute. See
[import.md](../../docs/import.md).

### `matomo.variable`

Declares a Matomo Tag Manager variable in the configured container draft.
v0.2 starts with Data Layer variables. Initial discovery looks up a variable
by `name` (defaulting to `key` when `name` is omitted). Once Agoraform
manages the resource, persist Matomo's variable ID in
[local state](../../docs/state.md), not in the manifest:

```yaml
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.variable.user_id
    attributes:
      type: dataLayer
      key: userId
```

| Attribute | Required | Description |
| --- | --- | --- |
| `type` | yes | Variable template. v0.2 supports `dataLayer`. |
| `key` | yes for `dataLayer` | Data Layer property name, sent to Matomo as `dataLayerName`. No leading or trailing whitespace. At most 300 characters. If `name` is omitted, `key` is also the Matomo variable name and must be at most 255 characters. |
| `name` | no | Tag Manager display name. Defaults to `key`. No leading or trailing whitespace. At most 255 characters. Internal spaces such as `User ID` are allowed. |

A missing unbound remote variable plans as a create. A changed `key` or
`name` plans as an update. Equivalent configuration, including an omitted
`name` that matches `key`, produces a zero-change plan. Deletion is out of
scope.

Remote fields such as `idvariable`, `idcontainerversion`, `status`,
`description`, `default_value`, `lookup_table`, and `typeMetadata` are
computed. Do not set them in configuration. Updates re-read the live
variable and send those unmanaged values back because
`TagManager.updateContainerVariable` otherwise resets omitted parameters.
`type` is immutable after create.

`agoraform import matomo.variable.NAME ID` reads an existing draft variable
by numeric id. It does not recreate the variable or emit `idVariable` as a
manifest attribute. See [import.md](../../docs/import.md).

Other Tag Manager variable templates are not managed in v0.2.

### `matomo.trigger`

Declares a Matomo Tag Manager trigger in the configured container draft.
v0.2 starts with Custom Event triggers. Initial discovery looks up a trigger
by `name` (defaulting to `event` when `name` is omitted). Once Agoraform
manages the resource, persist Matomo's trigger ID in
[local state](../../docs/state.md), not in the manifest:

```yaml
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStarted
```

| Attribute | Required | Description |
| --- | --- | --- |
| `type` | yes | Trigger template. v0.2 supports `customEvent`. |
| `event` | yes for `customEvent` | Data Layer event name, sent to Matomo as `eventName`. No leading or trailing whitespace. At most 300 characters. If `name` is omitted, `event` is also the Matomo trigger name and must be at most 255 characters. |
| `name` | no | Tag Manager display name. Defaults to `event`. No leading or trailing whitespace. At most 255 characters. Internal spaces such as `Trial Started` are allowed. |

A missing unbound remote trigger plans as a create. A changed `event` or
`name` plans as an update. Equivalent configuration, including an omitted
`name` that matches `event`, produces a zero-change plan. Deletion is out of
scope.

Remote fields such as `idtrigger`, `idcontainerversion`, `status`,
`description`, `conditions`, and `typeMetadata` are computed. Do not set
them in configuration. Updates re-read the live trigger and send unmanaged
`description` and `conditions` back because `TagManager.updateContainerTrigger`
otherwise resets omitted description to an empty string and omitted conditions
to an empty list. `type` is immutable after create.

`agoraform import matomo.trigger.NAME ID` reads an existing draft trigger
by numeric id. It does not recreate the trigger or emit `idTrigger` as a
manifest attribute. See [import.md](../../docs/import.md).

Other Tag Manager trigger templates and condition/expression builders are
not managed in v0.2.

### `matomo.tag`

Declares a Matomo Tag Manager tag in the configured container draft.
v0.2 starts with Matomo Analytics event tags. Initial discovery looks up a
tag by `name` (defaulting to `eventAction` when `name` is omitted and
`eventAction` is a string). Once Agoraform manages the resource, persist
Matomo's tag ID in [local state](../../docs/state.md), not in the manifest:

```yaml
apiVersion: agoraform.io/v1alpha1
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

| Attribute | Required | Description |
| --- | --- | --- |
| `type` | yes | Tag template. v0.2 supports `matomoAnalytics` (Matomo's `Matomo` tag with `trackingType` `event`). |
| `trigger` | yes | `$ref` to a `matomo.trigger` resource. Sent as `fireTriggerIds`. Apply resolves the provider-native trigger id at runtime. |
| `eventCategory` | yes | Event category. A string, or a `$ref` to a `matomo.variable` (sent as `{{Variable Name}}`). No leading or trailing whitespace. At most 500 characters when a string. |
| `eventAction` | yes | Event action. Same rules as `eventCategory`. If `name` is omitted and `eventAction` is a string, it is also the Matomo tag name and must be at most 255 characters. |
| `eventName` | no | Event name. Same string-or-`$ref` rules as `eventCategory`. |
| `eventValue` | no | Numeric event value (number or numeric string), or a `$ref` to a `matomo.variable` (sent as `{{Variable Name}}`). Non-numeric literal strings are rejected before mutation. |
| `name` | no | Tag Manager display name. Defaults to `eventAction` when that attribute is a string, otherwise the logical resource name. No leading or trailing whitespace. At most 255 characters. |

A missing unbound remote tag plans as a create. A changed configurable field
or a different trigger `$ref` plans as an update. Equivalent configuration,
including an omitted `name` that matches `eventAction`, produces a
zero-change plan. Deletion is out of scope.

Remote fields such as `idtag`, `idcontainerversion`, `status`,
`description`, `fireLimit`, `blockTriggerIds`, `matomoConfig`, and
`typeMetadata` are computed. Do not set them in configuration. Create
attaches the container's Matomo Configuration variable as `matomoConfig`.
Updates re-read the live tag and send unmanaged values back because
`TagManager.updateContainerTag` otherwise resets omitted parameters.
`type` is immutable after create.

`agoraform import matomo.tag.NAME ID` reads an existing draft tag by numeric
id. It does not recreate the tag or emit `idTag` as a manifest attribute.
When the fire trigger is already bound in local state, import reconstructs
`trigger` as a logical `$ref`. Event fields that use `{{Variable Name}}`
become `$ref`s when those variables are managed. Import the related trigger
(and any referenced variables) first. See [import.md](../../docs/import.md).

Other Tag Manager tag templates (including pageview, goal, and custom HTML)
are not managed in v0.2.

## HTTP client

`providers/matomo/client` provides:

- POST request construction (`module=API`, `format=JSON`)
- token authentication in the request body (never on the query string)
- context cancellation
- a 30s default timeout
- JSON decoding and Matomo `{"result":"error"}` mapping
- secret redaction in returned errors
- Goals helpers: `GetGoals`, `AddGoal`, `UpdateGoal`, and preservation-safe updates
- Tag Manager helpers: `GetContainer`, draft version resolution, `GetContainerVariables`, `AddContainerVariable`, preservation-safe `UpdateContainerVariable`, `GetContainerTriggers`, `AddContainerTrigger`, preservation-safe `UpdateContainerTrigger`, `GetContainerTags`, `AddContainerTag`, preservation-safe `UpdateContainerTag`, `GetAvailableEnvironments`, `CreateContainerVersion`, and `PublishContainerVersion`

Two API surfaces share that client:

- `Client.Analytics()` — analytics and management methods, including goals
- `Client.TagManager()` — Tag Manager methods (`TagManager.*`)

`CheckConnection` calls the non-mutating `API.getMatomoVersion` method.

## CLI

The CLI composition root registers the Matomo provider. Manifests may
use addresses such as `matomo.goal.trial_started`,
`matomo.variable.user_id`, `matomo.trigger.trial_started`, and
`matomo.tag.trial_started`. `validate`, `plan`, `apply`, and `import` call
`CheckConnection` once after the provider and resource type resolve, then
validate resource attributes and (for `plan`) read live objects.
`agoraform publish` validates publication configuration, then creates and
publishes a container version when the draft is not already represented by
the currently published environment. See [publish.md](../../docs/publish.md).

## Safety

- Tests use `httptest` only. They never contact a real Matomo instance.
- Tokens must not appear in errors, logs, plan output, or fixtures.
- `agoraform plan` still cannot mutate remote resources. `agoraform apply`
never calls Tag Manager version or publish endpoints.
