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

	// TypeCustomConversion is used in addresses such as
	// meta.custom_conversion.trial_started.
	TypeCustomConversion = "custom_conversion"

	// OutputCustomConversionID is the declared non-secret Custom Conversion id.
	OutputCustomConversionID = "customConversionId"
)
