package provider

import "github.com/TokenFlux/TokenRouter/internal/upstream/usageview"

const (
	UpstreamUsageAdapterSub2API         = usageview.UpstreamUsageAdapterSub2API
	UpstreamUsageAdapterNewAPI          = usageview.UpstreamUsageAdapterNewAPI
	UpstreamUsageAdapterZivv            = usageview.UpstreamUsageAdapterZivv
	UpstreamUsageAdapterKimiCoding      = usageview.UpstreamUsageAdapterKimiCoding
	UpstreamUsageAdapterZhipuCoding     = usageview.UpstreamUsageAdapterZhipuCoding
	UpstreamUsageAdapterKimiBalance     = usageview.UpstreamUsageAdapterKimiBalance
	UpstreamUsageAdapterDeepseekBalance = usageview.UpstreamUsageAdapterDeepseekBalance
	UpstreamUsageAdapterZCode           = usageview.UpstreamUsageAdapterZCode
	UpstreamUsageAdapterCline           = usageview.UpstreamUsageAdapterCline
	UpstreamUsageAdapterClinePass       = usageview.UpstreamUsageAdapterClinePass
	NewAPIUserAccessTokenCredentialKey  = "new_api_user_access_token"
	NewAPIUserIDCredentialKey           = "new_api_user_id"
	UpstreamUsageDefaultAdapter         = UpstreamUsageAdapterSub2API
)

// UsageAdapterSpec 声明用量适配器的目录属性。
type UsageAdapterSpec struct {
	Name, Label string
	Automatic   bool
}

var usageAdapterCatalog = []UsageAdapterSpec{
	{Name: UpstreamUsageAdapterSub2API, Label: "Sub2API / TokenRouter", Automatic: false},
	{Name: UpstreamUsageAdapterNewAPI, Label: "New API", Automatic: false},
	{Name: UpstreamUsageAdapterZivv, Label: "Zivv", Automatic: false},
	{Name: UpstreamUsageAdapterKimiCoding, Label: "Kimi Coding Plan", Automatic: true},
	{Name: UpstreamUsageAdapterZhipuCoding, Label: "Zhipu Coding Plan", Automatic: true},
	{Name: UpstreamUsageAdapterKimiBalance, Label: "Kimi Balance", Automatic: true},
	{Name: UpstreamUsageAdapterDeepseekBalance, Label: "DeepSeek Balance", Automatic: true},
	{Name: UpstreamUsageAdapterZCode, Label: "ZCode Start Plan", Automatic: false},
	{Name: UpstreamUsageAdapterCline, Label: "Cline", Automatic: false},
	{Name: UpstreamUsageAdapterClinePass, Label: "ClinePass", Automatic: false},
}

// UpstreamUsageAdapterCatalog 返回独立列表，调用方不能改变配置校验目录。
func UpstreamUsageAdapterCatalog() []UsageAdapterSpec {
	return append([]UsageAdapterSpec(nil), usageAdapterCatalog...)
}

// UpstreamUsageAdapterOptions 返回稳定排序的内置适配器列表。
func UpstreamUsageAdapterOptions() []UpstreamUsageAdapterOption {
	options := make([]UpstreamUsageAdapterOption, 0, len(usageAdapterCatalog))
	for _, registration := range usageAdapterCatalog {
		if registration.Automatic {
			continue
		}
		options = append(options, UpstreamUsageAdapterOption{Name: registration.Name, Label: registration.Label})
	}
	return options
}
