# Local state (0.1.0)

Agoraform 0.1.0 stores a small local identity map so logical resource
addresses can stay bound to provider-native remote IDs. The manifest
describes desired configuration. State stores only management metadata.

```text
matomo.goal.trial_started  ->  remote identity "12"
```

State never contains credentials, API tokens, passwords, or other
provider secrets.

## Why it exists

Providers usually identify remote objects with stable native IDs rather
than mutable human-readable fields. Without a persisted mapping:

- renaming a managed resource can look like a delete plus a create
- duplicate names make identity ambiguous
- a missing remote object can be treated as an unbound create
- `apply` cannot remember the ID returned by create
- `import` cannot remember the ID it bound

v0.1 therefore keeps a local file next to the manifest.

## File location

The default file is `agoraform.state.json` in the same directory as the
manifest. `agoraform plan -f site/agoraform.yaml` reads
`site/agoraform.state.json`. A missing file is empty state, not an error.

The state file is machine/account-local management metadata and should not
normally be committed to Git. The repository `.gitignore` ignores
`agoraform.state.json` at any directory depth. If a future workflow needs
shared or remote state, it should be designed explicitly rather than treating
the v0.1 local file as portable configuration.

The file is local only. Remote backends, workspaces, locking, encryption,
and history are out of scope.

## Format

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

| Field | Meaning |
| --- | --- |
| `version` | Schema version. v0.1 is `1`. |
| `resources` | Map of logical addresses to identity records. |
| `provider` | Provider name. Must match the address provider segment. |
| `remoteId` | Opaque provider-native identity. |

Provider-native IDs are not assumed to be globally unique across all resource
types. Duplicate ownership is rejected within the same provider and resource
type, while different resource types may legitimately reuse the same opaque
ID value.

Serialization is deterministic. Writes create and sync a private temporary
file beside the destination, then atomically replace the prior state. A failed
replacement leaves the previous valid state untouched; Agoraform never uses a
delete-then-rename fallback.

## Plan

When state contains an identity for a resource, `plan` asks the provider to
read that exact remote object by identity. Core then verifies that the
provider returned the same identity. Name or other discovery fields cannot
rebind a managed resource to a different remote object even if a provider
implementation accidentally ignores the identity hint.

If the bound remote object is gone, plan fails with a stale-identity
error. It does not plan a create.

Unbound resources may still be discovered by the provider when that is
safe. After a successful create or import, later commands should persist
the returned identity in state.

## Apply and import primitives

`agoraform apply` uses this store after each successful mutation:

- after a successful create, persist the returned identity
- after an update, keep or refresh the same identity
- do not write state for a failed mutation
- if a mutation succeeds but the identity cannot be written, apply
  reports a partial-convergence failure with recovery guidance and does
  not claim full success

`agoraform import ADDRESS REMOTE-ID` uses the same store:

- read the existing remote object by provider-native identity
- persist the logical-address → identity mapping
- reject a logical address that already has a binding
- never create, update, or delete the remote resource

A successful apply or import followed by plan resolves the same remote
object from state rather than rediscovering it by a mutable field such as
name. See [import.md](import.md).

`agoraform destroy` removes a state binding only after the provider confirms
the remote object was destroyed, removed, or is already absent. Failed and
unattempted resources keep their identities. State entries that are not in
the manifest are preserved; destroy does not prune them. See
[destroy.md](destroy.md).

## Recovery

Remote mutations and local state writes are not a distributed transaction.

If a create succeeds and the state file cannot be written, the remote object
exists without a local binding. Fix the state-file problem, then re-bind with
`agoraform import ADDRESS REMOTE-ID`. Do not assume that a later unchanged
plan repaired the missing identity.

If that write failed because the identity is already owned by another logical
address, resolve the conflict instead of importing the same identity twice.

If an update succeeds and the state file cannot be written, the existing
identity binding is normally still valid. Fix the file and rerun plan/apply;
import is not required.

If provider finalization fails after resource changes were persisted, those
bindings remain. Retry plan/apply after fixing the provider error.

## Matomo goals

Do not put `idGoal` in the manifest. Persist Matomo's goal ID in state:

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

Manifests that still contain `idGoal` are rejected with a diagnostic
telling you to move the identity into local state. `name` remains
immutable for a state-bound Matomo goal.

## Errors

Agoraform reports actionable errors for:

- malformed state JSON
- unsupported state version
- invalid resource addresses
- missing or empty identities
- duplicate identity records within one provider resource type
- provider/address mismatch
- a provider returning an identity different from the persisted binding
- a persisted identity whose remote object no longer exists
- a state file that cannot be written atomically
- a create that succeeded remotely but could not persist its identity
  (re-bind with `agoraform import` after fixing the state file, unless the
  identity is already owned by another address)
- an update that succeeded remotely but could not refresh local state
  (existing identity remains valid; fix the file and rerun plan/apply)
