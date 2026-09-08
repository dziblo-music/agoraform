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
	// AttrAdSetBudgetSharing declares whether an ad-set-budget campaign allows
	// budget sharing between its ad sets.
	AttrAdSetBudgetSharing = "adSetBudgetSharingEnabled"
	// AttrCampaign is a logical $ref from a child resource to its campaign.
	AttrCampaign = "campaign"
	// AttrStartTime and AttrEndTime are RFC3339 ad-set schedule timestamps.
	AttrStartTime = "startTime"
	AttrEndTime   = "endTime"
	// AttrBillingEvent is the provider-native ad-set billing event.
	AttrBillingEvent = "billingEvent"
	// AttrOptimizationGoal is the provider-native ad-set optimization goal.
	AttrOptimizationGoal = "optimizationGoal"
	// AttrBidAmount is an ad-set bid in the ad account currency's smallest unit.
	AttrBidAmount = "bidAmount"
	// AttrDestinationType is the ad-set conversion destination.
	AttrDestinationType = "destinationType"
	// AttrCustomConversion is a logical $ref to a managed Custom Conversion.
	AttrCustomConversion = "customConversion"
	// AttrTargeting is the typed, bounded ad-set targeting object.
	AttrTargeting = "targeting"
	// AttrPageID is the Facebook Page identity used to publish a creative.
	AttrPageID = "pageId"
	// AttrInstagramUserID is the optional Instagram account identity used for
	// Instagram delivery.
	AttrInstagramUserID = "instagramUserId"
	// AttrDestinationURL is the website URL opened by a creative.
	AttrDestinationURL = "destinationUrl"
	// AttrPrimaryText, AttrHeadline, and AttrDescription are the supported
	// website creative copy fields.
	AttrPrimaryText = "primaryText"
	AttrHeadline    = "headline"
	AttrDescription = "description"
	// AttrCallToAction is the provider-native website CTA type.
	AttrCallToAction = "callToAction"
	// AttrImageHash and AttrVideoID are mutually exclusive external media
	// identifiers. Agoraform does not own or upload these assets.
	AttrImageHash = "imageHash"
	AttrVideoID   = "videoId"
	// AttrURLTags is Meta's provider-native destination URL parameter string.
	AttrURLTags = "urlTags"

	// TypeCustomConversion is used in addresses such as
	// meta.custom_conversion.trial_started.
	TypeCustomConversion = "custom_conversion"
	// TypeCampaign is used in addresses such as meta.campaign.acquisition.
	TypeCampaign = "campaign"
	// TypeAdSet is used in addresses such as meta.ad_set.instagram.
	TypeAdSet = "ad_set"
	// TypeAdCreative is used in addresses such as meta.ad_creative.instagram.
	TypeAdCreative = "ad_creative"

	// OutputCustomConversionID is the declared non-secret Custom Conversion id.
	OutputCustomConversionID = "customConversionId"
	// OutputCampaignID is the declared non-secret Meta campaign id.
	OutputCampaignID = "campaignId"
	// OutputAdSetID is the declared non-secret Meta ad-set id.
	OutputAdSetID = "adSetId"
	// OutputAdCreativeID is the declared non-secret Meta ad creative id.
	OutputAdCreativeID = "adCreativeId"
)
