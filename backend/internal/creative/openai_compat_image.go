package creative

import (
	"github.com/TokenFlux/TokenRouter/internal/modelcatalog"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// Third-party image models served through an OpenAI-compatible endpoint need
// the same recognition as the native GPT Image family, and their requests need
// two deviations from the native shape. The model catalogue holds those
// registrations; the helpers here turn one registration into the recognition
// predicate and the request profile the creative adapter reads.

// CreativeOpenAICompatImageProfile describes how a registered third-party
// OpenAI-compatible image model deviates from the native request shape.
type CreativeOpenAICompatImageProfile struct {
	// SizeAsRatio makes the request carry the aspect ratio in the size field
	// instead of a WIDTHxHEIGHT pair.
	SizeAsRatio bool
	// RequireBase64Response adds response_format=b64_json, which the endpoint
	// needs when it returns a URL by default and the worker needs image bytes.
	RequireBase64Response bool
}

// CreativeOpenAICompatImageProfileFor returns the request profile of a
// registered third-party OpenAI-compatible image model, or nil for the native
// GPT Image family and for unregistered names. Aspect ratios are not part of
// the profile; CreativeCapabilitiesForModel reads them from the catalogue.
func CreativeOpenAICompatImageProfileFor(model string) *CreativeOpenAICompatImageProfile {
	if upstream.IsGPTImageGenerationModel(model) {
		return nil
	}
	registered := modelcatalog.OpenAICompatImageModelFor(model)
	if registered == nil {
		return nil
	}
	return &CreativeOpenAICompatImageProfile{
		SizeAsRatio:           registered.SizeAsRatio,
		RequireBase64Response: registered.RequireBase64Response,
	}
}

// IsCreativeOpenAICompatImageModel reports whether model is a registered
// third-party image model on an OpenAI-compatible endpoint.
func IsCreativeOpenAICompatImageModel(model string) bool {
	return CreativeOpenAICompatImageProfileFor(model) != nil
}

// IsCreativeOpenAIImageModel reports whether model generates images on the
// OpenAI platform: the native GPT Image family or a registered third-party
// model. The executor, the capability table, the size tiers and candidate
// discovery share this decision, so a model the catalogue lists is not refused
// by one of the others.
func IsCreativeOpenAIImageModel(model string) bool {
	return upstream.IsGPTImageGenerationModel(model) || IsCreativeOpenAICompatImageModel(model)
}
