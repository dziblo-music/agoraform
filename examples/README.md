# Examples

Example manifests live here. They are structurally valid v0.1 documents and
contain no credentials or personal data.

- [agoraform.yaml](agoraform.yaml) — minimal manifest using a Matomo-style
  resource address to illustrate `provider.type.name` addressing.

Validate an example from the repository root:

```bash
agoraform validate -f examples/agoraform.yaml
```

Provider implementations are added separately. Until a provider is registered
with the CLI, `validate` checks document structure, addresses, and duplicates.
`agoraform plan` requires a registered provider so it can read live state.
