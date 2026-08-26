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
```

See [Matomo Tag Manager publication](matomo-publishing.md).

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

Provider-neutral dependencies use a single-key `$ref` object:

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

Agoraform validates references and builds a directed dependency graph. Missing
references, self-references, and cycles fail before remote mutations.

At apply time, logical references are resolved to provider-native identities
and computed outputs in dependency order. Those provider-native values are not
written back into the manifest.

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

### `matomo.variable`

v0.2.0 supports Tag Manager Data Layer variables:

```yaml
- address: matomo.variable.user_id
  attributes:
    type: dataLayer
    key: userId
    name: User ID
```

`type` and `key` are required; `name` is optional and defaults to `key`.

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

v0.2.0 supports Matomo Analytics event tags:

```yaml
- address: matomo.tag.trial_started
  attributes:
    type: matomoAnalytics
    trigger:
      $ref: matomo.trigger.trial_started
    eventCategory: signup
    eventAction: trialStarted
```

Supported event fields may be literals or, where documented, references to
managed variables. See the [Matomo provider reference](../providers/matomo/README.md)
for the complete resource-specific schema and preservation behavior.

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
`MATOMO_TOKEN_AUTH`, and `MATOMO_SITE_ID`; Tag Manager resource/publication
work also requires `MATOMO_CONTAINER_ID`.

See [plan.md](plan.md), [apply.md](apply.md), [import.md](import.md), and
[state.md](state.md).
