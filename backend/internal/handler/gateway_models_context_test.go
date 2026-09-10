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

// 上游提供上下文元数据时透传；未提供时保持既有响应结构（字段省略）。
func TestModelsListWritersPassthroughUpstreamContextMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	metadata := map[string]service.ModelContextMetadata{
		"glm-5.1": {ContextLength: 200000, MaxCompletionTokens: 128000},
		// 仅提供部分字段的上游：只透传已知字段
		"partial-model": {ContextLength: 1048576},
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

	t.Run("OpenAI 形态透传上下文并省略缺失字段", func(t *testing.T) {
		body := runWriter(t, func(c *gin.Context) {
			writeOpenAIModelsList(c, []string{"glm-5.1", "partial-model", "no-context-model"}, metadata)
		})
		var payload struct {
			Data []struct {
				ID                  string `json:"id"`
				ContextLength       *int   `json:"context_length"`
				MaxCompletionTokens *int   `json:"max_completion_tokens"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &payload))
		require.Len(t, payload.Data, 3)

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
	})

	t.Run("Claude 兼容形态透传", func(t *testing.T) {
		body := runWriter(t, func(c *gin.Context) {
			writeModelsList(c, []string{"glm-5.1", "no-context-model"}, metadata)
		})
		require.Contains(t, body, `"context_length":200000`)
		require.Contains(t, body, `"max_completion_tokens":128000`)
		require.True(t, strings.Contains(body, `"id":"glm-5.1"`))
	})

	t.Run("复合 Key 形态透传带前缀的键", func(t *testing.T) {
		prefixed := map[string]service.ModelContextMetadata{
			"default/glm-5.1": {ContextLength: 200000, MaxCompletionTokens: 128000},
		}
		body := runWriter(t, func(c *gin.Context) {
			writeCompositeModelsList(c, []string{"default/glm-5.1", "tra/glm-5"}, prefixed)
		})
		require.Contains(t, body, `"context_length":200000`)
		require.Contains(t, body, `"max_completion_tokens":128000`)
		require.NotContains(t, body, `"context_length":0`)
	})

	t.Run("无元数据时与历史响应完全一致", func(t *testing.T) {
		body := runWriter(t, func(c *gin.Context) {
			writeOpenAIModelsList(c, []string{"glm-5.1"}, nil)
		})
		require.NotContains(t, body, "context_length")
		require.NotContains(t, body, "max_completion_tokens")
		require.Contains(t, body, `"id":"glm-5.1"`)
	})
}
