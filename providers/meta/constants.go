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
	// AttrDailyBudget is a daily budget in ad account currency units, so 20 in
	// a USD account means USD 20.00.
	AttrDailyBudget = "dailyBudget"
	// AttrLifetimeBudget is a lifetime budget in ad account currency units.
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
	// AttrBidAmount is an ad-set bid in ad account currency units.
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
	// identifiers for ad creatives that reference externally managed assets.
	// For managed local-file assets, use AttrImageRef on the creative and
	// declare a meta.image resource with AttrFile.
	AttrImageHash = "imageHash"
	AttrVideoID   = "videoId"
	// AttrImageRef is the managed image reference attribute on meta.ad_creative.
	// It accepts a $ref to a meta.image resource and is mutually exclusive with
	// AttrImageHash. The plan engine resolves the reference at apply time and
	// sends the provider-native image hash to the Meta API.
	AttrImageRef = "image"
	// AttrFile is the local file path attribute on meta.image resources.
	// The path is resolved relative to the working directory (run agoraform
	// from the same directory as the manifest for relative paths to work).
	AttrFile = "file"
	// AttrURLTags is Meta's provider-native destination URL parameter string.
	AttrURLTags = "urlTags"
	// AttrAdSet and AttrCreative are logical references from an ad to its
	// managed serving container and creative.
	AttrAdSet    = "adSet"
	AttrCreative = "creative"

	// TypeImage is used in addresses such as meta.image.trial_ad.
	// A meta.image resource uploads a local file during apply and captures the
	// provider-native image hash for use by meta.ad_creative resources.
	TypeImage = "image"
	// TypeCustomConversion is used in addresses such as
	// meta.custom_conversion.trial_started.
	TypeCustomConversion = "custom_conversion"
	// TypeCampaign is used in addresses such as meta.campaign.acquisition.
	TypeCampaign = "campaign"
	// TypeAdSet is used in addresses such as meta.ad_set.instagram.
	TypeAdSet = "ad_set"
	// TypeAdCreative is used in addresses such as meta.ad_creative.instagram.
	TypeAdCreative = "ad_creative"
	// TypeAd is used in addresses such as meta.ad.instagram.
	TypeAd = "ad"

	// OutputImageHash is the declared non-secret Meta image hash produced by
	// meta.image after a successful upload. meta.ad_creative resources resolve
	// image: {$ref: meta.image.name} to this output at apply time.
	OutputImageHash = "imageHash"
	// OutputCustomConversionID is the declared non-secret Custom Conversion id.
	OutputCustomConversionID = "customConversionId"
	// OutputCampaignID is the declared non-secret Meta campaign id.
	OutputCampaignID = "campaignId"
	// OutputAdSetID is the declared non-secret Meta ad-set id.
	OutputAdSetID = "adSetId"
	// OutputAdCreativeID is the declared non-secret Meta ad creative id.
	OutputAdCreativeID = "adCreativeId"
	// OutputAdID is the declared non-secret Meta ad id.
	OutputAdID = "adId"
)
