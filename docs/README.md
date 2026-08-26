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
- [Google Ads provider](../providers/googleads/README.md)
- [v0.3 Google Ads conversion example](../examples/googleads-conversion/README.md)

The 0.1.0 product release manages `matomo.goal` only. v0.2.0 also manages
Tag Manager `matomo.variable`, `matomo.trigger`, and `matomo.tag`, plus
declarative container publication through `plan` and `apply`. Unreleased
v0.3 work adds website `googleads.conversion_action` and
`googleads.customer_conversion_goal` resources; see the
[Google Ads conversion example](../examples/googleads-conversion/README.md).
The CLI remains provider-neutral.

Changelog: [CHANGELOG.md](../CHANGELOG.md).

## Local development

See [CONTRIBUTING.md](../CONTRIBUTING.md) for build, test, and pull request
workflow.
