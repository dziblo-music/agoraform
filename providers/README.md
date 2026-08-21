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

Register implementations with `provider.Registry`. The core never imports
Matomo or other vendor API types.

No production providers are implemented yet.

## Test provider

`internal/provider/fake` is an in-memory `widget` provider used by unit tests.
It is not a user-facing marketing provider. It exists to prove the core
contract and to support later plan/apply tests without network calls.
