# Import

`agoraform import` brings an existing remote resource under Agoraform
management without recreating it. It reads the live object, prints
deterministic YAML for configurable fields, and persists the
provider-native identity in [local state](state.md).

```text
logical address + remote id
   │
   ▼
resolve provider
   │
   ▼
read remote resource
   │
   ▼
emit configurable YAML
   │
   ▼
persist identity
   │
   ▼
review and add to manifest
```

## Command

```bash
agoraform import ADDRESS REMOTE-ID
agoraform import -f path/to/agoraform.yaml ADDRESS REMOTE-ID
```

Example:

```bash
agoraform import matomo.goal.trial_started 12
```

`ADDRESS` is the logical resource address (`provider.type.name`).
`REMOTE-ID` is the provider-native identity of the existing object, such as
a Matomo goal id or Tag Manager draft object id.

`--file` / `-f` locates local state the same way `plan` and `apply` do: next
to the named manifest. The default manifest path is `agoraform.yaml`, so
identity is written to `agoraform.state.json` in the current directory. Import
does not read or rewrite the manifest file.

## What import does

1. Resolve the provider and resource type from the logical address.
2. Reject a logical address that already has a state binding.
3. Read the existing remote resource through the provider import path.
4. Translate configurable remote fields into Agoraform YAML.
5. Persist the logical-address → provider-native identity mapping in local
   state.

Import never creates, updates, or deletes the remote resource.

## Generated configuration

Import prints a complete manifest fragment for review. Add the resource to
your Agoraform manifest; import will not edit the file for you.

```yaml
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      matchAttribute: event_action
      name: Trial Started
      pattern: trialStarted
      patternType: contains
```

Computed and read-only fields are omitted. Provider-native identity is not
emitted as a manifest attribute. Secrets never appear in the output.

After you add the generated configuration, `agoraform plan` against the
unchanged remote resource should report no changes. Plan resolves the object
through the persisted identity rather than rediscovering it by a mutable
field such as name.

If that bound remote object later disappears, plan reports a stale-identity
error instead of planning a create.

## Matomo goals

Import a Matomo goal by its numeric `idGoal`. That id is stored in local
state as `remoteId`. Do not copy it into YAML.

`name` remains immutable for a state-bound Matomo goal.

## Google Ads conversion actions

Import a website conversion action by its numeric conversion action ID, or
by the Google Ads resource name
`customers/{customerId}/conversionActions/{id}`. Agoraform stores the
numeric ID in local state as `remoteId`. Do not copy it into YAML.

```bash
agoraform import googleads.conversion_action.trial_started 123456789
```

## Matomo variables

Import a Tag Manager variable by its numeric `idvariable` in the configured
container draft. That id is stored in local state as `remoteId`. Do not
copy it into YAML. `type` remains immutable for a managed variable.

## Matomo triggers

Import a Tag Manager trigger by its numeric `idtrigger` in the configured
container draft. That id is stored in local state as `remoteId`. Do not
copy it into YAML. `type` remains immutable for a managed trigger.

## Matomo tags

Import a Tag Manager tag by its numeric `idtag` in the configured container
draft. That id is stored in local state as `remoteId`. Do not copy it into
YAML. `type` remains immutable for a managed tag.

Tag relationships are reconstructed as logical `$ref` values when the related
resources are already bound in local state:

- The fire trigger becomes `trigger: { $ref: matomo.trigger.NAME }`.
- Event fields that use `{{Variable Name}}` become `$ref`s to managed
  `matomo.variable` resources when those variables are bound. Unmanaged
  templates remain literal strings.

Import order matters. Import (or apply) related triggers and variables
before importing a tag:

```bash
agoraform import matomo.variable.user_id 2
agoraform import matomo.trigger.trial_started 4
agoraform import matomo.tag.trial_started 1
```

If the fire trigger is not bound, or the remote tag uses an unsupported
multi-trigger shape, import fails with actionable guidance instead of
emitting Matomo-native ids as configuration.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Import succeeded |
| `1` | Import failed (unknown type, missing remote resource, conflicting state, unrepresentable relationships, provider error, or state write failure) |
| `3` | Invalid invocation (wrong number of arguments or unknown flag) |

## Safety

- Import is explicit: one logical address and one remote identity.
- Bulk discovery and interactive selection are out of scope.
- Existing manifests are not rewritten.
- Related resources are not imported recursively.
- Provider secrets are never printed or persisted in state.
