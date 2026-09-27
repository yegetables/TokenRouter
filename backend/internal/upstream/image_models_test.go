package upstream

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 回归：系统原先只认 GPT/Grok 原生生图族，第三方兼容生图模型（如基元律动的
// qwen-image-2.0）在模型目录投影、选号与图片计费里都被判成非生图模型。
// 判定口径固定为：原生生图族 || 已登记前缀 || 名称含 image。
func TestIsImageGenerationModelCoversNativeRegisteredAndKeywordModels(t *testing.T) {
	for _, model := range []string{
		"gpt-image-1",                      // 原生生图族
		"grok-imagine",                     // 原生生图族（不含 image 子串）
		"qwen-image-2.0",                   // 已登记前缀
		"wan2.7-image",                     // 已登记前缀
		"gemini-3.1-flash-lite-image",      // 已登记前缀
		"some-vendor-vision-image-preview", // 未登记但名称含 image
	} {
		require.True(t, IsImageGenerationModel(model), model)
	}
	for _, model := range []string{"", "glm-5.2", "deepseek-flash", "kimi-k3"} {
		require.False(t, IsImageGenerationModel(model), model)
	}
}

// 登记前缀是独立于名称关键字的识别来源：未登记且不含 image 的模型不会被误判。
func TestIsOpenAICompatImageModelUsesRegisteredPrefixes(t *testing.T) {
	require.True(t, IsOpenAICompatImageModel("qwen-image-2.0"))
	require.True(t, IsOpenAICompatImageModel("wan2.7-image"))
	require.True(t, IsOpenAICompatImageModel("models/gemini-3.1-flash-lite-image"))
	require.False(t, IsOpenAICompatImageModel("some-vendor-draw-model"))
	require.False(t, IsOpenAICompatImageModel(""))
}
