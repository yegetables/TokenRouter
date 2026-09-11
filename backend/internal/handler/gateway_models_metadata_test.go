package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	middleware2 "github.com/TokenFlux/TokenRouter/internal/server/middleware"
	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gatewayMetadataItemForTest struct {
	ID                    string          `json:"id"`
	ContextLength         *int64          `json:"context_length"`
	ContextWindow         *int64          `json:"context_window"`
	MaxCompletionTokens   *int64          `json:"max_completion_tokens"`
	MaxOutputTokens       *int64          `json:"max_output_tokens"`
	SupportsTools         *bool           `json:"supports_tools"`
	SupportsReasoning     *bool           `json:"supports_reasoning"`
	SupportsVision        *bool           `json:"supports_vision"`
	InputModalities       []string        `json:"input_modalities"`
	ResponsesModes        []string        `json:"responses_modes"`
	ResponsesCapabilities json.RawMessage `json:"responses_capabilities"`
	Pricing               json.RawMessage `json:"pricing"`
}

func metadataCatalogForTest() map[string]service.UpstreamModelMetadata {
	contextWindow := int64(200000)
	maxOutput := int64(128000)
	supportsTools := true
	supportsVision := false
	reasoning := true
	return map[string]service.UpstreamModelMetadata{
		"glm-5.1": {
			ID:                    "glm-5.1",
			ContextWindow:         contextWindow,
			MaxOutputTokens:       maxOutput,
			Reasoning:             &reasoning,
			SupportsTools:         &supportsTools,
			SupportsVision:        &supportsVision,
			InputModalities:       []string{"text"},
			ResponsesModes:        []string{"stream"},
			ResponsesCapabilities: json.RawMessage(`{"available":true}`),
		},
	}
}

func TestWriteOpenAIModelsListAttachesUpstreamMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeOpenAIModelsList(c, []string{"glm-5.1", "unmapped-model"}, metadataCatalogForTest())

	require.Equal(t, http.StatusOK, recorder.Code)
	var got struct {
		Data []gatewayMetadataItemForTest `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
	require.Len(t, got.Data, 2)

	hit := got.Data[0]
	require.Equal(t, "glm-5.1", hit.ID)
	require.NotNil(t, hit.ContextLength)
	require.Equal(t, int64(200000), *hit.ContextLength)
	require.NotNil(t, hit.ContextWindow)
	require.Equal(t, int64(200000), *hit.ContextWindow)
	require.NotNil(t, hit.MaxCompletionTokens)
	require.Equal(t, int64(128000), *hit.MaxCompletionTokens)
	require.NotNil(t, hit.MaxOutputTokens)
	require.Equal(t, int64(128000), *hit.MaxOutputTokens)
	require.NotNil(t, hit.SupportsTools)
	require.True(t, *hit.SupportsTools)
	require.NotNil(t, hit.SupportsVision)
	require.False(t, *hit.SupportsVision)
	require.NotNil(t, hit.SupportsReasoning)
	require.True(t, *hit.SupportsReasoning)
	require.Equal(t, []string{"text"}, hit.InputModalities)
	require.Equal(t, []string{"stream"}, hit.ResponsesModes)
	require.JSONEq(t, `{"available":true}`, string(hit.ResponsesCapabilities))

	miss := got.Data[1]
	require.Equal(t, "unmapped-model", miss.ID)
	require.Nil(t, miss.ContextLength)
	require.Nil(t, miss.ContextWindow)
	require.Nil(t, miss.MaxCompletionTokens)
	require.Nil(t, miss.SupportsTools)
	require.Empty(t, miss.ResponsesCapabilities)
	require.Empty(t, miss.Pricing)
}

func TestWriteModelsListAttachesUpstreamMetadataOnClaudeShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeModelsList(c, []string{"glm-5.1"}, metadataCatalogForTest())

	require.Equal(t, http.StatusOK, recorder.Code)
	var got struct {
		Data []gatewayMetadataItemForTest `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
	require.Len(t, got.Data, 1)
	require.NotNil(t, got.Data[0].ContextLength)
	require.Equal(t, int64(200000), *got.Data[0].ContextLength)
}

func TestWriteModelsListWithoutCatalogKeepsHistoryShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeModelsList(c, []string{"claude-sonnet-4-6"}, nil)

	require.Equal(t, http.StatusOK, recorder.Code)
	var got struct {
		Data []gatewayMetadataItemForTest `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
	require.Len(t, got.Data, 1)
	require.Equal(t, "claude-sonnet-4-6", got.Data[0].ID)
	require.Nil(t, got.Data[0].ContextLength)
	require.Nil(t, got.Data[0].ContextWindow)
	require.Nil(t, got.Data[0].SupportsTools)
}

func TestGatewayModelsCompositeKeyAttachesUpstreamMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(9101)
	account := service.Account{
		ID:       1,
		Platform: service.PlatformOpenAI,
		Credentials: map[string]any{
			"model_mapping": map[string]any{"glm-5.1": "glm-5.1"},
		},
		Extra: map[string]any{
			service.UpstreamModelMetadataExtraKey: service.NewUpstreamModelMetadataSnapshot("upstream", map[string]service.UpstreamModelMetadata{
				"glm-5.1": {ID: "glm-5.1", ContextWindow: 200000, MaxOutputTokens: 128000},
			}),
		},
	}
	h := newGatewayModelsHandlerForTest(&gatewayModelsAccountRepoStub{byGroup: map[int64][]service.Account{
		groupID: {account},
	}})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		IsComposite: true,
		User:        &service.User{Status: service.StatusActive},
		CompositeGroups: []service.APIKeyCompositeGroup{
			{GroupID: groupID, Prefix: "tr", Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive}},
		},
	})

	h.Models(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var got struct {
		Data []gatewayMetadataItemForTest `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))

	var found *gatewayMetadataItemForTest
	for i := range got.Data {
		if got.Data[i].ID == "tr/glm-5.1" {
			found = &got.Data[i]
			break
		}
	}
	require.NotNil(t, found, "composite list should contain tr/glm-5.1")
	require.NotNil(t, found.ContextLength)
	require.Equal(t, int64(200000), *found.ContextLength)
	require.NotNil(t, found.MaxOutputTokens)
	require.Equal(t, int64(128000), *found.MaxOutputTokens)
}
