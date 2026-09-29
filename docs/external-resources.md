# External resources

An external resource is a reference to an object Agoraform does not own.
Plan and apply can read it and expose its identity to `$ref` consumers.
They never create it, update it, or recreate it when it is missing.
`agoraform destroy` never deletes, archives, pauses, or otherwise mutates it.

Use this for foundational objects that already exist, are shared with
another system, and must survive `agoraform destroy`. The primary example
is an existing Matomo Tag Manager container.

Managed import is a different contract. `agoraform import ADDRESS REMOTE-ID`
adopts the object. Destroy can later delete it.

## Declare a reference

```yaml
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.container.main
    lifecycle:
      ownership: external
      id: Aa000001

  - address: matomo.trigger.trial_started
    attributes:
      container:
        $ref: matomo.container.main
      type: customEvent
      event: trialStarted
      name: Trial Started
```

`lifecycle.ownership` is `external` or `managed`. Omitting `lifecycle` means
managed. `lifecycle.id` is the provider-native identity used to find the
object. It is a lookup key, not desired configuration. External resources
cannot declare `attributes`; Agoraform does not reconcile them.

Numeric ids may be unquoted. Agoraform stores the canonical identity returned
by the provider.

You can also bind the identity without putting it in the manifest first:

```bash
agoraform import --external matomo.container.main Aa000001
```

Import prints a `lifecycle` block to add to the manifest. It reads the remote
object and writes local state. It does not modify the object.

## What plan and apply do

For an external resource, plan:

- reads the object by the persisted identity, or by `lifecycle.id` when state
  has no binding yet
- fails if the object is missing, the id is ambiguous, or the provider cannot
  reference that resource type
- shows the resource as an external reference
- does not propose create, update, or delete

Apply persists or refreshes the local binding and ownership marker. It does
not call provider create or update for that resource. Managed resources that
`$ref` it still follow normal dependency order and receive the external
identity, including declared outputs.

A missing external object is an error. Agoraform will not create a replacement.

## Destroy

Destroy uses the ownership stored in `agoraform.state.json`, not only the
current manifest.

- `ownership: external` — remove the local binding only. The remote object
  stays. A later manifest that drops `lifecycle.ownership: external` still
  cannot destroy it until you explicitly adopt it.
- Removing the external resource from the manifest does not delete the remote
  object and does not prune its state entry. Destroy only touches resources
  that are still declared.
- A failed destroy of some other resource leaves the external binding in place,
  so a retry still refuses to mutate it.

## Changing ownership

A manifest edit cannot move a resource between managed and external.

| Current state | Desired contract | Command |
| --- | --- | --- |
| unbound | external | `agoraform import --external ADDRESS REMOTE-ID` or set `lifecycle` and apply |
| unbound | managed | `agoraform import ADDRESS REMOTE-ID` |
| managed | external | `agoraform import --external --release ADDRESS REMOTE-ID` |
| external | managed | `agoraform import --adopt ADDRESS` |

`--release` requires the remote id already stored for that address. It does
not delete the object. After release, destroy only drops local state.

`--adopt` reads the external object again, marks ownership managed, and prints
managed YAML. Replace the external `lifecycle` block with that YAML. Destroy
can then delete the object. Adopt fails if the remote object is gone, and it
does not create a new one.

Legacy state records without an `ownership` field stay managed.

## Literal ids and environment variables

`lifecycle.id` plus `$ref` is the provider-neutral way to depend on an
existing object. Prefer it when a managed resource should name that dependency
in the manifest.

Some providers also accept a literal provider id outside the manifest. Matomo
still supports `MATOMO_CONTAINER_ID` when no `matomo.container` resource is
declared. That environment variable selects a container for Tag Manager
children, but it is not a logical resource other manifests can `$ref`. An
external `matomo.container` is the first-class replacement when tags, triggers,
or variables should reference the container by address. Do not set
`MATOMO_CONTAINER_ID` in that mode.

Do not put provider-native ids in managed resource attributes. Managed identity
stays in local state. External identity is the exception, and it lives under
`lifecycle.id`.

## Providers

External mode is explicit. A provider rejects types it cannot read safely.

| Provider | External type |
| --- | --- |
| Matomo | `matomo.container` |
| Google Ads | `googleads.campaign` |
| Meta Ads | `meta.campaign` |

Other types in those providers stay managed. Google Ads external campaigns are
read by campaign id and do not require the campaign budget to be an Agoraform
resource, because the campaign is not reconciled. Meta campaigns are read by id.

`providers.matomo.publish: true` is a separate publication setting. It can
still publish the Tag Manager draft that contains managed tags inside an
external container. Publication does not create, update, or delete the
container resource itself.
