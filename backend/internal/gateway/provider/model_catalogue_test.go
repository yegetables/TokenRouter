package provider

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// 回归：模型目录的 Images/Edits 分支原先只认 GPT Image / Grok Imagine 原生族，
// 经 OpenAI 兼容协议接入的第三方生图模型（qwen-image-2.0 等）不会被投影为该提供商
// 的可请求模型，导致 /v1/images/* 在选路前就没有候选。放行口径必须与 images 端点门禁一致。
func TestCatalogueAllowsThirdPartyCompatImageModels(t *testing.T) {
	rules := catalogueRules{policy: ModelPolicy{Record: &provider.Record{Platform: capability.PlatformOpenAI}}}
	imageEntryProtocols := []capability.ProtocolID{
		capability.ProtocolImagesGenerations,
		capability.ProtocolImagesEdits,
	}
	for _, model := range []string{
		"gpt-image-1",
		"grok-imagine",
		"qwen-image-2.0",
		"wan2.7-image",
		"gemini-3.1-flash-lite-image",
	} {
		for _, source := range imageEntryProtocols {
			require.True(t, rules.SupportsClientProtocol(model, source), "%s @ %s", model, source)
		}
	}
	// 普通文本模型进入图片入口仍必须被拒，且不受关键字放行影响。
	for _, model := range []string{"glm-5.2", "deepseek-flash"} {
		for _, source := range imageEntryProtocols {
			require.False(t, rules.SupportsClientProtocol(model, source), "%s @ %s", model, source)
		}
		require.True(t, rules.SupportsClientProtocol(model, capability.ProtocolOpenAIChatCompletions), model)
	}
}
