# Apply execution

`agoraform apply` builds the same reviewed convergence plan as `agoraform plan`
and executes it in deterministic order.

```text
manifest + local state
        │
        ▼
validate / configure providers
        │
        ▼
build resource + provider-action plan
        │
        ▼
apply resource creates/updates
(prerequisites first)
        │
        ▼
persist identities
        │
        ▼
provider finalization actions
        │
        ▼
Apply complete
```

## Command

```bash
agoraform apply
agoraform apply -f path/to/manifest.yaml
agoraform apply path/to/manifest.yaml
```

The default manifest is `agoraform.yaml`.

## Resource execution

Creates and updates run sequentially in dependency order. `$ref` values are
resolved immediately before mutation to runtime bindings containing the
provider-native identity and computed outputs of prerequisites.

For updates, apply re-reads the exact identity-bound remote object immediately
before mutation so providers receive the complete live record needed to
preserve unmanaged fields.

Successful creates/updates persist identity bindings to
`agoraform.state.json`. Failed mutations do not create new bindings.

## Provider finalization

Provider-specific convergence is not represented by provider-specific CLI
verbs. Instead, optional finalization actions that appeared in the plan execute
only after **all** planned resource mutations succeed.

For Matomo:

```yaml
providers:
  matomo:
    publish: true
    environment: live
```

can cause apply to create and publish a Tag Manager container version after
variable/trigger/tag draft changes are complete.

If any preceding Tag Manager resource mutation fails, no version is created and
nothing is published.

Immediately before mutation, the Matomo provider rechecks publication
idempotency and publish permissions. This protects against duplicate versions
and stale plans.

If version creation succeeds but publication fails, apply prints the created
version detail before returning the publication error. It does not print a
successful `Apply complete` summary.

## Output

Resource-only apply:

```text
matomo.goal.trial_started: creating...
matomo.goal.trial_started: created

Apply complete! 1 created, 0 updated.
```

With declarative Matomo publication:

```text
matomo.tag.trial_started: updating...
matomo.tag.trial_started: updated
matomo.container.main: version 12 created
matomo.container.main: published to live

Apply complete! 0 created, 1 updated.
```

A zero-action apply prints:

```text
Apply complete! 0 created, 0 updated.
```

## Failure behavior

Apply stops at the first resource/state-write failure. Provider finalization is
not attempted after a resource failure.

A provider finalization failure is returned as an apply failure. Earlier
successful resource mutations are not rolled back; rollback and transactions
are out of scope.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Apply succeeded |
| `1` | Apply failed |
| `3` | Invalid invocation |

## Safety

- Manifest/provider configuration is validated before mutation.
- Dependency graph errors fail before mutation.
- Publication permission/environment preflight happens before resource
  mutations when publication is planned.
- Provider secrets are not written to output or local state.
- Destructive deletion, rollback, and parallel mutation are not implemented.

See [plan.md](plan.md), [state.md](state.md), and
[Matomo Tag Manager publication](matomo-publishing.md).
