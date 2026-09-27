package provider

import (
	"context"
	"slices"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/gateway/media"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
	"github.com/TokenFlux/TokenRouter/internal/upstream/anthropic"
	"github.com/TokenFlux/TokenRouter/internal/upstream/antigravity"
	"github.com/TokenFlux/TokenRouter/internal/upstream/gemini/codeassist"
	"github.com/TokenFlux/TokenRouter/internal/upstream/grok"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
	"github.com/TokenFlux/TokenRouter/internal/upstream/qoder"
)

// DefaultRequestModels 延续各平台原目录来源与未知平台回退，不另建模型缓存。
func DefaultRequestModels(platform string) []string {
	switch platform {
	case capability.PlatformOpenAI:
		return openai.DefaultModelIDs()
	case capability.PlatformGemini:
		ids := make([]string, 0, len(codeassist.DefaultModels))
		for _, model := range codeassist.DefaultModels {
			ids = append(ids, model.ID)
		}
		return ids
	case capability.PlatformAntigravity:
		models := antigravity.DefaultModels()
		ids := make([]string, 0, len(models))
		for _, model := range models {
			ids = append(ids, model.ID)
		}
		return ids
	case capability.PlatformQoder:
		return qoder.DefaultRequestModelIDs()
	case capability.PlatformGrok:
		return grok.DefaultModelIDs()
	default:
		return anthropic.DefaultModelIDs()
	}
}

func CatalogueDefaults() routing.CatalogueDefaults {
	return routing.CatalogueDefaults{Platform: DefaultRequestModels, Qoder: func(cn bool) []string {
		site := qoder.SiteGlobal
		if cn {
			site = qoder.SiteCN
		}
		return qoder.DefaultRequestModelIDsForSite(site)
	}}
}

type catalogueRules struct{ policy ModelPolicy }

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

func (v catalogueRules) Unrestricted() bool {
	return v.policy.Record.HasUnrestrictedModelScope(provideradapter.ModelDefaults())
}

func (v catalogueRules) QoderCN() bool {
	site, err := qoder.ParseSite(v.policy.Record.GetCredential("site"))
	return err == nil && site == qoder.SiteCN
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
