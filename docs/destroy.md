# Destroy

`agoraform destroy` tears down managed resources using the same provider-neutral
manifest graph, local identity state, and deterministic execution model as
`plan` and `apply`.

Destroy is a separate command. `plan` and `apply` never delete remote
resources, and removing a resource from the manifest does not destroy or
prune it. Only resources present in both the manifest and local state are
eligible for teardown.

```text
manifest + local state
        │
        ▼
validate / configure providers
        │
        ▼
build destroy plan
(reverse dependency order)
        │
        ▼
confirm (or --auto-approve)
        │
        ▼
destroy/remove supported resources
(dependents first)
        │
        ▼
provider finalization actions
(per provider, if the reviewed plan requires them
and that provider's teardown completed)
        │
        ▼
remove confirmed identities
        │
        ▼
Destroy complete
```

## Command

```bash
agoraform destroy
agoraform destroy -f path/to/manifest.yaml
agoraform destroy path/to/campaign/
agoraform destroy --auto-approve
```

The default path is `agoraform.yaml`. A directory path merges
`*.agoraform.yaml` and `*.agoraform.yml` files in that directory. See
[multi-file configuration](manifest.md#multi-file-configuration).

## Confirmation

Destroy is explicitly destructive.

- Interactive terminals require typing `yes` after the plan is shown.
- Non-interactive stdin fails before mutation unless `--auto-approve` is set.
- Declining confirmation exits 0 with no remote or state changes.

## Destroy set

The manifest defines the requested destroy set and dependency relationships.
Local state supplies provider-native identities.

- A manifest resource without a state binding is reported as not managed and
  is not changed remotely.
- Identities present in `agoraform.state.json` but absent from the manifest
  are preserved. Destroy does not prune them.
- Invalid graphs produce zero remote mutations.
- A resource whose state ownership is `external` is reference-only. Destroy
  removes the local binding when the resource is still declared, and it does
  not call the provider. Dropping `lifecycle.ownership: external` from the
  manifest does not make that object deletable. See
  [External resources](external-resources.md).

## Lifecycle capabilities

Providers declare the exact lifecycle operation that will be reviewed before
mutation. Core never assumes a generic HTTP DELETE.

| Capability | Plan | Remote | State |
| --- | --- | --- | --- |
| destroy | `destroy` | provider-native deletion | removed after confirmed terminal result and required finalization |
| remove | `remove` | provider-native remove/archive/disable | removed after confirmed terminal result and required finalization |
| unsupported | listed, not mutated | none | kept |
| provider-owned | listed, not mutated | none | kept |

Unsupported and provider-owned remnants do not block supported teardown. After
supported operations complete, destroy exits 0 and names any requested
state-bound resource that remains because it is unsupported or provider-owned.

A provider must return one of the documented terminal destroy statuses:
`destroyed`, `removed`, or `already-absent`. Empty or unknown statuses are
provider-contract failures. Agoraform preserves local state in that case rather
than assuming a destructive operation completed safely.

## Ordering

Destroy uses the apply dependency graph in reverse: dependents are removed
before prerequisites. For Matomo Tag Manager that means tags, then
variables/triggers, then a managed container. For Google Ads Search that
means ads and keywords, then campaign assets, then campaign criteria, then
the ad group, then the campaign, then the budget. Campaign-asset relationships
are detached before campaign teardown. Underlying `googleads.asset` objects
cannot be removed through AssetService and remain in state.

## Already absent and failures

Provider-confirmed already-absent resources are successful convergence.
Authentication, permission, malformed response, and wrong-target errors are
not treated as not-found.

A provider failure does not stop the run. Destroy prints the failure, keeps
that resource's state binding, and continues with every resource that does
not depend on it. Resources the failed one depends on (its prerequisites, such
as the campaign under a failed ad set or the container under a failed Matomo
variable) are skipped and keep their bindings too, because removing them could
cascade into or orphan the object that is still there. After the run, destroy
prints `Destroy incomplete!`, lists each failure and each skipped resource with
the resource that blocked it, and exits 1. Rerunning `agoraform destroy`
retries only what is still in state; resources that already completed are not
touched again.

```text
meta.ad_set.instagram: failed: ...
meta.campaign.acquisition: skipped; meta.ad_set.instagram depends on it and was not destroyed
...
Destroy incomplete! 7 destroyed, 3 removed, 1 failed, 1 skipped.
```

A local state-write failure is different: it stops execution immediately. If a
remote teardown succeeds but the state write fails, the identity is left in
place so retry can confirm the object is gone.

When the destroy plan contains a provider finalization, that provider's
successful resource teardowns keep their local state bindings until its
finalization also succeeds. Other providers' bindings are removed as soon as
each teardown is confirmed. If finalization fails or is skipped, retry uses
the kept bindings to confirm the resources are already terminal and plans the
provider action again.

## Provider finalization

Destroy finalization uses the same visibility rules as apply. Planned actions
such as Matomo publication appear in the destroy plan and run after resource
teardown, once per provider, only when every planned change for that provider
completed. If any of that provider's resources failed or were skipped, the
action is skipped and listed in the `Destroy incomplete!` error; other
providers' actions still run. A failed draft deletion therefore does not
publish, and a failed or skipped publication keeps the affected destroy
bindings so a subsequent destroy can safely retry finalization.

## Matomo

Supported Matomo destroy targets are `matomo.goal`, `matomo.variable`
(Data Layer and Matomo Configuration), `matomo.trigger`, `matomo.tag`, and
an Agoraform-managed `matomo.container`.

Matomo goals are planned as a provider-native `remove` operation because Matomo
marks deleted goals terminal rather than treating the historical goal identity
as a normal hard-deleted object. Tag Manager children and managed containers
are planned as `destroy` operations.

`MATOMO_CONTAINER_ID` continues to select an externally managed container.
Destroy never deletes that container merely because child resources inside it
are managed. A `matomo.container` present in the manifest and bound in state,
whether created or imported, is managed and eligible for deletion after its
children.

When child Tag Manager resources are destroyed and the container remains,
publication follows `providers.matomo.publish`. When the same plan will
delete the managed container, destroy does not create or publish an
intermediate container version; it deletes children and then the container.

A reference-only container (`lifecycle.ownership: external`) is never deleted,
but it can still be published. Destroy publishes it using the container id
bound in local state, or the manifest's `lifecycle.id` when the container has
no state binding, and then releases the local binding.

Matomo refuses to delete a variable while any tag in the container still uses
it, including tags that are not in the manifest. Destroy reports that refusal
as a failure for the variable, keeps its binding, and continues with the other
tags, triggers, goals, and providers. Publication is skipped for that run;
remove the remaining usage (or drop the variable from the manifest) and rerun
destroy to delete it and publish.

## Google Ads

Google Ads does not hard-delete serving objects. Removable types are planned
as provider-native `remove` operations on the Google Ads REST mutate
endpoint. Those operations set remote `status` to `REMOVED`. Destroy never
sends `status: ENABLED` or otherwise activates serving or spend.

Customer and campaign conversion goals are created by Google Ads. Destroy
reports them as provider-owned non-mutations: they stay in state, receive no
remove request, and do not block teardown of supported resources. The command
still completes successfully and lists those bindings.

| Type | Capability | Mutate | Terminal remote state | Already terminal | Serving/spend precondition |
| --- | --- | --- | --- | --- | --- |
| `googleads.conversion_action` | remove | `conversionActions:mutate` `remove` | `status=REMOVED` | `REMOVED` or not found | mutate `remove` only; never update status to a serving value |
| `googleads.customer_conversion_goal` | provider-owned | none | object remains | not applicable | Google Ads has no delete; reconcile `biddable` with apply |
| `googleads.campaign_budget` | remove | `campaignBudgets:mutate` `remove` | `status=REMOVED` | `REMOVED` or not found | refuse while `referenceCount > 0`; destroy campaigns first |
| `googleads.campaign` | remove | `campaigns:mutate` `remove` | `status=REMOVED` | `REMOVED` or not found | mutate `remove` only; paused is not terminal |
| `googleads.campaign_conversion_goal` | provider-owned | none | object remains | not applicable | Google Ads has no delete; reconcile `biddable` with apply |
| `googleads.ad_group` | remove | `adGroups:mutate` `remove` | `status=REMOVED` | `REMOVED` or not found | mutate `remove` only |
| `googleads.keyword` | remove | `adGroupCriteria:mutate` `remove` | `status=REMOVED` | `REMOVED` or not found | `remove` works for negative keywords; never update status to `ENABLED` |
| `googleads.responsive_search_ad` | remove | `adGroupAds:mutate` `remove` | `status=REMOVED` | `REMOVED` or not found | remove the ad-group-ad relationship only |
| `googleads.campaign_location` | remove | `campaignCriteria:mutate` `remove` | `status=REMOVED` | `REMOVED` or not found | mutate `remove` only |
| `googleads.campaign_language` | remove | `campaignCriteria:mutate` `remove` | `status=REMOVED` | `REMOVED` or not found | mutate `remove` only |
| `googleads.campaign_negative_keyword` | remove | `campaignCriteria:mutate` `remove` | `status=REMOVED` | `REMOVED` or not found | `remove` only; never update status to `ENABLED` |
| `googleads.asset` | unsupported | none | object remains | not applicable | AssetService has no remove/delete operation; detach campaign-asset relationships instead |
| `googleads.campaign_asset` | remove | `campaignAssets:mutate` `remove` | `status=REMOVED` | `REMOVED` or not found | detach the campaign relationship only |

### Meta Ads

| Type | Capability | API | Terminal remote state | Already terminal | Notes |
| --- | --- | --- | --- | --- | --- |
| `meta.pixel` | provider-owned | none | object remains | not applicable | Events Manager owns the Pixel/Dataset; Agoraform never deletes it |
| `meta.image` | provider-owned | none | object remains | not applicable | Uploaded images may be shared across creatives; Agoraform never deletes them |
| `meta.video` | delete | `DELETE /{id}` | not found after DELETE | not found | Meta may reject deletion while a creative still uses the video |
| `meta.custom_conversion` | remove | `DELETE /{id}` | `is_archived=true` or not found | archived or not found | Agoraform does not assume a hard delete |
| `meta.campaign` | remove | `DELETE /{id}` | `status=DELETED`, `ARCHIVED`, or not found | terminal status or not found | `PAUSED` is not terminal; delete never enables serving |
| `meta.ad_set` | remove | `DELETE /{id}` | `status=DELETED`, `ARCHIVED`, or not found | terminal status or not found | dependent ads are removed first |
| `meta.ad_creative` | delete | `DELETE /{id}` | `status=DELETED` or not found | terminal status or not found | Meta may reject deletion while an ad still uses the creative |
| `meta.ad` | remove | `DELETE /{id}` | `status=DELETED`, `ARCHIVED`, or not found | terminal status or not found | removed before its ad set and creative dependencies |

Before deleting a Meta object, destroy reads only its id, owning ad account,
and status. It does not map the full object into the manifest schema, so
fields Meta adds to an object (for example `targeting.user_age_unknown`)
cannot block deletion. An object owned by a different ad account is refused.

Meta can keep returning an object for a short time after accepting `DELETE`.
Destroy re-reads it with backoff (about 7 seconds in total) before reporting a
failure. If it is still readable after that, the failure says the delete was
accepted but not yet visible; the binding is kept, and the next destroy run
confirms it as already absent.

Paused, hidden, and enabled resources are distinct from `REMOVED`. Destroy
removes the former and treats the latter as already-absent convergence.
Closing a Google Ads customer, tearing down billing, and pruning identities
that are not in the manifest remain out of scope.

See the [Matomo + Google Ads lifecycle example](../examples/matomo-googleads/README.md)
for exact destroy ordering, `destroy` versus `remove` versus
`provider-owned` output, and the completion notice for a preserved
customer conversion goal.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success, including a declined confirmation with no mutations, and a completed teardown that left unsupported or provider-owned resources in state |
| `1` | Destroy failed, or finished with resources that failed or were skipped (`Destroy incomplete!`) |
| `3` | Invalid invocation |

## Out of scope

- Automatic destruction merely because a resource was removed from the
  manifest
- Deleting Matomo sites/websites
- Deleting externally managed containers
- Rollback to prior container versions
- Application snippet removal
- Closing Google Ads customer accounts or billing/payment teardown
