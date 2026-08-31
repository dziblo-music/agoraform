package googleads

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/dziblo-music/agoraform/internal/provider"
	"github.com/dziblo-music/agoraform/internal/resource"
)

const (
	googleAdsRemovedStatus = "REMOVED"

	customerConversionGoalDestroyGuidance = "customer conversion goals are created by Google Ads and cannot be deleted; Agoraform can reconcile biddable through apply, but destroy leaves the provider-owned object in local state"
	campaignConversionGoalDestroyGuidance = "campaign conversion goals are created by Google Ads and cannot be deleted; Agoraform can reconcile biddable through apply, but destroy leaves the provider-owned object in local state"
)

// resourceDestroyLifecycle is the documented Google Ads destroy contract for
// one registered resource type. Tests require this table to stay exhaustive
// over Provider.ResourceTypes().
type resourceDestroyLifecycle struct {
	Capability      provider.DestroyCapability
	Collection      string
	MutateOperation string
	TerminalState   string
	AlreadyTerminal string
	Precondition    string
}

// destroyLifecycleByType is the v0.5.0 Google Ads destroy matrix.
//
// Removable types use Google Ads mutate `remove` operations. Those operations
// set provider-native status REMOVED rather than hard-deleting the object, so
// capability is remove rather than destroy. CustomerConversionGoalService and
// CampaignConversionGoalService mutate only biddable (update); Google Ads REST
// v25 has no remove/delete operation for those provider-created objects.
var destroyLifecycleByType = map[string]resourceDestroyLifecycle{
	TypeConversionAction: {
		Capability:      provider.DestroyRemove,
		Collection:      conversionActionsCollection,
		MutateOperation: "remove",
		TerminalState:   "status=REMOVED",
		AlreadyTerminal: "status=REMOVED or not found",
		Precondition:    "mutate remove only; never update status to ENABLED, HIDDEN, or any serving value",
	},
	TypeCustomerConversionGoal: {
		Capability:      provider.DestroyProviderOwned,
		MutateOperation: "",
		AlreadyTerminal: "not applicable; object remains",
		Precondition:    customerConversionGoalDestroyGuidance,
	},
	TypeCampaignBudget: {
		Capability:      provider.DestroyRemove,
		Collection:      campaignBudgetsCollection,
		MutateOperation: "remove",
		TerminalState:   "status=REMOVED",
		AlreadyTerminal: "status=REMOVED or not found",
		Precondition:    "refuse remove while referenceCount > 0 so a live budget cannot keep funding campaigns; mutate remove only",
	},
	TypeCampaign: {
		Capability:      provider.DestroyRemove,
		Collection:      campaignsCollection,
		MutateOperation: "remove",
		TerminalState:   "status=REMOVED",
		AlreadyTerminal: "status=REMOVED or not found",
		Precondition:    "mutate remove only; never update status to ENABLED or PAUSED as a substitute for remove",
	},
	TypeCampaignConversionGoal: {
		Capability:      provider.DestroyProviderOwned,
		MutateOperation: "",
		AlreadyTerminal: "not applicable; object remains",
		Precondition:    campaignConversionGoalDestroyGuidance,
	},
	TypeAdGroup: {
		Capability:      provider.DestroyRemove,
		Collection:      adGroupsCollection,
		MutateOperation: "remove",
		TerminalState:   "status=REMOVED",
		AlreadyTerminal: "status=REMOVED or not found",
		Precondition:    "mutate remove only; never update status to ENABLED",
	},
	TypeKeyword: {
		Capability:      provider.DestroyRemove,
		Collection:      adGroupCriteriaCollection,
		MutateOperation: "remove",
		TerminalState:   "status=REMOVED",
		AlreadyTerminal: "status=REMOVED or not found",
		Precondition:    "mutate remove only, including negative keywords whose status cannot be updated; never update status to ENABLED",
	},
	TypeResponsiveSearchAd: {
		Capability:      provider.DestroyRemove,
		Collection:      adGroupAdsCollection,
		MutateOperation: "remove",
		TerminalState:   "status=REMOVED",
		AlreadyTerminal: "status=REMOVED or not found",
		Precondition:    "remove the ad-group-ad relationship only; never enable serving or mutate the underlying Ad as a destroy substitute",
	},
	TypeCampaignLocation: {
		Capability:      provider.DestroyRemove,
		Collection:      campaignCriteriaCollection,
		MutateOperation: "remove",
		TerminalState:   "status=REMOVED",
		AlreadyTerminal: "status=REMOVED or not found",
		Precondition:    "mutate remove only; never update status to ENABLED",
	},
	TypeCampaignLanguage: {
		Capability:      provider.DestroyRemove,
		Collection:      campaignCriteriaCollection,
		MutateOperation: "remove",
		TerminalState:   "status=REMOVED",
		AlreadyTerminal: "status=REMOVED or not found",
		Precondition:    "mutate remove only; never update status to ENABLED",
	},
}

type destroyRemote struct {
	found          bool
	status         string
	resourceName   string
	referenceCount *int64
}

// DestroyCapability implements provider.Destroyer.
func (p *Provider) DestroyCapability(res resource.Resource) (provider.DestroyCapability, error) {
	if res.Address.Provider != Name {
		return provider.DestroyUnsupported, nil
	}
	spec, ok := destroyLifecycleByType[res.Address.Type]
	if !ok {
		if provider.Supports(p, res.Address.Type) {
			return "", fmt.Errorf("googleads: destroy %s: resource type %q has no destroy lifecycle declaration", res.Address, res.Address.Type)
		}
		return provider.DestroyUnsupported, nil
	}
	return spec.Capability, nil
}

// Destroy implements provider.Destroyer.
func (p *Provider) Destroy(ctx context.Context, res resource.Resource) (provider.DestroyResult, error) {
	spec, ok := destroyLifecycleByType[res.Address.Type]
	if !ok {
		return provider.DestroyResult{}, notImplemented("destroy", res.Address)
	}
	if spec.Capability == provider.DestroyProviderOwned {
		return provider.DestroyResult{}, fmt.Errorf("googleads: destroy %s: %s", res.Address, spec.Precondition)
	}
	if spec.Capability != provider.DestroyRemove {
		return provider.DestroyResult{}, fmt.Errorf("googleads: destroy %s: unsupported destroy capability %q", res.Address, spec.Capability)
	}

	switch res.Address.Type {
	case TypeConversionAction:
		return p.destroyByID(ctx, res, spec, boundConversionActionIdentity, func(id, customerID string) (string, string) {
			return conversionActionResourceName(customerID, id), "SELECT conversion_action.resource_name, conversion_action.status FROM conversion_action WHERE conversion_action.id = " + id
		}, "conversionAction", false)
	case TypeCampaignBudget:
		return p.destroyCampaignBudget(ctx, res, spec)
	case TypeCampaign:
		return p.destroyByID(ctx, res, spec, boundCampaignIdentity, func(id, customerID string) (string, string) {
			return campaignResourceName(customerID, id), "SELECT campaign.resource_name, campaign.status FROM campaign WHERE campaign.id = " + id
		}, "campaign", false)
	case TypeAdGroup:
		return p.destroyByID(ctx, res, spec, boundAdGroupIdentity, func(id, customerID string) (string, string) {
			return adGroupResourceName(customerID, id), "SELECT ad_group.resource_name, ad_group.status FROM ad_group WHERE ad_group.id = " + id
		}, "adGroup", false)
	case TypeKeyword:
		return p.destroyComposite(ctx, res, spec, boundKeywordIdentity, parseKeywordID, adGroupCriterionResourceName, func(adGroupID, childID string) string {
			return "SELECT ad_group_criterion.resource_name, ad_group_criterion.status FROM ad_group_criterion WHERE ad_group.id = " + adGroupID + " AND ad_group_criterion.criterion_id = " + childID
		}, "adGroupCriterion")
	case TypeResponsiveSearchAd:
		return p.destroyComposite(ctx, res, spec, boundRSAIdentity, parseRSAID, adGroupAdResourceName, func(adGroupID, childID string) string {
			return "SELECT ad_group_ad.resource_name, ad_group_ad.status FROM ad_group_ad WHERE ad_group.id = " + adGroupID + " AND ad_group_ad.ad.id = " + childID
		}, "adGroupAd")
	case TypeCampaignLocation, TypeCampaignLanguage:
		return p.destroyComposite(ctx, res, spec, boundCampaignCriterionIdentity, parseCampaignCriterionID, campaignCriterionResourceName, func(campaignID, childID string) string {
			return "SELECT campaign_criterion.resource_name, campaign_criterion.status FROM campaign_criterion WHERE campaign.id = " + campaignID + " AND campaign_criterion.criterion_id = " + childID
		}, "campaignCriterion")
	default:
		return provider.DestroyResult{}, notImplemented("destroy", res.Address)
	}
}

type boundIdentityFunc func(resource.Resource) (string, bool, error)

type resourceNameAndQuery func(id, customerID string) (resourceName, query string)

type compositeIDFunc func(addr resource.Address, raw string) (parentID, childID string, err error)

func (p *Provider) destroyByID(ctx context.Context, res resource.Resource, spec resourceDestroyLifecycle, bound boundIdentityFunc, nameQuery resourceNameAndQuery, envelope string, requireUnreferenced bool) (provider.DestroyResult, error) {
	id, err := requireDestroyIdentity(res, bound)
	if err != nil {
		return provider.DestroyResult{}, err
	}
	c, err := p.Client()
	if err != nil {
		return provider.DestroyResult{}, err
	}
	resourceName, query := nameQuery(id, c.CustomerID())
	remote, err := p.inspectDestroy(ctx, res.Address, query, envelope)
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("googleads: destroy %s: %w", res.Address, err)
	}
	if !remote.found || remote.status == googleAdsRemovedStatus {
		return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
	}
	if requireUnreferenced && remote.referenceCount != nil && *remote.referenceCount > 0 {
		return provider.DestroyResult{}, fmt.Errorf("googleads: destroy %s: campaign budget %s is still referenced by %d campaign(s); Google Ads cannot remove a budget that still funds campaigns. Destroy dependent googleads.campaign resources first", res.Address, id, *remote.referenceCount)
	}
	if remote.resourceName != "" {
		resourceName = remote.resourceName
	}
	return p.mutateRemove(ctx, res, spec.Collection, resourceName)
}

func (p *Provider) destroyCampaignBudget(ctx context.Context, res resource.Resource, spec resourceDestroyLifecycle) (provider.DestroyResult, error) {
	return p.destroyByID(ctx, res, spec, boundCampaignBudgetIdentity, func(id, customerID string) (string, string) {
		return campaignBudgetResourceName(customerID, id), "SELECT campaign_budget.resource_name, campaign_budget.status, campaign_budget.reference_count FROM campaign_budget WHERE campaign_budget.id = " + id
	}, "campaignBudget", true)
}

func (p *Provider) destroyComposite(ctx context.Context, res resource.Resource, spec resourceDestroyLifecycle, bound boundIdentityFunc, parse compositeIDFunc, resourceName func(customerID, id string) string, query func(parentID, childID string) string, envelope string) (provider.DestroyResult, error) {
	id, err := requireDestroyIdentity(res, bound)
	if err != nil {
		return provider.DestroyResult{}, err
	}
	parentID, childID, err := parse(res.Address, id)
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("googleads: destroy %s: %w", res.Address, err)
	}
	c, err := p.Client()
	if err != nil {
		return provider.DestroyResult{}, err
	}
	remote, err := p.inspectDestroy(ctx, res.Address, query(parentID, childID), envelope)
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("googleads: destroy %s: %w", res.Address, err)
	}
	if !remote.found || remote.status == googleAdsRemovedStatus {
		return provider.DestroyResult{Status: provider.DestroyStatusAlreadyAbsent}, nil
	}
	name := resourceName(c.CustomerID(), id)
	if remote.resourceName != "" {
		name = remote.resourceName
	}
	return p.mutateRemove(ctx, res, spec.Collection, name)
}

func (p *Provider) mutateRemove(ctx context.Context, res resource.Resource, collection, resourceName string) (provider.DestroyResult, error) {
	c, err := p.Client()
	if err != nil {
		return provider.DestroyResult{}, err
	}
	_, err = c.Mutate(ctx, collection, []map[string]any{
		{"remove": resourceName},
	})
	if err != nil {
		return provider.DestroyResult{}, fmt.Errorf("googleads: destroy %s: %w", res.Address, err)
	}
	return provider.DestroyResult{Status: provider.DestroyStatusRemoved}, nil
}

func (p *Provider) inspectDestroy(ctx context.Context, addr resource.Address, query, envelope string) (destroyRemote, error) {
	c, err := p.Client()
	if err != nil {
		return destroyRemote{}, err
	}
	rows, err := c.Query(ctx, query)
	if err != nil {
		return destroyRemote{}, err
	}
	switch len(rows) {
	case 0:
		return destroyRemote{}, nil
	case 1:
		remote, err := decodeDestroyRow(rows[0], envelope)
		if err != nil {
			return destroyRemote{}, err
		}
		return remote, nil
	default:
		return destroyRemote{}, fmt.Errorf("multiple remote results returned for %s", addr)
	}
}

func decodeDestroyRow(raw json.RawMessage, envelope string) (destroyRemote, error) {
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return destroyRemote{}, fmt.Errorf("malformed destroy inspect result")
	}
	body, ok := wrapper[envelope]
	if !ok || len(body) == 0 {
		return destroyRemote{}, fmt.Errorf("malformed destroy inspect result: missing %s", envelope)
	}
	var fields struct {
		ResourceName   string          `json:"resourceName"`
		Status         string          `json:"status"`
		ReferenceCount json.RawMessage `json:"referenceCount"`
	}
	if err := json.Unmarshal(body, &fields); err != nil {
		return destroyRemote{}, fmt.Errorf("malformed destroy inspect result")
	}
	resourceName := strings.TrimSpace(fields.ResourceName)
	if resourceName == "" {
		return destroyRemote{}, fmt.Errorf("malformed destroy inspect result: missing resourceName")
	}
	remote := destroyRemote{
		found:        true,
		status:       normalizeEnum(fields.Status),
		resourceName: resourceName,
	}
	if count, err := parseJSONInt64(fields.ReferenceCount); err != nil {
		return destroyRemote{}, fmt.Errorf("malformed destroy inspect result: invalid referenceCount")
	} else if count != nil {
		remote.referenceCount = count
	}
	return remote, nil
}

func requireDestroyIdentity(res resource.Resource, bound boundIdentityFunc) (string, error) {
	id, ok, err := bound(res)
	if err != nil {
		return "", fmt.Errorf("googleads: destroy %s: %w", res.Address, err)
	}
	if !ok || strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("googleads: destroy %s: missing identity", res.Address)
	}
	return id, nil
}

func parseJSONInt64(raw json.RawMessage) (*int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil && strings.TrimSpace(n.String()) != "" {
		v, err := n.Int64()
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
	return nil, fmt.Errorf("invalid int64")
}
