package upstream

import "strings"

// IsGeminiImageGenerationModel 判断模型是否为图片生成模型
// 支持的模型：gemini-3.1-flash-image, gemini-3-pro-image, gemini-2.5-flash-image 等
func IsGeminiImageGenerationModel(model string) bool {
	modelLower := strings.ToLower(model)
	// 移除 models/ 前缀
	modelLower = strings.TrimPrefix(modelLower, "models/")

	// 精确匹配或前缀匹配
	return modelLower == "gemini-3.1-flash-image" ||
		modelLower == "gemini-3.1-flash-image-preview" ||
		strings.HasPrefix(modelLower, "gemini-3.1-flash-image-") ||
		modelLower == "gemini-3-pro-image" ||
		modelLower == "gemini-3-pro-image-preview" ||
		strings.HasPrefix(modelLower, "gemini-3-pro-image-") ||
		modelLower == "gemini-2.5-flash-image" ||
		modelLower == "gemini-2.5-flash-image-preview" ||
		strings.HasPrefix(modelLower, "gemini-2.5-flash-image-")
}

// IsGPTImageGenerationModel 判断模型是否属于 GPT 原生生图模型族。
func IsGPTImageGenerationModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "gpt-image-")
}

func IsGrokImageGenerationModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "grok-imagine" ||
		model == "grok-imagine-edit" ||
		strings.HasPrefix(model, "grok-imagine-image")
}

// 已登记的第三方 OpenAI 兼容生图模型前缀。登记后的模型即使名称不含 image 也会被识别。
const (
	OpenAICompatImagePrefixQwenImage       = "qwen-image"
	OpenAICompatImagePrefixWanImage        = "wan2.7-image"
	OpenAICompatImagePrefixGeminiFlashLite = "gemini-3.1-flash-lite-image"
)

var openAICompatImageModelPrefixes = []string{
	OpenAICompatImagePrefixQwenImage,
	OpenAICompatImagePrefixWanImage,
	OpenAICompatImagePrefixGeminiFlashLite,
}

// IsOpenAICompatImageModel 判断模型是否为已登记的第三方 OpenAI 兼容生图模型。
func IsOpenAICompatImageModel(model string) bool {
	normalized := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/")
	if normalized == "" {
		return false
	}
	for _, prefix := range openAICompatImageModelPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

// IsImageGenerationModel 判断模型是否为可承接图片入口的生图模型：
// 官方原生生图族（GPT Image / Grok Imagine）、已登记的第三方 OpenAI 兼容生图模型，
// 或名称含 image 的兼容模型。账号选路、模型目录投影与该模型的图片计费共用此判定。
func IsImageGenerationModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return false
	}
	return IsGPTImageGenerationModel(normalized) ||
		IsGrokImageGenerationModel(normalized) ||
		IsOpenAICompatImageModel(normalized) ||
		strings.Contains(normalized, "image")
}

// DefaultImageTaskGeminiModels 保留创作与批量图片的默认候选及顺序。
func DefaultImageTaskGeminiModels() []string {
	return []string{
		"gemini-2.0-flash-exp-image-generation",
		"gemini-2.5-flash-image",
		"gemini-3-pro-image",
		"gemini-3-pro-image-preview",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
		"gemini-3.1-flash-lite-image",
	}
}
