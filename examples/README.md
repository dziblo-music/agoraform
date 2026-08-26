# Examples

Example manifests live here. They are validated automatically by the Go test
suite and contain no credentials or private deployment values.

- [matomo-conversion](matomo-conversion/README.md) — primary v0.2.0 quickstart:
  complete Matomo Tag Manager conversion workflow with a Data Layer variable,
  Custom Event trigger, Matomo Analytics event tag, logical references, import
  guidance, and declarative publication.
- [agoraform.yaml](agoraform.yaml) — minimal `matomo.goal` example retained from
  v0.1.0.

Managed identities belong in `agoraform.state.json` beside the working
manifest, not in resource attributes. See [docs/state.md](../docs/state.md).

## v0.2.0 quickstart

`validate`, `plan`, `apply`, and `import` contact Matomo. Set the required
runtime configuration first:

```bash
export MATOMO_URL=https://matomo.example.com
export MATOMO_TOKEN_AUTH=replace-me
export MATOMO_SITE_ID=1
export MATOMO_CONTAINER_ID=replace-with-your-container-id

cp examples/matomo-conversion/agoraform.yaml agoraform.yaml

agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Review the first plan before applying. The example enables declarative
publication, so `plan` makes the potential publication visible and `apply`
publishes only after draft resources converge. The final `plan` should report
`No changes.` when configuration and remote state are unchanged.

See [matomo-conversion/README.md](matomo-conversion/README.md) for the
application-side event contract, Matomo verification, disabled-publication
workflow, and import guidance.

## Minimal goal example

For the smaller v0.1-style goal workflow:

```bash
cp examples/agoraform.yaml agoraform.yaml
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Provider credentials are supplied through `MATOMO_*` environment variables,
never in the manifest. See [providers/matomo/README.md](../providers/matomo/README.md).
