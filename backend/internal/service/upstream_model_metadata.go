package service

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// UpstreamModelMetadataExtraKey 是账号 Extra 中存放「上游模型元数据快照」的键。
// 与 sub2api 对齐，便于后续 Codex 目录直接消费同一份快照。
const UpstreamModelMetadataExtraKey = "upstream_model_metadata"

// UpstreamModelMetadata 是账号上游声明的模型上下文与能力。
//
// canonical 字段（context_window / max_output_tokens / input_modalities /
// reasoning / supported_reasoning_levels）与 sub2api 对齐；扩展字段（supports_*、
// responses_*）供普通 /v1/models 透传使用。价格字段一律不在此结构中，避免向
// 客户端泄露上游成本。
type UpstreamModelMetadata struct {
	ID                       string   `json:"id"`
	DisplayName              string   `json:"display_name,omitempty"`
	Description              string   `json:"description,omitempty"`
	Reasoning                *bool    `json:"reasoning,omitempty"`
	DefaultReasoningLevel    string   `json:"default_reasoning_level,omitempty"`
	SupportedReasoningLevels []string `json:"supported_reasoning_levels,omitempty"`
	InputModalities          []string `json:"input_modalities,omitempty"`
	ContextWindow            int64    `json:"context_window,omitempty"`
	MaxOutputTokens          int64    `json:"max_output_tokens,omitempty"`

	// 扩展：普通列表透传的能力字段（上游显式声明才带）。
	SupportsTools         *bool           `json:"supports_tools,omitempty"`
	SupportsVision        *bool           `json:"supports_vision,omitempty"`
	SupportsAnthropic     *bool           `json:"supports_anthropic,omitempty"`
	SupportsResponses     *bool           `json:"supports_responses,omitempty"`
	ResponsesModes        []string        `json:"responses_modes,omitempty"`
	ResponsesCapabilities json.RawMessage `json:"responses_capabilities,omitempty"`
}

// UpstreamModelMetadataSnapshot 是账号级的上游模型元数据快照。
// Models 的键统一小写归一，便于大小写不敏感查表。
type UpstreamModelMetadataSnapshot struct {
	Source   string                           `json:"source"`
	SyncedAt string                           `json:"synced_at"`
	Models   map[string]UpstreamModelMetadata `json:"models"`
}

// SetUpstreamModelMetadataSnapshot 写入账号的上游模型元数据快照。
func (a *Account) SetUpstreamModelMetadataSnapshot(snapshot UpstreamModelMetadataSnapshot) {
	if a == nil {
		return
	}
	if a.Extra == nil {
		a.Extra = make(map[string]any)
	}
	a.Extra[UpstreamModelMetadataExtraKey] = snapshot
}

// GetUpstreamModelMetadataSnapshot 读取账号的上游模型元数据快照；缺失或非法时返回 nil。
func (a *Account) GetUpstreamModelMetadataSnapshot() *UpstreamModelMetadataSnapshot {
	if a == nil || a.Extra == nil {
		return nil
	}
	raw, ok := a.Extra[UpstreamModelMetadataExtraKey]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case UpstreamModelMetadataSnapshot:
		cloned := value
		return &cloned
	case *UpstreamModelMetadataSnapshot:
		if value == nil {
			return nil
		}
		cloned := *value
		return &cloned
	case map[string]any:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var snapshot UpstreamModelMetadataSnapshot
		if err := json.Unmarshal(encoded, &snapshot); err != nil {
			return nil
		}
		return &snapshot
	default:
		return nil
	}
}

// GetUpstreamModelMetadata 按模型 ID（大小写不敏感）读取快照中的单条元数据。
func (a *Account) GetUpstreamModelMetadata(modelID string) (UpstreamModelMetadata, bool) {
	snapshot := a.GetUpstreamModelMetadataSnapshot()
	if snapshot == nil || len(snapshot.Models) == 0 {
		return UpstreamModelMetadata{}, false
	}
	key := strings.ToLower(strings.TrimSpace(modelID))
	if key == "" {
		return UpstreamModelMetadata{}, false
	}
	if metadata, ok := snapshot.Models[key]; ok {
		return metadata, true
	}
	return UpstreamModelMetadata{}, false
}

// upstreamModelCapabilityEntry 只解析需要透传的字段；价格类字段不落结构体。
// 同时兼容上游原名（context_length/max_completion_tokens/supports_*）与
// sub2api canonical 名（context_window/max_output_tokens/input_modalities）。
type upstreamModelCapabilityEntry struct {
	upstreamModelEntry
	DisplayName              string          `json:"display_name"`
	Name                     string          `json:"name"`
	Description              string          `json:"description"`
	ContextLength            *int64          `json:"context_length"`
	ContextWindow            *int64          `json:"context_window"`
	MaxContextLength         *int64          `json:"max_context_length"`
	MaxCompletionTokens      *int64          `json:"max_completion_tokens"`
	MaxOutputTokens          *int64          `json:"max_output_tokens"`
	Reasoning                *bool           `json:"reasoning"`
	SupportsReasoning        *bool           `json:"supports_reasoning"`
	SupportedReasoningLevels []string        `json:"supported_reasoning_levels"`
	InputModalities          []string        `json:"input_modalities"`
	SupportedModalities      []string        `json:"supported_modalities"`
	SupportsTools            *bool           `json:"supports_tools"`
	SupportsVision           *bool           `json:"supports_vision"`
	SupportsAnthropic        *bool           `json:"supports_anthropic"`
	SupportsResponses        *bool           `json:"supports_responses"`
	ResponsesModes           []string        `json:"responses_modes"`
	ResponsesCapabilities    json.RawMessage `json:"responses_capabilities"`
}

// ParseUpstreamModelMetadata 解析上游 /v1/models 响应，映射为 canonical + 扩展字段。
// 兼容 {data:[]}、{models:[]} 与顶层数组；未声明的字段一律省略，不做推断（
// vision 除外：显式声明 supports_vision 时预映射 input_modalities）。
func ParseUpstreamModelMetadata(body []byte) map[string]UpstreamModelMetadata {
	entries := make([]upstreamModelCapabilityEntry, 0)
	var response struct {
		Data   []upstreamModelCapabilityEntry `json:"data"`
		Models []upstreamModelCapabilityEntry `json:"models"`
	}
	if err := json.Unmarshal(body, &response); err == nil {
		entries = append(entries, response.Data...)
		entries = append(entries, response.Models...)
	}
	if len(entries) == 0 {
		var arrayResponse []upstreamModelCapabilityEntry
		if err := json.Unmarshal(body, &arrayResponse); err == nil {
			entries = append(entries, arrayResponse...)
		}
	}
	if len(entries) == 0 {
		return nil
	}

	result := make(map[string]UpstreamModelMetadata, len(entries))
	for i := range entries {
		metadata, ok := buildUpstreamModelMetadata(&entries[i])
		if !ok {
			continue
		}
		result[strings.ToLower(metadata.ID)] = metadata
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// buildUpstreamModelMetadata 将单条上游条目映射为快照元数据；无任何可透传字段时返回 false。
func buildUpstreamModelMetadata(entry *upstreamModelCapabilityEntry) (UpstreamModelMetadata, bool) {
	if entry == nil {
		return UpstreamModelMetadata{}, false
	}
	modelID := upstreamModelEntryID(entry.upstreamModelEntry)
	if modelID == "" {
		return UpstreamModelMetadata{}, false
	}

	metadata := UpstreamModelMetadata{
		ID:                       modelID,
		DisplayName:              firstNonEmptyMetadataValue(entry.DisplayName, entry.Name),
		Description:              strings.TrimSpace(entry.Description),
		Reasoning:                firstMetadataBoolPointer(entry.Reasoning, entry.SupportsReasoning),
		SupportedReasoningLevels: dedupeMetadataStrings(entry.SupportedReasoningLevels),
		ContextWindow:            firstPositiveMetadataInt64(entry.ContextLength, entry.ContextWindow, entry.MaxContextLength),
		MaxOutputTokens:          firstPositiveMetadataInt64(entry.MaxCompletionTokens, entry.MaxOutputTokens),
		SupportsTools:            entry.SupportsTools,
		SupportsVision:           entry.SupportsVision,
		SupportsAnthropic:        entry.SupportsAnthropic,
		SupportsResponses:        entry.SupportsResponses,
		ResponsesModes:           dedupeMetadataStrings(entry.ResponsesModes),
	}
	if len(entry.ResponsesCapabilities) > 0 {
		metadata.ResponsesCapabilities = entry.ResponsesCapabilities
	}
	metadata.InputModalities = resolveInputModalities(entry)

	if !upstreamModelMetadataIsUseful(metadata) {
		return UpstreamModelMetadata{}, false
	}
	return metadata, true
}

// resolveInputModalities 优先使用上游显式声明的模态；缺失时仅按显式 supports_vision 预映射。
func resolveInputModalities(entry *upstreamModelCapabilityEntry) []string {
	if modalities := normalizeMetadataModalities(entry.InputModalities); len(modalities) > 0 {
		return modalities
	}
	if modalities := normalizeMetadataModalities(entry.SupportedModalities); len(modalities) > 0 {
		return modalities
	}
	if entry.SupportsVision == nil {
		return nil
	}
	if *entry.SupportsVision {
		return []string{"text", "image"}
	}
	return []string{"text"}
}

// upstreamModelMetadataIsUseful 判断条目是否带有任何可透传的上下文或能力字段。
func upstreamModelMetadataIsUseful(metadata UpstreamModelMetadata) bool {
	return metadata.ContextWindow > 0 ||
		metadata.MaxOutputTokens > 0 ||
		metadata.Reasoning != nil ||
		len(metadata.SupportedReasoningLevels) > 0 ||
		len(metadata.InputModalities) > 0 ||
		metadata.SupportsTools != nil ||
		metadata.SupportsVision != nil ||
		metadata.SupportsAnthropic != nil ||
		metadata.SupportsResponses != nil ||
		len(metadata.ResponsesModes) > 0 ||
		len(metadata.ResponsesCapabilities) > 0
}

// NewUpstreamModelMetadataSnapshot 构造快照，键统一小写归一。
func NewUpstreamModelMetadataSnapshot(source string, models map[string]UpstreamModelMetadata) UpstreamModelMetadataSnapshot {
	normalized := make(map[string]UpstreamModelMetadata, len(models))
	for key, metadata := range models {
		modelKey := strings.ToLower(strings.TrimSpace(key))
		if modelKey == "" {
			continue
		}
		if strings.TrimSpace(metadata.ID) == "" {
			metadata.ID = modelKey
		}
		normalized[modelKey] = metadata
	}
	return UpstreamModelMetadataSnapshot{
		Source:   strings.TrimSpace(source),
		SyncedAt: time.Now().UTC().Format(time.RFC3339),
		Models:   normalized,
	}
}

func firstNonEmptyMetadataValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstPositiveMetadataInt64(values ...*int64) int64 {
	for _, value := range values {
		if value != nil && *value > 0 {
			return *value
		}
	}
	return 0
}

func firstMetadataBoolPointer(values ...*bool) *bool {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

// dedupeMetadataStrings 去空白、去重并排序，供稳定输出与写入。
func dedupeMetadataStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// normalizeMetadataModalities 去重并按固定顺序（text/image/audio/video/file）排列。
func normalizeMetadataModalities(values []string) []string {
	deduped := dedupeMetadataStrings(values)
	if len(deduped) == 0 {
		return nil
	}
	order := map[string]int{"text": 0, "image": 1, "audio": 2, "video": 3, "file": 4}
	sort.SliceStable(deduped, func(i, j int) bool {
		left, leftKnown := order[strings.ToLower(deduped[i])]
		right, rightKnown := order[strings.ToLower(deduped[j])]
		switch {
		case leftKnown && rightKnown:
			return left < right
		case leftKnown != rightKnown:
			return leftKnown
		default:
			return strings.ToLower(deduped[i]) < strings.ToLower(deduped[j])
		}
	})
	return deduped
}
