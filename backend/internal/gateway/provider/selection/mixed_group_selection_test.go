package selection

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/account"
	"github.com/TokenFlux/TokenRouter/internal/egress"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/protocol"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/scheduler"
	"github.com/stretchr/testify/require"
)

type mixedGroupAccounts struct {
	Accounts
	values       []provider.ExecutionAccount
	groupQueries []int64
}

func (s *mixedGroupAccounts) GetByID(_ context.Context, id int64) (*provider.ExecutionAccount, error) {
	for i := range s.values {
		if s.values[i].Record.ID == id {
			value := s.values[i]
			return &value, nil
		}
	}
	return nil, fmt.Errorf("account %d missing", id)
}

func (s *mixedGroupAccounts) ListSchedulableByGroupIDAndPlatforms(_ context.Context, group int64, platforms []string) ([]provider.ExecutionAccount, error) {
	s.groupQueries = append(s.groupQueries, group)
	var out []provider.ExecutionAccount
	for _, value := range s.values {
		if slices.Contains(platforms, value.Record.Platform) && slices.Contains(value.Record.GroupIDs, group) {
			out = append(out, value)
		}
	}
	return out, nil
}

func (s *mixedGroupAccounts) ListSchedulableByGroupIDAndPlatform(ctx context.Context, group int64, platform string) ([]provider.ExecutionAccount, error) {
	return s.ListSchedulableByGroupIDAndPlatforms(ctx, group, []string{platform})
}

func mixedGroupAccount(id int64, platform, model string, groupID int64) provider.ExecutionAccount {
	value := account.Record{ID: id, Platform: platform, Type: capability.AccountTypeAPIKey, Status: account.StatusActive, Schedulable: true, Concurrency: 2, GroupIDs: []int64{groupID}, Credentials: map[string]any{"api_key": "test-key", "model_whitelist": []string{model}}}
	return *provider.NewExecutionAccount(&value)
}

// 每个入口先验证模型与协议，再在同组跨平台选择；simple 同样保留成员边界。
func TestMixedGroupSelectsModelOnActualAccountPlatform(t *testing.T) {
	for _, mode := range []routing.GroupSchedulerType{routing.GroupSchedulerTypeBasic, routing.GroupSchedulerTypeAdvanced} {
		for _, simple := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/simple=%v", mode, simple), func(t *testing.T) {
				group := &routing.Group{ID: 91, Hydrated: true, Status: routing.StatusActive, SchedulerType: mode}
				repo := &mixedGroupAccounts{values: []provider.ExecutionAccount{
					mixedGroupAccount(1, capability.PlatformAnthropic, "claude-test", group.ID),
					mixedGroupAccount(2, capability.PlatformOpenAI, "gpt-test", group.ID),
					mixedGroupAccount(3, capability.PlatformGemini, "gemini-test", group.ID),
					mixedGroupAccount(4, capability.PlatformOpenAI, "gpt-test", 92),
				}}
				options := DefaultOptions()
				options.Simple = simple
				selector := NewCompatible(CompatibleDependencies{Reads: Reads{Accounts: repo}}, options)
				for _, source := range []protocol.ProtocolID{protocol.ProtocolAnthropicMessages, protocol.ProtocolOpenAIResponses, protocol.ProtocolOpenAIChatCompletions} {
					ctx := requeststate.WithClientProtocol(requeststate.WithGroup(context.Background(), group), source)
					for i, model := range []string{"claude-test", "gpt-test", "gemini-test"} {
						selected, _, err := selector.SelectAccountWithSchedulerForCapability(ctx, &group.ID, "", "", model, nil, egress.OpenAIUpstreamTransportHTTPSSE, account.OpenAIEndpointCapabilityTextGeneration, false, false)
						require.NoError(t, err, "%s %s", source, model)
						require.NotNil(t, selected)
						require.Equal(t, int64(i+1), selected.Account.Record.ID)
						if selected.ReleaseFunc != nil {
							selected.ReleaseFunc()
						}
					}
				}
				for _, queried := range repo.groupQueries {
					require.Equal(t, group.ID, queried)
				}
			})
		}
	}
}

func TestMixedGroupRequiresExplicitGroupAndHonorsForcedPlatform(t *testing.T) {
	group := &routing.Group{ID: 91, Hydrated: true, Status: routing.StatusActive}
	repo := &mixedGroupAccounts{values: []provider.ExecutionAccount{mixedGroupAccount(1, capability.PlatformAnthropic, "shared", group.ID), mixedGroupAccount(2, capability.PlatformOpenAI, "shared", group.ID)}}
	selector := NewCompatible(CompatibleDependencies{Reads: Reads{Accounts: repo}}, DefaultOptions())
	ctx := requeststate.WithClientProtocol(requeststate.WithGroup(context.Background(), group), protocol.ProtocolAnthropicMessages)
	_, _, err := selector.SelectAccountWithSchedulerForCapability(ctx, nil, "", "", "shared", nil, egress.OpenAIUpstreamTransportHTTPSSE, account.OpenAIEndpointCapabilityTextGeneration, false, false)
	require.Error(t, err)
	require.Empty(t, repo.groupQueries)
	selected, _, err := selector.SelectAccountWithSchedulerForCapability(ctx, &group.ID, "", "", "shared", nil, egress.OpenAIUpstreamTransportHTTPSSE, account.OpenAIEndpointCapabilityTextGeneration, false, false, capability.PlatformOpenAI)
	require.NoError(t, err)
	require.Equal(t, int64(2), selected.Account.Record.ID)
	if selected.ReleaseFunc != nil {
		selected.ReleaseFunc()
	}
}

// 图片别名必须映射到图片模型，通配白名单不能把文本模型变成图片模型。
func TestMixedGroupImageCandidateUsesFinalMappedModel(t *testing.T) {
	selector := NewCompatible(CompatibleDependencies{}, DefaultOptions())
	value := mixedGroupAccount(1, capability.PlatformOpenAI, "*", 91)
	value.Record.Credentials["model_mapping"] = map[string]any{"image-alias": "gpt-test"}
	ctx := context.WithValue(context.Background(), imageModelRequiredKey{}, true)
	require.Equal(t, "image_model_required", selector.candidateEligibilityReason(ctx, &value, "", "image-alias", false, ""))
	value.Record.Credentials["model_mapping"] = map[string]any{"image-alias": "gpt-image-1"}
	require.Empty(t, selector.candidateEligibilityReason(ctx, &value, "", "image-alias", false, ""))
}

// 回归：图片候选门禁原先用只认原生族的判定，把第三方兼容生图模型判成 image_model_required，
// 使唯一账号也被剔除、请求以 503 结束。放行口径必须与图片入口门禁一致。
func TestMixedGroupImageCandidateAllowsThirdPartyCompatModel(t *testing.T) {
	selector := NewCompatible(CompatibleDependencies{}, DefaultOptions())
	value := mixedGroupAccount(1, capability.PlatformOpenAI, "*", 91)
	ctx := context.WithValue(context.Background(), imageModelRequiredKey{}, true)
	for _, model := range []string{"gpt-image-1", "grok-imagine", "qwen-image-2.0", "wan2.7-image"} {
		require.Empty(t, selector.candidateEligibilityReason(ctx, &value, "", model, false, ""), model)
	}
	for _, model := range []string{"glm-5.2", "deepseek-flash"} {
		require.Equal(t, "image_model_required", selector.candidateEligibilityReason(ctx, &value, "", model, false, ""), model)
	}
}

type mixedSessionLimits struct {
	scheduler.SessionLimitCache
	blocked int64
	calls   map[int64][]string
}

func (s *mixedSessionLimits) RegisterSession(_ context.Context, id int64, hash string, _ int, _ time.Duration) (bool, error) {
	if s.calls == nil {
		s.calls = map[int64][]string{}
	}
	s.calls[id] = append(s.calls[id], hash)
	return id != s.blocked, nil
}

// Anthropic 的会话限制在通用选号循环中仍然生效，并继续尝试组内其它账号。
func TestMixedGroupRespectsAnthropicSessionLimit(t *testing.T) {
	group := &routing.Group{ID: 91, Hydrated: true, Status: routing.StatusActive}
	first := mixedGroupAccount(1, capability.PlatformAnthropic, "*", 91)
	first.Record.Type = capability.AccountTypeOAuth
	first.Record.Extra = map[string]any{"max_sessions": 1}
	second := mixedGroupAccount(2, capability.PlatformAnthropic, "*", 91)
	second.Record.Type = capability.AccountTypeOAuth
	second.Record.Priority = 1
	second.Record.Extra = map[string]any{"max_sessions": 1}
	limits := &mixedSessionLimits{blocked: 1}
	generic := NewGeneric(GenericDependencies{Sessions: limits}, DefaultOptions())
	repo := &mixedGroupAccounts{values: []provider.ExecutionAccount{first, second}}
	selector := NewCompatible(CompatibleDependencies{Reads: Reads{Accounts: repo}, Generic: generic}, DefaultOptions())
	ctx := requeststate.WithClientProtocol(requeststate.WithGroup(context.Background(), group), protocol.ProtocolAnthropicMessages)
	selected, _, err := selector.SelectAccountWithSchedulerForCapability(ctx, &group.ID, "", "same-session", "claude-test", nil, egress.OpenAIUpstreamTransportHTTPSSE, account.OpenAIEndpointCapabilityTextGeneration, false, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), selected.Account.Record.ID)
	require.Equal(t, []string{"same-session"}, limits.calls[1])
	require.Equal(t, []string{"same-session"}, limits.calls[2])
	if selected.ReleaseFunc != nil {
		selected.ReleaseFunc()
	}
}

type mixedSnapshot struct{ values []provider.ExecutionAccount }

func (s mixedSnapshot) ListAccounts(_ context.Context, _ *int64, _ string, _ bool) ([]account.Record, bool, error) {
	return provider.ExecutionRecords(s.values), false, nil
}

func (s mixedSnapshot) GetAccount(_ context.Context, id int64) (*account.Record, error) {
	for _, v := range s.values {
		if v.Record.ID == id {
			return provider.ExecutionRecord(&v), nil
		}
	}
	return nil, nil
}

// 快照中的旧成员关系不能让已移出分组的账号通过数据库复核。
func TestMixedGroupRechecksMembershipAfterSnapshot(t *testing.T) {
	for _, mode := range []routing.GroupSchedulerType{routing.GroupSchedulerTypeBasic, routing.GroupSchedulerTypeAdvanced} {
		t.Run(string(mode), func(t *testing.T) {
			group := &routing.Group{ID: 91, Hydrated: true, Status: routing.StatusActive, SchedulerType: mode}
			moved := mixedGroupAccount(1, capability.PlatformAnthropic, "*", 91)
			ready := mixedGroupAccount(2, capability.PlatformOpenAI, "*", 91)
			ready.Record.Priority = 1
			snapshot := mixedSnapshot{values: []provider.ExecutionAccount{moved, ready}}
			moved.Record.GroupIDs = []int64{92}
			repo := &mixedGroupAccounts{values: []provider.ExecutionAccount{moved, ready}}
			selector := NewCompatible(CompatibleDependencies{Reads: Reads{Accounts: repo, Snapshot: snapshot}}, DefaultOptions())
			ctx := requeststate.WithClientProtocol(requeststate.WithGroup(context.Background(), group), protocol.ProtocolAnthropicMessages)
			selected, _, err := selector.SelectAccountWithSchedulerForCapability(ctx, &group.ID, "", "", "shared", nil, egress.OpenAIUpstreamTransportHTTPSSE, account.OpenAIEndpointCapabilityTextGeneration, false, false)
			require.NoError(t, err)
			require.Equal(t, int64(2), selected.Account.Record.ID)
			if selected.ReleaseFunc != nil {
				selected.ReleaseFunc()
			}
		})
	}
}
