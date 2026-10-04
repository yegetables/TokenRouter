package provider

import (
	"context"
	"slices"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/gateway/media"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/modelcatalog"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

type catalogueRules struct{ policy ModelPolicy }

// CatalogueDefaults 从统一目录提供候选，专用路由别名由提供商配置读取器加入。
func CatalogueDefaults(catalog modelcatalog.Reader) routing.CatalogueDefaults {
	ids := func() []string {
		if catalog == nil {
			return nil
		}
		return catalog.ModelIDs()
	}
	return routing.CatalogueDefaults{Platform: func(string) []string { return ids() }}
}

// SupportsClientProtocol 不把专用 Embeddings、Images 模型展示为普通对话候选。
// 提供商模型别名先按同一规则展开，再判断已有适配器支持的调用形状。
func (v catalogueRules) SupportsClientProtocol(model string, source capability.ProtocolID) bool {
	model = v.policy.Mapped(model)
	embedding := strings.HasPrefix(strings.ToLower(model), "text-embedding-")
	if embedding || source == capability.ProtocolEmbeddings {
		return embedding && source == capability.ProtocolEmbeddings
	}
	// 与 images 端点放行、账号选路共用同一生图模型判定，第三方兼容生图模型不在这里被漏掉。
	image := media.IsImageGenerationModel(model)
	if source == capability.ProtocolImagesGenerations || source == capability.ProtocolImagesEdits {
		return image
	}
	if image && source != capability.ProtocolOpenAIResponses && source != capability.ProtocolResponsesWebSocket {
		return false
	}
	if source == capability.ProtocolImageBatches {
		return upstream.IsGeminiImageGenerationModel(model)
	}
	return true
}

func (v catalogueRules) ConfiguredModels() []string {
	return v.policy.Record.GetConfiguredRequestModels(provideradapter.ModelDefaults())
}

func (v catalogueRules) Mapping() map[string]string {
	return provider.ResolveModelMapping(v.policy.Record, provideradapter.ModelDefaults())
}

func (v catalogueRules) Supports(ctx context.Context, model string) bool {
	return v.policy.Supports(ctx, model)
}

func (v catalogueRules) UpstreamModels(ctx context.Context, model string) []string {
	return v.policy.ListingModels(ctx, model)
}

// CatalogueProvider 返回模型目录需要的提供商信息。
func CatalogueProvider(value *provider.Record, route requeststate.AttemptRoute) routing.CatalogueProvider {
	snapshot := (ModelPolicy{Record: value, Route: route}).CandidateSnapshot()
	groups := make([]int64, len(value.ProviderGroups))
	for i, g := range value.ProviderGroups {
		groups[i] = g.GroupID
	}
	return routing.CatalogueProvider{ProviderSnapshot: snapshot, GroupIDs: slices.Clone(value.GroupIDs), ProviderGroupIDs: groups, Passthrough: value.IsOpenAIPassthroughEnabled(), Rules: catalogueRules{ModelPolicy{Record: value, Route: route}}}
}

func CatalogueProviders(values []provider.Record) []routing.CatalogueProvider {
	if values == nil {
		return nil
	}
	out := make([]routing.CatalogueProvider, len(values))
	for i := range values {
		out[i] = CatalogueProvider(&values[i], requeststate.AttemptRoute{})
	}
	return out
}
