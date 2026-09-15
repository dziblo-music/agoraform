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
	// For managed local-file assets, use AttrImageRef / AttrVideoRef and
	// declare a meta.image or meta.video resource with source.file.
	AttrImageHash = "imageHash"
	AttrVideoID   = "videoId"
	// AttrImageRef is the managed image reference attribute on meta.ad_creative.
	// It accepts a $ref to a meta.image resource and is mutually exclusive with
	// AttrImageHash. The plan engine resolves the reference at apply time and
	// sends the provider-native image hash to the Meta API.
	AttrImageRef = "image"
	// AttrVideoRef is the managed video reference attribute on meta.ad_creative.
	// It accepts a $ref to a meta.video resource and is mutually exclusive with
	// AttrVideoID. The plan engine resolves the reference at apply time and
	// sends the provider-native video id only after Meta reports the video ready.
	AttrVideoRef = "video"
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
	// TypeVideo is used in addresses such as meta.video.product_demo.
	// A meta.video resource uploads a local file during apply, waits until
	// Meta reports the video ready, and captures the numeric video id for
	// use by meta.ad_creative resources.
	TypeVideo = "video"
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
	// OutputVideoID is the declared non-secret Meta video id produced by
	// meta.video after upload and processing complete. meta.ad_creative
	// resources resolve video: {$ref: meta.video.name} to this output at
	// apply time only when the video is ready to serve.
	OutputVideoID = "videoId"
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
