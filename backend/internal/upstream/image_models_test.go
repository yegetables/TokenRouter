package upstream

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIsImageGenerationModelCoversNativeRegisteredAndKeywordModels maps names
// from each recognition source. It protects the shared decision: the native
// families, a registered third-party prefix, and an unregistered name that
// merely contains "image" all pass, while ordinary text models do not.
func TestIsImageGenerationModelCoversNativeRegisteredAndKeywordModels(t *testing.T) {
	for _, model := range []string{
		"gpt-image-1",                      // native GPT Image family
		"grok-imagine",                     // native Grok family, without the "image" substring
		"qwen-image-2.0",                   // registered third-party prefix
		"wan2.7-image",                     // registered third-party prefix
		"gemini-3.1-flash-lite-image",      // registered third-party prefix
		"some-vendor-vision-image-preview", // unregistered, but the name contains "image"
	} {
		require.True(t, IsImageGenerationModel(model), model)
	}
	for _, model := range []string{"", "glm-5.2", "deepseek-flash", "kimi-k3"} {
		require.False(t, IsImageGenerationModel(model), model)
	}
}

// TestIsOpenAICompatImageModelUsesRegisteredPrefixes maps registered and
// unregistered names. It protects the prefix registration as a source separate
// from the "image" keyword: a model with neither is not recognized.
func TestIsOpenAICompatImageModelUsesRegisteredPrefixes(t *testing.T) {
	require.True(t, IsOpenAICompatImageModel("qwen-image-2.0"))
	require.True(t, IsOpenAICompatImageModel("wan2.7-image"))
	require.True(t, IsOpenAICompatImageModel("models/gemini-3.1-flash-lite-image"))
	require.False(t, IsOpenAICompatImageModel("some-vendor-draw-model"))
	require.False(t, IsOpenAICompatImageModel(""))
}
