# Changelog

All notable changes to Agoraform are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and Agoraform versions follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

Git tags are `v` plus the SemVer identifier (`v0.1.0`). `agoraform --version`
prints the SemVer identifier without the prefix (`0.1.0`).

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
