# Publish (v0.2)

`agoraform publish` creates a Matomo Tag Manager container version from the
current draft and publishes that version to the configured environment. It
is a deliberate step after `apply`. Apply writes Tag Manager resources to
the container draft only and never publishes as a side effect.

```text
validate -> plan -> apply -> publish -> plan
```

The last `plan` reports no changes when configuration, local state, and
the remote draft are unchanged. Publication snapshots the draft; it does
not rewrite managed Tag Manager resources.

```text
manifest
   │
   ▼
validate
   │
   ▼
check publication configuration
   │
   ▼
compare draft to the published environment
   │
   ▼
create version (only when needed)
   │
   ▼
publish version
   │
   ▼
report result
```

## Command

```bash
agoraform publish
agoraform publish -f path/to/manifest.yaml
agoraform publish path/to/manifest.yaml
```

If no path is given, Agoraform reads `agoraform.yaml` in the current
directory.

`publish` loads and validates the manifest using the same file selection
conventions as `validate`, `plan`, and `apply`. It does not create or
update individual resources. Missing or invalid publication configuration
fails before a version is created.

v0.2 publishes the single Matomo Tag Manager container identified by
`MATOMO_CONTAINER_ID` to `MATOMO_ENVIRONMENT` (default `live`).

## Idempotency

Re-running `publish` when the configured container draft is already
represented by the currently published version does not create or publish
a duplicate version. Comparison uses Tag Manager tags, triggers, and
variables and ignores provider-native ids and other computed metadata.

## Output

When a version is created and published:

```text
matomo.container.main: creating version...
matomo.container.main: published
```

When the draft is already published:

```text
matomo.container.main: no publication required
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Publish succeeded, including a no-op |
| `1` | Publish failed (invalid manifest, missing configuration, API error) |
| `3` | Invalid invocation (unknown flag or conflicting file arguments) |

## Safety

- Publication is never an automatic side effect of `apply`.
- Authentication tokens are never printed in publish output or errors.
- Missing site, container, or environment configuration fails before
  `TagManager.createContainerVersion` or `TagManager.publishContainerVersion`.
- Version-creation and publication failures are reported with the Matomo
  method and a secret-safe message.
