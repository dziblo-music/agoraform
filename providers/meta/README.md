# Meta Ads provider

The `meta` provider is the shared Meta Marketing API foundation for Agoraform
v0.6.0. It is registered with the same provider-neutral lifecycle used by the
existing providers. Campaign, ad-set, creative, ad, targeting, and conversion
resource types are intentionally implemented by separate v0.6.0 issues.

## Runtime configuration

Supply both required values through environment variables or a local
`.agoraform.env` file:

```dotenv
META_ACCESS_TOKEN=replace-with-access-token
META_AD_ACCOUNT_ID=act_123456789012345
```

`META_AD_ACCOUNT_ID` accepts either `act_123456789012345` or
`123456789012345`; Agoraform normalizes both to the `act_` form. Other forms
are rejected before an API call.

For unattended automation, prefer a Meta Business system-user access token
with access to the selected ad account and the `ads_management` permission.
Agoraform remains compatible with other Meta token types that Meta supports
for the same API operations. Agoraform consumes an existing token; it does
not create Meta apps, Business Manager accounts, system users, or tokens.

The access token must not be placed in `agoraform.yaml`, command arguments,
state, examples, or source control. The ad-account selection is runtime
configuration as well, which makes one manifest reusable across environments.

Declare the provider with an empty configuration block:

```yaml
apiVersion: agoraform.io/v1alpha1
providers:
  meta: {}
resources: []
```

## Connection validation

`agoraform validate` and `agoraform plan` perform read-only checks when the
manifest declares `meta` or contains a Meta resource. Validation:

1. checks that the token reports a granted `ads_management` permission;
2. reads the configured ad account and confirms its identity.

Authentication failures, insufficient permissions, and inaccessible accounts
are reported separately. Both checks use versioned GET requests and do not
create, update, or delete any Meta object.

## API version policy

Agoraform v0.6.0 pins all Meta Graph and Marketing API requests to **v26.0**.
It never calls an unversioned endpoint or an implicit `latest` API.

The version is centralized in `providers/meta/client/version.go`. Upgrading it
requires a reviewed code change, review of Meta's version changelog and
migration guidance, and successful provider client/resource tests against the
new version before release. Patch releases do not silently switch API
versions.

## Client behavior

The reusable client provides versioned GET, form-encoded POST and DELETE,
cursor pagination, per-request timeouts, context cancellation, bounded JSON
responses, and Meta error mapping. API code/subcode, transient classification,
request ID, and trace ID are retained when available.

The client classifies transient and rate-limit failures but does not
automatically retry requests. Resource implementations must decide whether an
operation is safe to retry, particularly for mutations. Diagnostics redact
the configured token and common credential-bearing headers/query parameters.

Automated tests use local HTTP servers only and never call Meta production
services.
