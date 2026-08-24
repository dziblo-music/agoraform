# Plan engine (v0.1)

`agoraform plan` compares desired manifest resources with provider-reported
live state and prints a deterministic execution plan. It never mutates
remote resources.

```text
manifest
   │
   ▼
desired resources

provider.Reader
   │
   ▼
live resources

 desired ─────┐
              ├── diff ──► plan
 live    ─────┘
```

## Command

```bash
agoraform plan
agoraform plan -f path/to/manifest.yaml
agoraform plan path/to/manifest.yaml
```

If no path is given, Agoraform reads `agoraform.yaml` in the current
directory.

`plan` loads and validates the manifest, reads local identity state from
`agoraform.state.json` next to the manifest, then reads each desired
resource through the registered provider. A remote miss
(`provider.ErrNotFound`) is a create when the resource is unbound. A
persisted identity that is missing remotely is a stale-state error, not a
create.

The Matomo provider is registered with the CLI. `matomo.goal` resources
are planned against the configured Matomo site: a missing remote goal is
a create, a changed supported field is an update, and an equivalent
remote goal (including omitted defaults and computed fields such as
`idgoal`) is unchanged. Unit tests that need a generic resource
lifecycle use the in-memory `fake` provider.

## Change model

v0.1 supports:

| Action | Meaning |
| --- | --- |
| create | desired resource is absent remotely |
| update | configurable attributes differ |
| unchanged | configurable attributes match |

Destructive deletion is out of scope. Resources that exist remotely but are
not in the manifest are ignored.

The machine-usable plan (`internal/plan.Plan`) is independent of terminal
output. Rendering is a separate `Format` step. `agoraform apply` consumes
the same plan representation and executes only its create and update
actions. See [apply.md](apply.md).

## Normalization

The generic diff compares configurable attributes only. Computed/read-only
fields on `RemoteResource.Computed` never produce changes.

Omitted and nil attribute values are treated as absent. Providers may
implement `provider.Normalizer` to fill or drop defaults so semantically
equivalent values do not diff.

## Output

Creates and updates are listed in address order. Attribute paths are sorted.
Unchanged resources are omitted from the action list.

```text
Agoraform will perform the following actions:

+ fake.widget.trial_started

    title: "Trial Started"

~ fake.widget.user_id

    color:
      "uid" -> "userId"

Plan: 1 to create, 1 to update, 0 to destroy.
```

A zero-change plan prints:

```text
No changes. Desired configuration matches live resources.

Plan: 0 to create, 0 to update, 0 to destroy.
```

Re-running `plan` against unchanged desired and live state produces the same
text.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Plan succeeded and no changes are required |
| `1` | Planning failed (invalid manifest, unknown provider, read error) |
| `2` | Plan succeeded and changes are present |
| `3` | Invalid invocation (unknown flag or conflicting file arguments) |

Code `2` is a successful plan, not a command failure. CI/GitOps jobs that
want to fail when infrastructure would change can treat `2` as actionable.

## Safety

`plan` accepts `provider.Reader` only: `Name`, `ResourceTypes`, `Validate`,
and `Read`. It cannot call `Create`, `Update`, or `Import`.

## Local state

`plan` reads [local state](state.md) so managed resources keep a stable
provider-native identity. The default file is `agoraform.state.json` beside
the manifest. `plan` never writes that file.
