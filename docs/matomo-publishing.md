# Matomo Tag Manager publication

Agoraform keeps its CLI provider-neutral. Matomo Tag Manager publication is
therefore declarative provider desired state, not a separate `publish`
command.

```yaml
apiVersion: agoraform.io/v1alpha1
providers:
  matomo:
    publish: true
    environment: live
resources:
  # matomo.container and/or matomo.variable / matomo.trigger / matomo.tag
```

`publish` defaults to `false`. When publication is enabled and `environment`
is omitted, the target defaults to `live`.

`publish` and `environment` are safe, reviewable desired state and belong in
the manifest. Matomo connection and authentication values remain runtime
configuration:

```text
MATOMO_URL
MATOMO_TOKEN_AUTH
MATOMO_SITE_ID
MATOMO_CONTAINER_ID   # only when no matomo.container resource is declared
```

Never put authentication tokens in the manifest.

When a `matomo.container` resource is declared, publication uses that
resource's provider-native identity. When it is omitted,
`MATOMO_CONTAINER_ID` selects an existing container and the plan addresses
publication as `matomo.container.external`.

## Plan

`agoraform plan` is still non-mutating. When applying the reviewed plan would
publish the configured Tag Manager container, the plan includes an explicit
provider action, for example:

```text
> matomo.container.main: publish -> live
```

When `MATOMO_CONTAINER_ID` selects an existing container instead, the same
action is addressed as `matomo.container.external`. The address always
identifies the container that will be published; it is not a hard-coded
orchestration name.

A planned create or update of a managed `matomo.variable`, `matomo.trigger`,
or `matomo.tag` makes potential publication visible when `publish: true`:

```text
> matomo.container.main: publish -> live [conditional]
```

The action is conditional because the planned mutations may converge the draft
back to the version already published. Apply makes the final decision against
the converged draft immediately before version creation. When there are no
planned draft resource changes, Agoraform compares the current draft with the
published version: a difference produces a definite publication action, while
equivalent fingerprints omit the action.

Provider-native IDs and version metadata are ignored for that comparison.
Behavioral tag state is not ignored: for example, a paused tag and an active
tag are different container configurations.

## Apply

`agoraform apply` executes provider finalization only after every planned
resource create/update succeeds:

```text
validate -> plan -> apply draft changes -> create version -> publish
```

If a variable, trigger, or tag mutation fails, Agoraform does not create or
publish a container version.

Immediately before publication, Agoraform rechecks whether publication is
still required. This prevents repeated unchanged applies from creating
duplicate versions.

A successful apply may report:

```text
matomo.container.main: version 12 created
matomo.container.main: published to live
```

If version creation succeeds but publication fails, the error still reports
that the version was created so the partial remote mutation is visible.

An empty, JSON `null`, unreadable, oversized, or otherwise unrecognizable
publish response is not treated as success. The publish request has already
been sent, so Agoraform cannot know whether Matomo completed publication. The
resulting error says the outcome is uncertain and tells you to inspect the
remote container before retrying. Do not create another version until that
status is known.

## Permission and environment preflight

Before any version is created, Agoraform asks Matomo for the environments the
current credentials are actually allowed to publish to using
`TagManager.getAvailableEnvironmentsWithPublishCapability`.

If the configured environment exists but the credentials cannot publish to
it, planning/apply fails before `TagManager.createContainerVersion`. An
unknown environment is also rejected before mutation.

## Disabled publication

With:

```yaml
providers:
  matomo:
    publish: false
```

Tag Manager resources are reconciled in the draft only. No version creation or
publication action is planned or executed.

Omitting `providers.matomo` has the same publication behavior: publication is
disabled.

## Scope

v0.2.0 intentionally does not add provider-specific CLI verbs. Rollback,
scheduled publication, approval workflows, generalized deployment pipelines,
and multi-container deployment orchestration remain out of scope. Container
deletion is a later destroy/lifecycle issue.
