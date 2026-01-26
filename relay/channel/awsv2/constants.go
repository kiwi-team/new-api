package awsv2

import "strings"

// ChannelName is the name of this channel
const ChannelName = "awsv2"

// awsModelIDMap maps model names to AWS Bedrock model IDs
var awsModelIDMap = map[string]string{
	// Claude models
	"claude-instant-1.2":         "anthropic.claude-instant-v1",
	"claude-2.0":                 "anthropic.claude-v2",
	"claude-2.1":                 "anthropic.claude-v2:1",
	"claude-3-sonnet-20240229":   "anthropic.claude-3-sonnet-20240229-v1:0",
	"claude-3-opus-20240229":     "anthropic.claude-3-opus-20240229-v1:0",
	"claude-3-haiku-20240307":    "anthropic.claude-3-haiku-20240307-v1:0",
	"claude-3-5-sonnet-20240620": "anthropic.claude-3-5-sonnet-20240620-v1:0",
	"claude-3-5-sonnet-20241022": "anthropic.claude-3-5-sonnet-20241022-v2:0",
	"claude-3-5-haiku-20241022":  "anthropic.claude-3-5-haiku-20241022-v1:0",
	"claude-3-7-sonnet-20250219": "anthropic.claude-3-7-sonnet-20250219-v1:0",
	"claude-sonnet-4-20250514":   "anthropic.claude-sonnet-4-20250514-v1:0",
	"claude-opus-4-20250514":     "anthropic.claude-opus-4-20250514-v1:0",
	"claude-opus-4-1-20250805":   "anthropic.claude-opus-4-1-20250805-v1:0",
	"claude-sonnet-4-5-20250929": "anthropic.claude-sonnet-4-5-20250929-v1:0",
	"claude-haiku-4-5-20251001":  "anthropic.claude-haiku-4-5-20251001-v1:0",
	"claude-opus-4-5-20251101":   "anthropic.claude-opus-4-5-20251101-v1:0",
	// Nova models
	"nova-micro-v1:0":   "amazon.nova-micro-v1:0",
	"nova-lite-v1:0":    "amazon.nova-lite-v1:0",
	"nova-pro-v1:0":     "amazon.nova-pro-v1:0",
	"nova-premier-v1:0": "amazon.nova-premier-v1:0",
	// Nova 2 models
	"nova-2-lite-v1:0": "amazon.nova-2-lite-v1:0",
	"nova-2-v1:0":      "amazon.nova-2-v1:0",
}

// awsModelCanCrossRegionMap defines which models can use cross-region inference
var awsModelCanCrossRegionMap = map[string]map[string]bool{
	"anthropic.claude-3-sonnet-20240229-v1:0":   {"us": true, "eu": true, "ap": true},
	"anthropic.claude-3-opus-20240229-v1:0":     {"us": true},
	"anthropic.claude-3-haiku-20240307-v1:0":    {"us": true, "eu": true, "ap": true},
	"anthropic.claude-3-5-sonnet-20240620-v1:0": {"us": true, "eu": true, "ap": true},
	"anthropic.claude-3-5-sonnet-20241022-v2:0": {"us": true, "ap": true},
	"anthropic.claude-3-5-haiku-20241022-v1:0":  {"us": true},
	"anthropic.claude-3-7-sonnet-20250219-v1:0": {"us": true, "ap": true, "eu": true},
	"anthropic.claude-sonnet-4-20250514-v1:0":   {"us": true, "ap": true, "eu": true},
	"anthropic.claude-opus-4-20250514-v1:0":     {"us": true},
	"anthropic.claude-opus-4-1-20250805-v1:0":   {"us": true},
	"anthropic.claude-sonnet-4-5-20250929-v1:0": {"us": true, "ap": true, "eu": true},
	"anthropic.claude-opus-4-5-20251101-v1:0":   {"us": true, "ap": true, "eu": true},
	"anthropic.claude-haiku-4-5-20251001-v1:0":  {"us": true, "ap": true, "eu": true},
	// Nova models
	"amazon.nova-micro-v1:0":   {"us": true, "eu": true, "apac": true},
	"amazon.nova-lite-v1:0":    {"us": true, "eu": true, "apac": true},
	"amazon.nova-pro-v1:0":     {"us": true, "eu": true, "apac": true},
	"amazon.nova-premier-v1:0": {"us": true},
	// Nova 2 models
	"amazon.nova-2-lite-v1:0": {"us": true},
	"amazon.nova-2-v1:0":      {"us": true},
}

var awsRegionCrossModelPrefixMap = map[string]string{
	"us": "us",
	"eu": "eu",
	"ap": "apac",
}

// getAwsModelID returns the AWS model ID for a given model name
func getAwsModelID(requestModel string) string {
	if awsModelIDName, ok := awsModelIDMap[requestModel]; ok {
		return awsModelIDName
	}
	return requestModel
}

// getAwsRegionPrefix extracts the region prefix from a region ID
func getAwsRegionPrefix(awsRegionId string) string {
	parts := strings.Split(awsRegionId, "-")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// awsModelCanCrossRegion checks if a model supports cross-region inference
func awsModelCanCrossRegion(awsModelId, awsRegionPrefix string) bool {
	regionSet, exists := awsModelCanCrossRegionMap[awsModelId]
	return exists && regionSet[awsRegionPrefix]
}

// awsModelCrossRegion returns the cross-region model ID
func awsModelCrossRegion(awsModelId, awsRegionPrefix string) string {
	modelPrefix, find := awsRegionCrossModelPrefixMap[awsRegionPrefix]
	if !find {
		return awsModelId
	}
	return modelPrefix + "." + awsModelId
}

// isNova2Model checks if the model is a Nova 2 model (supports reasoning)
func isNova2Model(modelId string) bool {
	return strings.Contains(modelId, "nova-2")
}
