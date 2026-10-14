package modelcatalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOpenAICompatImageModelForMatchesRegisteredPrefixes maps registered and
// unregistered names. It protects prefix matching, including a models/ prefix,
// and the empty-name case that must not match.
func TestOpenAICompatImageModelForMatchesRegisteredPrefixes(t *testing.T) {
	for _, model := range []string{"qwen-image-2.0", "wan2.7-image", "models/gemini-3.1-flash-lite-image"} {
		require.NotNil(t, OpenAICompatImageModelFor(model), model)
	}
	for _, model := range []string{"", "gpt-image-1", "some-vendor-draw-model"} {
		require.Nil(t, OpenAICompatImageModelFor(model), model)
	}
}

// TestMediaForModelReportsRegisteredCompatImageModels maps the media lookup for
// a registered family and the native family. It protects the registration as
// the source of the aspect ratios and the single size tier, while the native
// GPT Image family keeps its own sizes and qualities.
func TestMediaForModelReportsRegisteredCompatImageModels(t *testing.T) {
	compat := MediaForModel("openai", "qwen-image-2.0")
	require.NotNil(t, compat)
	require.Equal(t, []string{"1K"}, compat.ImageSizes)
	require.Equal(t, []string{"1:1", "4:3", "3:4", "16:9", "9:16"}, compat.AspectRatios)
	require.Empty(t, compat.Qualities)

	native := MediaForModel("openai", "gpt-image-1")
	require.NotNil(t, native)
	require.Equal(t, []string{"1K", "2K"}, native.ImageSizes)
	require.Contains(t, native.Qualities, "high")

	require.Nil(t, MediaForModel("openai", "glm-5.2"))
}
