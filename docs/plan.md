# Plan engine

`agoraform plan` compares desired manifest state with provider-reported remote
state and prints deterministic actions. It is non-mutating.

```text
manifest
   │
   ├─ provider desired state
   └─ resources + dependencies
              │
              ▼
        provider reads
              │
              ▼
        resource diffs
              │
              ▼
   provider finalization planning
              │
              ▼
             plan
```

## Command

```bash
agoraform plan
agoraform plan -f path/to/manifest.yaml
agoraform plan path/to/manifest.yaml
```

The default manifest is `agoraform.yaml`.

Plan loads and validates the manifest, configures registered providers with
non-secret provider desired state, validates connectivity, reads local identity
state, and then reads desired resources in dependency order.

A missing unbound remote resource becomes a create. A bound identity missing
remotely is a stale-state error rather than an implicit replacement. Provider
normalizers may remove computed/default noise before diffing.

## Provider actions

Some desired states require provider-level convergence after resource CRUD.
These actions are still part of the reviewed plan and do not become new
provider-specific CLI commands.

For example, Matomo Tag Manager publication can appear as:

```text
> matomo.container.main: publish -> live
```

The provider decides whether that finalization is required using only
non-mutating reads. If managed Tag Manager draft resources already have planned
changes and `providers.matomo.publish` is enabled, the publication consequence
is shown as conditional in the same plan before apply:

```text
> matomo.container.main: publish -> live [conditional]
```

Conditional provider actions still count as planned work. The provider checks
the converged draft after resource mutations and may skip the action when no
publication is required.

A plan containing only a provider action still exits with code `2` because
`apply` has work to do.

## Resource actions

| Action | Meaning |
| --- | --- |
| create | desired resource is absent remotely |
| update | configurable attributes differ |
| unchanged | desired and comparable remote state match |

Destructive deletion is not implemented yet. Unmanaged remote objects are
ignored.

## Output

Example resource changes:

```text
Agoraform will perform the following actions:

+ matomo.goal.trial_started

    name: "Trial Started"
    matchAttribute: "event_action"
    pattern: "trialStarted"

Plan: 1 to create, 0 to update, 0 to destroy.
```

Example with Matomo publication:

```text
Agoraform will perform the following actions:

~ matomo.tag.trial_started

    eventAction:
      "trialStart" -> "trialStarted"

> matomo.container.main: publish -> live [conditional]

Plan: 0 to create, 1 to update, 0 to destroy, 1 provider action.
```

Zero change:

```text
No changes. Desired configuration matches live resources.

Plan: 0 to create, 0 to update, 0 to destroy.
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Plan succeeded and no actions are required |
| `1` | Planning failed |
| `2` | Plan succeeded and resource/provider actions are present |
| `3` | Invalid invocation |

## Safety

Plan never calls provider mutation methods. Resource reconciliation uses the
read-only `provider.Reader` contract. Optional provider finalization planning
is also required to be non-mutating.

For Matomo publication, permission/environment checks happen during planning
without creating a version. Draft-versus-published comparison happens during
planning for definite actions and again after convergence before any version is
created.

See [Matomo Tag Manager publication](matomo-publishing.md).
