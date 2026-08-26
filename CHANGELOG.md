# Changelog

All notable changes to Agoraform are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and Agoraform versions follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

Git tags are `v` plus the SemVer identifier (`v0.2.0`). `agoraform --version`
prints the SemVer identifier without the prefix (`0.2.0`).

## [Unreleased]

### Added

- Google Ads provider foundation (`googleads`) with environment-based OAuth and developer-token authentication, customer ID normalization, and a reusable REST client for query and mutate operations.

## [0.2.0] - 2026-08-26

Agoraform 0.2.0 expands the Matomo provider from analytics goals into a
small, dependency-aware Matomo Tag Manager workflow. It can manage a Data
Layer variable, Custom Event trigger, and Matomo Analytics event tag, adopt
supported existing Tag Manager resources, and declaratively publish the
resulting container through the normal `plan`/`apply` lifecycle.

### Added

- Explicit resource references using `$ref: provider.type.name` objects, while ordinary strings remain provider-owned values.
- A provider-neutral directed dependency graph with stable prerequisite-first ordering.
- Manifest load, `validate`, `plan`, and `apply` reject malformed or missing references, self-references, and dependency cycles before remote mutations.
- `apply` executes creates and updates in dependency order and resolves provider-native identities and outputs for dependents at runtime.
- `matomo.variable` for Tag Manager Data Layer variables (read, create, update, import).
- `matomo.trigger` for Tag Manager Custom Event triggers (read, create, update, import).
- `matomo.tag` for Matomo Analytics event tags that reference managed triggers and variables (read, create, update, import).
- Tag Manager import reconstructs logical `$ref` relationships from local state when related resources are already imported.
- Declarative Matomo Tag Manager publication through `providers.matomo.publish` and `providers.matomo.environment`.
- Provider-neutral plan/apply finalization hooks so provider-specific convergence remains visible without adding provider-specific CLI commands.
- Capability-aware publication preflight and idempotent draft-versus-published comparison.
- Partial-apply diagnostics when a remote mutation succeeds but state persistence or provider finalization fails.
- A complete secret-free Matomo conversion-tracking example under `examples/matomo-conversion/`.

### Changed

- `plan` reads resources in deterministic dependency order so Tag Manager tags can compare logical references without leaking provider-native IDs.
- `agoraform import` for `matomo.tag` requires related fire triggers, and preferably managed variables, to already be bound in local state before reconstructing configuration.
- `plan` surfaces a Matomo container publication action whenever `apply` may publish the reviewed Tag Manager draft.
- `apply` performs configured publication only after all planned draft resource mutations succeed, then rechecks whether publication is still required before creating a version.
- Repeated unchanged apply does not create duplicate container versions.
- Tag Manager publication comparison treats behavioral tag status, such as active versus paused, as meaningful while ignoring provider-native IDs and version metadata.
- Publication rejects empty, JSON `null`, unreadable, oversized, and other unconfirmed publish responses instead of treating them as success.

### Supported in 0.2.0

| Area | Scope |
| --- | --- |
| Commands | `validate`, `plan`, `apply`, `import` |
| Provider | Matomo |
| Resources | `matomo.goal`, `matomo.variable`, `matomo.trigger`, `matomo.tag` |
| References | Logical `$ref` dependencies with prerequisite-first planning/apply |
| Tag Manager import | Supported variables, triggers, and tags; dependencies imported first |
| Publication | Declarative through `providers.matomo.publish` / `environment`, visible in `plan` and executed during `apply` |
| State | Local `agoraform.state.json` beside the manifest |
| Mutations | Create and update only |

### Limitations

- Matomo is the only provider; Google Ads and Meta Ads are not implemented.
- v0.2.0 manages one configured Matomo Tag Manager container via `MATOMO_CONTAINER_ID`.
- Tag Manager support is intentionally limited to Data Layer variables, Custom Event triggers, and Matomo Analytics event tags covered by this release.
- There is no provider-specific `agoraform publish` command; publication is declarative desired state in the provider configuration.
- Rollback, scheduled publication, approval workflows, multi-container deployment orchestration, and generalized deployment pipelines are not implemented.
- Remote state, workspaces, locking, and encryption are not implemented.
- `apply` does not delete remote resources and there is no `destroy` command yet.
- `plan` ignores remote objects that are not represented by the manifest/local state.
- Live `validate`, `plan`, `apply`, and `import` require reachable Matomo configuration; Tag Manager operations additionally require `MATOMO_CONTAINER_ID`.
- Pre-1.0: breaking CLI or manifest changes may appear in later `0.x` releases and will be documented.

### Upgrade notes

0.2.0 keeps the v0.1.0 provider-neutral command surface. Existing
`matomo.goal` manifests continue to work. Tag Manager publication is opt-in:
omitting `providers.matomo.publish` or setting it to `false` leaves the draft
unpublished.

See the [v0.2.0 conversion example](examples/matomo-conversion/README.md) and
[release guide](docs/release.md).

## [0.1.0] - 2026-08-24

First public release.

### Added

- CLI commands: `validate`, `plan`, `apply`, `import`
- v0.1 YAML manifest schema (`apiVersion: agoraform.io/v1alpha1`)
- Local identity state in `agoraform.state.json`
- Matomo provider with a single managed resource: `matomo.goal` (read, create, update, import)
- GitHub Actions release workflow that verifies SemVer tags and `main` ancestry, then publishes draft multi-platform binaries
- Reproducible release packaging with Go 1.26.7, GoReleaser 2.17.1, SHA-256 checksums, and immutable GitHub Action pins

### Supported in 0.1.0

| Area | Scope |
| --- | --- |
| Commands | `validate`, `plan`, `apply`, `import` |
| Provider | Matomo |
| Resource | `matomo.goal` |
| State | Local `agoraform.state.json` beside the manifest |
| Mutations | Create and update only |

### Limitations

- Google Ads and Meta Ads are not implemented
- Matomo Tag Manager resources and container publishing are not implemented
- Remote state, workspaces, locking, and encryption are not implemented
- `apply` does not delete remote resources
- `plan` ignores remote objects that are not in the manifest
- `validate`, `plan`, `apply`, and `import` require a reachable Matomo instance and `MATOMO_URL`, `MATOMO_TOKEN_AUTH`, and `MATOMO_SITE_ID`
- Pre-1.0: breaking CLI or manifest changes may appear in later `0.x` releases and will be documented

### Install

See the [README](README.md#install) and [docs/release.md](docs/release.md).

[Unreleased]: https://github.com/dziblo-music/agoraform/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/dziblo-music/agoraform/releases/tag/v0.2.0
[0.1.0]: https://github.com/dziblo-music/agoraform/releases/tag/v0.1.0
