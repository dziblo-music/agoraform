# Providers

Provider implementations live under this directory.

Each provider should keep API-specific types and clients inside its own
package. The Agoraform core under `internal/` must remain provider-independent.

## Core contract

Providers implement `internal/provider.Provider`:

- `Name` / `ResourceTypes` for registry lookup
- `Validate` for provider-specific configuration checks
- `Read` for current remote state (`provider.ErrNotFound` if absent)
- `Create` / `Update` for apply-time mutations
- `Import` for binding an existing remote identity to a logical address

`agoraform plan` uses `provider.Reader` only (`Name`, `ResourceTypes`,
`Validate`, `Read`). `agoraform apply` then dispatches `Create` and
`Update` for the actions in that plan. `agoraform import` calls `Import`
to read an existing remote identity; implementations must not create,
update, or delete the remote resource. Providers may also implement
`provider.ImportIDNormalizer` so aliases such as resource names are stored
as canonical identities, and `provider.Normalizer` so defaults and omitted
values do not create false diffs. Computed/read-only fields belong on
`RemoteResource.Computed`, not in comparable attributes.

Register implementations with `provider.Registry`. The core never imports
Matomo, Google Ads, or other vendor API types.

## Local provider configuration

Provider credentials and connection settings remain environment variables,
but they do not need to be exported manually for every shell session.
Agoraform automatically loads an optional `.agoraform.env` file next to the
selected `agoraform.yaml` before providers are initialized. Existing process
environment variables take precedence over file values.

The Agoraform source repository ignores `.agoraform.env`; projects using
Agoraform should add the same entry to their own `.gitignore`. See
[Local provider configuration](../docs/local-configuration.md) for format,
precedence, security guidance, and examples.

## Matomo

[`matomo/`](matomo/) is the first production provider. It registers as
`matomo`, loads `MATOMO_URL`, `MATOMO_TOKEN_AUTH`, `MATOMO_SITE_ID`, and
`MATOMO_CONTAINER_ID` from the environment, and implements `matomo.goal`
and Tag Manager `matomo.variable`, `matomo.trigger`, and `matomo.tag`.

Tag Manager versions are not implemented yet. See
[matomo/README.md](matomo/README.md).

## Google Ads

[`googleads/`](googleads/) registers as `googleads` and loads Google Ads
credentials from `GOOGLE_ADS_DEVELOPER_TOKEN`, `GOOGLE_ADS_CLIENT_ID`,
`GOOGLE_ADS_CLIENT_SECRET`, `GOOGLE_ADS_REFRESH_TOKEN`,
`GOOGLE_ADS_CUSTOMER_ID`, and optional `GOOGLE_ADS_LOGIN_CUSTOMER_ID`. It
manages website `googleads.conversion_action` resources, customer
`googleads.customer_conversion_goal` biddability, daily Search
`googleads.campaign_budget` resources, Search `googleads.campaign`
resources, and campaign `googleads.campaign_conversion_goal` biddability,
and uses an authenticated REST client for query and mutate
operations.

See [googleads/README.md](googleads/README.md) and the
[conversion-measurement example](../examples/googleads-conversion/README.md).

## Test provider

`internal/provider/fake` is an in-memory `widget` provider used by unit tests.
It is not a user-facing marketing provider. It exists to prove the core
contract and to support plan, apply, and import tests without network calls.
