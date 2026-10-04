package app

import (
	"context"
	"net/http"
	"time"

	keyhttp "github.com/TokenFlux/TokenRouter/internal/apikey/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"

	"github.com/TokenFlux/TokenRouter/internal/routing/capability"

	"github.com/TokenFlux/TokenRouter/internal/ops"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/billing"

	identitysettings "github.com/TokenFlux/TokenRouter/internal/identity"

	"github.com/TokenFlux/TokenRouter/internal/app/bootstrap"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"

	"github.com/TokenFlux/TokenRouter/internal/config"

	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"

	"github.com/TokenFlux/TokenRouter/internal/protocol"

	"github.com/TokenFlux/TokenRouter/internal/selfcapture"

	"github.com/TokenFlux/TokenRouter/internal/server/middleware"

	"github.com/TokenFlux/TokenRouter/internal/site"

	"github.com/gin-gonic/gin"
)

// provideSecretEncryptor 使用启动配置创建 AES 加密器。
func provideSecretEncryptor(cfg *config.Config) (identitysettings.SecretEncryptor, error) {
	return bootstrap.NewAESEncryptor(cfg)
}

func provideAnnouncementExpiry(repo site.AnnouncementRepository) *site.AnnouncementExpiryService {
	return site.NewAnnouncementExpiryService(repo, time.Minute)
}

func provideApplication(server *http.Server, manager *lifecycle.Manager, _ *runtimeReady, opsService *ops.OpsService, _ *ops.ErrorLogQueue) *Application {
	lifecycle.TrackRequests(server, manager)
	manager.Register(lifecycle.Hook{Name: "OpsWSRuntime", StartOrder: 983, StopOrder: 17, Stop: func(context.Context) error { opsService.Realtime().Stop(); return nil }})
	return &Application{Server: server, lifecycle: manager}
}

// installBackgroundTasks 登记共享的后台任务跟踪器，并注入各使用方。
func installBackgroundTasks(manager *lifecycle.Manager) *lifecycle.Tasks {
	tasks := lifecycle.NewTasks()
	manager.Register(lifecycle.Hook{Name: "ApplicationBackgroundTasks", StartOrder: 932, StopOrder: 68, Stop: tasks.Stop})
	return tasks
}

// provideGatewayRouteMiddleware 装配网关路由中间件；载荷捕获包装在错误记录中间件外层。
func provideGatewayRouteMiddleware(apiKeyAuth keyhttp.APIKeyAuthMiddleware, apiKeyService *apikey.APIKeyService, subscriptionService *billing.SubscriptionService, opsService *ops.OpsService, cfg *config.Config, queue *ops.ErrorLogQueue, clients *messageHTTPBindings, selfCaptureStore *selfcapture.Store) gatewayhttp.RouteMiddleware {
	var clientFallback gatewayhttp.ClientGroupFallbackResolver
	if clients != nil {
		clientFallback = clients.bindings.ClientGroupFallback
	}
	return gatewayhttp.RouteMiddleware{
		ClientGroupFallback: clientFallback,
		APIKeyAuth:          gin.HandlerFunc(apiKeyAuth), GoogleAPIKeyAuth: newGatewayAuthorization(apiKeyService, subscriptionService, cfg, true),
		BodyLimit: middleware.RequestBodyLimit(cfg.Gateway.MaxBodySize), TextBodyLimit: middleware.RequestBodyLimit(cfg.Gateway.TextMaxBodySize), ClientRequestID: middleware.ClientRequestID(), OpsErrorLogger: selfcapture.Middleware(selfCaptureStore, gatewayhttp.OpsErrorLoggerMiddleware(opsService, queue, provideOpsObservationAccess()), selfcapture.EnvEnabled()), EndpointNormalization: gatewayhttp.InboundEndpointMiddleware(),
		RequireGroupAnthropic: provideGroupAssignmentGuard(gatewayhttp.AnthropicErrorWriter), RequireGroupGoogle: provideGroupAssignmentGuard(gatewayhttp.GoogleErrorWriter), ForceAntigravity: keyhttp.ForcePlatform(capability.PlatformAntigravity), ForcedPlatform: keyhttp.GetForcePlatformFromContext,
		Access: func(c *gin.Context) gatewayhttp.RouteAccess {
			key, ok := keyhttp.GetAPIKeyFromContext(c)
			if !ok || key == nil {
				return gatewayhttp.RouteAccess{}
			}
			access := gatewayhttp.RouteAccess{Composite: key.IsComposite}
			if key.Group != nil {
				access.HasGroup = true
				access.AllowedProtocols = key.Group.AllowedProtocols
			}
			return access
		}, InstallClientProtocol: func(c *gin.Context, p protocol.ProtocolID) {
			ctx := requeststate.WithClientProtocol(c.Request.Context(), p)
			if key, ok := keyhttp.GetAPIKeyFromContext(c); ok && key != nil && key.Group != nil {
				ctx = requeststate.WithGroup(ctx, key.Group)
			}
			c.Request = c.Request.WithContext(ctx)
		}, ObserveBusinessLimit: gatewayhttp.MarkOpsClientBusinessLimited,
	}
}

// provideGroupAssignmentGuard 为分组检查绑定 Key 读取和 Ops 记录函数，检查规则由 gateway/httpapi 实现。
func provideGroupAssignmentGuard(writer func(*gin.Context, int, string)) gin.HandlerFunc {
	return gatewayhttp.RequireGroupAssignment(gatewayhttp.GroupAssignmentOptions{Access: gatewayhttp.EffectiveGroupAssignment, WriteError: writer, Rejected: func(c *gin.Context) {
		gatewayhttp.MarkOpsClientBusinessLimited(c, gatewayhttp.OpsClientBusinessLimitedReasonAPIKeyGroupUnassigned)
		middleware.MarkIngressRejected(c, middleware.IngressRejectGroupUnassigned)
	}})
}
