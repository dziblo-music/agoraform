# Changelog

All notable changes to Agoraform are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and Agoraform versions follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).

Git tags are `v` plus the SemVer identifier (`v0.5.0`). `agoraform --version`
prints the SemVer identifier without the prefix (`0.5.0`).

## [Unreleased]

### Added

- Meta website conversion measurement for v0.6.0: `meta.pixel` binds an
  existing Pixel/Dataset event source through import or unique-name adopt
  (Agoraform does not create or delete Pixels), and `meta.custom_conversion`
  creates, reads, updates, imports, and archives website Custom Conversions
  against a logical `$ref` to that pixel. Pixel `pixelId` and custom
  conversion `customConversionId` are declared non-secret outputs. Import
  reconstructs the pixel relationship only when a bound `meta.pixel` is
  unique. Rule, pixel, and `eventType` are immutable after create. Destroy
  calls Marketing API `DELETE` and treats `is_archived` or absence as the
  terminal state. Browser Pixel/`fbq` and Conversions API delivery stay
  outside Agoraform.
- Registered the `meta` provider foundation for v0.6.0 with runtime-only
  `META_ACCESS_TOKEN` and normalized `META_AD_ACCOUNT_ID` configuration,
  read-only validation of `ads_management` and ad-account access, and no
  Meta-specific CLI path.
- Added a reusable Meta Graph and Marketing API client pinned to v26.0 with
  authenticated GET/POST/DELETE requests, context-aware timeouts, cursor
  pagination, bounded JSON decoding, API code/subcode and request/trace ID
  mapping, transient classification without automatic mutation retries, and
  secret-safe diagnostics.

## [0.5.0] - 2026-09-01

Agoraform 0.5.0 is a provider-completeness and lifecycle release. It does
not add a new provider. It closes remaining Matomo bootstrap and teardown
gaps, completes Google Ads destroy/remove semantics for the existing
resource set, adds the provider-neutral `agoraform destroy` command, and
proves cross-provider outputs so a Google Ads conversion action can feed a
Matomo Google Ads conversion tag.

The supported lifecycle for resources present in the manifest and bound in
state is:

```text
validate -> plan -> apply -> plan -> import/adopt -> destroy
```

`agoraform destroy` is explicit. Removing a resource from the manifest does
not destroy or prune it. State-only identities are preserved. Destroy may
finish supported teardown while leaving provider-owned Google Ads conversion
goals in state; that run reports the remnants and returns non-zero instead
of claiming full teardown.

### Added

- A complete secret-free v0.5.0 Matomo + Google Ads lifecycle example under
  `examples/matomo-googleads/`, automatically validated by the test suite.
  The primary manifest manages a Tag Manager container, Matomo Configuration
  and Data Layer variables, a `trialStarted` trigger, a Google Ads website
  conversion action, and a Matomo Google Ads conversion tag that consumes
  `conversionId` and `conversionLabel` through `{ $ref, output }`. The
  documentation covers greenfield apply, import/adoption, no-op
  reconciliation, destroy ordering including provider-owned Google Ads
  remnants, and a compact external-container variant using
  `MATOMO_CONTAINER_ID`.
- Cross-provider import matching: `agoraform import` builds a read-only
  catalog of declared non-sensitive outputs from already-bound state
  resources. Providers can request a unique match by provider, resource type,
  and one or more named output values without importing another provider
  package. A unique match becomes `{ $ref, output }`; missing and ambiguous
  matches never guess. The catalog is ephemeral and is not persisted.
- Cross-provider resource outputs: manifests can select one declared
  non-sensitive named output with `{ $ref, output }` while address-only
  `$ref` stays unchanged. Apply resolves the value after the prerequisite
  converges. Plan renders the logical selector and does not fabricate unknown
  values. Sensitive, unknown, unavailable, and wrong-kind outputs fail before
  the dependent mutation. `googleads.conversion_action` exposes `conversionId`
  and `conversionLabel`. The state schema is unchanged.
- `matomo.tag` `type: googleAdsConversion` for Matomo Tag Manager's Google
  Ads conversion template. Tags consume `conversionId` and `conversionLabel`
  from a managed `googleads.conversion_action` through `{ $ref, output }`,
  fire from a logical `matomo.trigger`, and preserve unmanaged template
  parameters on update. Equivalent conversion IDs such as `9988776655` and
  `AW-9988776655` compare equal. Import reconstructs `{ $ref, output }` only
  on a unique bound-output match; absent and ambiguous matches emit literals.
- `matomo.container` for Matomo Tag Manager containers (read, create, update, import, destroy). Declare `name`, `context` (`web`, `android`, or `ios`), and optional `description`. Provider-native container IDs, draft versions, publication state, and unmanaged Matomo flags remain computed. Context is immutable. A container present in the manifest and bound in state is managed and eligible for destroy; a container selected only by `MATOMO_CONTAINER_ID` is never deleted.
- `agoraform destroy` for provider-neutral teardown. The command plans supported destructions in reverse dependency order, requires interactive confirmation or `--auto-approve`, removes local state only after a confirmed remote terminal or already-absent result, and runs planned provider finalizations after those mutations succeed. Unsupported and provider-owned resources remain in state and do not block supported teardown.
- Matomo destroy for `matomo.goal`, Data Layer and Matomo Configuration `matomo.variable`, `matomo.trigger`, `matomo.tag`, and Agoraform-managed `matomo.container`. Tag Manager child deletions follow the existing publication contract: `publish: false` leaves the draft unpublished, `publish: true` publishes after successful draft mutations, already-published retries do not create duplicate versions, and a plan that deletes its managed container does not publish an intermediate version.
- Google Ads destroy/removal for every v0.3/v0.4 resource type. Removable types use Google Ads mutate `remove` (terminal `status=REMOVED`). Customer and campaign conversion goals are provider-owned: they are not mutated, remain in state, and cause a non-zero destroy result after supported teardown. Destroy never enables serving or spend.
- Managed-container mode for Tag Manager children: `matomo.variable`, `matomo.trigger`, and `matomo.tag` declare `container: { $ref: matomo.container.* }` so apply resolves the provider-native container identity from the resource graph. v0.5.0 allows at most one managed Matomo container per manifest.
- `matomo.variable` `type: matomoConfiguration` for the Matomo Configuration variable required by Matomo Analytics tags. Declare `name`, `matomoUrl`, `siteId`, and optional `enableLinkTracking`. Updates preserve unowned template parameters such as cookies, domains, and custom dimensions. `matomo.tag` can reference the managed variable with `matomoConfiguration: { $ref: matomo.variable.* }`. Existing containers can still use a single pre-existing configuration variable through implicit discovery or import.

### Changed

- Tag Manager operations and publication select the container from the resource binding or `MATOMO_CONTAINER_ID` without mutating provider-global configuration. Publication is addressed as the managed `matomo.container` resource, or as `matomo.container.external` when an existing container is selected by environment. Mixing a managed container with `MATOMO_CONTAINER_ID` is rejected before mutation.
- Existing `MATOMO_CONTAINER_ID` workflows remain supported when no `matomo.container` resource is declared. Child resources omit the `container` attribute in that mode.

### Supported in 0.5.0

| Area | Scope |
| --- | --- |
| Commands | `validate`, `plan`, `apply`, `import`, `destroy` |
| Providers | Matomo, Google Ads (`googleads`); no new providers |
| Matomo | `matomo.goal`, `matomo.container`, Data Layer and Matomo Configuration `matomo.variable`, `matomo.trigger`, Matomo Analytics and Google Ads conversion `matomo.tag` |
| Google Ads conversion | `googleads.conversion_action`, `googleads.customer_conversion_goal` |
| Google Ads Search | `googleads.campaign_budget`, `googleads.campaign`, `googleads.campaign_conversion_goal`, `googleads.ad_group`, `googleads.keyword`, `googleads.responsive_search_ad`, `googleads.campaign_location`, `googleads.campaign_language` |
| References | Address-only `$ref` plus optional named `{ $ref, output }` selectors |
| Import | Supported Matomo and Google Ads resources; unique bound-output reconstruction; no mutation |
| Matomo publication | Declarative through provider configuration, visible in `plan`, executed during `apply` and `destroy` |
| Destroy | Explicit command; reverse dependency order; no automatic prune of resources removed from the manifest |
| State | Local `agoraform.state.json` beside the manifest; schema unchanged |
| Mutations | Create, update, and explicit destroy/remove |

### Limitations

- v0.5.0 does not add Meta Ads or any other new provider.
- Matomo website/site provisioning is not implemented.
- Google Ads billing, payment, and customer-account lifecycle are not implemented.
- Application event delivery, gtag.js, Google Tag, consent handling, and conversion-event emission remain outside Agoraform.
- Cross-provider transactional rollback is not implemented.
- There is no drift command and no deletion reconciliation for resources removed from configuration.
- Google Ads campaign support remains Search only. Performance Max, Display, Video, Shopping, App, Dynamic Search Ads, and other campaign families are not implemented.
- Agoraform does not generate creative, upload image or video assets, enable spend automatically, or apply Google Ads optimization recommendations.
- Offline, call, app, and conversion-event upload workflows are not implemented; `googleads.conversion_action` remains limited to supported website (`WEBPAGE`) actions.
- Customer and campaign conversion goals are created by Google Ads. Destroy reports them as provider-owned, leaves them in state, and returns non-zero after supported teardown.
- Immutable identity fields such as keyword text/match type/negative, location, language, and ad-group parent fail planning instead of destroying and recreating the remote object. `apply` still does not delete remote resources.
- Google Ads authentication uses developer-token + single-user OAuth refresh-token credentials. Service-account, multi-user, and interactive OAuth flows are not implemented.
- Remote state, workspaces, locking, and encryption are not implemented.
- v0.5.0 allows at most one managed Matomo Tag Manager container per manifest.
- Pre-1.0: breaking CLI or manifest changes may appear in later `0.x` releases and will be documented.

### Upgrade notes

0.5.0 does not remove or rename the v0.2.0 Matomo command/resource surface,
the v0.3.0 Google Ads conversion-measurement surface, or the v0.4.0 Search
campaign surface. Existing v0.2–v0.4 manifests and `agoraform.state.json`
files continue to work. The local state schema is unchanged.

`MATOMO_CONTAINER_ID` workflows remain supported when no `matomo.container`
resource is declared. Managed-container mode, Matomo Configuration
variables, Google Ads conversion tags, `{ $ref, output }`, and
`agoraform destroy` are opt-in. Mixing a managed container with
`MATOMO_CONTAINER_ID` is rejected before mutation.

`agoraform destroy` tears down resources that are both present in the
manifest and bound in state. It does not prune identities that remain only
in state after a resource is deleted from configuration.

See the [v0.5.0 Matomo + Google Ads lifecycle example](examples/matomo-googleads/README.md),
[destroy guide](docs/destroy.md), and [release guide](docs/release.md).

## [0.4.0] - 2026-08-28

Agoraform 0.4.0 adds complete Google Ads Search campaign management while
preserving the Matomo functionality from 0.2.0 and the Google Ads conversion
measurement from 0.3.0. The release can declare, reconcile, and import a
paused Search campaign graph from daily budget through campaign, conversion
goals, targeting, ad group, keywords, and Responsive Search Ad without taking
responsibility for creative generation, spend enablement, or non-Search
campaign families.

### Added

- `googleads.campaign_budget` for daily Search campaign budgets (read, create, update, import). Amounts are declared in account-currency units and compared through deterministic micros normalization. Shared versus dedicated budgets are explicit. Equivalent remote state, including numeric aliases such as `50` and `50.00`, produces no plan diff. Provider-native IDs, resource names, micros, period, type, and status remain computed. Search campaigns can reference a budget by logical Agoraform address (`$ref: googleads.campaign_budget.*`).
- `googleads.campaign` for Search campaigns (read, create, update, import). Campaigns attach a budget with `$ref: googleads.campaign_budget.*`, declare supported Search bidding and network settings, and default new campaigns to `PAUSED`. Unsupported channel types and Dynamic Search Ads settings fail validation before mutation and during read/import. Equivalent remote state produces no plan diff. Provider-native IDs, resource names, serving status, and channel type remain computed.
- `googleads.campaign_conversion_goal` for campaign-level website conversion-goal biddability (read, adopt, update, import). Google Ads creates these objects automatically for campaign/category/origin combinations; Agoraform reconciles `biddable` and never attempts unsupported create or delete operations. Goals reference campaigns with `$ref: googleads.campaign.*`. Equivalent remote state produces no plan diff.
- `googleads.ad_group` for Search standard ad groups (read, create, update, import). Ad groups reference campaigns with `$ref: googleads.campaign.*`. Optional max CPC bids are declared in account-currency units and compared through deterministic micros normalization. New ad groups default to `PAUSED` and `SEARCH_STANDARD`. Unsupported types such as Shopping or Dynamic Search Ads fail validation before mutation. Equivalent remote state produces no plan diff. Provider-native IDs, resource names, and API-only fields remain computed.
- `googleads.keyword` for Search ad-group keyword criteria, including negative keywords (read, create, update, import). Keywords reference ad groups with `$ref: googleads.ad_group.*` and declare `text`, `matchType` (`EXACT`, `PHRASE`, `BROAD`), and optional `negative`. Text is normalized without changing user intent. Text, match type, negative, and ad group are immutable; plan reports that instead of hiding replacement. Positive keywords default to `PAUSED` and support mutable status/CPC bid overrides. Negative keywords default to `ENABLED`; their status is immutable because Google Ads does not allow updating negative ad-group criteria. Keyword-level final URLs, tracking templates, and custom URL parameters are out of scope and fail read/import with guidance. Equivalent remote state produces no plan diff. Provider-native IDs and resource names remain computed.
- `googleads.responsive_search_ad` for Search Responsive Search Ads (read, create, update, import). RSAs reference ad groups with `$ref: googleads.ad_group.*` and declare final URLs, 3–15 headlines, 2–4 descriptions, and optional display paths or pinning. Google Ads minimum headline/description/URL constraints are validated before mutation. Status updates the ad-group-ad relationship in place; creative changes replace the underlying ad lists through AdService and are visible in plan output. Ad group is immutable. Equivalent remote state, including provider-created asset metadata, produces no plan diff. Provider-native ad IDs and asset IDs remain computed.
- `googleads.campaign_location` and `googleads.campaign_language` for Search campaign targeting criteria (read, create, update, import). Locations and languages reference campaigns with `$ref: googleads.campaign.*`. Manifest values prefer reviewable names and ISO codes; Agoraform resolves them to Google Ads geo/language constants and fails on missing or ambiguous targets before mutation. Excluded locations use `negative: true`. Campaign-level presence vs interest behavior is declared on `googleads.campaign` as `locationTargeting`. Campaign, location/language, and negative are immutable; equivalent remote state, including `US` / `United States` and `en` / `English`, produces no plan diff.
- `agoraform import` for the v0.4.0 Google Ads Search campaign resource set. Existing daily budgets, Search campaigns, campaign conversion goals, Search ad groups, keywords including negatives, Responsive Search Ads, and location/language criteria can be adopted without mutation. Parent resources must be imported first so logical `$ref` values can be reconstructed. Unsupported remote campaign, ad, criterion, and budget types, including Dynamic Search Ads campaigns and keywords with criterion-level URL or tracking settings, fail with guidance instead of emitting lossy YAML. Imported configuration against unchanged Google Ads state produces a zero-change plan.
- A complete secret-free Google Ads Search campaign example under `examples/googleads-search/`, automatically validated by the test suite. The example covers conversion measurement, a dedicated daily budget, a paused Search campaign, campaign conversion-goal biddability, location and language targeting, a Search ad group, explicit match-type keywords with negatives, and a paused Responsive Search Ad.

### Supported in 0.4.0

| Area | Scope |
| --- | --- |
| Commands | `validate`, `plan`, `apply`, `import` |
| Providers | Matomo, Google Ads (`googleads`) |
| Google Ads conversion | `googleads.conversion_action`, `googleads.customer_conversion_goal` |
| Google Ads Search | `googleads.campaign_budget`, `googleads.campaign`, `googleads.campaign_conversion_goal`, `googleads.ad_group`, `googleads.keyword`, `googleads.responsive_search_ad`, `googleads.campaign_location`, `googleads.campaign_language` |
| Google Ads import | Supported conversion measurement and Search campaign resources; no mutation; parents imported first |
| Matomo | All v0.2.0 goal, Tag Manager, dependency, import, and publication behavior |
| State | Local `agoraform.state.json` beside the manifest |
| Mutations | Create and update only; new Search serving objects default to `PAUSED` |

### Limitations

- v0.4.0 Google Ads campaign support is Search only. Performance Max, Display, Video, Shopping, App, Dynamic Search Ads, and other campaign families are not implemented.
- Agoraform does not generate creative, upload image or video assets, enable spend automatically, or apply Google Ads optimization recommendations.
- Application instrumentation, gtag.js, Google Tag, Google Tag Manager, consent handling, and conversion-event emission remain outside the Google Ads provider.
- Offline, call, app, and conversion-event upload workflows are not implemented; `googleads.conversion_action` remains limited to supported website (`WEBPAGE`) actions.
- Customer and campaign conversion goals are created by Google Ads. Agoraform can adopt/read them and reconcile supported `biddable` settings but cannot create or delete them.
- Immutable identity fields such as keyword text/match type/negative, location, language, and ad-group parent fail planning instead of destroying and recreating the remote object. `apply` still does not delete remote resources and there is no `destroy` command.
- Google Ads authentication uses developer-token + single-user OAuth refresh-token credentials. Service-account, multi-user, and interactive OAuth flows are not implemented.
- Remote state, workspaces, locking, and encryption are not implemented.
- Matomo Tag Manager remains limited to one configured container and the resource types documented for v0.2.0.
- Pre-1.0: breaking CLI or manifest changes may appear in later `0.x` releases and will be documented.

### Upgrade notes

0.4.0 does not remove or rename the v0.3.0 Google Ads conversion-measurement
surface or the v0.2.0 Matomo command/resource surface. Search campaign
resources are opt-in by declaring them in the manifest. Existing
conversion-only and Matomo-only manifests continue to work with the same
runtime configuration.

See the [v0.4.0 Google Ads Search campaign example](examples/googleads-search/README.md),
[Google Ads setup guide](docs/google-ads-setup.md), and
[release guide](docs/release.md).

## [0.3.0] - 2026-08-27

Agoraform 0.3.0 adds Google Ads conversion measurement as the second provider
workflow while preserving the Matomo functionality from 0.2.0. The release can
declare, reconcile, and import supported website conversion actions and manage
customer-level website conversion-goal biddability without taking responsibility
for website tag execution or conversion-event delivery.

### Added

- Google Ads provider foundation (`googleads`) with environment-based OAuth and developer-token authentication, customer ID normalization, and a reusable REST client for query and mutate operations.
- `googleads.conversion_action` for website conversion actions such as Trial Started (read, create, update, import), with Google Ads enum/default normalization so equivalent remote state produces no plan diff.
- `googleads.customer_conversion_goal` for account-default website conversion-goal biddability (read, adopt, update, import). Google Ads creates these objects automatically; Agoraform reconciles `biddable` and never attempts unsupported create or delete operations.
- `agoraform import` for Google Ads conversion measurement. Existing website conversion actions and supported customer conversion-goal settings can be bound without mutation; resource names and `CATEGORY~ORIGIN` aliases are stored as canonical identities, computed fields and secrets are omitted, and unsupported remote types or settings fail with guidance instead of emitting lossy YAML.
- Computed Google Ads conversion metadata, including conversion ID/label when returned by tag snippets, for use by external website/tag-manager configuration without placing provider-native values in normal manifest attributes.
- A complete secret-free Google Ads conversion-measurement quickstart under `examples/googleads-conversion/`, automatically validated by the test suite.
- Google Ads account/API/OAuth setup documentation covering developer tokens, Google Auth Platform, Desktop-app OAuth credentials, refresh-token generation, customer IDs, and local runtime configuration.

### Supported in 0.3.0

| Area | Scope |
| --- | --- |
| Commands | `validate`, `plan`, `apply`, `import` |
| Providers | Matomo, Google Ads (`googleads`) |
| Google Ads resources | `googleads.conversion_action`, `googleads.customer_conversion_goal` |
| Google Ads conversion type | Supported website (`WEBPAGE`) conversion actions |
| Customer goal management | Existing provider-created `WEBSITE` goals; reconcile `biddable` only |
| Google Ads import | Supported conversion actions and customer conversion-goal settings; no mutation |
| Matomo | All v0.2.0 goal, Tag Manager, dependency, import, and publication behavior |
| State | Local `agoraform.state.json` beside the manifest |
| Mutations | Create and update only |

### Limitations

- v0.3.0 Google Ads support does not manage search campaigns, budgets, ad groups, keywords, targeting, ads, or creative.
- Offline, call, app, and conversion-event upload workflows are not implemented; `googleads.conversion_action` is limited to supported website (`WEBPAGE`) actions.
- Customer conversion goals are created by Google Ads. Agoraform can adopt/read them and reconcile supported mutable settings but cannot create or delete them.
- Application instrumentation, gtag.js, Google Tag, Google Tag Manager, consent handling, and conversion-event emission remain outside the Google Ads provider.
- Google Ads authentication in v0.3.0 uses developer-token + single-user OAuth refresh-token credentials. Service-account, multi-user, and interactive OAuth flows are not implemented.
- Remote state, workspaces, locking, and encryption are not implemented.
- `apply` does not delete remote resources and there is no `destroy` command yet.
- Matomo Tag Manager remains limited to one configured container and the resource types documented for v0.2.0.
- Pre-1.0: breaking CLI or manifest changes may appear in later `0.x` releases and will be documented.

### Upgrade notes

0.3.0 does not remove or rename the v0.2.0 Matomo command/resource surface.
Google Ads is opt-in by declaring `providers.googleads` and supplying the
required `GOOGLE_ADS_*` runtime configuration. Existing Matomo-only manifests
continue to work without Google Ads credentials.

See the [v0.3.0 Google Ads conversion example](examples/googleads-conversion/README.md),
[Google Ads setup guide](docs/google-ads-setup.md), and
[release guide](docs/release.md).

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

[Unreleased]: https://github.com/dziblo-music/agoraform/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/dziblo-music/agoraform/releases/tag/v0.5.0
[0.4.0]: https://github.com/dziblo-music/agoraform/releases/tag/v0.4.0
[0.3.0]: https://github.com/dziblo-music/agoraform/releases/tag/v0.3.0
[0.2.0]: https://github.com/dziblo-music/agoraform/releases/tag/v0.2.0
[0.1.0]: https://github.com/dziblo-music/agoraform/releases/tag/v0.1.0
