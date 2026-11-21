// router 包负责 Gin 引擎构建和路由注册。
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/client"
	lockerhttp "github.com/BwCloudWeGo/bw-cli/internal/locker/handler"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// New 使用配置好的中间件和版本化 API 路由构建网关 Gin 引擎。
// lockerHandler 为进程内挂载的智能快寄柜模块，允许为 nil（仅跑脚手架接口时）。
func New(clients *client.Clients, lockerHandler *lockerhttp.HTTPHandler, log *zap.Logger, middlewareCfg config.MiddlewareConfig) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(
		middleware.CORS(middlewareCfg.CORS),
		middleware.RequestID(),
		middleware.RequestLogger(log),
		gin.Recovery(),
	)
	r.OPTIONS("/*path", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	registerHealthRoutes(r)
	registerAPIRoutes(r, clients, lockerHandler, log, middlewareCfg)
	return r
}
