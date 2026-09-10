package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 上游提供元数据（上下文 + 能力标签）时透传；未提供时保持既有响应结构（字段省略）。
func TestModelsListWritersPassthroughUpstreamContextMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	enabled, disabled := true, false
	metadata := map[string]service.UpstreamModelMetadata{
		"glm-5.1": {
			ContextLength: 200000, MaxCompletionTokens: 128000,
			SupportsVision:    &disabled, // 上游显式声明 false，必须原样透传
			SupportsTools:     &enabled,
			SupportsReasoning: &enabled,
			SupportsAnthropic: &enabled,
			ResponsesModes:    []string{"native"},
			ResponsesCapabilities: json.RawMessage(
				`{"available":true,"modes":["native"],"stream":true,"tools":true}`),
		},
		// 仅提供部分字段的上游：只透传已知字段
		"partial-model": {ContextLength: 1048576},
		// 只声明能力标签、没有上下文数字
		"caps-only": {SupportsVision: &enabled},
	}

	runWriter := func(t *testing.T, write func(*gin.Context)) string {
		t.Helper()
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		write(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		return recorder.Body.String()
	}

	t.Run("OpenAI 形态透传上下文与能力标签并省略缺失字段", func(t *testing.T) {
		body := runWriter(t, func(c *gin.Context) {
			writeOpenAIModelsList(c, []string{"glm-5.1", "partial-model", "caps-only", "no-context-model"}, metadata)
		})
		var payload struct {
			Data []struct {
				ID                  string `json:"id"`
				ContextLength       *int   `json:"context_length"`
				MaxCompletionTokens *int   `json:"max_completion_tokens"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &payload))
		require.Len(t, payload.Data, 4)

		byID := make(map[string]struct {
			ID                  string `json:"id"`
			ContextLength       *int   `json:"context_length"`
			MaxCompletionTokens *int   `json:"max_completion_tokens"`
		}, len(payload.Data))
		for _, item := range payload.Data {
			byID[item.ID] = item
		}

		require.NotNil(t, byID["glm-5.1"].ContextLength)
		require.Equal(t, 200000, *byID["glm-5.1"].ContextLength)
		require.NotNil(t, byID["glm-5.1"].MaxCompletionTokens)
		require.Equal(t, 128000, *byID["glm-5.1"].MaxCompletionTokens)

		require.NotNil(t, byID["partial-model"].ContextLength)
		require.Equal(t, 1048576, *byID["partial-model"].ContextLength)
		require.Nil(t, byID["partial-model"].MaxCompletionTokens, "上游未提供的字段必须省略")

		require.Nil(t, byID["no-context-model"].ContextLength, "上游无元数据的模型保持历史结构")
		require.Nil(t, byID["no-context-model"].MaxCompletionTokens)
		require.NotContains(t, body, `"context_length":0`)

		// 能力标签：显式 false 也要下发（区别于「上游未声明」）
		require.Contains(t, body, `"supports_vision":false`)
		require.Contains(t, body, `"supports_tools":true`)
		require.Contains(t, body, `"supports_reasoning":true`)
		require.Contains(t, body, `"supports_anthropic":true`)
		require.Contains(t, body, `"responses_modes":["native"]`)
		require.Contains(t, body, `"responses_capabilities":{"available":true,"modes":["native"],"stream":true,"tools":true}`)

		// 只声明能力标签的模型：能力标签下发、上下文数字仍然省略
		var capsOnly struct {
			Data []struct {
				ID             string `json:"id"`
				ContextLength  *int   `json:"context_length"`
				SupportsVision *bool  `json:"supports_vision"`
				SupportsTools  *bool  `json:"supports_tools"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &capsOnly))
		for _, item := range capsOnly.Data {
			if item.ID != "caps-only" {
				continue
			}
			require.NotNil(t, item.SupportsVision)
			require.True(t, *item.SupportsVision)
			require.Nil(t, item.ContextLength, "未声明的上下文数字必须省略")
			require.Nil(t, item.SupportsTools, "未声明的能力标签必须省略")
		}

		// 仅声明上下文的模型不应凭空出现能力标签
		require.Contains(t, body, `"context_length":1048576`)
		partialIdx := strings.Index(body, `"id":"partial-model"`)
		require.Positive(t, partialIdx)
	})

	t.Run("Claude 兼容形态透传", func(t *testing.T) {
		body := runWriter(t, func(c *gin.Context) {
			writeModelsList(c, []string{"glm-5.1", "no-context-model"}, metadata)
		})
		require.Contains(t, body, `"context_length":200000`)
		require.Contains(t, body, `"max_completion_tokens":128000`)
		require.True(t, strings.Contains(body, `"id":"glm-5.1"`))
		require.Contains(t, body, `"supports_vision":false`)
		require.Contains(t, body, `"supports_tools":true`)
		require.Contains(t, body, `"responses_modes":["native"]`)
	})

	t.Run("复合 Key 形态透传带前缀的键", func(t *testing.T) {
		prefixed := map[string]service.UpstreamModelMetadata{
			"default/glm-5.1": {
				ContextLength: 200000, MaxCompletionTokens: 128000,
				SupportsTools: &enabled, ResponsesModes: []string{"native"},
			},
		}
		body := runWriter(t, func(c *gin.Context) {
			writeCompositeModelsList(c, []string{"default/glm-5.1", "tra/glm-5"}, prefixed)
		})
		require.Contains(t, body, `"context_length":200000`)
		require.Contains(t, body, `"max_completion_tokens":128000`)
		require.Contains(t, body, `"supports_tools":true`)
		require.NotContains(t, body, `"context_length":0`)
	})

	t.Run("无元数据时与历史响应完全一致", func(t *testing.T) {
		body := runWriter(t, func(c *gin.Context) {
			writeOpenAIModelsList(c, []string{"glm-5.1"}, nil)
		})
		for _, key := range []string{"context_length", "max_completion_tokens",
			"supports_vision", "supports_tools", "supports_reasoning",
			"supports_responses", "supports_anthropic", "responses_modes", "responses_capabilities"} {
			require.NotContains(t, body, key, "无元数据时不得出现 %s", key)
		}
		require.Contains(t, body, `"id":"glm-5.1"`)
	})
}
