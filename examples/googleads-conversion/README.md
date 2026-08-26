# Google Ads conversion measurement

This example manages a complete website `Trial Started` conversion in Google
Ads:

- `googleads.conversion_action.trial_started` is a website `SIGNUP` conversion
  action;
- `googleads.customer_conversion_goal.signup` makes that `SIGNUP` / `WEBSITE`
  goal an account-default optimization goal;
- the logical `$ref` creates an explicit dependency so Agoraform creates the
  conversion action before reconciling goal biddability.

Provider-native IDs, conversion labels, and credentials do not belong in the
manifest.

## What Agoraform manages here

Agoraform reconciles Google Ads **configuration**: the conversion action and
whether its website category is biddable as an account-default goal.

It does not:

- emit conversion events from your application;
- install or execute gtag.js, Google Tag, Google Tag Manager, or server-side
  tags;
- manage campaigns, budgets, ad groups, keywords, ads, or creative.

Application-side event emission and tag execution remain outside the Google
Ads provider. After the conversion action exists, use the identifiers Google
Ads assigns so an external tag manager or website tag can report the same
conversion.

## Prerequisites

You need a Google Ads customer that the API can manage, a developer token,
and a previously issued OAuth 2.0 refresh token for an app with Google Ads
API access. Interactive OAuth consent is not implemented.

`GOOGLE_ADS_CUSTOMER_ID` is the 10-digit customer that owns the conversion
action. Hyphens are optional. If you authenticate through a manager account,
also set `GOOGLE_ADS_LOGIN_CUSTOMER_ID`. Customer IDs are not secrets, but
they are account-specific, so this reusable example does not embed them.

Set the non-secret IDs normally and load secret values from your usual secret
manager. For an interactive Bash session where secret-manager injection is not
available, `read -s` keeps the typed secret values out of shell command history:

```bash
export GOOGLE_ADS_CLIENT_ID=replace-with-your-oauth-client-id
export GOOGLE_ADS_CUSTOMER_ID=1234567890
# export GOOGLE_ADS_LOGIN_CUSTOMER_ID=1234567890  # manager account, if required

read -rsp "Google Ads developer token: " GOOGLE_ADS_DEVELOPER_TOKEN; echo
export GOOGLE_ADS_DEVELOPER_TOKEN
read -rsp "Google Ads OAuth client secret: " GOOGLE_ADS_CLIENT_SECRET; echo
export GOOGLE_ADS_CLIENT_SECRET
read -rsp "Google Ads refresh token: " GOOGLE_ADS_REFRESH_TOKEN; echo
export GOOGLE_ADS_REFRESH_TOKEN
```

Do not replace the secret prompts above with literal secret values in commands.
Keep tokens and client secrets out of shell history, logs, source control, and
the manifest. On automated systems, inject them from your normal secret manager
instead of storing them in scripts.

The empty `providers.googleads: {}` block is valid. There are no non-secret
Google Ads YAML fields yet; putting OAuth secrets or the developer token in
the manifest is rejected.

Copy the example into a working directory so its generated
`agoraform.state.json` remains local:

```bash
cp examples/googleads-conversion/agoraform.yaml ./agoraform.yaml
```

## Conversion identifiers for website tags

After the conversion action exists, Google Ads assigns a conversion ID and
conversion label. Website tags typically report the event as
`AW-{conversionId}/{conversionLabel}`. Those values are computed runtime
metadata, not manifest attributes, and they must not be committed as
production configuration.

Find them in Google Ads under **Goals -> Conversions -> Trial Started -> Tag
setup**, or from the conversion action's tag snippets. Agoraform stores the
numeric conversion-action ID in local state as `remoteId` and can surface
`conversionId` / `conversionLabel` as computed fields when Google Ads returns
them in those snippets.

Your tag manager or application must still fire the conversion once per
successful trial start. The following gtag shape is illustrative only; replace
the placeholders with the IDs from your own account, and follow the consent
and privacy rules for your site:

```javascript
gtag("event", "conversion", {
  send_to: "AW-CONVERSION_ID/CONVERSION_LABEL"
});
```

Google Tag Manager, Google Tag, and server-side tagging use the same
conversion ID and label. Agoraform does not configure those tools.

## Apply

Run the full lifecycle from the directory containing the copied manifest:

```bash
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Review the first plan before applying it. A greenfield account typically
shows a create for the conversion action. The customer conversion goal is
created by Google Ads, not by Agoraform, so the same plan may show an adopt
(or an update of `biddable`) for `googleads.customer_conversion_goal.signup`:

```text
+ googleads.conversion_action.trial_started
* googleads.customer_conversion_goal.signup (adopt)
```

`apply` creates the website conversion action first, then binds the
provider-created `SIGNUP~WEBSITE` goal and reconciles `biddable`. Agoraform
never creates or deletes customer conversion goals. If the expected goal is
still missing after the conversion action exists, the provider reports that
Google Ads creates the object automatically and that Agoraform cannot create
it.

The final plan must report `No changes.` when the manifest and remote
configuration are unchanged.

## Verify in Google Ads

In the Google Ads customer from `GOOGLE_ADS_CUSTOMER_ID`:

1. Open **Goals -> Conversions** and confirm **Trial Started** exists as a
   website conversion with category **Sign-up**.
2. Confirm counting is one conversion per click, the action is primary for
   its goal, and the default value is `0` unless you intentionally changed
   those fields.
3. Open **Goals -> Conversion goals** and confirm the **Sign-up** website
   goal is an account-default optimization goal (`biddable: true`).
4. Open the conversion action's tag setup and record the conversion ID and
   conversion label for your external tag configuration.

Then fire a test conversion from your website or tag manager and confirm that
Google Ads records it against **Trial Started**. Diagnosing tag execution,
consent mode, and event delivery is outside Agoraform.

## Adopt existing resources

The same manifest is import-compatible. If an equivalent website conversion
action and customer conversion goal already exist, import them using the
identities shown by Google Ads. Import the conversion action first:

```bash
agoraform import googleads.conversion_action.trial_started CONVERSION_ACTION_ID
agoraform import googleads.customer_conversion_goal.signup SIGNUP~WEBSITE
```

`CONVERSION_ACTION_ID` is the numeric conversion action ID, or the resource
name `customers/{customerId}/conversionActions/{id}`. The goal identity is
`SIGNUP~WEBSITE`, or the resource name
`customers/{customerId}/customerConversionGoals/SIGNUP~WEBSITE`.

Each command prints canonical YAML for review and records the remote identity
in `agoraform.state.json`; it does not edit the manifest or mutate Google Ads.
Computed fields such as resource names, tag snippets, `conversionId`, and
`conversionLabel` are omitted from that YAML. Import also does not emit the
optional `conversionAction` `$ref`; add it after import when the matching
conversion action is also managed, as this example does.

Compare the printed attributes with this example, adjust the manifest if
needed, then run `agoraform plan`. An equivalent imported configuration
against unchanged Google Ads state should report `No changes.` See
[Import](../../docs/import.md) for the complete adoption workflow.
