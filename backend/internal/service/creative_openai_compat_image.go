package service

import "strings"

// creativeOpenAICompatImageProfile 描述经 OpenAI 兼容 images 协议接入的第三方生图模型契约。
// 新增同类模型只需在 creativeOpenAICompatImageProfiles 登记一条，无需改动执行器与目录逻辑。
type creativeOpenAICompatImageProfile struct {
	// aspectRatios 是暴露给创作台的比例集合。
	aspectRatios []string
	// sizeAsRatio 为 true 时请求 size 传比例串（如 9:16）；否则传 WIDTHxHEIGHT 像素尺寸。
	sizeAsRatio bool
	// requireResponseFormat 为 true 时显式请求 response_format=b64_json。
	// 上游默认只回 data[].url 时必须开启，否则创作台解析不到图片本体。
	requireResponseFormat bool
	// maxReferenceImages 是编辑允许上传的源图上限。
	maxReferenceImages int
}

// creativeOpenAICompatImageProfiles 按模型名前缀登记第三方生图模型契约，匹配小写归一后的模型名。
var creativeOpenAICompatImageProfiles = []struct {
	prefix  string
	profile creativeOpenAICompatImageProfile
}{
	{
		// baipiao 上游：size 传比例，默认返回 base64，编辑仅支持单张源图。
		prefix: "gemini-3.1-flash-lite-image",
		profile: creativeOpenAICompatImageProfile{
			aspectRatios:       []string{"1:1", "2:3", "9:16", "4:3"},
			sizeAsRatio:        true,
			maxReferenceImages: 1,
		},
	},
	{
		// 基元律动官方 qwen-image-2.0：size 必须为 WIDTHxHEIGHT，默认返回 url，需显式要 base64。
		prefix: "qwen-image",
		profile: creativeOpenAICompatImageProfile{
			aspectRatios:          []string{"1:1", "4:3", "3:4", "16:9", "9:16"},
			requireResponseFormat: true,
			maxReferenceImages:    1,
		},
	},
	{
		// 基元律动官方 wan2.7-image：契约同 qwen-image。
		prefix: "wan2.7-image",
		profile: creativeOpenAICompatImageProfile{
			aspectRatios:          []string{"1:1", "4:3", "3:4", "16:9", "9:16"},
			requireResponseFormat: true,
			maxReferenceImages:    1,
		},
	},
}

// creativeOpenAICompatImageProfileFor 返回模型命中的第三方生图契约；未登记返回 nil。
func creativeOpenAICompatImageProfileFor(model string) *creativeOpenAICompatImageProfile {
	normalized := creativeNormalizedModelID(model)
	for i := range creativeOpenAICompatImageProfiles {
		if strings.HasPrefix(normalized, creativeOpenAICompatImageProfiles[i].prefix) {
			return &creativeOpenAICompatImageProfiles[i].profile
		}
	}
	return nil
}

// isCreativeOpenAIThirdPartyImageModel 判断是否为已登记的第三方 OpenAI 兼容生图模型。
func isCreativeOpenAIThirdPartyImageModel(model string) bool {
	return creativeOpenAICompatImageProfileFor(model) != nil
}

// isCreativeOpenAIImageModel 判断 OpenAI 兼容平台上的生图模型：GPT Image 原生族与已登记第三方模型。
func isCreativeOpenAIImageModel(model string) bool {
	return IsGPTImageGenerationModel(model) || isCreativeOpenAIThirdPartyImageModel(model)
}
