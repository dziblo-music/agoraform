# Manifest format (`v1alpha1`)

Agoraform configuration is a versioned YAML document that lists desired
marketing-infrastructure resources by logical address.

## Document shape

```yaml
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
```

The `v1alpha1` schema is intentionally small. It describes desired resources
only. Provider credentials, modules, and expressions are out of scope.

| Field | Required | Description |
| --- | --- | --- |
| `apiVersion` | yes | Must be `agoraform.io/v1alpha1`. |
| `resources` | no | List of desired resources. Omitted or empty is valid. |
| `resources[].address` | yes | Logical address of the form `provider.type.name`. |
| `resources[].attributes` | no | Configurable attributes for the resource. Provider-native identities do not belong here. |

Do not put API tokens, passwords, or other secrets in a manifest. Provider
authentication belongs in environment or runtime configuration, not in Git.

Existing v0.1.0 manifests that do not use resource references remain valid.

## Resource addresses

A resource address is three lowercase identifiers separated by dots:

```text
provider.type.name
```

Examples:

```text
matomo.goal.trial_started
```

`fake.widget` exists only in unit tests. It is not a user-facing provider.

Each segment must start with a letter (`a-z`) and may then contain letters,
digits, or underscores. Addresses are compared exactly; duplicates are
rejected.

Addresses are used in validation errors, plans, imports, and any later
identity mapping. Parsing an address and rendering it with `String()` must
round-trip.

## Resource references

References are explicit attribute values using a single-key `$ref` mapping.
The `$ref` value is the full logical `provider.type.name` address of another
desired resource, never a provider-native id.

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
```

A reference object must contain exactly one key, `$ref`, whose value must be a
valid logical resource address. References can appear in nested maps and lists.
There is no expression language, interpolation, or attribute path selection.

Plain strings always remain plain provider-owned values, even when they happen
to look like a logical resource address. For example, this is a literal string,
not a dependency:

```yaml
attributes:
  pattern: checkout.step.complete
```

Agoraform walks explicit references and builds a directed dependency graph. A
resource that references another depends on it. Equivalent graphs have a stable
prerequisite-first order; unrelated resources keep address order as the
tie-breaker.

`validate`, `plan`, and `apply` reject:

- a reference to a resource that is not in the manifest
- a resource that references itself
- a dependency cycle

Diagnostics include the referring resource, the attribute path, and the
missing or cyclic addresses. Graph checks run during manifest load, before
provider reads or mutations.

Plans keep logical resource addresses. Generated import YAML serializes a
`resource.Ref` back into the same explicit `$ref` form. Provider-native
identities will be resolved at runtime by provider/resource execution and
belong in [local state](state.md), not in the manifest.

Matomo Tag Manager types such as `matomo.trigger` and `matomo.tag` are shown
here as the intended reference syntax. They are not added as managed resource
types by this change; unknown types still fail provider validation when that
provider is registered.

## Attributes

`attributes` is a YAML mapping of configurable provider fields. Except for the
explicit `$ref` form described above, the core treats attribute values as opaque
and providers own their schemas. Provider-native identities are stored in
[local state](state.md), not in the manifest.

Computed (read-only) fields belong to the live/remote resource a provider
returns and must not be copied into configuration as mutable attributes.

## Matomo Goal (`matomo.goal`)

v0.1 manages a Matomo analytics goal. On initial discovery, Agoraform may
find a goal by `name`. Once the resource is managed, persist Matomo's
`idGoal` in [local state](state.md). Do not copy it into attributes.

```yaml
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
```

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Goal name. Immutable once local state binds the logical resource to a remote goal. |
| `matchAttribute` | yes | How the goal matches. One of `url`, `title`, `file`, `external_website`, `manually`, `visit_duration`, `visit_total_actions`, `visit_total_pageviews`, `event_action`, `event_category`, `event_name`. |
| `pattern` | unless `manually` | Value to match. Numeric match attributes require a number. |
| `patternType` | no | `contains` (default), `exact`, or `regex`. Numeric match attributes default to `greater_than`. |

`pattern` and `patternType` must be omitted when `matchAttribute` is
`manually`. Remote response fields such as lowercase `idgoal`, `idsite`,
`revenue`, `description`, `case_sensitive`, and `allow_multiple` remain
computed/unmanaged in v0.1 and cannot be configured as mutable Goal fields.

Because Matomo's `Goals.updateGoal` endpoint writes a complete goal record,
Agoraform re-reads the goal immediately before an update and carries forward
unmanaged values such as description, revenue, case sensitivity, multiple-
conversion behavior, and event-value-as-revenue. A managed-field update must
not reset those settings.

For `patternType: exact`, Agoraform mirrors Matomo's validation requirement:
`url`, `file`, and `external_website` patterns must start with `http://` or
`https://`; event attributes and `title` are exempt.

Omitted `patternType` is treated as the Matomo default, so an equivalent
remote goal produces a zero-change plan.

## Validation

```bash
agoraform validate
agoraform validate -f path/to/manifest.yaml
agoraform validate path/to/manifest.yaml
```

If no path is given, Agoraform reads `agoraform.yaml` in the current
directory.

`validate` reports actionable errors for:

- malformed YAML
- unsupported or missing `apiVersion`
- missing resource addresses
- invalid or duplicate addresses
- malformed or missing resource references, self-references, and dependency cycles
- unknown providers or resource types, when a provider is registered
- provider-specific required-field failures, when a provider is registered

The CLI registers the Matomo provider. `matomo.goal` is a supported resource
type. `validate` and `plan` check provider connection settings, then
goal-specific required fields. Provider/type checks for other providers
(including the test-only `fake` provider) run when those providers are
registered.

Matomo credentials come from `MATOMO_URL`, `MATOMO_TOKEN_AUTH`, and
`MATOMO_SITE_ID`. See [the Matomo provider README](../providers/matomo/README.md).

`agoraform plan` uses the same manifest. See [plan.md](plan.md) for
reconciliation, output, and exit codes. See [import.md](import.md) to bring
an existing remote resource under management. See [state.md](state.md) for
the local identity file.
