# Local state (v0.1)

Agoraform v0.1 stores a small local identity map so logical resource
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

Serialization is deterministic. Writes replace the file atomically so an
interrupted save does not leave a partial document.

## Plan

When state contains an identity for a resource, `plan` reads that remote
object by identity. Name or other discovery fields cannot rebind a
managed resource to a different remote object.

If the bound remote object is gone, plan fails with a stale-identity
error. It does not plan a create.

Unbound resources may still be discovered by the provider when that is
safe. After a successful create or import, later commands should persist
the returned identity in state.

## Apply and import primitives

`apply` and `import` commands are not implemented yet. The state store
already exposes the operations those commands need:

- after a successful create, persist the returned identity
- after an update, keep or refresh the same identity
- after an import of `<address> <remote-id>`, persist that mapping
- do not write state for a failed mutation

A successful apply or import followed by plan should then resolve the
same remote object from state.

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
- duplicate identity records
- provider/address mismatch
- a persisted identity whose remote object no longer exists
- a state file that cannot be written atomically
