# Application instrumentation contracts

Agoraform manages the provider-side marketing infrastructure: conversion
actions, pixels, Tag Manager containers, goals, and publication. It does not
install browser code, generate application source files, or deliver events.

The `applicationEvents` block in a manifest declares the provider-neutral
instrumentation contract between the managed marketing infrastructure and the
application that must emit events.

A developer reading the contract can answer:

> What does my application need to emit for this campaign to work?

Agoraform **does manage**:

- The declarative `applicationEvents` contract in the manifest.
- Provider-side conversion and analytics configuration.
- Logical references between application events and managed provider resources.
- Non-secret provider identifiers needed for external instrumentation.

Agoraform **does not manage**:

- Application source-code generation or modification.
- SDK or package installation.
- Browser Pixel installation.
- `fbq()`, `window._mtm.push()`, or `gtag()` execution.
- Meta Conversions API HTTP transport.
- Server functions, endpoints, or workers used to send events.
- Storage or transmission of customer PII.
- Deployment of application changes.

---

## Manifest schema

```yaml
applicationEvents:
  <event-name>:
    matomo:
      event: <data-layer-event-name>
      fields:            # optional list of declared data-layer fields
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

Each top-level key under `applicationEvents` is the **logical application
event name**. At least one provider binding (`matomo`, `googleAds`, `meta`)
is required.

---

## Matomo binding

The Matomo binding describes the Data Layer event contract for a Matomo Tag
Manager `customEvent` trigger.

| Field | Required | Description |
| --- | --- | --- |
| `event` | yes | Data Layer event name consumed by the managed `customEvent` trigger. |
| `fields` | no | Data-layer field names declared as part of the tracking contract. Informational only; Agoraform does not validate field values. |

The application must push the declared event to the data layer:

```javascript
window._mtm = window._mtm || [];
window._mtm.push({ event: "trialStarted" });
```

If the contract declares fields, they should be included in the push:

```javascript
window._mtm.push({
  event: "trialStarted",
  userId: currentUserId,
});
```

---

## Google Ads binding

The Google Ads binding references the managed conversion action whose
conversion identifiers are needed for external instrumentation (for example
`gtag.js`).

| Field | Required | Description |
| --- | --- | --- |
| `conversion` | yes | `$ref` to the managed `googleads.conversion_action` resource. |

After `agoraform apply`, `agoraform integrations` displays the Conversion ID
and Conversion label required by `gtag.js`:

```javascript
gtag('event', 'conversion', {
  'send_to': 'AW-123456789/AbCdEfGh',
});
```

Where `AW-123456789` is the Conversion ID and `AbCdEfGh` is the Conversion
label from the managed conversion action.

---

## Meta binding

The Meta binding references the managed Pixel/Dataset event source and declares
the standard or custom event name and the delivery mechanism.

| Field | Required | Description |
| --- | --- | --- |
| `eventSource` | yes | `$ref` to the managed `meta.pixel` resource. |
| `eventName` | yes | Standard or custom Meta event name (for example `StartTrial` or `Purchase`). |
| `delivery` | no | `browser`, `server`, or `both`. Default: `browser`. |

### Browser Pixel delivery

When `delivery` is `browser` or `both`, the application initialises the Meta
Pixel with the Pixel ID from `agoraform integrations` and emits the event:

```javascript
fbq('init', '987654321');   // Pixel ID from agoraform integrations
fbq('track', 'StartTrial');
```

### Server (Conversions API) delivery

When `delivery` is `server` or `both`, the application sends the event to the
Meta Conversions API. A conceptual server event:

```javascript
// POST https://graph.facebook.com/v19.0/{pixel-id}/events
{
  "data": [{
    "event_name": "StartTrial",
    "event_time": 1700000000,
    "event_id": "trial-abc123",     // deduplication ID
    "action_source": "website"
  }]
}
```

When `delivery` is `both`, the same logical conversion event is sent through
both the browser Pixel **and** the Conversions API. Providers require a stable
`event_id` that matches between the browser and server payloads to deduplicate
the event. Choose a value derived from the user session or server-generated
event ID.

---

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
      event: trialStarted
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

---

## Inspecting the integration contract

```
agoraform integrations
```

The `integrations` command reads the manifest and live provider state to
display the non-secret identifiers that application instrumentation must use.

Example output after `agoraform apply`:

```
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

For resources that have not yet been applied, the command shows
`(not yet applied)` instead of provider identifiers.

---

## Lifecycle

| Command | Behaviour |
| --- | --- |
| `validate` | Validates event contracts; reports invalid references, wrong resource types, and missing required fields without contacting providers. |
| `plan` | Makes changes to provider resources visible. Event contracts themselves are not provider resources. |
| `apply` | Configures only the managed provider resources. Never edits application code. |
| `integrations` | Reads the manifest and provider state to display the non-secret identifiers application instrumentation must use. |
| `import` | Preserves existing provider relationships. |
| `destroy` | Removes only provider resources Agoraform owns; never edits application code. |

---

## Security

- The `applicationEvents` block never contains secrets.
- Access tokens, API secrets, user PII, and other sensitive values must not
  appear in manifests, `applicationEvents` declarations, or integration output.
- `agoraform integrations` displays only the provider identifiers (numeric IDs,
  conversion labels) that are intrinsically public — the same values a
  developer would find in their provider dashboard.
