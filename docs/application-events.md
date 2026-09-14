# Application instrumentation contracts

Agoraform manages provider-side marketing infrastructure. Application code remains responsible for emitting browser and server events. The `applicationEvents` block connects those two sides with a provider-neutral, machine-readable contract.

A developer should be able to answer: **What does my application need to emit for this campaign to work?**

Agoraform manages the contract, provider resources, logical references, and non-secret provider identifiers. It does not generate or modify application source, install SDKs, execute `fbq()`, `gtag()`, or `window._mtm.push()`, send Conversions API requests, handle customer PII, or deploy application code.

## Manifest schema

```yaml
applicationEvents:
  <event-name>:
    matomo:
      trigger:
        $ref: matomo.trigger.<name>
      fields:            # optional data-layer fields
        - <field-name>
    googleAds:
      conversion:
        $ref: googleads.conversion_action.<name>
    meta:
      eventSource:
        $ref: meta.pixel.<name>
      eventName: <Meta-event-name>
      delivery: browser | server | both   # default: browser
```

Each key under `applicationEvents` is a logical application event. At least one provider binding is required.

## Matomo binding

The Matomo binding references a managed `matomo.trigger` resource whose `type` must be `customEvent`. The Data Layer event name is **derived from that trigger's `event` attribute** rather than copied into `applicationEvents`. This prevents the application contract from drifting away from the managed Tag Manager trigger.

```yaml
resources:
  - address: matomo.trigger.trial_started
    attributes:
      type: customEvent
      event: trialStarted

applicationEvents:
  trial_started:
    matomo:
      trigger:
        $ref: matomo.trigger.trial_started
      fields:
        - userId
```

The application emits:

```javascript
window._mtm = window._mtm || [];
window._mtm.push({
  event: "trialStarted",
  userId: currentUserId,
});
```

If the managed trigger changes from `trialStarted` to another event name, validation and integration output follow the referenced trigger automatically.

## Google Ads binding

The Google Ads binding references the managed website conversion action:

```yaml
googleAds:
  conversion:
    $ref: googleads.conversion_action.trial_started
```

After the resource has an identity, `agoraform integrations` reads its live non-secret conversion ID and conversion label. Application instrumentation can then use them with `gtag.js`:

```javascript
gtag('event', 'conversion', {
  'send_to': 'AW-123456789/AbCdEfGh',
});
```

When the resource has not yet been applied, `agoraform integrations` prints `(not yet applied)` and does **not** require Google Ads credentials merely to inspect the declared contract.

## Meta binding

The Meta binding references the managed Pixel/Dataset event source and declares the provider event name and delivery mechanism:

```yaml
meta:
  eventSource:
    $ref: meta.pixel.main
  eventName: StartTrial
  delivery: both
```

Browser delivery uses the Pixel ID shown by `agoraform integrations`:

```javascript
fbq('init', '987654321');
fbq('track', 'StartTrial');
```

Server delivery sends the same logical event through the Meta Conversions API. Conceptually:

```javascript
{
  "data": [{
    "event_name": "StartTrial",
    "event_time": 1700000000,
    "event_id": "trial-abc123",
    "action_source": "website"
  }]
}
```

For `delivery: both`, use the same stable `event_id` in browser and server payloads so Meta can deduplicate the conversion.

When the Pixel/Dataset resource has not yet been applied, `agoraform integrations` prints `(not yet applied)` without requiring Meta credentials.

## Complete example

```yaml
apiVersion: agoraform.io/v1alpha1
providers:
  matomo:
    publish: true
    environment: live
  googleads: {}
  meta: {}

resources:
  - address: matomo.container.main
    attributes:
      name: Main Website
      context: web

  - address: matomo.trigger.trial_started
    attributes:
      container:
        $ref: matomo.container.main
      type: customEvent
      event: trialStarted
      name: Trial started

  - address: googleads.conversion_action.trial_started
    attributes:
      name: Trial Started
      category: SIGNUP
      value: 0
      count: ONE
      primaryForGoal: true

  - address: meta.pixel.main
    attributes:
      name: Website

applicationEvents:
  trial_started:
    matomo:
      trigger:
        $ref: matomo.trigger.trial_started
      fields:
        - userId
    googleAds:
      conversion:
        $ref: googleads.conversion_action.trial_started
    meta:
      eventSource:
        $ref: meta.pixel.main
      eventName: StartTrial
      delivery: both
```

## Inspecting the contract

```text
agoraform integrations
```

Example output:

```text
Application event: trial_started

Matomo
  Data Layer event: trialStarted
  Fields: userId

Google Ads
  Conversion action: googleads.conversion_action.trial_started
  Conversion ID: AW-123456789
  Conversion label: AbCdEfGh

Meta
  Event source: meta.pixel.main
  Pixel ID: 987654321
  Event: StartTrial
  Delivery: both
```

`integrations` validates manifest-level references offline first. It contacts a provider only when local state contains an identity for a referenced resource and live provider output must be resolved.

## Plan and apply lifecycle

Application contracts are not remote provider resources, but changes to them can change what external application code must emit. Agoraform therefore records a **non-secret fingerprint** of each successfully applied application contract in local state.

`agoraform plan` reports contract changes separately from provider-resource changes:

```text
Application integration changes:
  ~ applicationEvents.trial_started
```

The section can show `+`, `~`, or `-` for added, changed, or removed contracts. A contract-only change makes `plan` return the normal "changes present" exit code (`2`).

`agoraform apply` performs provider mutations first. Only after they succeed does it record the new application-contract fingerprints in local state. The fingerprints contain no credentials, provider secrets, or PII.

| Command | Behaviour |
| --- | --- |
| `validate` | Validates application-event syntax, references, resource types, and the Matomo trigger relationship without contacting providers. |
| `plan` | Shows provider-resource changes plus externally consumed application-contract changes. |
| `apply` | Applies provider resources, then records the successfully applied contract fingerprints locally. Never edits application code. |
| `integrations` | Shows the current contract and resolves live non-secret IDs only for resources that already have state identities. |
| `import` | Imports provider identities; those identities can immediately be used by `integrations`. |
| `destroy` | Removes only provider resources Agoraform owns; never edits application code. |

## Security

- `applicationEvents` must never contain access tokens, API secrets, customer PII, or other sensitive runtime values.
- Local application-event state stores only SHA-256 fingerprints of the declared external contract.
- `agoraform integrations` whitelists the specific non-secret provider outputs it needs instead of dumping provider state.
