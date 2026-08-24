# Matomo provider

The Matomo provider is Agoraform's first production provider. It loads
credentials from the environment, talks to Matomo through
`providers/matomo/client`, and manages the v0.1 `matomo.goal` resource.

Tag Manager variables, triggers, tags, and versions are out of scope for
v0.1.

## Configuration

Credentials are never read from manifests. Set them in the process
environment:

```text
MATOMO_URL           required   Matomo base URL, for example https://matomo.example.com
MATOMO_TOKEN_AUTH    required   API token
MATOMO_SITE_ID       required for goals   analytics site id
MATOMO_CONTAINER_ID  optional   default Tag Manager container id
```

### Precedence

1. Values passed to `matomo.New` / `matomo.NewWithHTTPClient` (tests and
   programmatic construction)
2. `MATOMO_*` environment variables (`matomo.NewFromEnv`)

There is no manifest or Git-tracked file source for tokens. Do not put
`token_auth` in `MATOMO_URL`.

## Resources

### `matomo.goal`

Declares a Matomo analytics goal. Initial create/discovery looks up a goal by
`name`. Once Agoraform manages the resource, persist Matomo's goal ID in
[local state](../../docs/state.md), not in the manifest:

```yaml
apiVersion: agoraform.io/v1alpha1
resources:
  - address: matomo.goal.trial_started
    attributes:
      name: Trial Started
      matchAttribute: event_action
      pattern: trialStarted
```

```json
{
  "version": 1,
  "resources": {
    "matomo.goal.trial_started": {
      "provider": "matomo",
      "remoteId": "12"
    }
  }
}
```

| Attribute | Required | Description |
| --- | --- | --- |
| `name` | yes | Goal name. Immutable once local state binds the resource. |
| `matchAttribute` | yes | `url`, `title`, `file`, `external_website`, `manually`, `visit_duration`, `visit_total_actions`, `visit_total_pageviews`, `event_action`, `event_category`, or `event_name`. |
| `pattern` | unless `manually` | Value to match. |
| `patternType` | no | `contains` (default), `exact`, or `regex`. Numeric match attributes default to `greater_than`. |

Lowercase `idgoal` from Matomo responses remains a computed field on the live
resource. Do not set `idGoal` in configuration; manifests that still contain
it are rejected. A stale persisted identity is an error, not a create
candidate, so Agoraform will not silently create a duplicate after a rename
or remote loss.

Other Matomo Goal API fields (`description`, `revenue`, `caseSensitive`,
`allowMultipleConversionsPerVisit`, and `useEventValueAsRevenue`) are not
managed in v0.1. They are nevertheless preserved during updates: Agoraform
re-reads the live goal immediately before mutation and sends those values back
to Matomo because `Goals.updateGoal` otherwise resets omitted parameters to
API defaults.

For `patternType: exact`, `url`, `file`, and `external_website` patterns must
start with `http://` or `https://`, matching Matomo's validation behavior.

A missing unbound remote goal plans as a create. A changed supported field
plans as an update. Goal deletion is out of scope. `agoraform apply`
executes those planned creates and updates and persists Matomo's goal ID
in local state.

## HTTP client

`providers/matomo/client` provides:

- POST request construction (`module=API`, `format=JSON`)
- token authentication in the request body (never on the query string)
- context cancellation
- a 30s default timeout
- JSON decoding and Matomo `{"result":"error"}` mapping
- secret redaction in returned errors
- Goals helpers: `GetGoals`, `AddGoal`, `UpdateGoal`, and preservation-safe updates

Two API surfaces share that client:

- `Client.Analytics()` — analytics and management methods, including goals
- `Client.TagManager()` — Tag Manager methods (`TagManager.*`)

`CheckConnection` calls the non-mutating `API.getMatomoVersion` method.

## CLI

The CLI composition root registers the Matomo provider. Manifests may
use addresses such as `matomo.goal.trial_started`. `validate` and `plan`
call `CheckConnection` once after the provider and resource type resolve,
then validate goal attributes and (for `plan`) read live goals.

## Safety

- Tests use `httptest` only. They never contact a real Matomo instance.
- Tokens must not appear in errors, logs, plan output, or fixtures.
- `agoraform plan` still cannot mutate remote resources.
