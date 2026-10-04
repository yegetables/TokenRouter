package app

import (
	"context"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/gateway/httpapi/textattempt"
	"github.com/TokenFlux/TokenRouter/internal/gateway/promptpolicy"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"go.uber.org/zap"

	keyhttp "github.com/TokenFlux/TokenRouter/internal/apikey/httpapi"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	opscore "github.com/TokenFlux/TokenRouter/internal/ops"
	"github.com/TokenFlux/TokenRouter/internal/pkg/timezone"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	usagehttp "github.com/TokenFlux/TokenRouter/internal/usage/httpapi"

	batchhttp "github.com/TokenFlux/TokenRouter/internal/batchimage/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/config"

	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// legacyRouteMiddleware 在夹具构造时将 Key 服务绑定到路由中间件。
func legacyRouteMiddleware(auth keyhttp.APIKeyAuthMiddleware, keys *apikey.APIKeyService, subscriptions *billing.SubscriptionService, ops *opscore.OpsService, settings *routing.RuntimeSettings, cfg *config.Config) gatewayhttp.RouteMiddleware {
	var native *apikey.APIKeyService
	if keys != nil {
		native = keys
	}
	value := provideGatewayRouteMiddleware(auth, native, subscriptions, ops, cfg, nil, nil, nil)
	options := gatewayhttp.GroupAssignmentOptions{Access: func(c *gin.Context) gatewayhttp.GroupAssignmentAccess {
		key, ok := keyhttp.GetAPIKeyFromContext(c)
		if !ok || key == nil {
			return gatewayhttp.GroupAssignmentAccess{}
		}
		_, noGroup := c.Get(gatewayhttp.CompositeKeyNoGroupContextKey)
		return gatewayhttp.GroupAssignmentAccess{Loaded: true, Assigned: key.GroupID != nil, CompositeNoGroup: key.IsComposite && noGroup}
	}, Rejected: func(c *gin.Context) {
		gatewayhttp.MarkOpsClientBusinessLimited(c, gatewayhttp.OpsClientBusinessLimitedReasonAPIKeyGroupUnassigned)
		middleware.MarkIngressRejected(c, middleware.IngressRejectGroupUnassigned)
	}}
	options.WriteError = gatewayhttp.AnthropicErrorWriter
	value.RequireGroupAnthropic = gatewayhttp.RequireGroupAssignment(options)
	options.WriteError = gatewayhttp.GoogleErrorWriter
	value.RequireGroupGoogle = gatewayhttp.RequireGroupAssignment(options)
	return value
}

func RegisterGatewayRoutes(
	r *gin.Engine,
	h *routeTestHandlers,
	apiKeyAuth keyhttp.APIKeyAuthMiddleware,
	apiKeyService *apikey.APIKeyService,
	subscriptionService *billing.SubscriptionService,
	opsService *opscore.OpsService,
	settingService *routing.RuntimeSettings,
	cfg *config.Config,
) {
	// 路由测试使用 HTTP 处理器和已绑定的执行组件。
	var shared *messageHTTPBindings
	var runtime *textattempt.Runtime
	var activity *gatewayRequestActivity
	if h.TextEnabled {
		shared = provideMessageHTTPBindings(gatewayprovider.NewRoutePlanner(nil), nil, provideSchedulerSharedState(nil, nil), nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil)
		runtime = textattempt.New(textattempt.Bindings{})
		activity = &gatewayRequestActivity{Operations: lifecycle.NewOperations("route-fixture")}
	}
	openAITokensHTTP := h.OpenAITokensHTTP
	if openAITokensHTTP == nil {
		openAITokensHTTP = provideOpenAITokensHTTP(nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil)
	}
	countTokensHTTP := h.CountTokensHTTP
	if countTokensHTTP == nil && h.TextEnabled {
		countTokensHTTP = gatewayhttp.NewCountTokensHandler(cfg.Gateway.MaxBodySize, 0, gatewayhttp.CountHTTPPorts{ReadAccess: keyhttp.GetAPIKeyFromContext, Funding: routeCountUnavailable{}, ObserveCompatibility: func(*zap.Logger) {}}, (*promptpolicy.Service)(nil))
	}
	qoderCompatibleHTTP := h.QoderCompatibleHTTP
	if qoderCompatibleHTTP == nil {
		qoderCompatibleHTTP = provideQoderCompatibleHTTP(nil, nil, nil, nil, nil, nil, nil, nil, GatewayCompletionRecorders{}, nil, nil, nil)
	}
	compatibleTextHTTP := h.CompatibleTextHTTP
	if compatibleTextHTTP == nil && h.TextEnabled {
		compatibleTextHTTP = provideCompatibleTextHTTP(shared, runtime, activity)
	}
	geminiNativeHTTP := h.GeminiNativeHTTP
	if geminiNativeHTTP == nil && h.TextEnabled {
		geminiNativeHTTP = provideGeminiNativeHTTP(shared, nil, runtime, activity, nil)
	}
	commonOpenAI := provideOpenAIAttemptBindings(nil, nil, nil, nil, nil, nil, GatewayCompletionRecorders{}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	openAIRuntime := provideOpenAITextAttemptRuntime(commonOpenAI)
	mediaRuntime := provideMediaRuntime(nil, nil, nil, nil, commonOpenAI, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	openAITextHTTP := h.OpenAITextHTTP
	if openAITextHTTP == nil && h.OpenAIEnabled {
		openAITextHTTP = provideOpenAITextHTTP(nil, nil, nil, nil, nil, nil, nil, nil, nil, openAIRuntime, activity, nil, nil, nil, nil)
	}
	responsesWSHTTP := h.ResponsesWSHTTP
	if responsesWSHTTP == nil && h.OpenAIEnabled {
		responsesWSHTTP = provideResponsesWSHTTP(nil, nil, nil, nil, commonOpenAI, nil, nil, nil, activity, nil, nil, nil)
	}
	modelsHTTP := h.ModelsHTTP
	if modelsHTTP == nil && h.TextEnabled {
		modelsHTTP = provideModelsHTTP(nil, nil, nil, nil)
	}
	messagesHTTP := h.MessagesHTTP
	if messagesHTTP == nil && h.TextEnabled {
		messagesHTTP = provideMessagesHTTP(shared, runtime, activity)
	}

	mediaHTTP, auxiliaryHTTP, liveHTTP, searchHTTP := h.MediaHTTP, h.AuxiliaryHTTP, h.LiveHTTP, h.SearchHTTP
	if h.OpenAIEnabled {
		if mediaHTTP == nil {
			mediaHTTP = provideMediaHTTP(mediaRuntime, activity)
		}
		if auxiliaryHTTP == nil {
			auxiliaryHTTP = provideAuxiliaryHTTP(mediaRuntime, activity)
		}
		if liveHTTP == nil {
			liveHTTP = provideLiveHTTP(nil, nil, nil, nil, nil)
		}
	}
	if searchHTTP == nil && h.TextEnabled {
		searchHTTP = gatewayhttp.NewSearchHandler(gatewayhttp.SearchPorts{})
	}

	qoderChat := gin.HandlerFunc(qoderCompatibleHTTP.ChatCompletions)
	if h.QoderChat != nil {
		qoderChat = h.QoderChat.ChatCompletions
	}
	publicUsage := usagehttp.NewPublicUsageHandler(nil, nil, nil, nil, usagehttp.PublicUsageContext{
		Key: keyhttp.GetAPIKeyFromContext,
		Billing: func(c *gin.Context) (*billing.APIKeyBillingContext, bool) {
			return gatewayhttp.GetAPIKeyBillingContext(c)
		},
		Subscription: gatewayhttp.SubscriptionFromContext,
	}, timezone.NewCalendar(time.Local)).Usage
	if h.PublicUsage != nil {
		publicUsage = h.PublicUsage.Usage
	}
	gatewayhttp.RegisterGatewayRoutes(r, gatewayhttp.RouteEndpoints{CountTokens: countTokensHTTP, QoderCompatible: qoderCompatibleHTTP, CompatibleText: compatibleTextHTTP, GeminiNative: geminiNativeHTTP, OpenAIText: openAITextHTTP, OpenAITokens: openAITokensHTTP, ResponsesWS: responsesWSHTTP, Models: modelsHTTP, Messages: messagesHTTP, Media: mediaHTTP, Auxiliary: auxiliaryHTTP, Live: liveHTTP, Search: searchHTTP, PublicUsage: publicUsage, QoderChat: qoderChat}, legacyRouteMiddleware(apiKeyAuth, apiKeyService, subscriptionService, opsService, settingService, cfg), func(group *gin.RouterGroup) { batchhttp.RegisterGatewayRoutes(group, h.BatchImage) })
}

// 空路由夹具的上游计数器缺失时，返回依赖错误。
type routeCountUnavailable struct{}

func (routeCountUnavailable) CheckKey(context.Context, *apikey.APIKey, *billing.UserSubscription, string, bool) error {
	return billing.ErrBillingServiceUnavailable
}
