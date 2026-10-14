package modelcatalog

import "strings"

// openAICompatImageModels lists the registered families in match order. A
// shorter prefix must not shadow a longer one, so specific names come first.
var openAICompatImageModels = []OpenAICompatImageModel{
	{
		// This endpoint takes the aspect ratio in size and returns base64 by
		// default, so only the ratio list and the ratio encoding are registered.
		Prefix:       "gemini-3.1-flash-lite-image",
		AspectRatios: []string{"1:1", "2:3", "9:16", "4:3"},
		SizeAsRatio:  true,
	},
	{
		// This endpoint requires WIDTHxHEIGHT in size and returns a URL unless
		// the request asks for base64, so both deviations are registered.
		Prefix:                "qwen-image",
		AspectRatios:          []string{"1:1", "4:3", "3:4", "16:9", "9:16"},
		RequireBase64Response: true,
	},
	{
		// Same request contract as qwen-image.
		Prefix:                "wan2.7-image",
		AspectRatios:          []string{"1:1", "4:3", "3:4", "16:9", "9:16"},
		RequireBase64Response: true,
	},
}

// Third-party models served through an OpenAI-compatible images endpoint do not
// follow the native GPT Image request shape. Each registration records the model
// name prefix, the aspect ratios the endpoint accepts, and the request
// deviations the creative adapter must apply.
//
// This registry is the single source for those model names. Image recognition,
// the creative capability lists, and the OpenAI-compatible request encoding all
// read it, so a new compatible model is registered once.

// OpenAICompatImageModel describes one registered third-party image model family.
type OpenAICompatImageModel struct {
	Prefix       string   // normalized model name prefix that selects the family
	AspectRatios []string // aspect ratios the endpoint accepts, in display order
	// SizeAsRatio makes the adapter send the aspect ratio in the size field.
	// A false value sends a WIDTHxHEIGHT pair instead.
	SizeAsRatio bool
	// RequireBase64Response makes the adapter ask for response_format=b64_json.
	// A false value leaves the field off, which suits an endpoint that already
	// returns base64.
	RequireBase64Response bool
}

// OpenAICompatImageModelFor returns the registered family whose prefix matches
// the normalized model name, or nil when the name is not registered. Callers
// use nil to tell an unregistered name from a registered family.
func OpenAICompatImageModelFor(model string) *OpenAICompatImageModel {
	normalized := strings.TrimPrefix(normalize(model), "models/")
	if normalized == "" {
		return nil
	}
	for i := range openAICompatImageModels {
		if strings.HasPrefix(normalized, openAICompatImageModels[i].Prefix) {
			return &openAICompatImageModels[i]
		}
	}
	return nil
}

// OpenAICompatImageModels returns a copy of the registered families. The copies
// share their aspect ratio slices with the registry, so callers must not edit
// them.
func OpenAICompatImageModels() []OpenAICompatImageModel {
	return append([]OpenAICompatImageModel(nil), openAICompatImageModels...)
}
