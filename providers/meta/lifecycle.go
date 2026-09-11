package meta

import "github.com/dziblo-music/agoraform/internal/provider"

// CreateCapability describes how a declared Meta resource first becomes
// managed. Meta event sources are external objects; every other v0.6 resource
// is created by Agoraform.
type CreateCapability string

const (
	CreateSupported          CreateCapability = "supported"
	CreateExternalImportOnly CreateCapability = "external/import-only"
)

// UpdateCapability describes whether Agoraform mutates fields in place or
// only verifies an externally owned binding.
type UpdateCapability string

const (
	UpdateSupported UpdateCapability = "supported"
	UpdateReadOnly  UpdateCapability = "read-only/external"
)

// RelationshipLifecycle declares a managed edge which import must rebuild.
// ExternalAllowed is deliberately false for every v0.6 Meta relationship:
// the manifest schema requires lossless logical references.
type RelationshipLifecycle struct {
	Attribute       string
	ResourceType    string
	Output          string
	Required        bool
	ExternalAllowed bool
}

// ResourceLifecycle is the auditable provider contract for a Meta resource.
// It records lifecycle behavior that must not be inferred independently by
// individual handlers.
type ResourceLifecycle struct {
	RemoteIdentity  string
	Create          CreateCapability
	Update          UpdateCapability
	MutableFields   []string
	ImmutableFields []string
	ImportSupported bool
	Destroy         provider.DestroyCapability
	TerminalState   string
	AlreadyTerminal string
	Prerequisites   []RelationshipLifecycle
	BudgetOwnership string
	ServingSafety   string
}

var lifecycleByType = map[string]ResourceLifecycle{
	TypeImage: {
		RemoteIdentity:  "sha256 fingerprint of uploaded file content (state ID) + Meta image hash (Fingerprint/computed output)",
		Create:          CreateSupported,
		Update:          UpdateReadOnly,
		ImmutableFields: []string{AttrFile},
		ImportSupported: false,
		Destroy:         provider.DestroyProviderOwned,
		AlreadyTerminal: "not applicable; Meta image assets are provider-owned and not deleted by Agoraform",
		ServingSafety:   imageDestroyGuidance,
	},
	TypePixel: {
		RemoteIdentity:  "numeric Pixel/Dataset id",
		Create:          CreateExternalImportOnly,
		Update:          UpdateReadOnly,
		ImportSupported: true,
		Destroy:         provider.DestroyProviderOwned,
		AlreadyTerminal: "not applicable; object remains",
		ServingSafety:   pixelDestroyGuidance,
	},
	TypeCustomConversion: {
		RemoteIdentity:  "numeric custom conversion id",
		Create:          CreateSupported,
		Update:          UpdateSupported,
		MutableFields:   []string{AttrName, AttrDefaultValue},
		ImmutableFields: []string{AttrPixel, AttrRule, AttrEventType},
		ImportSupported: true,
		Destroy:         provider.DestroyRemove,
		TerminalState:   "is_archived=true or not found after DELETE",
		AlreadyTerminal: "is_archived=true or not found",
		Prerequisites: []RelationshipLifecycle{
			{Attribute: AttrPixel, ResourceType: TypePixel, Output: OutputPixelID, Required: true},
		},
		ServingSafety: "DELETE only; never changes delivery state",
	},
	TypeCampaign: {
		RemoteIdentity:  "numeric campaign id",
		Create:          CreateSupported,
		Update:          UpdateSupported,
		MutableFields:   []string{AttrName, AttrStatus, AttrSpecialAdCategories, AttrDailyBudget, AttrLifetimeBudget, AttrBidStrategy, AttrAdSetBudgetSharing},
		ImmutableFields: []string{AttrObjective, AttrBuyingType, "budget ownership/type"},
		ImportSupported: true,
		Destroy:         provider.DestroyRemove,
		TerminalState:   "status=DELETED or ARCHIVED, or not found after DELETE",
		AlreadyTerminal: "status=DELETED or ARCHIVED, or not found",
		BudgetOwnership: "exactly one of campaign-level budget or ad-set-level budget; import preserves the remote ownership shape",
		ServingSafety:   "DELETE only; PAUSED is not terminal and destroy never sends ACTIVE",
	},
	TypeAdSet: {
		RemoteIdentity:  "numeric ad set id",
		Create:          CreateSupported,
		Update:          UpdateSupported,
		MutableFields:   []string{AttrName, AttrStatus, AttrDailyBudget, AttrLifetimeBudget, AttrEndTime, AttrTargeting, AttrBidStrategy, AttrBidAmount},
		ImmutableFields: []string{AttrCampaign, AttrBillingEvent, AttrOptimizationGoal, AttrDestinationType, AttrPixel, AttrCustomConversion, AttrStartTime, "budget ownership/type"},
		ImportSupported: true,
		Destroy:         provider.DestroyRemove,
		TerminalState:   "status=DELETED or ARCHIVED, or not found after DELETE",
		AlreadyTerminal: "status=DELETED or ARCHIVED, or not found",
		Prerequisites: []RelationshipLifecycle{
			{Attribute: AttrCampaign, ResourceType: TypeCampaign, Output: OutputCampaignID, Required: true},
			{Attribute: AttrPixel, ResourceType: TypePixel, Output: OutputPixelID},
			{Attribute: AttrCustomConversion, ResourceType: TypeCustomConversion, Output: OutputCustomConversionID},
		},
		BudgetOwnership: "owns daily/lifetime budget only when the referenced campaign has no campaign-level budget",
		ServingSafety:   "DELETE only; PAUSED is not terminal and destroy never sends ACTIVE",
	},
	TypeAdCreative: {
		RemoteIdentity:  "numeric ad creative id",
		Create:          CreateSupported,
		Update:          UpdateSupported,
		MutableFields:   []string{AttrName},
		ImmutableFields: []string{AttrPageID, AttrInstagramUserID, AttrDestinationURL, AttrPrimaryText, AttrHeadline, AttrDescription, AttrCallToAction, AttrImageHash, AttrVideoID, AttrURLTags},
		ImportSupported: true,
		Destroy:         provider.DestroyDelete,
		TerminalState:   "status=DELETED or not found after DELETE",
		AlreadyTerminal: "status=DELETED or not found",
		ServingSafety:   "DELETE only; dependent ads must be removed first",
	},
	TypeAd: {
		RemoteIdentity:  "numeric ad id",
		Create:          CreateSupported,
		Update:          UpdateSupported,
		MutableFields:   []string{AttrName, AttrStatus, AttrCreative},
		ImmutableFields: []string{AttrAdSet},
		ImportSupported: true,
		Destroy:         provider.DestroyRemove,
		TerminalState:   "status=DELETED or ARCHIVED, or not found after DELETE",
		AlreadyTerminal: "status=DELETED or ARCHIVED, or not found",
		Prerequisites: []RelationshipLifecycle{
			{Attribute: AttrAdSet, ResourceType: TypeAdSet, Output: OutputAdSetID, Required: true},
			{Attribute: AttrCreative, ResourceType: TypeAdCreative, Output: OutputAdCreativeID, Required: true},
		},
		ServingSafety: "DELETE only; PAUSED is not terminal and destroy never sends ACTIVE",
	},
}

// Lifecycle returns a defensive copy of the lifecycle declaration for typ.
func Lifecycle(typ string) (ResourceLifecycle, bool) {
	spec, ok := lifecycleByType[typ]
	if !ok {
		return ResourceLifecycle{}, false
	}
	spec.MutableFields = append([]string(nil), spec.MutableFields...)
	spec.ImmutableFields = append([]string(nil), spec.ImmutableFields...)
	spec.Prerequisites = append([]RelationshipLifecycle(nil), spec.Prerequisites...)
	return spec, true
}
