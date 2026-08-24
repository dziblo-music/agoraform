# Examples

Example manifests live here. They are structurally valid v0.1 documents and
contain no credentials or personal data.

- [agoraform.yaml](agoraform.yaml) — minimal `matomo.goal` manifest.

Validate an example from the repository root:

```bash
agoraform validate -f examples/agoraform.yaml
```

`matomo.goal` is implemented. `validate` and `plan` require `MATOMO_URL`,
`MATOMO_TOKEN_AUTH`, and `MATOMO_SITE_ID`. Structural loading of this file
is covered by unit tests.

Provider credentials are supplied through `MATOMO_*` environment variables,
never in the manifest. See [providers/matomo/README.md](../providers/matomo/README.md).
