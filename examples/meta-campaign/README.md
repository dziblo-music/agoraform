# Meta campaign example

This v0.6.0 example declares a standard auction campaign for website
acquisition. It is explicitly paused and uses a campaign-level lifetime
budget.

`lifetimeBudget` is expressed in the ad account currency's smallest unit. In
a USD account, `50000` means USD 500.00. Review the planned `status` and budget
before applying; changing `status` to `ACTIVE` enables serving when the future
ad-set and ad resources are also active and eligible.

Set `META_ACCESS_TOKEN` and `META_AD_ACCOUNT_ID`, then run:

```bash
agoraform validate -f examples/meta-campaign/agoraform.yaml
agoraform plan -f examples/meta-campaign/agoraform.yaml
agoraform apply -f examples/meta-campaign/agoraform.yaml
```

Import an existing supported auction campaign without changing its serving
status:

```bash
agoraform import -f examples/meta-campaign/agoraform.yaml \
  meta.campaign.acquisition 777888999000111
```

Ad sets, targeting, creatives, and ads are intentionally outside this example
until their v0.6.0 resources are implemented.
