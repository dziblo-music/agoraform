# Examples

Example manifests live here. They are validated automatically by the Go test
suite and contain no credentials or private deployment values.

- [agoraform.yaml](agoraform.yaml) — minimal v0.1.0 `matomo.goal` manifest.
- [matomo-conversion](matomo-conversion/README.md) — complete v0.2.0 Matomo Tag
  Manager conversion workflow with a variable, trigger, tag, and declarative
  publication.

Managed identities belong in `agoraform.state.json` beside the manifest,
not in resource attributes. See [docs/state.md](../docs/state.md).

## Workflow

`validate`, `plan`, `apply`, and `import` contact Matomo. Set
`MATOMO_URL`, `MATOMO_TOKEN_AUTH`, and `MATOMO_SITE_ID` first. Structural
loading of this file is covered by unit tests without a live instance.

From the root of an extracted release archive or repository checkout:

```bash
export MATOMO_URL=https://matomo.example.com
export MATOMO_TOKEN_AUTH=replace-me
export MATOMO_SITE_ID=1

cp examples/agoraform.yaml agoraform.yaml

agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

The second `plan` should report no changes when the remote goal and local
state are unchanged.

Import an existing goal without recreating it:

```bash
agoraform import matomo.goal.trial_started 12
```

Add the printed YAML to `agoraform.yaml`, then run `agoraform plan` again.

Provider credentials are supplied through `MATOMO_*` environment variables,
never in the manifest. See [providers/matomo/README.md](../providers/matomo/README.md).
