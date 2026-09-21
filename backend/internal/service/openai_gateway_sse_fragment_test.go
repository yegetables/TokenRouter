//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 回归 2026-09-21 生产问题（opencode 报 "Invalid ... stream event" 中断整轮对话）：
//
// 上游中转站在生成中途断连（http2: client connection lost）时，bufio.ScanLines 会把
// 缓冲区里"没有换行结尾"的半截 SSE 行按 atEOF 语义当成最后一个 token 返回。原样透传后
// 客户端流停在一个没有空行闭合的半截事件上，handler 随后补发的合成错误帧被 SSE 语义
// 并进同一个事件，data 变成 `{半截JSON}\n{"error":...}`——两个 JSON 挤在一个 data 字段，
// 严格客户端解析失败后中断整轮对话。
//
// 修复要求：未以换行结束的残行整行丢弃，且截断判定不能被削弱（残行也算"上游发过数据"）。

const rawSSEFragmentCutChunk = `data: {"id":"chatcmpl_cut","object":"chat.completion.chunk","model":"deepseek-v4-pro","choices":[{"index":0,"delta":{"content":"half an ans"}}]}`

// 上游在行中间断开：完整的上一事件照常输出，没有换行结尾的残行必须被丢弃。
func TestUpstreamSSEScannerDropsUnterminatedFragment(t *testing.T) {
	svc := &OpenAIGatewayService{}
	body := &openAIChatStreamReadErrorCloser{
		payload: []byte(rawSSEFragmentCutChunk + "\n\n" +
			`data: {"id":"chatcmpl_cut","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"cut here`),
		err: errors.New("http2: client connection lost"),
	}

	scanner := svc.newUpstreamSSEScanner(body)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	require.Equal(t, []string{rawSSEFragmentCutChunk, ""}, lines,
		"未以换行结束的残行必须整行丢弃，不能透传给客户端")
	require.True(t, scanner.droppedFragment, "必须记录丢弃过残行，供截断判定使用")
	require.Error(t, scanner.Err(), "底层读错误仍要保留给调用方分类")
}

// 正常流（每行都以换行结束）行为必须完全不变。
func TestUpstreamSSEScannerKeepsWellFormedStream(t *testing.T) {
	svc := &OpenAIGatewayService{}
	scanner := svc.newUpstreamSSEScanner(strings.NewReader("data: a\n\ndata: b\n\ndata: [DONE]\n\n"))

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	require.Equal(t, []string{"data: a", "", "data: b", "", "data: [DONE]", ""}, lines)
	require.False(t, scanner.droppedFragment)
	require.NoError(t, scanner.Err())
}

// 已写出内容后上游断在行中间：残行不得出现在下游，且必须仍判为上游截断。
func TestForwardAsRawChatCompletions_DropsUnterminatedFragmentAfterOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &openAIChatStreamReadErrorCloser{
			payload: []byte(rawSSEFragmentCutChunk + "\n\n" +
				`data: {"id":"chatcmpl_cut","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"cut here`),
			err: errors.New("http2: client connection lost"),
		},
	}}

	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	_, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")
	require.Error(t, err, "上游截断必须报错，不能静默当成成功收尾")

	code, _, ok := OpenAIUpstreamStreamReadErrorDetails(err)
	require.True(t, ok)
	require.Equal(t, OpenAIUpstreamStreamReadErrorCode, code)

	downstream := rec.Body.String()
	require.Contains(t, downstream, `"content":"half an ans"`, "已收到的完整事件仍要透传")
	require.NotContains(t, downstream, "cut here", "半截残行不得透传给客户端")
}

// 残行是唯一数据、且此前只写出过非 data 行（如上游 keepalive 注释）时，
// 截断判定仍必须成立：否则会退回"半截回答被记成 200 成功"的静默截断。
func TestForwardAsRawChatCompletions_FragmentOnlyStreamIsNotSilentSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-v4-pro","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &openAIChatStreamReadErrorCloser{
			// 先写出一行注释（非 data 行，本身不构成截断信号），再断在半截 data 行上。
			// 此时 terminal.sawDataLine 只能来自"丢弃过残行"这一事实。
			payload: []byte(": keepalive\n" +
				`data: {"id":"chatcmpl_cut","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"only`),
			err: errors.New("http2: client connection lost"),
		},
	}}

	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	_, err := svc.forwardAsRawChatCompletions(context.Background(), c, rawChatCompletionsTestAccount(), body, "")
	require.Error(t, err, "只有残行时也不能当成 200 成功收尾")

	code, _, ok := OpenAIUpstreamStreamReadErrorDetails(err)
	require.True(t, ok, "必须带类型化的上游读取错误")
	require.Equal(t, OpenAIUpstreamStreamReadErrorCode, code)
	require.NotContains(t, rec.Body.String(), "only", "残行不得透传")
}
