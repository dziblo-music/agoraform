# Documentation

User and contributor documentation beyond the root README and CONTRIBUTING
guides lives here.

## Contents

- [Manifest format (0.1.0)](manifest.md)
- [Plan engine (0.1.0)](plan.md)
- [Apply execution (0.1.0)](apply.md)
- [Import (0.1.0)](import.md)
- [Matomo Tag Manager publication (v0.2.0)](matomo-publishing.md)
- [Local state (0.1.0)](state.md)
- [Releasing](release.md)
- [Matomo provider](../providers/matomo/README.md)

The 0.1.0 product release manages `matomo.goal` only. Unreleased work for
v0.2.0 also manages Tag Manager `matomo.variable`, `matomo.trigger`, and
`matomo.tag`, plus declarative container publication through `plan` and
`apply`. The CLI remains provider-neutral; there is no Matomo-specific
`publish` command.

Changelog: [CHANGELOG.md](../CHANGELOG.md).

## Local development

See [CONTRIBUTING.md](../CONTRIBUTING.md) for build, test, and pull request
workflow.
