# Matomo provider

The Matomo provider is Agoraform's first production provider. This package
is the foundation: configuration, authentication, a reusable HTTP client,
and registration with the core provider contract.

Resource types such as goals and Tag Manager objects are implemented in
follow-up issues. They should call through `providers/matomo/client`
instead of issuing raw HTTP requests.

## Configuration

Credentials are never read from manifests. Set them in the process
environment:

```text
MATOMO_URL           required   Matomo base URL, for example https://matomo.example.com
MATOMO_TOKEN_AUTH    required   API token
MATOMO_SITE_ID       optional   default analytics site id
MATOMO_CONTAINER_ID  optional   default Tag Manager container id
```

### Precedence

1. Values passed to `matomo.New` / `matomo.NewWithHTTPClient` (tests and
   programmatic construction)
2. `MATOMO_*` environment variables (`matomo.NewFromEnv`)

There is no manifest or Git-tracked file source for tokens. Do not put
`token_auth` in `MATOMO_URL`.

## HTTP client

`providers/matomo/client` provides:

- POST request construction (`module=API`, `format=JSON`)
- token authentication in the request body (never on the query string)
- context cancellation
- a 30s default timeout
- JSON decoding and Matomo `{"result":"error"}` mapping
- secret redaction in returned errors

Two API surfaces share that client:

- `Client.Analytics()` — analytics and management methods
- `Client.TagManager()` — Tag Manager methods (`TagManager.*`)

`CheckConnection` calls the non-mutating `API.getMatomoVersion` method.

## CLI

The CLI composition root registers the Matomo provider. Manifests may
use the `matomo` address prefix. Until a resource type is implemented,
`agoraform validate` and `agoraform plan` report an unknown resource
type for addresses such as `matomo.goal.trial_started`.

When a supported Matomo resource type is resolved, `validate` and `plan`
call `CheckConnection` once before resource-specific validation.

## Safety

- Tests use `httptest` only. They never contact a real Matomo instance.
- Tokens must not appear in errors, logs, plan output, or fixtures.
- `agoraform plan` still cannot mutate remote resources.
