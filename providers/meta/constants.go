package meta

const (
	// AttrName is the human-readable Meta object name.
	AttrName = "name"
	// AttrPixel is a logical $ref to a managed website Pixel/Dataset.
	AttrPixel = "pixel"
	// AttrRule is the Meta-native Custom Conversion matching rule.
	AttrRule = "rule"
	// AttrEventType is the Custom Conversion custom_event_type category.
	AttrEventType = "eventType"
	// AttrDefaultValue is the optional default_conversion_value.
	AttrDefaultValue = "defaultValue"
	// AttrObjective is the Meta Outcome-Driven Ad Experiences campaign objective.
	AttrObjective = "objective"
	// AttrStatus is the configured campaign serving status.
	AttrStatus = "status"
	// AttrSpecialAdCategories declares Meta special-ad categories. An empty list
	// explicitly declares that none apply.
	AttrSpecialAdCategories = "specialAdCategories"
	// AttrBuyingType is the campaign buying type. The initial schema supports
	// the regular AUCTION workflow only.
	AttrBuyingType = "buyingType"
	// AttrDailyBudget is a campaign-level daily budget in the ad account
	// currency's smallest unit (for example cents for USD).
	AttrDailyBudget = "dailyBudget"
	// AttrLifetimeBudget is a campaign-level lifetime budget in the ad account
	// currency's smallest unit.
	AttrLifetimeBudget = "lifetimeBudget"
	// AttrBidStrategy is the campaign-level Meta bid strategy used with a
	// campaign-level budget.
	AttrBidStrategy = "bidStrategy"

	// TypeCustomConversion is used in addresses such as
	// meta.custom_conversion.trial_started.
	TypeCustomConversion = "custom_conversion"
	// TypeCampaign is used in addresses such as meta.campaign.acquisition.
	TypeCampaign = "campaign"

	// OutputCustomConversionID is the declared non-secret Custom Conversion id.
	OutputCustomConversionID = "customConversionId"
	// OutputCampaignID is the declared non-secret Meta campaign id.
	OutputCampaignID = "campaignId"
)
