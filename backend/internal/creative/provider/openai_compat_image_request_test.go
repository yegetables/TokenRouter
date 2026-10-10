package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/creative"
)

// 比例式第三方模型请求 size 传比例串，像素式与 GPT Image 传 WIDTHxHEIGHT。
func TestCreativeOpenAIRequestImageSizeByProfile(t *testing.T) {
	require.Equal(t, "9:16", CreativeOpenAIRequestImageSize("gemini-3.1-flash-lite-image", "1K", "9:16"))
	require.Equal(t, "1:1", CreativeOpenAIRequestImageSize("gemini-3.1-flash-lite-image", "1K", "7:5"),
		"未知比例回退契约第一项")
	require.Equal(t, "1536x1024", CreativeOpenAIRequestImageSize("qwen-image-2.0", "1K", "16:9"))
	require.Equal(t, "1536x1024", CreativeOpenAIRequestImageSize("gpt-image-1", "1K", "16:9"))
}

// 第三方模型不发送 GPT Image 专有的 output_format，且按契约显式索要 b64_json。
func TestBuildCreativeOpenAIRequestBodyThirdPartyContract(t *testing.T) {
	run := creative.CreativeRun{Operation: creative.CreativeOperationGenerate, ImageSize: "1K", AspectRatio: "9:16"}
	payload := creative.CreativeRunPayload{Prompt: "draw a cat"}

	body, contentType, err := BuildCreativeOpenAIRequestBody(run, payload, "gemini-3.1-flash-lite-image")
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.Equal(t, "9:16", parsed["size"])
	require.NotContains(t, parsed, "output_format", "第三方模型不得携带 output_format")
	require.NotContains(t, parsed, "response_format", "该契约默认返回 base64，无需显式索要")

	// 需要显式索要 base64 的第三方模型。
	body, _, err = BuildCreativeOpenAIRequestBody(creative.CreativeRun{
		Operation: creative.CreativeOperationGenerate, ImageSize: "1K", AspectRatio: "16:9",
	}, payload, "qwen-image-2.0")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.Equal(t, "b64_json", parsed["response_format"])
	require.NotContains(t, parsed, "output_format")
	require.Equal(t, "1536x1024", parsed["size"])

	// GPT Image 原生族行为不变。
	body, _, err = BuildCreativeOpenAIRequestBody(run, payload, "gpt-image-1")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.Equal(t, "png", parsed["output_format"])
	require.Equal(t, "1024x1536", parsed["size"])
}
