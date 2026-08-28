# Matomo conversion tracking

This example manages a complete `trialStarted` conversion workflow in one
Matomo Tag Manager container:

- `matomo.variable.user_id` reads `userId` from the data layer;
- `matomo.trigger.trial_started` listens for the `trialStarted` custom event;
- `matomo.tag.trial_started` records a Matomo Analytics event and uses the
  managed variable as its event name;
- `providers.matomo.publish: true` publishes the resulting container version
  to the `live` environment during `apply`.

The logical `$ref` values create explicit dependencies. Agoraform therefore
creates the variable and trigger before the tag and resolves their Matomo IDs
at apply time. Provider-native IDs and credentials do not belong in the
manifest.

## Prerequisites

You need a Matomo instance with Tag Manager enabled, an existing website and
container, and an API token allowed to manage the container and publish to the
target environment. Find the site ID and container ID in Matomo; neither is a
secret, but they are runtime settings so this reusable example does not embed
them.

To create the container from Agoraform instead of selecting one with
`MATOMO_CONTAINER_ID`, declare a `matomo.container` resource and add
`container: { $ref: matomo.container.main }` to each child. See the
[Matomo provider reference](../../providers/matomo/README.md).

The target container must also already contain a **Matomo Configuration**
variable. Matomo Analytics tags reference that container-level variable for the
Matomo URL, site ID, and tracking configuration; v0.2 does not manage
`MatomoConfiguration` variables declaratively. In Matomo Tag Manager, create
one under **Variables -> Create New Variable -> Matomo Configuration** before
running this example if the container does not already have one. The variable
may use Matomo's normal scalar or structured options such as domains, custom
dimensions, and custom data.

Set the four required environment variables:

```bash
export MATOMO_URL=https://matomo.example.com
export MATOMO_TOKEN_AUTH=replace-with-your-api-token
export MATOMO_SITE_ID=1
export MATOMO_CONTAINER_ID=replace-with-your-container-id
```

Keep `MATOMO_TOKEN_AUTH` out of shell history, logs, source control, and the
manifest. On a real system, load it from your usual secret manager. Change
`providers.matomo.environment` if you intentionally publish somewhere other
than `live`, and ensure the token has publish capability for that environment.

Copy the example into a working directory so its generated
`agoraform.state.json` remains local:

```bash
cp examples/matomo-conversion/agoraform.yaml ./agoraform.yaml
```

## Application event contract

After the Matomo Tag Manager container snippet has initialized
`window._mtm`, the application emits this object when a trial is successfully
created:

```javascript
window._mtm.push({
  event: "trialStarted",
  userId: "..."
})
```

Both names are case-sensitive and must match the manifest. Emit the event once
per successful conversion. Use a stable, non-secret, non-email identifier for
`userId`, and apply the privacy and consent rules appropriate to your site.

## Apply and publish

Run the full lifecycle from the directory containing the copied manifest:

```bash
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Review the first plan before applying it. Because publication is declarative,
there is no separate `agoraform publish` command: the plan shows
`matomo.container.external: publish -> live [conditional]` when this example
selects an existing container with `MATOMO_CONTAINER_ID`. After all three draft
resources converge, `apply` creates and publishes a container version only if
the resulting draft differs from live. The final plan must report `No changes.`
when the manifest and remote configuration are unchanged.

In Matomo, open Tag Manager for `MATOMO_CONTAINER_ID` and verify that the draft
contains the **Trial user ID** variable, **Trial started** trigger, and
**Track trial started** tag. Then open Versions/Environments and verify that
the newly created version is published to **live** and contains those same
resources. Use Matomo Tag Manager preview/debug mode to push a test event and
confirm that the trigger fires the tag, then verify the `trial` / `started`
event in Matomo's visitor or event reporting.

## Adopt existing resources

The same manifest is import-compatible. If equivalent resources already
exist, import dependencies before the tag, using the numeric IDs shown by
Matomo:

```bash
agoraform import matomo.variable.user_id VARIABLE_ID
agoraform import matomo.trigger.trial_started TRIGGER_ID
agoraform import matomo.tag.trial_started TAG_ID
```

Each command prints canonical YAML for review and records the remote identity
in `agoraform.state.json`; it does not edit the manifest or mutate Matomo.
Compare the printed attributes with this example, adjust the manifest if
needed, then run `agoraform plan`. See [Import](../../docs/import.md) for the
complete adoption workflow.
