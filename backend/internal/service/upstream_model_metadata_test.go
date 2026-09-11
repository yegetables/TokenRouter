package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const upstreamModelMetadataFixture = `{
  "object": "list",
  "data": [
    {
      "id": "GLM-5.1",
      "object": "model",
      "owned_by": "tokenrhythm",
      "context_length": 200000,
      "max_completion_tokens": 128000,
      "currency": "CNY",
      "input_price_per_million": "8.00000000",
      "pricing": {"prompt": "8.00000000"},
      "discount_pricing": {"prompt": null},
      "has_discount": false,
      "supports_tools": true,
      "supports_reasoning": true,
      "supports_vision": false,
      "supports_anthropic": true,
      "supports_responses": false,
      "responses_modes": ["stream"],
      "responses_capabilities": {"available": false, "compact": true}
    },
    {
      "id": "kimi-k2.6",
      "context_length": 256000,
      "supports_vision": true
    },
    {
      "id": "no-metadata-model"
    }
  ]
}`

func TestParseUpstreamModelMetadataMapsCanonicalAndExtensions(t *testing.T) {
	catalog := ParseUpstreamModelMetadata([]byte(upstreamModelMetadataFixture))

	require.Contains(t, catalog, "glm-5.1", "模型 ID 必须小写归一")
	metadata := catalog["glm-5.1"]
	require.Equal(t, int64(200000), metadata.ContextWindow)
	require.Equal(t, int64(128000), metadata.MaxOutputTokens)
	require.NotNil(t, metadata.Reasoning)
	require.True(t, *metadata.Reasoning)
	require.Equal(t, []string{"text"}, metadata.InputModalities, "supports_vision=false 映射为纯文本")
	require.NotNil(t, metadata.SupportsTools)
	require.True(t, *metadata.SupportsTools)
	require.NotNil(t, metadata.SupportsVision)
	require.False(t, *metadata.SupportsVision)
	require.NotNil(t, metadata.SupportsAnthropic)
	require.True(t, *metadata.SupportsAnthropic)
	require.NotNil(t, metadata.SupportsResponses)
	require.False(t, *metadata.SupportsResponses)
	require.Equal(t, []string{"stream"}, metadata.ResponsesModes)
	require.JSONEq(t, `{"available": false, "compact": true}`, string(metadata.ResponsesCapabilities))

	// 价格字段不得进入快照。
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "price")
	require.NotContains(t, string(raw), "currency")
	require.NotContains(t, string(raw), "discount")

	// supports_vision=true 预映射 text+image。
	require.Contains(t, catalog, "kimi-k2.6")
	require.Equal(t, []string{"text", "image"}, catalog["kimi-k2.6"].InputModalities)

	// 无任何可透传字段的条目被丢弃。
	require.NotContains(t, catalog, "no-metadata-model")
}

func TestParseUpstreamModelMetadataAcceptsCanonicalNames(t *testing.T) {
	body := []byte(`{"data":[{"id":"m","context_window":1000,"max_output_tokens":500,"input_modalities":["text","image"],"reasoning":true}]}`)

	catalog := ParseUpstreamModelMetadata(body)
	require.Contains(t, catalog, "m")
	require.Equal(t, int64(1000), catalog["m"].ContextWindow)
	require.Equal(t, int64(500), catalog["m"].MaxOutputTokens)
	require.Equal(t, []string{"text", "image"}, catalog["m"].InputModalities)
	require.NotNil(t, catalog["m"].Reasoning)
}

func TestParseUpstreamModelMetadataSupportsModelsEnvelope(t *testing.T) {
	body := []byte(`{"models":[{"slug":"m2","context_length":1234}]}`)
	catalog := ParseUpstreamModelMetadata(body)
	require.Contains(t, catalog, "m2")
	require.Equal(t, int64(1234), catalog["m2"].ContextWindow)
	require.Nil(t, ParseUpstreamModelMetadata([]byte(`{"data":[]}`)))
	require.Nil(t, ParseUpstreamModelMetadata([]byte(`not-json`)))
}

func TestAccountUpstreamModelMetadataSnapshotRoundTrip(t *testing.T) {
	account := &Account{Extra: map[string]any{}}
	snapshot := NewUpstreamModelMetadataSnapshot("upstream", map[string]UpstreamModelMetadata{
		"GLM-5.1": {ID: "GLM-5.1", ContextWindow: 123, MaxOutputTokens: 45},
	})
	account.SetUpstreamModelMetadataSnapshot(snapshot)

	metadata, ok := account.GetUpstreamModelMetadata("glm-5.1")
	require.True(t, ok)
	require.Equal(t, int64(123), metadata.ContextWindow)
	require.Equal(t, int64(45), metadata.MaxOutputTokens)

	// 经过 map[string]any（JSON 落库/回读形态）仍可解析。
	account.Extra[UpstreamModelMetadataExtraKey] = map[string]any{
		"source":    "upstream",
		"synced_at": "2026-01-01T00:00:00Z",
		"models": map[string]any{
			"glm-5.2": map[string]any{"id": "glm-5.2", "context_window": float64(7)},
		},
	}
	metadata, ok = account.GetUpstreamModelMetadata("GLM-5.2")
	require.True(t, ok)
	require.Equal(t, int64(7), metadata.ContextWindow)
}
