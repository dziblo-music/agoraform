# Apply execution

`agoraform apply` builds the same reviewed convergence plan as `agoraform plan`
and executes it in deterministic order.

The high-level apply lifecycle is owned by the core apply package. The CLI is a
thin adapter around that lifecycle, so resource mutations and provider
finalizations follow the same ordering and failure rules for every canonical
apply entry point.

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

The lower-level core `Execute` primitive performs resource CRUD only. The
canonical high-level `Run` path builds the plan, attaches provider
finalizations, calls `Execute`, and then executes finalizations after successful
resource convergence.

## Provider finalization

Provider-specific convergence is not represented by provider-specific CLI
verbs. Instead, optional finalization actions that appeared in the plan execute
only after **all** planned resource mutations and required state writes
succeed.

For Matomo:

```yaml
providers:
  matomo:
    publish: true
    environment: live
```

can cause apply to create and publish a Tag Manager container version after
variable/trigger/tag draft changes are complete.

A provider-only plan is also valid. For example, if the Matomo draft already
differs from the published environment but no managed Tag Manager resource
needs CRUD, apply can still execute the planned publication action.

If any preceding Tag Manager resource mutation or required state write fails,
no finalization is attempted.

Immediately before mutation, the Matomo provider rechecks publication
idempotency and publish permissions. This protects against duplicate versions
and stale plans. A conditional publication can therefore complete as a no-op;
when that happens the provider reports `no publication required` rather than
creating another version.

If version creation succeeds but publication fails, apply prints the created
version detail before returning a partial-convergence failure. Earlier resource
changes remain applied. It does not print a successful `Apply complete`
summary.

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

Apply complete! 0 created, 1 updated, 1 provider action completed.
```

A publication-only apply can produce:

```text
matomo.container.main: version 13 created
matomo.container.main: published to live

Apply complete! 0 created, 0 updated, 1 provider action completed.
```

A conditional finalization that becomes unnecessary after resource convergence
still reports that decision explicitly:

```text
matomo.container.main: no publication required

Apply complete! 0 created, 1 updated, 1 provider action completed.
```

Here, "provider action completed" means the reviewed finalization was evaluated
successfully; it does not necessarily mean that the provider performed an
additional mutation.

A zero-action apply prints:

```text
Apply complete! 0 created, 0 updated.
```

## Failure behavior

Apply stops at the first resource/state-write failure. Provider finalization is
not attempted after a resource or required state-persistence failure.

A provider finalization failure is returned as an apply failure. Earlier
successful resource mutations are not rolled back; rollback and transactions
are out of scope.

Post-mutation failures are reported as partial convergence. That is distinct
from a failure that happens before any remote mutation (validation, planning,
or a provider create/update error). Incomplete apply still exits `1`.

## Recovery

Remote provider operations and local state writes are not a distributed
transaction. When apply fails after a remote change, the remote object is left
as-is and Agoraform does not delete or roll it back.

### Create succeeded, state write failed

The remote resource exists, but `agoraform.state.json` has no identity binding.
Fix the state-file problem (permissions, disk, or a conflicting binding), then
re-bind the provider-native identity:

```text
matomo.variable.user_id was created remotely with id 12, but its state binding could not be saved.
Fix the state-file problem, then run:
  agoraform import matomo.variable.user_id 12
```

If the write failed because that identity is already bound to another address,
resolve the ownership conflict. Do not import the same identity onto a second
address.

A later plan that rediscovers the resource by mutable fields and reports it
unchanged does **not** repair the missing binding.

### Update succeeded, state write failed

Updates are identity-bound. The previously persisted identity normally remains
valid. Fix the state-file problem, then rerun `agoraform plan` and
`agoraform apply`. Do not treat this as a missing identity that requires
import.

### Provider finalization failed after resource changes

Earlier resource creates and updates remain applied. Provider progress details
stay visible, including Matomo's `version N created` line when a container
version was created before publish failed. Fix the provider error, then rerun
`agoraform plan` and `agoraform apply`. Retrying apply does not roll back those
resource changes, and Matomo publication remains idempotent when the converged
draft is already published.

### Publication response did not confirm success

If Matomo returns HTTP 200 but the body is empty, JSON `null`, not a
release ID, unreadable, or larger than Agoraform will accept, apply reports
a partial-convergence failure whose outcome is uncertain. Inspect the remote container to see whether the version was
published. Do not create another version until that status is known;
retrying apply immediately can snapshot another unused version.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Apply succeeded |
| `1` | Apply failed |
| `3` | Invalid invocation |

## Safety

- Manifest/provider configuration is validated before mutation.
- Dependency graph errors fail before mutation.
- Provider finalization planning happens before resource mutations.
- Publication permission/environment preflight happens before resource
  mutations when publication is planned.
- Provider secrets are not written to output or local state.
- Destructive deletion, rollback, and parallel mutation are not implemented.

See [plan.md](plan.md), [state.md](state.md), and
[Matomo Tag Manager publication](matomo-publishing.md).
