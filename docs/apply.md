# Apply execution (0.1.0)

`agoraform apply` turns a validated execution plan into remote creates and
updates. It never diffs desired and live state itself: the [plan
engine](plan.md) is the source of truth for what to execute, and [local
state](state.md) is the source of truth for which provider-native object a
managed resource owns.

```text
manifest
   │
   ▼
load local state
   │
   ▼
validate (including dependency graph)
   │
   ▼
read remote resources
   │
   ▼
build plan
   │
   ▼
order by dependency graph
   │
   ▼
resolve refs, then create / update
(prerequisites first, sequential)
   │
   ▼
persist identities
   │
   ▼
report result
```

## Command

```bash
agoraform apply
agoraform apply -f path/to/manifest.yaml
agoraform apply path/to/manifest.yaml
```

If no path is given, Agoraform reads `agoraform.yaml` in the current
directory.

`apply` loads and validates the manifest, including resource references and
the dependency graph, loads local identity state from
`agoraform.state.json` next to the manifest, then builds the same plan as
`agoraform plan`. Missing references, self-references, and cycles fail
before any mutation. Only after that plan is produced does it call provider
`Create` or `Update`. For an update, apply keeps the planned action, binds
the desired resource to the planned identity, reads that exact remote
object, and passes the full live resource to `Update`. It does not
reconstruct live state from the plan's comparable `Before` attributes.
There is no interactive approval prompt in v0.1.

Apply executes sequentially in deterministic dependency order: prerequisite
resources run before dependents, and unrelated resources keep address order
as the tie-breaker. Immediately before each create or update, apply replaces
explicit `$ref` values with runtime bindings that carry the prerequisite's
provider-native identity and computed outputs. For unchanged prerequisites,
those bindings come from the live resource observed while planning rather
than a second apply-time read. Providers translate the bindings into native
API values. The bindings are not written into the manifest, plan output, or
user-authored configuration. If a prerequisite has no identity after it has
been applied (or skipped as unchanged), apply fails with an actionable
diagnostic and does not mutate the dependent.

The Matomo provider is registered with the CLI. `matomo.goal`,
`matomo.variable`, `matomo.trigger`, and `matomo.tag` resources can be
created and updated. Tag Manager mutations write the container draft
only; `apply` never publishes a container version. Use
[`agoraform publish`](publish.md) for that explicit step. Unit tests that
need a generic resource lifecycle use the in-memory `fake` provider.

## Change model

v0.1 executes:

| Action | Meaning |
| --- | --- |
| create | provision a missing unbound resource |
| update | reconcile configurable attributes on a bound resource |

Unchanged resources are skipped. Destructive deletion is out of scope.
Unsupported action types are rejected before any mutation.

Execution is sequential and follows deterministic dependency order.
Prerequisites execute before dependents. Apply stops at the first provider
or state-write failure, so a failed prerequisite never executes its
dependents. Failure diagnostics include the resource address and attempted
operation.

## State and identity

After a successful create, apply persists the provider-native identity
returned by the provider. After a successful update, it retains or refreshes
the existing binding. Failed mutations do not write a new identity.

If a mutation succeeds but the identity cannot be written to local state,
apply reports that clearly and does not print a successful apply summary.

A successful apply followed by `agoraform plan` against unchanged
configuration, local state, and remote resources produces no changes. The
next plan resolves the resource through the persisted identity rather than
rediscovering it by a mutable field such as name.

State never contains provider credentials.

## Output

Creates and updates print progress, then a summary:

```text
matomo.goal.trial_started: creating...
matomo.goal.trial_started: created

Apply complete! 1 created, 0 updated.
```

A zero-change apply performs no mutations and prints:

```text
Apply complete! 0 created, 0 updated.
```

The rendering is deterministic and independent of provider-specific types.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Apply succeeded |
| `1` | Apply failed (invalid manifest, planning error, provider error, or state write failure) |
| `3` | Invalid invocation (unknown flag or conflicting file arguments) |

Unlike `plan`, a successful apply that made changes still exits `0`.

## Safety

- Configuration and local state are validated before any mutation.
- Invalid dependency graphs produce zero mutations.
- Apply reuses the plan engine; it does not reimplement reconciliation.
- Provider secrets are never printed in apply output or persisted in state.
- v0.1 does not delete remote resources, run mutations in parallel, or
  roll back earlier successes when a later resource fails.
