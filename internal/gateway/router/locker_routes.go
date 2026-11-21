package router

import (
	"github.com/gin-gonic/gin"

	lockerhttp "github.com/BwCloudWeGo/bw-cli/internal/locker/handler"
)

// registerLockerRoutes 挂载智能快寄柜业务模块（工单 01~06 落地范围）。
func registerLockerRoutes(v1 *gin.RouterGroup, h *lockerhttp.HTTPHandler) {
	if h == nil {
		return
	}
	lockerhttp.RegisterRoutes(v1, h)
}
