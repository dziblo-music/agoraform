# Google Ads Search campaign

This example manages a complete paused SaaS paid-acquisition Search campaign
in Google Ads:

- `googleads.conversion_action.trial_started` is a website `SIGNUP`
  conversion action;
- `googleads.customer_conversion_goal.signup` makes that `SIGNUP` /
  `WEBSITE` goal an account-default optimization goal;
- `googleads.campaign_budget.search_acquisition` is a dedicated daily
  Search budget;
- `googleads.campaign.search_acquisition` is a paused Search campaign that
  maximizes conversions;
- `googleads.campaign_conversion_goal.trial_signup` keeps `SIGNUP` /
  `WEBSITE` biddable on that campaign;
- `googleads.campaign_location.united_states` and
  `googleads.campaign_language.english` set presence-based United States
  and English targeting;
- `googleads.ad_group.trial` is a paused Search standard ad group;
- positive keywords declare `EXACT`, `PHRASE`, and `BROAD` match types;
- negative keywords exclude common non-buyer queries such as jobs, careers,
  and login;
- `googleads.responsive_search_ad.trial` is paused placeholder Search ad
  copy on `https://example.com/`.

Logical `$ref` values create the dependency graph. Agoraform therefore
creates the conversion action and budget before the campaign, the campaign
before targeting, conversion-goal reconciliation, and the ad group, and the
ad group before keywords and the Responsive Search Ad.

Provider-native IDs, customer IDs, and credentials do not belong in the
manifest. Replace the placeholder landing page, headlines, descriptions,
keywords, budget, and geo targeting with values for your own product before
enabling spend.

## What Agoraform manages here

Agoraform reconciles Google Ads **configuration**: conversion measurement,
budget, Search campaign, campaign conversion-goal biddability, targeting,
ad group, keywords, and Responsive Search Ad copy.

It does not:

- generate headlines or descriptions;
- install website tags or emit conversion events;
- enable campaigns, ad groups, keywords, or ads unless you change `status`;
- optimize bids, budgets, or keywords after apply;
- manage Display, video, Performance Max, or Dynamic Search Ads campaigns.

Application-side event emission and tag execution remain outside the Google
Ads provider. After the conversion action exists, use the identifiers Google
Ads assigns so an external tag manager or website tag can report the same
conversion. See the [conversion-measurement example](../googleads-conversion/README.md)
for the tag-identifier workflow.

## Prerequisites

You need a Google Ads customer that the API can manage, a developer token,
and a previously issued OAuth 2.0 refresh token for an app with Google Ads
API access. Interactive OAuth consent is not implemented.

`GOOGLE_ADS_CUSTOMER_ID` is the 10-digit customer that owns the campaign.
Hyphens are optional. If you authenticate through a manager account, also
set `GOOGLE_ADS_LOGIN_CUSTOMER_ID`. Customer IDs are not secrets, but they
are account-specific, so this reusable example does not embed them.

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
cp examples/googleads-search/agoraform.yaml ./agoraform.yaml
```

If you already applied the [conversion-measurement example](../googleads-conversion/README.md)
in the same customer, import the existing conversion action and customer
conversion goal before applying this campaign instead of creating a second
`Trial Started` action. See [Adopt existing resources](#adopt-existing-resources).

## Apply

Run the full lifecycle from the directory containing the copied manifest:

```bash
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

Review the first plan before applying it. A greenfield account typically
shows creates for the conversion action, dedicated budget, paused Search
campaign, targeting, ad group, keywords, and Responsive Search Ad. Customer
and campaign conversion goals are created by Google Ads, not by Agoraform,
so the same plan may show an adopt (or an update of `biddable`) for those
resources:

```text
+ googleads.conversion_action.trial_started
* googleads.customer_conversion_goal.signup (adopt)
+ googleads.campaign_budget.search_acquisition
+ googleads.campaign.search_acquisition
* googleads.campaign_conversion_goal.trial_signup (adopt)
+ googleads.campaign_location.united_states
+ googleads.campaign_language.english
+ googleads.ad_group.trial
+ googleads.keyword.saas_software_exact
+ googleads.keyword.start_free_trial_phrase
+ googleads.keyword.online_software_broad
+ googleads.keyword.jobs_neg
+ googleads.keyword.careers_neg
+ googleads.keyword.login_neg
+ googleads.responsive_search_ad.trial
```

`apply` follows the `$ref` graph: conversion action and budget first, then
the campaign, then targeting, conversion-goal reconciliation, and the ad
group, then keywords and the ad. Agoraform never creates or deletes
customer or campaign conversion goals. If an expected goal is still missing
after the conversion action and campaign exist, the provider reports that
Google Ads creates the object automatically and that Agoraform cannot create
it.

New campaign, ad group, positive keyword, and ad resources are `PAUSED`.
Negative keywords are created `ENABLED` because Google Ads does not allow
paused negative ad-group criteria. Do not change those statuses to
`ENABLED` until you have verified the configuration in Google Ads.

The final plan must report `No changes.` when the manifest and remote
configuration are unchanged.

## Verify in Google Ads

In the Google Ads customer from `GOOGLE_ADS_CUSTOMER_ID`, confirm the
campaign is still paused before enabling spend:

1. Open **Goals -> Conversions** and confirm **Trial Started** exists as a
   website conversion with category **Sign-up**.
2. Open **Goals -> Conversion goals** and confirm the **Sign-up** website
   goal is an account-default optimization goal.
3. Open **Campaigns -> Search acquisition** and confirm status is **Paused**,
   bidding is Maximize conversions, the daily budget is `50` in the account
   currency, and Search/Search partners are on while Display is off.
4. Open the campaign's **Settings -> Conversion goals** and confirm
   **Sign-up** / website is selected for this campaign.
5. Open **Settings -> Locations** and confirm **United States** is targeted
   with presence targeting, then **Languages** and confirm **English**.
6. Open the **Trial** ad group and confirm it is **Paused**.
7. Open **Keywords** and confirm the three positive keywords with explicit
   `EXACT`, `PHRASE`, and `BROAD` match types, plus the jobs, careers, and
   login negatives.
8. Open **Ads** and confirm the Responsive Search Ad is **Paused**, uses
   `https://example.com/`, and shows the placeholder headlines, descriptions,
   and `trial/start` display path.

Replace the example.com landing page and placeholder copy with your own
product URLs and legal/ad-policy-compliant text before enabling anything.
Then fire a test conversion from your website or tag manager and confirm
that Google Ads records it against **Trial Started**. Diagnosing tag
execution, consent mode, and event delivery is outside Agoraform.

When you intentionally enable spend, change `status` to `ENABLED` on the
campaign, ad group, positive keywords, and ad in the manifest, run `plan`,
and review that update before `apply`. Enabling spend is a manual,
reviewed change; Agoraform does not activate campaigns automatically.

## Adopt existing resources

The same manifest is import-compatible. If an equivalent paused Search
campaign already exists, import resources in dependency order using the
identities shown by Google Ads. Import parents first so Agoraform can
reconstruct logical `$ref` values:

```bash
agoraform import googleads.conversion_action.trial_started CONVERSION_ACTION_ID
agoraform import googleads.customer_conversion_goal.signup SIGNUP~WEBSITE
agoraform import googleads.campaign_budget.search_acquisition CAMPAIGN_BUDGET_ID
agoraform import googleads.campaign.search_acquisition CAMPAIGN_ID
agoraform import googleads.campaign_conversion_goal.trial_signup CAMPAIGN_ID~SIGNUP~WEBSITE
agoraform import googleads.campaign_location.united_states CAMPAIGN_ID~LOCATION_CRITERION_ID
agoraform import googleads.campaign_language.english CAMPAIGN_ID~LANGUAGE_CRITERION_ID
agoraform import googleads.ad_group.trial AD_GROUP_ID
agoraform import googleads.keyword.saas_software_exact AD_GROUP_ID~CRITERION_ID
agoraform import googleads.keyword.start_free_trial_phrase AD_GROUP_ID~CRITERION_ID
agoraform import googleads.keyword.online_software_broad AD_GROUP_ID~CRITERION_ID
agoraform import googleads.keyword.jobs_neg AD_GROUP_ID~CRITERION_ID
agoraform import googleads.keyword.careers_neg AD_GROUP_ID~CRITERION_ID
agoraform import googleads.keyword.login_neg AD_GROUP_ID~CRITERION_ID
agoraform import googleads.responsive_search_ad.trial AD_GROUP_ID~AD_ID
```

Numeric IDs may also be written as Google Ads resource names such as
`customers/{customerId}/campaigns/{id}`. Keyword identities are
`adGroupId~criterionId`. Location and language identities are
`campaignId~criterionId`. The Responsive Search Ad identity is
`adGroupId~adId`. The customer conversion-goal identity is `SIGNUP~WEBSITE`.
The campaign conversion-goal identity is `CAMPAIGN_ID~SIGNUP~WEBSITE`.

Each command prints canonical YAML for review and records the remote identity
in `agoraform.state.json`; it does not edit the manifest or mutate Google Ads.
Computed fields such as resource names, micros, geo target constants, asset
IDs, tag snippets, `conversionId`, and `conversionLabel` are omitted from
that YAML. Import also does not emit optional `conversionAction` `$ref`
values; add those after import when the matching conversion action is also
managed, as this example does.

Compare the printed attributes with this example, adjust the manifest if
needed, then run `agoraform plan`. An equivalent imported configuration
against unchanged Google Ads state should report `No changes.` See
[Import](../../docs/import.md) for the complete adoption workflow.
