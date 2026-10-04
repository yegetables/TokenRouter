package creative

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 回归：第三方 OpenAI 兼容生图模型（qwen-image-2.0 / wan2.7-image /
// gemini-3.1-flash-lite-image）必须同时通过执行器门禁与能力表，
// 否则会出现"目录能列出、执行必失败"的不可重试错误。
func TestCreativePlatformImageModelAllowsThirdPartyCompatModels(t *testing.T) {
	for _, model := range []string{"gpt-image-1", "gpt-image-2", "qwen-image-2.0", "wan2.7-image", "gemini-3.1-flash-lite-image"} {
		t.Run(model, func(t *testing.T) {
			require.True(t, IsCreativeOpenAIImageModel(model))
			require.True(t, CreativePlatformImageModel(PlatformOpenAI, model), "执行器门禁必须放行")
		})
	}
}

func TestCreativePlatformImageModelRejectsTextModels(t *testing.T) {
	for _, model := range []string{"gpt-5.4", "deepseek-flash", "glm-5.3"} {
		t.Run(model, func(t *testing.T) {
			require.False(t, IsCreativeOpenAIImageModel(model))
			require.False(t, CreativePlatformImageModel(PlatformOpenAI, model))
		})
	}
}

// 比例式模型（size 传比例串）固定 1K 档位；像素式模型沿用平台档位。
func TestCreativeFilterImageSizesForModelThirdPartyProfiles(t *testing.T) {
	sizes := []string{"1K", "2K", "4K"}

	require.Equal(t, []string{"1K"},
		CreativeFilterImageSizesForModel(PlatformOpenAI, "gemini-3.1-flash-lite-image", sizes),
		"比例式模型的分辨率档位无意义，固定 1K")

	require.Equal(t, sizes,
		CreativeFilterImageSizesForModel(PlatformOpenAI, "qwen-image-2.0", sizes),
		"像素式第三方模型沿用平台档位")
}

// 能力表按契约暴露比例与编辑源图上限；不支持的 quality/background 留空。
func TestCreativeCapabilitiesForModelThirdPartyProfiles(t *testing.T) {
	caps := CreativeCapabilitiesForModel(PlatformOpenAI, "gemini-3.1-flash-lite-image")
	require.Equal(t, []string{"1:1", "2:3", "9:16", "4:3"}, caps.AspectRatios)
	require.Equal(t, 1, caps.MaxReferenceImages)
	require.Empty(t, caps.Qualities, "第三方模型不支持 quality")
	require.Empty(t, caps.BackgroundOptions, "第三方模型不支持 background")

	caps = CreativeCapabilitiesForModel(PlatformOpenAI, "qwen-image-2.0")
	require.Equal(t, []string{"1:1", "4:3", "3:4", "16:9", "9:16"}, caps.AspectRatios)
	require.Equal(t, 1, caps.MaxReferenceImages)

	// 未登记的普通文本模型仍然只拿到空能力集。
	text := CreativeCapabilitiesForModel(PlatformOpenAI, "gpt-5.4")
	require.Empty(t, text.AspectRatios)
}

func TestCreativeOpenAICompatImageProfilePrefixMatch(t *testing.T) {
	require.NotNil(t, CreativeOpenAICompatImageProfileFor("qwen-image-2.0"))
	require.NotNil(t, CreativeOpenAICompatImageProfileFor("Qwen-Image-3.0"), "按小写归一后前缀匹配")
	require.Nil(t, CreativeOpenAICompatImageProfileFor("gpt-image-1"))
	require.Nil(t, CreativeOpenAICompatImageProfileFor(""))
}
