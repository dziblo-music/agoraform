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
(if the reviewed plan requires them)
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
agoraform destroy --auto-approve
```

The default manifest is `agoraform.yaml`.

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
supported operations complete, destroy exits non-zero when any requested
state-bound resource remains unsupported or provider-owned.

A provider must return one of the documented terminal destroy statuses:
`destroyed`, `removed`, or `already-absent`. Empty or unknown statuses are
provider-contract failures. Agoraform preserves local state in that case rather
than assuming a destructive operation completed safely.

## Ordering

Destroy uses the apply dependency graph in reverse: dependents are removed
before prerequisites. For Matomo Tag Manager that means tags, then
variables/triggers, then a managed container. For Google Ads Search that
means ads and keywords, then campaign criteria, then the ad group, then the
campaign, then the budget.

## Already absent and failures

Provider-confirmed already-absent resources are successful convergence.
Authentication, permission, malformed response, and wrong-target errors are
not treated as not-found.

Execution stops at the first provider or state-write failure. Remaining and
failed resources keep their state bindings so retry is deterministic. If a
remote teardown succeeds but the state write fails, the identity is left in
place so retry can confirm the object is gone.

When the destroy plan contains provider finalization, successful resource
teardowns keep their local state bindings until finalization also succeeds. If
finalization fails, retry can use those bindings to confirm the resources are
already terminal and retry the provider action safely. State is removed only
after the full reviewed lifecycle for those resources completes.

## Provider finalization

Destroy finalization uses the same visibility and failure rules as apply.
Planned actions such as Matomo publication appear in the destroy plan and run
only after every destructive mutation succeeds. A failed draft deletion does
not publish, and a failed publication keeps the affected destroy bindings so a
subsequent destroy can safely retry finalization.

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

## Google Ads

Google Ads does not hard-delete serving objects. Removable types are planned
as provider-native `remove` operations on the Google Ads REST mutate
endpoint. Those operations set remote `status` to `REMOVED`. Destroy never
sends `status: ENABLED` or otherwise activates serving or spend.

Customer and campaign conversion goals are created by Google Ads. Destroy
reports them as provider-owned non-mutations: they stay in state, receive no
remove request, and do not block teardown of supported resources. The command
still exits non-zero while those bindings remain.

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

### Meta Ads

| Type | Capability | API | Terminal remote state | Already terminal | Notes |
| --- | --- | --- | --- | --- | --- |
| `meta.pixel` | provider-owned | none | object remains | not applicable | Events Manager owns the Pixel/Dataset; Agoraform never deletes it |
| `meta.custom_conversion` | remove | `DELETE /{id}` | `is_archived=true` or not found | archived or not found | Agoraform does not assume a hard delete |
| `meta.campaign` | remove | `DELETE /{id}` | `status=DELETED`, `ARCHIVED`, or not found | terminal status or not found | `PAUSED` is not terminal; delete never enables serving |

Paused, hidden, and enabled resources are distinct from `REMOVED`. Destroy
removes the former and treats the latter as already-absent convergence.
Closing a Google Ads customer, tearing down billing, and pruning identities
that are not in the manifest remain out of scope.

See the [Matomo + Google Ads lifecycle example](../examples/matomo-googleads/README.md)
for exact destroy ordering, `destroy` versus `remove` versus
`provider-owned` output, and the non-zero exit caused by a preserved
customer conversion goal.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success, including a declined confirmation with no mutations |
| `1` | Destroy failed, or supported teardown completed with unsupported or provider-owned remnants still in state |
| `3` | Invalid invocation |

## Out of scope

- Automatic destruction merely because a resource was removed from the
  manifest
- Deleting Matomo sites/websites
- Deleting externally managed containers
- Rollback to prior container versions
- Application snippet removal
- Closing Google Ads customer accounts or billing/payment teardown
