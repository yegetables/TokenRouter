package media

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// 回归：images 端点原先只放行 gpt-image-* 与 grok-imagine*，导致经 OpenAI 兼容协议接入的
// 第三方生图模型（qwen-image-2.0 等）被 400 拒绝；普通文本模型仍必须被拒。
func TestValidateImageModelAllowsThirdPartyImageAliases(t *testing.T) {
	for _, model := range []string{
		"qwen-image-2.0",
		"wan2.7-image",
		"gemini-3.1-flash-lite-image",
		"gpt-image-1",
		// grok-imagine 不含 "image" 子串（imagine ≠ image），必须靠原生判定放行。
		"grok-imagine",
	} {
		t.Run(model, func(t *testing.T) {
			require.NoError(t, ValidateImageModel(model))
			require.True(t, IsImageBillingModelAlias(model))
		})
	}
}

func TestValidateImageModelRejectsNonImageModels(t *testing.T) {
	for _, model := range []string{"gpt-5.4", "deepseek-flash", "glm-5.3"} {
		t.Run(model, func(t *testing.T) {
			require.ErrorContains(t, ValidateImageModel(model), "images endpoint requires an image model")
			require.False(t, IsImageBillingModelAlias(model))
		})
	}
}
