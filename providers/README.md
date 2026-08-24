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
`provider.Normalizer` so defaults and omitted values do not create false
diffs. Computed/read-only fields belong on `RemoteResource.Computed`, not
in comparable attributes.

Register implementations with `provider.Registry`. The core never imports
Matomo or other vendor API types.

## Matomo

[`matomo/`](matomo/) is the first production provider. It registers as
`matomo`, loads `MATOMO_URL`, `MATOMO_TOKEN_AUTH`, and `MATOMO_SITE_ID`
from the environment, and implements the v0.1 `matomo.goal` resource.

Tag Manager resource types are not implemented yet. See
[matomo/README.md](matomo/README.md).

## Test provider

`internal/provider/fake` is an in-memory `widget` provider used by unit tests.
It is not a user-facing marketing provider. It exists to prove the core
contract and to support plan, apply, and import tests without network calls.
