package service

import (
	"context"
	"log/slog"
	"strings"
)

// ResolveUpstreamModelMetadata 返回分组内 OpenAI 兼容账号声明的模型上下文与能力，
// 键为客户端可见模型 ID（小写归一）。只读账号快照，不发起任何上游请求。
//
// 分组可能包含多个账号：上游模型 ID 直接作为可见 ID 注册；账号映射的客户端 ID
// 指向某上游模型时同样注册。同名模型只需一条，先到先得（能力冲突暂不处理）。
func (s *GatewayService) ResolveUpstreamModelMetadata(ctx context.Context, groupID *int64, platform string) map[string]UpstreamModelMetadata {
	result := make(map[string]UpstreamModelMetadata)
	if s != nil && s.accountRepo != nil && groupID != nil {
		accounts, err := s.listRequestableModelAccounts(ctx, groupID)
		if err != nil {
			slog.Warn("failed to load accounts for upstream model metadata",
				"group_id", derefGroupID(groupID),
				"platform", platform,
				"error", err)
		} else {
			accounts = filterRequestableModelAccounts(accounts, platform)
			for i := range accounts {
				account := &accounts[i]
				snapshot := account.GetUpstreamModelMetadataSnapshot()
				if snapshot == nil || len(snapshot.Models) == 0 {
					continue
				}

				for key, metadata := range snapshot.Models {
					registerUpstreamModelMetadata(result, key, metadata)
				}
				// 账号映射：客户端 ID → 上游模型 ID，命中快照时也注册该客户端 ID。
				for clientModel, upstreamModel := range account.GetModelMapping() {
					metadata, ok := account.GetUpstreamModelMetadata(upstreamModel)
					if !ok {
						continue
					}
					registerUpstreamModelMetadata(result, clientModel, metadata)
				}
			}
		}
	}
	// 内置模板兜底：账号快照优先，未覆盖的模型用平台内置上下文与能力补齐。
	for key, metadata := range BuiltinUpstreamModelMetadata(platform) {
		registerUpstreamModelMetadata(result, key, metadata)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// registerUpstreamModelMetadata 按小写键注册元数据，已存在时不覆盖。
func registerUpstreamModelMetadata(target map[string]UpstreamModelMetadata, key string, metadata UpstreamModelMetadata) {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return
	}
	if _, exists := target[normalized]; exists {
		return
	}
	target[normalized] = metadata
}
