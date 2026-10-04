package selfcapture

import (
	"os"
	"strconv"
	"strings"

	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"

	"github.com/gin-gonic/gin"
)

// EnvEnabled 读取捕获开关；默认开启，显式设为 0/false/off 时关闭。
func EnvEnabled() bool {
	v := strings.TrimSpace(os.Getenv("SELF_CAPTURE_PAYLOADS"))
	if v == "" {
		return true
	}
	enabled, err := strconv.ParseBool(v)
	if err != nil {
		return true
	}
	return enabled
}

// RegisterAdminRoutes 注册管理端查询端点；组鉴权、限流和审计由 app 预先安装。
func RegisterAdminRoutes(admin *gin.RouterGroup, store *Store) {
	if admin == nil || store == nil {
		return
	}
	group := admin.Group("/self")
	group.GET("/request-payloads/:client_request_id", func(c *gin.Context) {
		entry, err := store.GetByClientRequestID(c.Request.Context(), c.Param("client_request_id"))
		if err != nil {
			response.NotFound(c, "request payload not found")
			return
		}
		response.Success(c, entry)
	})
}
