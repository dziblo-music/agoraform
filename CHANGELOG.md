# Changelog

All notable changes to Agoraform are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and Agoraform versions follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

Git tags are `v` plus the SemVer identifier (`v0.1.0`). `agoraform --version`
prints the SemVer identifier without the prefix (`0.1.0`).

## [Unreleased]

### Added

- Explicit resource references using `$ref: provider.type.name` objects, while ordinary strings remain provider-owned values
- A provider-neutral directed dependency graph with stable prerequisite-first ordering
- Manifest load, `validate`, `plan`, and `apply` reject malformed or missing references, self-references, and dependency cycles before remote reads or mutations
- `apply` executes creates and updates in dependency order and resolves provider-native identities and outputs for dependents at runtime
- Plans and import YAML keep logical addresses; provider-native identities stay out of configuration
- Matomo Tag Manager `matomo.variable` for Data Layer variables in the configured container draft (read, create, update, import)
- Matomo Tag Manager `matomo.trigger` for Custom Event triggers in the configured container draft (read, create, update, import)
- Matomo Tag Manager `matomo.tag` for Matomo Analytics event tags that reference triggers and variables by logical address (read, create, update, import)
- Tag Manager import reconstructs logical `$ref` relationships from local state so imported tags plan with zero changes against unchanged remotes

### Changed

- `plan` reads resources in deterministic dependency order (prerequisites first) so Tag Manager tags can compare trigger and variable `$ref`s without leaking provider-native ids
- `agoraform import` for `matomo.tag` requires related fire triggers (and prefers managed variables) to already be bound in local state before reconstructing configuration

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

[0.1.0]: https://github.com/dziblo-music/agoraform/releases/tag/v0.1.0
