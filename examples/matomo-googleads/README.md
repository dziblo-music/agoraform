# Matomo + Google Ads lifecycle

This example is the v0.5.0 end-to-end acceptance workflow: Agoraform creates a
Matomo Tag Manager container, conversion measurement in Google Ads, and a
Matomo Google Ads conversion tag that consumes the Google Ads conversion
outputs through a cross-provider reference.

The primary manifest manages:

- `matomo.container.main` as a greenfield Tag Manager container;
- `matomo.variable.config` as the Matomo Configuration variable;
- `matomo.variable.user_id` reading `userId` from the data layer;
- `matomo.trigger.trial_started` listening for the `trialStarted` custom event;
- `googleads.conversion_action.trial_started` as a website `SIGNUP` conversion;
- `googleads.customer_conversion_goal.signup` so `SIGNUP` / `WEBSITE` is
  biddable as an account-default optimization goal;
- `matomo.tag.google_ads_trial_started` as a Google Ads conversion tag that
  selects `conversionId` and `conversionLabel` from the managed conversion
  action and uses the data-layer variable as `conversionTransactionId`;
- `providers.matomo.publish: true` so `apply` publishes the converged draft
  to `live`.

Every Tag Manager child names the managed container with
`container: { $ref: matomo.container.main }`. Provider-native IDs,
credentials, and customer IDs do not belong in the manifest.

An [external-container variant](#external-container-variant) keeps an existing
container selected by `MATOMO_CONTAINER_ID`. Agoraform never deletes that
container.

## What Agoraform manages here

Agoraform reconciles **configuration**: the Matomo container and Tag Manager
resources, the Google Ads conversion action, customer conversion-goal
biddability, and declarative container publication.

It does not:

- emit conversion events from your application;
- install or manage Google Tag (`gtag.js`) in the Matomo container;
- generate application code;
- enable Google Ads spend or manage campaigns.

The Google Ads conversion tag depends on a Google Tag in the same container.
Add that tag in Matomo Tag Manager if it is not already present. Agoraform
does not own it.

## Prerequisites

You need a non-production Matomo instance with Tag Manager enabled, a website
the API can manage, and a Google Ads customer the API can manage. Use a
Google Ads test account when practical.

The primary manifest **omits** `MATOMO_CONTAINER_ID`. Mixing that environment
variable with `matomo.container.main` is rejected before mutation.

Replace `matomoUrl` and `siteId` on `matomo.variable.config` with the Matomo
URL and site ID for your instance. Those values are tracking configuration,
not credentials; this reusable example uses placeholders.

Set Matomo and Google Ads runtime configuration. Load secret values from your
usual secret manager. For an interactive Bash session where secret-manager
injection is not available, `read -s` keeps typed secrets out of shell
command history:

```bash
export MATOMO_URL=https://matomo.example.com
export MATOMO_SITE_ID=1
export GOOGLE_ADS_CLIENT_ID=replace-with-your-oauth-client-id
export GOOGLE_ADS_CUSTOMER_ID=1234567890
# export GOOGLE_ADS_LOGIN_CUSTOMER_ID=1234567890  # manager account, if required

read -rsp "Matomo API token: " MATOMO_TOKEN_AUTH; echo
export MATOMO_TOKEN_AUTH
read -rsp "Google Ads developer token: " GOOGLE_ADS_DEVELOPER_TOKEN; echo
export GOOGLE_ADS_DEVELOPER_TOKEN
read -rsp "Google Ads OAuth client secret: " GOOGLE_ADS_CLIENT_SECRET; echo
export GOOGLE_ADS_CLIENT_SECRET
read -rsp "Google Ads refresh token: " GOOGLE_ADS_REFRESH_TOKEN; echo
export GOOGLE_ADS_REFRESH_TOKEN
```

Do not replace the secret prompts with literal secret values in commands.
Keep tokens and client secrets out of shell history, logs, source control,
and the manifest.

Copy the example into a working directory so its generated
`agoraform.state.json` remains local:

```bash
cp examples/matomo-googleads/agoraform.yaml ./agoraform.yaml
```

## Application event contract

After the Matomo Tag Manager container snippet has initialized
`window._mtm`, the application emits this object when a trial is successfully
created:

```javascript
window._mtm.push({
  event: "trialStarted",
  userId: "..."
})
```

Both names are case-sensitive and must match the manifest. Emit the event
once per successful conversion. Use a stable, non-secret, non-email
identifier for `userId`, and apply the privacy and consent rules appropriate
to your site.

Agoraform does not generate or own that application code. Matomo Tag Manager
fires the conversion tag; Google Ads owns the conversion action.

## Apply

Run the full lifecycle from the directory containing the copied manifest:

```text
validate -> plan -> apply -> plan
```

```bash
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Review the first plan before applying it. A greenfield account typically
shows creates for the container, Tag Manager children, and conversion
action. The customer conversion goal is created by Google Ads, not by
Agoraform, so the same plan may show an adopt (or an update of `biddable`)
for `googleads.customer_conversion_goal.signup`. Publication is conditional
until apply compares the converged draft with live:

```text
+ googleads.conversion_action.trial_started
* googleads.customer_conversion_goal.signup (adopt)
+ matomo.container.main
+ matomo.tag.google_ads_trial_started
+ matomo.trigger.trial_started
+ matomo.variable.config
+ matomo.variable.user_id
> matomo.container.main: publish -> live [conditional]
```

Plan **display** is address order. **Apply** follows the `$ref` graph,
prerequisites first. The conversion action is created before the Matomo
conversion tag so `conversionId` and `conversionLabel` can be resolved:

```text
googleads.conversion_action.trial_started
googleads.customer_conversion_goal.signup
matomo.container.main
matomo.trigger.trial_started
matomo.variable.config
matomo.variable.user_id
matomo.tag.google_ads_trial_started
```

Then, if the converged draft still differs from live, apply creates a
container version and publishes it to `live`. Agoraform never creates or
deletes customer conversion goals.

The final plan must report `No changes.` when the manifest and remote
configuration are unchanged. Repeated unchanged apply must not create
duplicate container versions.

## Destroy

`agoraform destroy` tears down the managed graph in reverse `$ref` order.
Interactive terminals require typing `yes`. Non-interactive sessions need
`--auto-approve`.

```bash
agoraform destroy
```

Because this plan deletes the Agoraform-managed container, destroy does not
publish an intermediate version. It deletes Tag Manager children, then the
container. Google Ads conversion actions are a provider-native `remove`
(`status=REMOVED`). The customer conversion goal is provider-owned: it is
listed, not mutated, and remains in state.

Expected destroy plan:

```text
Agoraform will destroy the following resources:

- matomo.tag.google_ads_trial_started
- matomo.variable.user_id
- matomo.variable.config
- matomo.trigger.trial_started
- matomo.container.main
- googleads.conversion_action.trial_started (remove)

The following resources cannot be destroyed and will remain in state:

- googleads.customer_conversion_goal.signup (provider-owned)

Destroy: 6 to destroy, 1 unsupported.
```

`destroy` without a parenthetical label is Matomo deletion. `(remove)` is
the Google Ads mutate remove. `(provider-owned)` is a preserved binding,
not a teardown.

Supported teardown still runs. The command then exits non-zero while the
provider-owned goal remains:

```text
Destroy complete! 5 destroyed, 1 removed, 1 unsupported remaining.
destroy left 1 resource in state because destroy is unsupported: googleads.customer_conversion_goal.signup
```

Closing the Google Ads customer and removing the website snippet are out of
scope. See [Destroy](../../docs/destroy.md).

## Verify

In Matomo, for the website from `MATOMO_SITE_ID`:

1. Open Tag Manager and confirm **Main Website** exists as a web container
   created by Agoraform (not a container you selected with
   `MATOMO_CONTAINER_ID`).
2. Confirm the draft contains **Matomo Configuration**, **Trial user ID**,
   **Trial started**, and **Google Ads trial started**.
3. Open the Google Ads conversion tag and confirm it uses the conversion ID
   and conversion label assigned to **Trial Started** in Google Ads, and that
   it fires from the **Trial started** trigger.
4. Open Versions/Environments and confirm a version was published to **live**
   only when the draft differed from that environment.
5. Run `agoraform apply` again without changing the manifest and confirm no
   additional container version is created.

In the Google Ads customer from `GOOGLE_ADS_CUSTOMER_ID`:

1. Open **Goals -> Conversions** and confirm **Trial Started** exists as a
   website conversion with category **Sign-up**.
2. Confirm counting is one conversion per click, the action is primary for
   its goal, and the default value is `0` unless you intentionally changed
   those fields.
3. Open **Goals -> Conversion goals** and confirm the **Sign-up** website
   goal is an account-default optimization goal (`biddable: true`).

Install the container snippet on a non-production page, add Google Tag in
the Matomo container if it is missing, then use Tag Manager preview/debug
to push the `trialStarted` event. Confirm the conversion tag fires and that
Google Ads records the conversion against **Trial Started**. Diagnosing tag
execution, consent mode, and event delivery is outside Agoraform.

## Adopt existing resources

The same manifest is import-compatible. If equivalent resources already
exist, import dependencies first so Agoraform can reconstruct logical `$ref`
values and unique `{ $ref, output }` selectors. Import the Google Ads
conversion action before the Matomo conversion tag:

```bash
agoraform import googleads.conversion_action.trial_started CONVERSION_ACTION_ID
agoraform import googleads.customer_conversion_goal.signup SIGNUP~WEBSITE
agoraform import matomo.container.main CONTAINER_ID
agoraform import matomo.variable.config VARIABLE_ID
agoraform import matomo.variable.user_id VARIABLE_ID
agoraform import matomo.trigger.trial_started TRIGGER_ID
agoraform import matomo.tag.google_ads_trial_started TAG_ID
```

`CONVERSION_ACTION_ID` is the numeric conversion action ID, or the resource
name `customers/{customerId}/conversionActions/{id}`. The goal identity is
`SIGNUP~WEBSITE`. Matomo IDs are the provider-native container id and the
numeric Tag Manager variable, trigger, and tag ids.

Each command prints canonical YAML for review and records the remote identity
in `agoraform.state.json`; it does not edit the manifest or mutate Matomo or
Google Ads. Import reconstructs `conversionId` and `conversionLabel` as
`{ $ref, output }` only when the bound conversion action uniquely matches.
Ambiguous and absent matches emit literals.

Compare the printed attributes with this example, adjust the manifest if
needed, then run `agoraform plan`. An equivalent imported configuration
against unchanged remote state must report `No changes.` See
[Import](../../docs/import.md).

## External-container variant

To manage Tag Manager children inside an existing container, copy
[`external/agoraform.yaml`](external/agoraform.yaml) instead and set
`MATOMO_CONTAINER_ID`. Omit `matomo.container` and every `container: { $ref }`
attribute. Destroy never deletes that external container.

```bash
export MATOMO_CONTAINER_ID=replace-with-your-container-id
cp examples/matomo-googleads/external/agoraform.yaml ./agoraform.yaml
```

Apply order is the same `$ref` graph without the managed container:

```text
googleads.conversion_action.trial_started
googleads.customer_conversion_goal.signup
matomo.trigger.trial_started
matomo.variable.config
matomo.variable.user_id
matomo.tag.google_ads_trial_started
```

Publication is addressed as `matomo.container.external`. Destroy removes the
Tag Manager children and the conversion action, then publishes the remaining
container when `publish: true` and the draft still differs from live. The
external container itself is protected from deletion.

Expected destroy plan:

```text
Agoraform will destroy the following resources:

- matomo.tag.google_ads_trial_started
- matomo.variable.user_id
- matomo.variable.config
- matomo.trigger.trial_started
- googleads.conversion_action.trial_started (remove)

> matomo.container.external: publish -> live [conditional]

The following resources cannot be destroyed and will remain in state:

- googleads.customer_conversion_goal.signup (provider-owned)

Destroy: 5 to destroy, 1 unsupported, 1 provider action.
```

The customer conversion goal still causes a non-zero destroy result after
supported teardown. Import the children without a container resource, or
import `matomo.container.main` and switch to the primary manifest if you
want Agoraform to own the container.
