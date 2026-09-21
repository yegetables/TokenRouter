package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 回归 2026-09-21 生产问题：上游在 SSE 事件写完 data 行、结束空行尚未写出时断连
// （http2: client connection lost）。服务层已把那一行透传给客户端，handler 补发
// 合成错误帧前必须先闭合该事件；否则客户端拿到的 data 是
// `{chunk}\n{"error":...}`——两个 JSON 挤在一个 data 字段里，严格客户端（opencode）
// 会以 "Invalid ... stream event" 中断整轮对话。
func TestOpenAIHandleStreamingAwareError_ClosesPendingSSEEventBeforeInjectingError(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointChatCompletions)

	// 模拟服务层已写出的最后一行：完整 chunk，但没有结束空行。
	pending := `data: {"id":"chatcmpl_cut","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"cut here"}}]}`
	_, err := c.Writer.WriteString(pending + "\n")
	require.NoError(t, err)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "Upstream response stream was interrupted", true)

	body := w.Body.String()
	require.True(t, strings.HasSuffix(body, "\n\n"), "流必须以事件边界结束: %q", body)

	payloads := sseDataPayloads(t, body)
	require.Len(t, payloads, 2, "应为：已透传的 chunk 事件 + 独立的错误事件，实际 body=%q", body)
	for _, payload := range payloads {
		require.True(t, json.Valid([]byte(payload)), "每个 SSE data 都必须能单独解析为 JSON: %q", payload)
	}
	require.Contains(t, payloads[1], `"error"`)
}

// 未写出任何响应体字节时（只提交过响应头）不得凭空补空行，
// 否则流首会多出一个空事件。
func TestOpenAIHandleStreamingAwareError_NoLeadingBlankEventWithoutBody(t *testing.T) {
	c, w := newGinContextForEndpoint(t, EndpointChatCompletions)

	h := &OpenAIGatewayHandler{}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "boom", true)

	require.True(t, strings.HasPrefix(w.Body.String(), "event: error\n"),
		"没有写出过响应体时不应补前导空行: %q", w.Body.String())
}

// sseDataPayloads 按 SSE 事件边界切分 body，返回每个事件的 data 字段
// （同一事件内的多行 data 按 SSE 规范用 \n 连接）。
func sseDataPayloads(t *testing.T, body string) []string {
	t.Helper()
	var out []string
	for _, block := range strings.Split(body, "\n\n") {
		var data []string
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		if len(data) > 0 {
			out = append(out, strings.Join(data, "\n"))
		}
	}
	return out
}
