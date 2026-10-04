package app

import (
	"context"
	"database/sql"

	"github.com/TokenFlux/TokenRouter/internal/app/lifecycle"
	"github.com/TokenFlux/TokenRouter/internal/selfcapture"
)

// provideSelfCaptureStore 创建请求载荷捕获存储并登记停用钩子。
// 数据库不可用时返回 nil，所有挂载点对 nil 安全。
func provideSelfCaptureStore(db *sql.DB, manager *lifecycle.Manager) *selfcapture.Store {
	store := selfcapture.NewStore(db)
	if store != nil {
		manager.Register(lifecycle.Hook{Name: "SelfCaptureStore", StartOrder: -80, StopOrder: 890, Stop: func(context.Context) error {
			store.Close()
			return nil
		}})
	}
	return store
}
