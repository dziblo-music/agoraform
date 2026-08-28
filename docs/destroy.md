# Destroy

`agoraform destroy` tears down managed resources using the same provider-neutral
manifest graph, local identity state, and deterministic execution model as
`plan` and `apply`.

```text
manifest + local state
        │
        ▼
validate / configure providers
        │
        ▼
build destroy plan
(reverse dependency order)
        │
        ▼
confirm (or --auto-approve)
        │
        ▼
destroy supported resources
(dependents first)
        │
        ▼
remove confirmed identities
        │
        ▼
provider finalization actions
        │
        ▼
Destroy complete
```

## Command

```bash
agoraform destroy
agoraform destroy -f path/to/manifest.yaml
agoraform destroy --auto-approve
```

The default manifest is `agoraform.yaml`.

## Confirmation

Destroy is explicitly destructive.

- Interactive terminals require typing `yes` after the plan is shown.
- Non-interactive stdin fails before mutation unless `--auto-approve` is set.
- Declining confirmation exits 0 with no remote or state changes.

## Destroy set

The manifest defines the requested destroy set and dependency relationships.
Local state supplies provider-native identities.

- A manifest resource without a state binding is reported as not managed and
  is not changed remotely.
- Identities present in `agoraform.state.json` but absent from the manifest
  are preserved. Destroy does not prune them.
- Invalid graphs produce zero remote mutations.

## Lifecycle capabilities

Providers declare whether a resource can be destroyed. Core never assumes a
generic HTTP DELETE.

| Capability | Plan | Remote | State |
| --- | --- | --- | --- |
| supported | destroy/remove | provider-native teardown | removed after confirmed terminal or already-absent result |
| unsupported | listed, not mutated | none | kept |
| provider-owned | listed, not mutated | none | kept |

Unsupported and provider-owned remnants do not block supported teardown. After
supported operations complete, destroy exits non-zero when any requested
state-bound resource remains unsupported or provider-owned.

## Ordering

Destroy uses the apply dependency graph in reverse: dependents are removed
before prerequisites. For Matomo Tag Manager that means tags, then
variables/triggers, then a managed container.

## Already absent and failures

Provider-confirmed already-absent resources are successful convergence. The
matching state binding is removed. Authentication, permission, malformed
response, and wrong-target errors are not treated as not-found.

Execution stops at the first provider or state-write failure. Remaining and
failed resources keep their state bindings so retry is deterministic. If a
remote teardown succeeds but the state write fails, the identity is left in
place so retry can confirm the object is gone.

## Provider finalization

Destroy finalization uses the same visibility and failure rules as apply.
Planned actions such as Matomo publication appear in the destroy plan and run
only after every destructive mutation succeeds. A failed draft deletion does
not publish.

## Matomo

Supported Matomo destroy targets are `matomo.goal`, `matomo.variable`
(Data Layer and Matomo Configuration), `matomo.trigger`, `matomo.tag`, and
an Agoraform-managed `matomo.container`.

`MATOMO_CONTAINER_ID` continues to select an externally managed container.
Destroy never deletes that container merely because child resources inside it
are managed. A `matomo.container` present in the manifest and bound in state,
whether created or imported, is managed and eligible for deletion after its
children.

When child Tag Manager resources are destroyed and the container remains,
publication follows `providers.matomo.publish`. When the same plan will
delete the managed container, destroy does not create or publish an
intermediate container version; it deletes children and then the container.

## Google Ads

Google Ads destroy/removal is not implemented yet. Those resources are
reported as unsupported, remain in state, and do not block supported Matomo
teardown.

## Out of scope

- Automatic destruction merely because a resource was removed from the
  manifest
- Deleting Matomo sites/websites
- Deleting externally managed containers
- Rollback to prior container versions
- Application snippet removal
