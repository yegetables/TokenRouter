//go:build wireinject

package app

import (
	serverhttp "github.com/TokenFlux/TokenRouter/internal/server/httpapi"

	"github.com/TokenFlux/TokenRouter/internal/server"

	"github.com/google/wire"
)

// selfCaptureProviders 汇总 fork 专属请求载荷捕获的 Wire provider。
var selfCaptureProviders = wire.NewSet(
	provideSelfCaptureStore,
)

// httpAssemblyProviders 汇总HTTP 入口的 Wire provider。
var httpAssemblyProviders = wire.NewSet(
	nativeHTTPProviders,
	server.ProviderSet,
	provideHTTPOptions,
	provideHTTPRouteMount,
	provideAuthRouteMount,
	provideUserRouteMount,
	provideAdminRouteMount,
	provideGatewayRouteMount,
	providePaymentRouteMount,
	provideForwardedSettings,
	providePanelSettings,
	serverhttp.NewPanelSettingsHandler,
	provideRouterRuntime,
)
