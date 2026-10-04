package app

import (
	routeapikey "github.com/TokenFlux/TokenRouter/internal/apikey/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/apikey/httpapi/dto"
	routeaudit "github.com/TokenFlux/TokenRouter/internal/audit/httpapi"
	routebackup "github.com/TokenFlux/TokenRouter/internal/backup/httpapi"
	routebilling "github.com/TokenFlux/TokenRouter/internal/billing/httpapi"
	routecreative "github.com/TokenFlux/TokenRouter/internal/creative/httpapi"
	routeegress "github.com/TokenFlux/TokenRouter/internal/egress/httpapi"
	routegateway "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	routeidentity "github.com/TokenFlux/TokenRouter/internal/identity/httpapi"
	routemoderation "github.com/TokenFlux/TokenRouter/internal/moderation/httpapi"
	routenotification "github.com/TokenFlux/TokenRouter/internal/notification/httpapi"
	routeops "github.com/TokenFlux/TokenRouter/internal/ops/httpapi"
	routepromotion "github.com/TokenFlux/TokenRouter/internal/promotion/httpapi"
	routeprovider "github.com/TokenFlux/TokenRouter/internal/provider/httpapi"
	routerouting "github.com/TokenFlux/TokenRouter/internal/routing/httpapi"
	routingdto "github.com/TokenFlux/TokenRouter/internal/routing/httpapi/dto"
	routescheduler "github.com/TokenFlux/TokenRouter/internal/scheduler/httpapi"
	routesearch "github.com/TokenFlux/TokenRouter/internal/search/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/selfcapture"
	serverhttp "github.com/TokenFlux/TokenRouter/internal/server/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/server/middleware"
	routesettings "github.com/TokenFlux/TokenRouter/internal/settings/httpapi"
	routesite "github.com/TokenFlux/TokenRouter/internal/site/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/team/httpapi"
	routeusageadmin "github.com/TokenFlux/TokenRouter/internal/usage/httpapi/admin"
	"github.com/gin-gonic/gin"
)

// provideAdminRouteMount 将管理员 HTTP 处理器和跨模块读取函数绑定到路由注册函数。
func provideAdminRouteMount(eAdminTLSFingerprintProfile *routeegress.TLSFingerprintProfileHandler,
	eAdminSchedulerDiagnostics *routescheduler.DiagnosticsHandler,
	eAdminTLSFingerprintRouter *routeegress.TLSFingerprintRouterHandler,
	eAdminProviderCodexImport *routeprovider.CodexImportHandler,
	eAdminProviderOAuthUsage *routeprovider.OAuthUsageHandler,
	eAdminProviderManagement *routeprovider.ManagementHandler,
	eAdminContentModeration *routemoderation.ContentModerationHandler,
	eAdminAntigravityOAuth *routeprovider.AntigravityOAuthHandler,
	eAdminErrorPassthrough *routegateway.ErrorPassthroughHandler,
	eAdminCodexInviteReset *routeprovider.CodexInviteResetHandler,
	eAdminDataManagement *routebackup.DataManagementHandler,
	eAdminProviderArchive *routeprovider.ArchiveHandler,
	eAdminUserAttribute *routeidentity.UserAttributeHandler,
	eAdminUpstreamUsage *routeprovider.UpstreamUsageHandler,
	eAdminScheduledTest *routeprovider.ScheduledTestHandler,
	eAdminProviderOllama *routeprovider.OllamaUsageHandler,
	eAdminSubscription *routebilling.AdminSubscriptionHandler,
	eAdminAnnouncement *routesite.AdminAnnouncementHandler,
	eAdminProviderTests *routeprovider.TestHandler,
	eAdminOpenAIOAuth *routeprovider.OpenAIOAuthHandler,
	eAdminGeminiOAuth *routeprovider.GeminiOAuthHandler,
	eAdminProviderCRS *routeprovider.CRSHandler,
	eAdminQoderOAuth *routeprovider.QoderOAuthHandler,
	eAdminDashboard *routeusageadmin.DashboardHandler,
	eAdminAffiliate *routepromotion.AffiliateHandler,
	eAdminGrokOAuth *routeprovider.GrokOAuthHandler,
	eAdminAuditLog *routeaudit.AuditLogHandler,
	eAdminPricing *routerouting.PricingHandler,
	eModelAttributes *routerouting.ModelAttributeHandler,
	eAdminSetting *routesettings.Handler,
	ePreAggregation *routesettings.PreAggregationHandler,
	eCreativeSettings *routecreative.SettingsHandler,
	eGatewaySettings *routegateway.RuntimeSettingsHandler,
	eIdentitySettings *routeidentity.AdminKeySettingsHandler,
	eProviderSettings *routeprovider.RuntimeSettingsHandler,
	ePanelSettings *serverhttp.PanelSettingsHandler,
	eAdminSystem *routeops.SystemHandler,
	eAdminRedeem *routebilling.AdminRedeemHandler,
	eNotification *routenotification.Handler,
	eAdminBackup *routebackup.BackupHandler,
	eAdminAPIKey *routeapikey.AdminAPIKeyHandler[routingdto.Group],
	eAdminProxy *routeegress.ProxyHandler,
	eAdminGroup *routerouting.GroupHandler,
	eAdminOAuth *routeprovider.ClaudeOAuthHandler,
	eAdminUsage *routeusageadmin.UsageHandler,
	eAdminPromo *routepromotion.PromoHandler,
	eAdminTeam *httpapi.AdminHandler,
	eAdminUser *routeidentity.AdminUserHandler[dto.APIKey[routingdto.Group]],
	eAdminOps *routeops.OpsHandler,
	eSearch *routesearch.Handler,
	selfCaptureStore *selfcapture.Store,
) adminRouteMount {
	return func(v1 *gin.RouterGroup, security httpRouteSecurity, protocolCatalog gin.HandlerFunc) {
		admin := v1.Group("/admin")
		admin.Use(security.Admin)
		// 面板全局按用户限流（默认管理员豁免，可在系统设置中关闭豁免）
		admin.Use(security.Panel.Global())
		// 审计中间件挂在认证之后：所有管理面变更类操作 + 敏感读取入审计日志
		admin.Use(security.Audit)
		admin.Use(middleware.ProviderTerminology())
		{
			// 只读能力目录：提供商与分组表单共用后端定义。
			admin.GET("/protocol-capabilities", protocolCatalog)
			// 仪表盘
			{
				routeusageadmin.RegisterDashboardRoutes(admin, eAdminDashboard)
			}

			// 用户管理
			{
				routeidentity.RegisterUserManagementRoutes(admin, eAdminUser, eAdminUserAttribute)
			}

			// 分组管理
			{
				routerouting.RegisterGroupRoutes(admin, eAdminGroup)
			}

			// 提供商管理
			{
				routeprovider.RegisterProviderRoutes(admin, routeprovider.ProviderRouteEndpoints{
					ProviderArchive:     eAdminProviderArchive,
					ProviderCRS:         eAdminProviderCRS,
					ProviderCodexImport: eAdminProviderCodexImport,
					ProviderManagement:  eAdminProviderManagement,
					ProviderOAuthUsage:  eAdminProviderOAuthUsage,
					ProviderOllama:      eAdminProviderOllama,
					ProviderTests:       eAdminProviderTests,
					CodexInviteReset:    eAdminCodexInviteReset,
					OAuth:               eAdminOAuth,
					OpenAIOAuth:         eAdminOpenAIOAuth,
					UpstreamUsage:       eAdminUpstreamUsage,
				}, security.StepUp, func(providers *gin.RouterGroup) {
					routescheduler.RegisterProviderDiagnostics(providers, eAdminSchedulerDiagnostics)
				})
			}

			// 公告管理
			{
				routesite.RegisterAnnouncementRoutes(admin, eAdminAnnouncement)
			}

			// OpenAI OAuth 管理
			{
				routeprovider.RegisterOpenAIOAuthRoutes(admin, eAdminOpenAIOAuth)
			}

			// Gemini OAuth 管理
			{
				routeprovider.RegisterGeminiOAuthRoutes(admin, eAdminGeminiOAuth)
			}

			// Antigravity OAuth 管理
			{
				routeprovider.RegisterAntigravityOAuthRoutes(admin, eAdminAntigravityOAuth)
			}

			// Qoder OAuth 管理
			{
				routeprovider.RegisterQoderOAuthRoutes(admin, eAdminQoderOAuth)
			}

			// Grok OAuth 管理
			{
				routeprovider.RegisterGrokOAuthRoutes(admin, eAdminGrokOAuth)
			}

			// 代理管理
			{
				routeegress.RegisterProxyRoutes(admin, eAdminProxy, security.StepUp)
			}

			// 卡密管理
			{
				routebilling.RegisterRedeemCodeRoutes(admin, eAdminRedeem)
			}

			// 优惠码管理
			{
				routepromotion.RegisterPromoCodeRoutes(admin, eAdminPromo)
			}

			// 系统设置
			{
				adminSettings := admin.Group("/settings")
				routesettings.RegisterSettingsSettingsRoutes(adminSettings, eAdminSetting, ePreAggregation)
				routecreative.RegisterCreativeSettingsRoutes(adminSettings, eCreativeSettings)
				routeidentity.RegisterIdentitySettingsRoutes(adminSettings, eIdentitySettings)
				routeprovider.RegisterProviderSettingsRoutes(adminSettings, eProviderSettings)
				serverhttp.RegisterPanelSettingsRoutes(adminSettings, ePanelSettings)
				routegateway.RegisterGatewaySettingsRoutes(adminSettings, eGatewaySettings)
				routenotification.RegisterSettingsRoutes(adminSettings, eNotification)
				routesearch.RegisterSettingsRoutes(adminSettings, eSearch)
			}

			// 数据管理
			{
				routebackup.RegisterDataManagementRoutes(admin, eAdminDataManagement, security.StepUp)
			}

			// 数据库备份恢复
			{
				routebackup.RegisterBackupRoutes(admin, eAdminBackup, security.StepUp)
			}

			// 运维监控（Ops）
			{
				routeops.RegisterOpsRoutes(admin, eAdminOps)
			}

			// 系统管理
			{
				routeops.RegisterSystemRoutes(admin, eAdminSystem)
			}

			// 订阅管理
			{
				routebilling.RegisterSubscriptionRoutes(admin, eAdminSubscription)
			}

			// 使用记录管理
			{
				routeusageadmin.RegisterUsageRoutes(admin, eAdminUsage)
			}

			// fork 专属：请求载荷详情查询
			{
				selfcapture.RegisterAdminRoutes(admin, selfCaptureStore)
			}

			// 用户属性管理
			{
				routeidentity.RegisterUserAttributeRoutes(admin, eAdminUserAttribute)
			}

			// 错误透传规则管理
			{
				routegateway.RegisterErrorPassthroughRoutes(admin, eAdminErrorPassthrough)
			}

			// TLS 指纹模板管理
			{
				routeegress.RegisterTLSFingerprintProfileRoutes(admin, eAdminTLSFingerprintProfile)
			}

			// TLS 路由器管理
			{
				routeegress.RegisterTLSFingerprintRouterRoutes(admin, eAdminTLSFingerprintRouter)
			}

			// API Key 管理
			{
				routeapikey.RegisterAdminAPIKeyRoutes(admin, eAdminAPIKey)
			}

			// 定时测试计划
			{
				routeprovider.RegisterScheduledTestRoutes(admin, eAdminScheduledTest)
			}

			// 价格管理
			{
				routerouting.RegisterPricingRoutes(admin, eAdminPricing)
				routerouting.RegisterModelAttributeRoutes(admin, eModelAttributes)
			}

			// 风控中心
			{
				routemoderation.RegisterContentModerationRoutes(admin, eAdminContentModeration)
			}

			// 邀请返利
			{
				routepromotion.RegisterAffiliateRoutes(admin, eAdminAffiliate)
			}

			// 操作审计日志
			{
				routeaudit.RegisterAuditLogRoutes(admin, eAdminAuditLog)
			}

			// 团队运维管理。
			teams := admin.Group("/teams")
			{
				teams.GET("", eAdminTeam.List)
				teams.POST("", eAdminTeam.Create)
				teams.GET("/:id", eAdminTeam.Get)
				teams.GET("/:id/members", eAdminTeam.ListMembers)
				teams.GET("/:id/usage", eAdminTeam.GetUsage)
				teams.PATCH("/:id", eAdminTeam.Update)
				teams.POST("/:id/force-transfer", security.StepUp, eAdminTeam.ForceTransfer)
				teams.DELETE("/:id", security.StepUp, eAdminTeam.Dissolve)
			}
		}
	}
}
