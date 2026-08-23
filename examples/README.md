# Examples

Example manifests live here. They are structurally valid v0.1 documents and
contain no credentials or personal data.

- [agoraform.yaml](agoraform.yaml) — minimal manifest using a Matomo-style
  resource address to illustrate `provider.type.name` addressing.

Validate an example from the repository root:

```bash
agoraform validate -f examples/agoraform.yaml
```

The example uses a Matomo-style address. The Matomo provider is registered
with the CLI, but `matomo.goal` is not implemented yet, so `validate` and
`plan` report an unknown resource type. Structural loading of this file is
covered by unit tests.

Provider credentials are supplied through `MATOMO_*` environment variables,
never in the manifest. See [providers/matomo/README.md](../providers/matomo/README.md).
