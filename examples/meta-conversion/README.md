# Meta website conversion measurement

This example binds an existing website Pixel/Dataset and manages a website
`Trial Started` Custom Conversion in Meta Ads:

- `meta.pixel.website` is an import/adopt binding for the account Pixel or
  Dataset that already exists in Events Manager;
- `meta.custom_conversion.trial_started` is a `START_TRIAL` website Custom
  Conversion whose rule matches the `StartTrial` standard event;
- the logical `$ref` keeps the remote pixel id out of YAML.

Provider-native IDs, access tokens, and credentials do not belong in the
manifest.

## What Agoraform manages here

Agoraform reconciles Meta **configuration**: which Pixel/Dataset the
conversion reads and how that Custom Conversion is defined.

It does not:

- install Pixel JavaScript on your website;
- emit `fbq('track', 'StartTrial')` or custom browser events;
- send Conversions API server events;
- manage Meta apps, Business Manager, or application SDKs.

After the pixel is bound, application-side instrumentation uses the
declared `pixelId` output as the event-source identifier. The event name in
`rule` (`StartTrial` here) is the contract those external tags must emit.

## Prerequisites

You need a Meta ad account the Marketing API can manage and an access token
with `ads_management`. Prefer a Business system-user token for automation.
Agoraform consumes the token; it does not create apps or tokens.

```bash
export META_AD_ACCOUNT_ID=act_123456789012345
read -rsp "Meta access token: " META_ACCESS_TOKEN; echo
export META_ACCESS_TOKEN
```

Do not put the token in the manifest, examples, or source control.

Copy the example into a working directory so generated state stays local:

```bash
cp examples/meta-conversion/agoraform.yaml ./agoraform.yaml
```

Import the existing pixel (required unless apply can uniquely adopt by
name), then plan and apply the Custom Conversion:

```bash
agoraform import meta.pixel.website YOUR_PIXEL_ID
agoraform plan
agoraform apply
```

An equivalent remote Custom Conversion can be imported after the pixel is
bound:

```bash
agoraform import meta.custom_conversion.trial_started YOUR_CUSTOM_CONVERSION_ID
```

A second `agoraform plan` against unchanged Meta configuration should report
no changes. `agoraform destroy` archives the Custom Conversion through the
Marketing API and leaves the Pixel/Dataset in Events Manager.

See the [Meta provider reference](../../providers/meta/README.md) for the
API-verified field contract, immutable update rules, and destroy semantics.
