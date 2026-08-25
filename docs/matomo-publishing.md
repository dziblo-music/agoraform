# Matomo Tag Manager publication (v0.2.0)

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
  # matomo.variable / matomo.trigger / matomo.tag resources
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
MATOMO_CONTAINER_ID
```

Never put authentication tokens in the manifest.

## Plan

`agoraform plan` is still non-mutating. When applying the reviewed plan would
publish the configured Tag Manager container, the plan includes an explicit
provider action, for example:

```text
> matomo.container.main: publish -> live
```

A planned create or update of a managed `matomo.variable`, `matomo.trigger`,
or `matomo.tag` causes publication to be planned when `publish: true`. When
there are no draft resource changes, Agoraform compares the current draft with
the version already published to the configured environment and omits the
provider action when they are equivalent.

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
and multi-container deployment orchestration remain out of scope.
