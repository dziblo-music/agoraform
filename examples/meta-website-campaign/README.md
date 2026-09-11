# Meta Ads website conversion campaign

This example manages a complete paused website conversion campaign in Meta
Ads, from the event source through the serving ad:

- `meta.pixel.website` binds the existing Pixel/Dataset event source that
  already lives in Events Manager;
- `meta.custom_conversion.trial_started` is a `START_TRIAL` website Custom
  Conversion whose rule matches the `StartTrial` browser event;
- `meta.campaign.website_acquisition` is a paused `OUTCOME_SALES` auction
  campaign that delegates budget ownership to its ad sets;
- `meta.ad_set.instagram_trial` is a paused ad set with a daily budget,
  `OFFSITE_CONVERSIONS` optimization against the managed Custom Conversion,
  and typed United States / Instagram placement targeting;
- `meta.ad_creative.instagram_trial` is a link creative that references an
  externally prepared image and placeholder Page and Instagram identities;
- `meta.ad.instagram_trial` is the paused ad that binds the ad set to the
  creative.

Logical `$ref` values create the dependency graph. Each resource below sits
above the resources it references:

```text
meta.ad.instagram_trial
├── meta.ad_set.instagram_trial
│   ├── meta.campaign.website_acquisition
│   ├── meta.custom_conversion.trial_started
│   │   └── meta.pixel.website
│   └── meta.pixel.website
└── meta.ad_creative.instagram_trial
```

The ad set references the pixel directly as well as through its Custom
Conversion, and Agoraform rejects the configuration if those two pixels
disagree.

Provider-native IDs for Agoraform-managed resources and access tokens do not
belong in the manifest. External values that the creative must reference —
the Page ID, optional Instagram user ID, and image hash or video ID — remain
literal manifest attributes because Agoraform does not manage those objects.
Replace the placeholder landing page, copy, budget, targeting, Page ID,
Instagram user ID, and image hash with values for your own account before
enabling delivery.

## What Agoraform manages here

Agoraform reconciles Meta Ads **configuration objects and the relationships
between them**: which event source the Custom Conversion reads, the campaign
objective and budget ownership, the ad set's budget, optimization, and
targeting, the creative's copy and destination, and which creative an ad
serves.

Everything else stays outside Agoraform:

| Concern | Owner |
| --- | --- |
| Meta app, Business Manager, system user, access-token provisioning | You, before running Agoraform |
| Page and Instagram account administration | Meta Business Suite |
| Pixel/Dataset creation and ownership | Events Manager |
| Browser Pixel code and `fbq('track', 'StartTrial')` | Your website |
| Conversions API server events | Your backend |
| Image and video asset production and upload | Your creative pipeline and Meta media upload |
| Enabling delivery and spend | A deliberate, reviewed manifest change |

Agoraform never uploads media bytes. `imageHash` is an identifier for an
image that already exists in the ad account, and `pageId` /
`instagramUserId` name accounts you already administer. The event name in
the Custom Conversion `rule` (`StartTrial` here) is the contract your
external instrumentation must emit; the declared `pixelId` output is the
event-source identifier those tags initialize.

## Prerequisites

You need a Meta ad account the Marketing API can manage, an access token
with `ads_management`, a Pixel/Dataset in that account, a Facebook Page (and
optionally an Instagram account) you administer, and an image already
uploaded to the ad account's image library. Prefer a Business system-user
token for automation. Agoraform consumes the token; it does not create apps,
businesses, system users, or tokens.

`META_AD_ACCOUNT_ID` accepts `act_123456789012345` or the bare numeric id.
It is account-specific rather than secret, so this reusable example does not
embed it. Load the token from your usual secret manager. For an interactive
Bash session where secret-manager injection is not available, `read -s`
keeps the typed token out of shell command history:

```bash
export META_AD_ACCOUNT_ID=act_123456789012345

read -rsp "Meta access token: " META_ACCESS_TOKEN; echo
export META_ACCESS_TOKEN
```

Do not replace the prompt above with a literal token. Keep tokens out of
shell history, logs, source control, and the manifest. The empty
`providers.meta: {}` block is valid; there are no non-secret Meta YAML
fields, and credential-like keys in the manifest are rejected.

Copy the example into a working directory so its generated
`agoraform.state.json` remains local:

```bash
cp examples/meta-website-campaign/agoraform.yaml ./agoraform.yaml
```

Then replace the placeholders in the copied manifest:

| Placeholder | Replace with |
| --- | --- |
| `pageId: "123456789012345"` | The numeric ID of a Page you administer |
| `instagramUserId: "234567890123456"` | Your Instagram account ID, or delete the field |
| `imageHash: "0123456789abcdef0123456789abcdef"` | The hash of an image already in the ad account |
| `destinationUrl: https://example.com/trial` | Your landing page |
| `name: Website` on the pixel | The exact name of your Pixel/Dataset |

## Apply

Run the full lifecycle from the directory containing the copied manifest:

```bash
agoraform validate
agoraform plan
agoraform apply
agoraform plan
```

`validate` checks the manifest and the relationships between resources
without contacting Meta for mutations: budget ownership, the
`OFFSITE_CONVERSIONS` / `OUTCOME_SALES` pairing, and the requirement that
the ad set and its Custom Conversion share the same pixel. It also performs
read-only checks that the token reports `ads_management` and that the ad
account is reachable.

Review the first plan before applying it. Agoraform creates five resources
and adopts the pixel:

```text
+ meta.ad_creative.instagram_trial
+ meta.campaign.website_acquisition
* meta.pixel.website (adopt)
+ meta.custom_conversion.trial_started
+ meta.ad_set.instagram_trial
+ meta.ad.instagram_trial
```

That order is the topological order of the `$ref` graph, with the address
string as the tie-breaker among resources that have no unmet prerequisites.
The creative and campaign have no dependencies, the Custom Conversion waits
for the pixel, the ad set waits for the campaign, pixel, and Custom
Conversion, and the ad waits for the ad set and creative.

`meta.pixel.website` is adopted, not created. Agoraform never creates or
deletes a Pixel/Dataset. Adoption succeeds when exactly one pixel in the ad
account has the declared name; a missing or ambiguous name is an error. Bind
a specific pixel explicitly instead if you prefer:

```bash
agoraform import meta.pixel.website YOUR_PIXEL_ID
```

The campaign, ad set, and ad are created `PAUSED`. Meta does not begin
delivery for this graph until every one of them is `ACTIVE` and eligible, so
apply cannot start spend on its own.

The final plan must report `No changes.` when the manifest and remote
configuration are unchanged.

## Safety and replacement

Enabling delivery is a manual, reviewed change: set `status: ACTIVE` on the
campaign, ad set, and ad, run `plan`, read the before/after values, and only
then `apply`.

Agoraform does not hide a destroy-and-recreate behind an immutable identity
change. Changing one of these fails `plan` with guidance instead:

- campaign `objective`, `buyingType`, or budget ownership and type;
- ad set `campaign`, `billingEvent`, `optimizationGoal`, `destinationType`,
  `pixel`, `customConversion`, `startTime`, or budget ownership and type;
- every ad creative field except `name` — Page and Instagram identity,
  destination, copy, CTA, media, and URL tags are immutable;
- the ad's parent `adSet`.

Declare a new logical resource address for those changes. Because a creative
is immutable, the supported way to change ad copy or media is to add a new
`meta.ad_creative` resource and repoint the ad's `creative` `$ref`;
Agoraform updates that relationship in place and the plan shows the swap.

Budgets are declared in ad account currency units. In a USD account,
`dailyBudget: 50` means USD 50.00 per day. This example puts the
budget on the ad set, so the campaign declares no budget and
`adSetBudgetSharingEnabled: false`. The reverse arrangement — a
campaign-level budget with no ad-set budget — is shown in the
[campaign-only example](../meta-campaign/README.md). Switching between the
two is an ownership change and is rejected in place.

A lifetime budget is also supported on the ad set, but it requires both
`startTime` and `endTime`, and `startTime` is immutable after create. This
example uses a daily budget so the shipped manifest does not carry a
schedule that goes stale.

## Destroy

`agoraform destroy --auto-approve` tears down the managed graph in reverse
dependency order:

```text
Agoraform will destroy the following resources:

- meta.ad.instagram_trial (remove)
- meta.ad_set.instagram_trial (remove)
- meta.custom_conversion.trial_started (remove)
- meta.campaign.website_acquisition (remove)
- meta.ad_creative.instagram_trial

The following resources cannot be destroyed and will remain in state:

- meta.pixel.website (provider-owned)
```

Every remote mutation is a Marketing API `DELETE`. Destroy never raises a
budget or changes a status to a serving value. The ad is removed before the
ad set and creative it depends on, which also avoids Meta rejecting the
creative deletion while an ad still references it. The Custom Conversion is
removed before its pixel.

`meta.pixel.website` is intentionally preserved. Its lifecycle belongs to
Events Manager and Business Manager, so Agoraform reports it as
provider-owned, leaves the remote object alone, and keeps the binding in
state. Supported teardown still runs to completion, and the command then
exits non-zero because that binding remains. That exit code means "teardown
was deliberately incomplete", not "teardown failed".

Terminal states are idempotent: a `DELETED` or `ARCHIVED` object, an
archived Custom Conversion, or a missing object all count as already
destroyed, so repeating `destroy` is safe.

If one deletion fails partway through — a transient Meta error, or Meta
refusing to delete a creative that something outside this manifest still
references — Agoraform unbinds only the resources it confirmed reached a
terminal state. The failed resource and every resource it had not attempted
yet stay bound in `agoraform.state.json`. Rerun `agoraform destroy` after
resolving the cause; it resumes from the remaining bindings in the same
order and skips what is already terminal.

Removing a resource from this manifest does not destroy it. See
[Destroy](../../docs/destroy.md).

## Verify in Meta Ads

Verify in a dedicated non-production ad account. Every campaign, ad set, and
ad in this example ships `PAUSED`, so the whole verification runs without
enabling delivery or spend.

In Ads Manager for the account in `META_AD_ACCOUNT_ID`:

1. Open **Events Manager** and confirm the Pixel/Dataset Agoraform adopted is
   the one you expected, and that **Trial Started** appears as a Custom
   Conversion on it with the `StartTrial` rule and the `START_TRIAL`
   category.
2. Open **Campaigns** and confirm **Website Acquisition** is **Off**
   (paused), its objective is Sales, and no campaign budget is set.
3. Open the **Instagram Trial Acquisition** ad set and confirm it is **Off**,
   its daily budget matches the manifest, its conversion location is Website,
   its performance goal optimizes for the **Trial Started** conversion, and
   its audience shows United States, ages 25–55, Instagram placements only.
4. Open **Instagram Trial Ad** and confirm it is **Off**, uses the expected
   Page and Instagram account, shows the placeholder copy and image, and
   links to the destination URL with the configured URL parameters.
5. Confirm nothing in the account is delivering and no spend has accrued.

Then run `agoraform plan` again and confirm it reports `No changes.`, which
demonstrates that reading back the live configuration converges. Run
`agoraform apply` a second time and confirm it is a no-op, which
demonstrates idempotence.

Replace the example.com landing page, placeholder copy, and placeholder
identities with your own product URLs and policy-compliant text before
enabling anything. Diagnosing Pixel execution, consent, and Conversions API
delivery is outside Agoraform.

## Adopt existing resources

The same manifest is import-compatible with an equivalent campaign that was
built by hand in Ads Manager. Import is read-only: it prints canonical YAML
for review and records the remote identity in `agoraform.state.json`. It
never edits the manifest and never mutates Meta.

Import in dependency order, because each Meta relationship is reconstructed
from identities that are already bound in local state:

```bash
agoraform import meta.pixel.website YOUR_PIXEL_ID
agoraform import meta.custom_conversion.trial_started YOUR_CUSTOM_CONVERSION_ID
agoraform import meta.campaign.website_acquisition YOUR_CAMPAIGN_ID
agoraform import meta.ad_creative.instagram_trial YOUR_AD_CREATIVE_ID
agoraform import meta.ad_set.instagram_trial YOUR_AD_SET_ID
agoraform import meta.ad.instagram_trial YOUR_AD_ID
```

Every identity is the numeric Meta object ID. The printed YAML reconstructs
these logical references:

| Imported resource | Reconstructed references |
| --- | --- |
| `meta.custom_conversion.trial_started` | `pixel: { $ref: meta.pixel.website }` |
| `meta.ad_set.instagram_trial` | `campaign`, `pixel`, and `customConversion` `$ref` values |
| `meta.ad.instagram_trial` | `adSet` and `creative` `$ref` values |

A reference is emitted only when the remote ID has exactly one binding in
local state. A missing or ambiguous match fails the import before anything
is written, rather than writing a raw Meta ID into YAML — none of these
relationship fields accepts a literal ID. Importing out of order is
therefore a clean failure, not a corrupt state file.

External media and identity values are the exception: `pageId`,
`instagramUserId`, and `imageHash` come back as literals, because Agoraform
does not own those objects. Computed values such as effective status are
omitted.

Import preserves the remote configured status. It never pauses a running
campaign and never activates a paused one.

Compare the printed attributes with this example, adjust the manifest to
match anything you configured differently, then run `agoraform plan`. An
equivalent imported configuration against unchanged Meta state reports
`No changes.` See [Import](../../docs/import.md) for the complete adoption
workflow, and the [Meta provider reference](../../providers/meta/README.md)
for the full field contract.
