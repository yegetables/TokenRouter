// 第三方 OpenAI 兼容生图模型的契约注册表：执行器与目录共用同一来源。
package creative

import (
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// creativeOpenAICompatImageProfiles 按模型名前缀登记第三方生图模型契约，匹配小写归一后的模型名。
// 编辑源图上限沿用能力表的初始值 1，与这三个上游的实际能力一致，因此不单独登记。
var creativeOpenAICompatImageProfiles = []struct {
	prefix  string
	profile CreativeOpenAICompatImageProfile
}{
	{
		// baipiao 上游：size 传比例，默认返回 base64。
		prefix: upstream.OpenAICompatImagePrefixGeminiFlashLite,
		profile: CreativeOpenAICompatImageProfile{
			AspectRatios: []string{"1:1", "2:3", "9:16", "4:3"},
			SizeAsRatio:  true,
		},
	},
	{
		// 基元律动官方 qwen-image-2.0：size 必须为 WIDTHxHEIGHT，默认返回 url，需显式要 base64。
		prefix: upstream.OpenAICompatImagePrefixQwenImage,
		profile: CreativeOpenAICompatImageProfile{
			AspectRatios:          []string{"1:1", "4:3", "3:4", "16:9", "9:16"},
			RequireResponseFormat: true,
		},
	},
	{
		// 基元律动官方 wan2.7-image：契约同 qwen-image。
		prefix: upstream.OpenAICompatImagePrefixWanImage,
		profile: CreativeOpenAICompatImageProfile{
			AspectRatios:          []string{"1:1", "4:3", "3:4", "16:9", "9:16"},
			RequireResponseFormat: true,
		},
	},
}

// CreativeOpenAICompatImageProfile 描述经 OpenAI 兼容 images 协议接入的第三方生图模型契约。
// 新增同类模型只需在 creativeOpenAICompatImageProfiles 登记一条，无需改动执行器与目录逻辑。
type CreativeOpenAICompatImageProfile struct {
	// AspectRatios 是暴露给创作台的比例集合。不按契约收窄会让用户选到上游不支持的比例。
	AspectRatios []string
	// SizeAsRatio 为 true 时请求 size 传比例串（如 9:16）；否则传 WIDTHxHEIGHT 像素尺寸。
	SizeAsRatio bool
	// RequireResponseFormat 为 true 时显式请求 response_format=b64_json。
	// 上游默认只回 data[].url 时必须开启，否则创作台解析不到图片本体。
	RequireResponseFormat bool
}

// CreativeOpenAICompatImageProfileFor 返回模型命中的第三方生图契约；未登记返回 nil。
func CreativeOpenAICompatImageProfileFor(model string) *CreativeOpenAICompatImageProfile {
	normalized := CreativeNormalizedModelID(model)
	for i := range creativeOpenAICompatImageProfiles {
		if strings.HasPrefix(normalized, creativeOpenAICompatImageProfiles[i].prefix) {
			return &creativeOpenAICompatImageProfiles[i].profile
		}
	}
	return nil
}

// IsCreativeOpenAICompatImageModel 判断是否为已登记的第三方 OpenAI 兼容生图模型。
// 登记名单由 upstream 唯一持有，创作台只读取，不另存一份前缀。
func IsCreativeOpenAICompatImageModel(model string) bool {
	return upstream.IsOpenAICompatImageModel(model)
}

// IsCreativeOpenAIImageModel 判断 OpenAI 兼容平台上的生图模型：GPT Image 原生族与已登记第三方模型。
// 执行器、能力表、尺寸档位与候选模型都必须走这一个判定，避免目录能列出而执行器拒绝。
func IsCreativeOpenAIImageModel(model string) bool {
	return upstream.IsGPTImageGenerationModel(model) || IsCreativeOpenAICompatImageModel(model)
}
