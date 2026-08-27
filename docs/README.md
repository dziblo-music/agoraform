# Documentation

User and contributor documentation beyond the root README and CONTRIBUTING
guides lives here.

## Contents

- [Manifest format (0.1.0)](manifest.md)
- [Plan engine (0.1.0)](plan.md)
- [Apply execution (0.1.0)](apply.md)
- [Import (0.1.0)](import.md)
- [Local provider configuration](local-configuration.md)
- [Matomo Tag Manager publication (v0.2.0)](matomo-publishing.md)
- [Google Ads account and OAuth setup (v0.3.0)](google-ads-setup.md)
- [Local state (0.1.0)](state.md)
- [Releasing](release.md)
- [Matomo provider](../providers/matomo/README.md)
- [Google Ads provider](../providers/googleads/README.md)
- [v0.2 Matomo conversion example](../examples/matomo-conversion/README.md)
- [v0.3 Google Ads conversion example](../examples/googleads-conversion/README.md)

The 0.1.0 product release manages `matomo.goal` only. v0.2.0 also manages
Tag Manager `matomo.variable`, `matomo.trigger`, and `matomo.tag`, plus
declarative container publication through `plan` and `apply`. v0.3.0 adds
supported website `googleads.conversion_action` and
`googleads.customer_conversion_goal` resources plus import/adoption of existing
conversion measurement; see the [Google Ads setup guide](google-ads-setup.md)
and [Google Ads conversion example](../examples/googleads-conversion/README.md).
Current `main` also manages daily Search `googleads.campaign_budget`
resources, Search `googleads.campaign` resources, Search
`googleads.ad_group` resources, and campaign
`googleads.campaign_conversion_goal` biddability.

Provider credentials and connection settings can be stored locally in an
optional [`.agoraform.env`](local-configuration.md) file instead of being
exported manually for each shell session. Credentials remain outside manifests,
plan output, and local state. The CLI remains provider-neutral.

Changelog: [CHANGELOG.md](../CHANGELOG.md).

## Local development

See [CONTRIBUTING.md](../CONTRIBUTING.md) for build, test, and pull request
workflow.
