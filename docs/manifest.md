# Manifest format (v0.1)

Agoraform configuration is a versioned YAML document that lists desired
marketing-infrastructure resources by logical address.

## Document shape

```yaml
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      event: trialStarted
```

The v0.1 schema is intentionally small. It describes desired resources only.
Provider credentials, modules, and expressions are out of scope.

| Field | Required | Description |
| --- | --- | --- |
| `apiVersion` | yes | Must be `agoraform.io/v1alpha1`. |
| `resources` | no | List of desired resources. Omitted or empty is valid. |
| `resources[].address` | yes | Logical address of the form `provider.type.name`. |
| `resources[].attributes` | no | Configurable attributes for the resource. |

Do not put API tokens, passwords, or other secrets in a manifest. Provider
authentication belongs in environment or runtime configuration, not in Git.

## Resource addresses

A resource address is three lowercase identifiers separated by dots:

```text
provider.type.name
```

Examples:

```text
matomo.goal.trial_started
fake.widget.homepage
```

Each segment must start with a letter (`a-z`) and may then contain letters,
digits, or underscores. Addresses are compared exactly; duplicates are
rejected.

Addresses are used in validation errors, plans, imports, and any later
identity mapping. Parsing an address and rendering it with `String()` must
round-trip.

## Attributes

`attributes` is a YAML mapping of configurable fields. Nested maps and lists
are allowed. The core treats values as opaque; providers interpret their own
schemas.

Computed (read-only) fields belong to the live/remote resource a provider
returns. They must not be set in the manifest. For example, a provider may
report a remote serial or generated identifier after create, while the
manifest only declares user-configured fields such as `title`.

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
- unknown providers or resource types, when a provider is registered
- provider-specific required-field failures, when a provider is registered

Until a real provider is registered with the CLI, `validate` performs
structural checks only. Provider/type checks run in tests through the fake
provider, and will apply to the CLI as providers are added.

`agoraform plan` uses the same manifest. See [plan.md](plan.md) for
reconciliation, output, and exit codes.
