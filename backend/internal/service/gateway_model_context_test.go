//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/stretchr/testify/require"
)

type modelContextAccountRepoStub struct {
	AccountRepository
	updates chan map[string]any
}

func (r *modelContextAccountRepoStub) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	if r.updates != nil {
		copied := make(map[string]any, len(updates))
		for k, v := range updates {
			copied[k] = v
		}
		r.updates <- copied
	}
	return nil
}

type modelContextHTTPUpstreamStub struct {
	HTTPUpstream
	body       string
	statusCode int
	calls      int
	lastURL    string
	lastAuth   string
}

func (s *modelContextHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.calls++
	s.lastURL = req.URL.String()
	s.lastAuth = req.Header.Get("Authorization")
	code := s.statusCode
	if code == 0 {
		code = http.StatusOK
	}
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     http.Header{},
	}, nil
}

// 上游带元数据（TokenRhythm 形态，含能力标签与边界值）与不带元数据（DeepSeek 形态）。
const (
	upstreamModelsWithContext = `{"object":"list","data":[
		{"id":"glm-5.1","object":"model","context_length":200000,"max_completion_tokens":128000,
		 "supports_vision":false,"supports_tools":true,"supports_reasoning":true,
		 "supports_responses":true,"supports_anthropic":true,
		 "responses_modes":["native"],
		 "responses_capabilities":{"available":true,"modes":["native"],"stream":true,"tools":true,
		   "background":false,"compact":false,"webSearch":false,"mcp":false,
		   "codeInterpreter":false,"imageGeneration":false,"fileSearch":false,"cancel":false}},
		{"id":"minimax-m2.7","object":"model","context_window":200000,"max_output_tokens":192000,
		 "supports_vision":true,"responses_modes":[],"responses_capabilities":null},
		{"id":"caps-only","object":"model","supports_vision":true},
		{"id":"plain-model","object":"model"}
	]}`
	upstreamModelsWithoutContext = `{"object":"list","data":[
		{"id":"deepseek-flash","object":"model","owned_by":"deepseek"},
		{"id":"deepseek-v4-pro","object":"model","owned_by":"deepseek"}
	]}`
)

func TestExtractUpstreamModelInfos(t *testing.T) {
	infos, err := extractUpstreamModelInfos([]byte(upstreamModelsWithContext))
	require.NoError(t, err)
	require.Len(t, infos, 4)

	byID := make(map[string]UpstreamModelInfo, len(infos))
	for _, info := range infos {
		byID[info.ID] = info
	}
	require.Equal(t, 200000, byID["glm-5.1"].ContextLength)
	require.Equal(t, 128000, byID["glm-5.1"].MaxCompletionTokens)
	// 字段别名（context_window / max_output_tokens）同样识别
	require.Equal(t, 200000, byID["minimax-m2.7"].ContextLength)
	require.Equal(t, 192000, byID["minimax-m2.7"].MaxCompletionTokens)
	// 上游未声明上下文时保持 0，表示未知（不得猜测填充）
	require.Zero(t, byID["plain-model"].ContextLength)
	require.Zero(t, byID["plain-model"].MaxCompletionTokens)

	// 能力标签：显式 false 与显式 true 都要原样保留（指针区分「未声明」与「false」）
	require.NotNil(t, byID["glm-5.1"].SupportsVision)
	require.False(t, *byID["glm-5.1"].SupportsVision)
	require.True(t, *byID["glm-5.1"].SupportsTools)
	require.True(t, *byID["glm-5.1"].SupportsReasoning)
	require.True(t, *byID["glm-5.1"].SupportsResponses)
	require.True(t, *byID["glm-5.1"].SupportsAnthropic)
	require.Equal(t, []string{"native"}, byID["glm-5.1"].ResponsesModes)
	require.JSONEq(t, `{"available":true,"modes":["native"],"stream":true,"tools":true,
		"background":false,"compact":false,"webSearch":false,"mcp":false,
		"codeInterpreter":false,"imageGeneration":false,"fileSearch":false,"cancel":false}`,
		string(byID["glm-5.1"].ResponsesCapabilities))
	// 空数组 / JSON null 归一为 nil（与「未声明」同义，避免下发空值或 null）
	require.True(t, *byID["minimax-m2.7"].SupportsVision)
	require.Nil(t, byID["minimax-m2.7"].ResponsesModes)
	require.Nil(t, byID["minimax-m2.7"].ResponsesCapabilities)
	// 只声明能力标签、没有上下文数字的模型也要保留
	require.True(t, *byID["caps-only"].SupportsVision)
	require.Zero(t, byID["caps-only"].ContextLength)
	// 完全未声明的模型：所有字段为零值
	require.Nil(t, byID["plain-model"].SupportsVision)
	require.Nil(t, byID["plain-model"].SupportsTools)
	require.Nil(t, byID["plain-model"].ResponsesModes)
	require.Nil(t, byID["plain-model"].ResponsesCapabilities)

	// 上游不提供上下文信息（DeepSeek /v1/models）时全部为 0
	infos, err = extractUpstreamModelInfos([]byte(upstreamModelsWithoutContext))
	require.NoError(t, err)
	require.Len(t, infos, 2)
	for _, info := range infos {
		require.Zero(t, info.ContextLength)
		require.Zero(t, info.MaxCompletionTokens)
	}

	// 裸数组形态
	infos, err = extractUpstreamModelInfos([]byte(`[{"id":"a","context_length":1000}]`))
	require.NoError(t, err)
	require.Len(t, infos, 1)
	require.Equal(t, 1000, infos[0].ContextLength)

	_, parseErr := extractUpstreamModelInfos([]byte(`{"data":`))
	require.Error(t, parseErr)
}

func TestUpstreamModelMetadataForAccounts(t *testing.T) {
	svc := &GatewayService{}

	// 无账号 / 无快照 → nil（调用方保持既有响应结构）
	require.Nil(t, svc.UpstreamModelMetadataForAccounts(nil))
	require.Nil(t, svc.UpstreamModelMetadataForAccounts([]Account{{ID: 1}}))
	require.Nil(t, svc.UpstreamModelMetadataForAccounts([]Account{{ID: 1, Extra: map[string]any{
		upstreamModelContextExtraKey: map[string]any{"fetched_at": "2026-09-10T00:00:00Z", "models": map[string]any{}},
	}}}))

	accounts := []Account{
		{ID: 1, Extra: map[string]any{
			upstreamModelContextExtraKey: map[string]any{
				"fetched_at": "2026-09-10T00:00:00Z",
				"models": map[string]any{
					"glm-5.1": map[string]any{"context_length": 200000},
				},
			},
		}},
		{ID: 2, Extra: map[string]any{
			upstreamModelContextExtraKey: map[string]any{
				"fetched_at": "2026-09-10T00:00:00Z",
				"models": map[string]any{
					// 同一模型另一账号提供补齐字段
					"glm-5.1":         map[string]any{"max_completion_tokens": 128000},
					"deepseek-flash":  map[string]any{"context_length": 1048576, "max_completion_tokens": 393216},
					"unknown-context": map[string]any{},
				},
			},
		}},
		// 损坏数据必须被忽略
		{ID: 3, Extra: map[string]any{upstreamModelContextExtraKey: "not-an-object"}},
	}

	metadata := svc.UpstreamModelMetadataForAccounts(accounts)
	require.Len(t, metadata, 2)
	require.Equal(t, 200000, metadata["glm-5.1"].ContextLength)
	require.Equal(t, 128000, metadata["glm-5.1"].MaxCompletionTokens) // 多账号合并补齐
	require.Equal(t, 1048576, metadata["deepseek-flash"].ContextLength)
	require.Equal(t, 393216, metadata["deepseek-flash"].MaxCompletionTokens)
	_, exists := metadata["unknown-context"]
	require.False(t, exists, "上游未提供任何字段的模型不应进入元数据")
}

func TestRefreshUpstreamModelContextPersistsAndSkipsUpstreamWithoutContext(t *testing.T) {
	config := &config.Config{}

	t.Run("上游提供上下文时落盘", func(t *testing.T) {
		repo := &modelContextAccountRepoStub{updates: make(chan map[string]any, 1)}
		upstream := &modelContextHTTPUpstreamStub{body: upstreamModelsWithContext}
		svc := &GatewayService{accountRepo: repo, httpUpstream: upstream, cfg: config}

		account := &Account{
			ID:          7,
			Platform:    PlatformDeepseek,
			Type:        AccountTypeAPIKey,
			Concurrency: 10,
			Credentials: map[string]any{
				"base_url": "https://api.deepseek.com",
				"api_key":  "sk-test-key",
			},
		}
		require.NoError(t, svc.refreshUpstreamModelContext(context.Background(), account))
		require.Equal(t, "https://api.deepseek.com/v1/models", upstream.lastURL)
		require.Equal(t, "Bearer sk-test-key", upstream.lastAuth)

		updates := <-repo.updates
		raw, ok := updates[upstreamModelContextExtraKey]
		require.True(t, ok)
		snapshot, err := json.Marshal(raw)
		require.NoError(t, err)
		require.Contains(t, string(snapshot), `"context_length":200000`)
		require.Contains(t, string(snapshot), `"max_completion_tokens":128000`)
		require.Contains(t, string(snapshot), `"supports_tools":true`)
		require.Contains(t, string(snapshot), `"supports_vision":false`, "显式 false 必须保留")
		require.Contains(t, string(snapshot), `"responses_modes":["native"]`)
		require.Contains(t, string(snapshot), `"responses_capabilities"`)
		require.Contains(t, string(snapshot), "caps-only", "仅声明能力标签的模型同样落快照")
		require.NotContains(t, string(snapshot), "plain-model", "未声明任何元数据的模型不落快照")
	})

	t.Run("上游不提供上下文时写入空快照以生效 TTL", func(t *testing.T) {
		repo := &modelContextAccountRepoStub{updates: make(chan map[string]any, 1)}
		upstream := &modelContextHTTPUpstreamStub{body: upstreamModelsWithoutContext}
		svc := &GatewayService{accountRepo: repo, httpUpstream: upstream, cfg: config}

		account := &Account{
			ID:          8,
			Platform:    PlatformDeepseek,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": "https://api.deepseek.com", "api_key": "sk-test-key"},
		}
		require.NoError(t, svc.refreshUpstreamModelContext(context.Background(), account))

		updates := <-repo.updates
		snapshot, err := json.Marshal(updates[upstreamModelContextExtraKey])
		require.NoError(t, err)
		require.Contains(t, string(snapshot), `"models":{}`)
		require.NotContains(t, string(snapshot), "deepseek-flash")
	})
}

func TestScheduleUpstreamModelContextRefresh(t *testing.T) {
	config := &config.Config{}

	t.Run("快照新鲜时不请求上游", func(t *testing.T) {
		fresh := time.Now().UTC().Format(time.RFC3339)
		repo := &modelContextAccountRepoStub{}
		upstream := &modelContextHTTPUpstreamStub{body: upstreamModelsWithContext}
		svc := &GatewayService{accountRepo: repo, httpUpstream: upstream, cfg: config}

		account := Account{
			ID:       9,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				upstreamModelContextExtraKey: map[string]any{
					"fetched_at": fresh,
					"models":     map[string]any{"glm-5.1": map[string]any{"context_length": 200000}},
				},
			},
		}
		svc.scheduleUpstreamModelContextRefresh([]Account{account})
		time.Sleep(80 * time.Millisecond)
		require.Zero(t, upstream.calls)
	})

	t.Run("快照过期时后台刷新", func(t *testing.T) {
		stale := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
		repo := &modelContextAccountRepoStub{updates: make(chan map[string]any, 1)}
		upstream := &modelContextHTTPUpstreamStub{body: upstreamModelsWithContext}
		svc := &GatewayService{accountRepo: repo, httpUpstream: upstream, cfg: config}

		account := Account{
			ID:       10,
			Platform: PlatformOpenAI,
			Type:     AccountTypeAPIKey,
			Extra: map[string]any{
				upstreamModelContextExtraKey: map[string]any{"fetched_at": stale, "models": map[string]any{}},
			},
			Credentials: map[string]any{"base_url": "https://example.com", "api_key": "sk-test-key"},
		}
		svc.scheduleUpstreamModelContextRefresh([]Account{account})

		select {
		case <-repo.updates:
		case <-time.After(2 * time.Second):
			t.Fatal("过期快照应触发后台刷新")
		}
		require.Equal(t, 1, upstream.calls)
	})

	t.Run("非 OpenAI 兼容账号不参与", func(t *testing.T) {
		repo := &modelContextAccountRepoStub{}
		upstream := &modelContextHTTPUpstreamStub{body: upstreamModelsWithContext}
		svc := &GatewayService{accountRepo: repo, httpUpstream: upstream, cfg: config}

		svc.scheduleUpstreamModelContextRefresh([]Account{
			{ID: 11, Platform: PlatformAnthropic, Type: AccountTypeOAuth},
			{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		})
		time.Sleep(80 * time.Millisecond)
		require.Zero(t, upstream.calls)
	})

	t.Run("失败退避窗口内不重复抓取", func(t *testing.T) {
		repo := &modelContextAccountRepoStub{}
		upstream := &modelContextHTTPUpstreamStub{body: upstreamModelsWithContext}
		svc := &GatewayService{accountRepo: repo, httpUpstream: upstream, cfg: config}

		accountID := int64(13)
		upstreamModelContextLastAttempt.Store(accountID, time.Now())
		t.Cleanup(func() { upstreamModelContextLastAttempt.Delete(accountID) })

		svc.scheduleUpstreamModelContextRefresh([]Account{{
			ID:          accountID,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Credentials: map[string]any{"base_url": "https://example.com", "api_key": "sk-test-key"},
		}})
		time.Sleep(80 * time.Millisecond)
		require.Zero(t, upstream.calls, "退避窗口内不应发起上游抓取")
	})
}

// 多账号合并时：已有声明（含显式 false）不被更完整的一方覆盖；
// 只有能力标签的模型不会被 isZero 丢弃。
func TestUpstreamModelMetadataMergePreservesDeclaredFields(t *testing.T) {
	svc := &GatewayService{}
	explicitFalse := false
	accounts := []Account{
		{ID: 1, Extra: map[string]any{
			upstreamModelContextExtraKey: map[string]any{
				"fetched_at": "2026-09-10T00:00:00Z",
				"models": map[string]any{
					"glm-5.1": map[string]any{
						"supports_vision": false,
						"responses_modes": []any{"native"},
					},
					"caps-only": map[string]any{"supports_tools": true},
				},
			},
		}},
		{ID: 2, Extra: map[string]any{
			upstreamModelContextExtraKey: map[string]any{
				"fetched_at": "2026-09-10T00:00:00Z",
				"models": map[string]any{
					"glm-5.1": map[string]any{
						"context_length":  200000,
						"supports_vision": true, // 不应覆盖账号 1 的显式 false
						"supports_tools":  true,
					},
					"caps-only": map[string]any{"context_length": 128000},
				},
			},
		}},
	}

	metadata := svc.UpstreamModelMetadataForAccounts(accounts)
	require.Len(t, metadata, 2)

	glm := metadata["glm-5.1"]
	require.NotNil(t, glm.SupportsVision)
	require.False(t, *glm.SupportsVision, "已声明的显式 false 不被其它账号覆盖")
	require.Equal(t, explicitFalse, *glm.SupportsVision)
	require.NotNil(t, glm.SupportsTools)
	require.True(t, *glm.SupportsTools)
	require.Equal(t, 200000, glm.ContextLength, "缺失字段由其它账号补齐")
	require.Equal(t, []string{"native"}, glm.ResponsesModes)

	capsOnly := metadata["caps-only"]
	require.NotNil(t, capsOnly.SupportsTools)
	require.Equal(t, 128000, capsOnly.ContextLength)
}

// 单元级别确认：只带能力标签的元数据不被视为空值。
func TestUpstreamModelMetadataIsZero(t *testing.T) {
	require.True(t, UpstreamModelMetadata{}.isZero())
	require.True(t, UpstreamModelMetadata{ContextLength: 0, ResponsesModes: nil}.isZero())

	enabled := true
	require.False(t, UpstreamModelMetadata{SupportsVision: &enabled}.isZero())
	require.False(t, UpstreamModelMetadata{ContextLength: 1000}.isZero())
	require.False(t, UpstreamModelMetadata{ResponsesModes: []string{"native"}}.isZero())
	require.False(t, UpstreamModelMetadata{ResponsesCapabilities: json.RawMessage(`{"available":true}`)}.isZero())
}

func TestSupportsUpstreamModelContextSync(t *testing.T) {
	require.False(t, supportsUpstreamModelContextSync(nil))
	require.False(t, supportsUpstreamModelContextSync(&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}))
	require.False(t, supportsUpstreamModelContextSync(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
	require.True(t, supportsUpstreamModelContextSync(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}))
	require.True(t, supportsUpstreamModelContextSync(&Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey}))
}
