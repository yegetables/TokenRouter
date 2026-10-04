package selection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// TestMixedGroupImageCandidateUsesFinalMappedModel 验证图片别名必须映射到图片模型，通配白名单不能把文本模型变成图片模型。
func TestMixedGroupImageCandidateUsesFinalMappedModel(t *testing.T) {
	selector := NewCompatible(CompatibleDependencies{}, DefaultOptions())
	value := mixedGroupProvider(1, capability.PlatformOpenAI, "*", 91)
	value.Record.Credentials["model_mapping"] = map[string]any{"image-alias": "gpt-test"}
	ctx := context.WithValue(context.Background(), imageModelRequiredKey{}, true)
	require.Equal(t, "image_model_required", selector.candidateEligibilityReason(ctx, &value, "", "image-alias", false, ""))
	value.Record.Credentials["model_mapping"] = map[string]any{"image-alias": "gpt-image-1"}
	require.Empty(t, selector.candidateEligibilityReason(ctx, &value, "", "image-alias", false, ""))
}

// TestMixedGroupImageCandidateAllowsThirdPartyCompatModel 检验第三方兼容生图模型能通过图片候选门禁。
// 回归背景：门禁若仍按原生族判定，会把它们判成 image_model_required，唯一账号因此被剔除、请求以 503 结束。
func TestMixedGroupImageCandidateAllowsThirdPartyCompatModel(t *testing.T) {
	selector := NewCompatible(CompatibleDependencies{}, DefaultOptions())
	value := mixedGroupProvider(1, capability.PlatformOpenAI, "*", 91)
	ctx := context.WithValue(context.Background(), imageModelRequiredKey{}, true)
	for _, model := range []string{"gpt-image-1", "grok-imagine", "qwen-image-2.0", "wan2.7-image"} {
		require.Empty(t, selector.candidateEligibilityReason(ctx, &value, "", model, false, ""), model)
	}
	for _, model := range []string{"glm-5.2", "deepseek-flash"} {
		require.Equal(t, "image_model_required", selector.candidateEligibilityReason(ctx, &value, "", model, false, ""), model)
	}
}
