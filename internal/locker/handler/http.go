// Package handler 暴露智能快寄柜业务的 HTTP 接口。
// 本项目为进程内模块挂载到 gateway，沿用 dto/service 分层，不额外引入 gRPC。
package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/internal/locker/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/locker/service"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
)

// DefaultUserID 演示环境默认用户，真实环境由登录态注入。
const DefaultUserID = "u_1001"

// HTTPHandler 寄存业务 HTTP 处理器。
type HTTPHandler struct {
	svc *service.Service
}

// New 创建处理器。
func New(svc *service.Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// RegisterRoutes 在给定路由组上注册 22 个接口。
func RegisterRoutes(group *gin.RouterGroup, h *HTTPHandler) {
	g := group.Group("/locker")
	{
		// 工单 03 首页：城市 / 网点 / 柜门 / 引导
		g.GET("/cities", h.Cities)
		g.GET("/stations", h.Stations)
		g.GET("/stations/:id/cabinets", h.Cabinets)
		g.GET("/guide", h.Guide)

		// 工单 04 寄存：试算 / 下单 / 支付回调
		g.POST("/quote", h.Quote)
		g.POST("/orders", h.CreateOrder)
		g.POST("/payments/notify", h.PayNotify)

		// 工单 05 订单：列表 / 详情 / 取出 / 补缴 / 授权
		g.GET("/orders", h.ListOrders)
		g.GET("/orders/:order_no", h.OrderDetail)
		g.POST("/orders/:order_no/pickup", h.Pickup)
		g.POST("/orders/:order_no/overdue-pay", h.OverduePay)
		g.POST("/orders/:order_no/grants", h.CreateGrant)
		g.POST("/grants/:grant_no/use", h.UseGrant)

		// 工单 02 支付：余额 / 明细 / 充值
		g.GET("/balance", h.Balance)
		g.GET("/balance/flows", h.Flows)
		g.POST("/recharges", h.CreateRecharge)
		g.POST("/recharges/notify", h.RechargeNotify)
		g.GET("/recharges", h.Recharges)

		// 工单 06 个人中心
		g.GET("/profile", h.Profile)
		g.GET("/coupons", h.Coupons)

		// 工单 01 消息通知
		g.GET("/notices", h.Notices)
		g.POST("/notices/:id/read", h.ReadNotice)
	}
}

func currentUser(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-User-Id")); v != "" {
		return v
	}
	return DefaultUserID
}

func pageParams(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return page, size
}

func optFloat(c *gin.Context, key string) *float64 {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &v
}

func badBody(c *gin.Context, err error) {
	httpx.Error(c, apperrors.InvalidArgument("invalid_body", "request body is invalid: "+err.Error()))
}

// ---------- 首页 ----------

func (h *HTTPHandler) Cities(c *gin.Context) {
	out, err := h.svc.Cities(c.Request.Context(), c.Query("keyword"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) Stations(c *gin.Context) {
	page, size := pageParams(c)
	out, err := h.svc.Stations(c.Request.Context(), c.Query("city_id"), c.Query("keyword"), optFloat(c, "lat"), optFloat(c, "lng"), page, size)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) Cabinets(c *gin.Context) {
	out, err := h.svc.Cabinets(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) Guide(c *gin.Context) {
	httpx.OK(c, h.svc.Guide(c.Request.Context()))
}

// ---------- 寄存 ----------

func (h *HTTPHandler) Quote(c *gin.Context) {
	var cmd dto.QuoteCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		badBody(c, err)
		return
	}
	cmd.UserID = currentUser(c)
	out, err := h.svc.Quote(c.Request.Context(), cmd)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) CreateOrder(c *gin.Context) {
	var cmd dto.CreateOrderCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		badBody(c, err)
		return
	}
	cmd.UserID = currentUser(c)
	out, err := h.svc.CreateOrder(c.Request.Context(), cmd)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *HTTPHandler) PayNotify(c *gin.Context) {
	var cmd dto.PayNotifyCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		badBody(c, err)
		return
	}
	out, err := h.svc.PayNotify(c.Request.Context(), cmd)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

// ---------- 订单 ----------

func (h *HTTPHandler) ListOrders(c *gin.Context) {
	page, size := pageParams(c)
	out, err := h.svc.ListOrders(c.Request.Context(), currentUser(c), c.DefaultQuery("status", "all"), page, size)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) OrderDetail(c *gin.Context) {
	out, err := h.svc.OrderDetail(c.Request.Context(), currentUser(c), c.Param("order_no"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) Pickup(c *gin.Context) {
	out, err := h.svc.Pickup(c.Request.Context(), currentUser(c), c.Param("order_no"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) OverduePay(c *gin.Context) {
	out, err := h.svc.OverduePay(c.Request.Context(), currentUser(c), c.Param("order_no"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) CreateGrant(c *gin.Context) {
	out, err := h.svc.CreateGrant(c.Request.Context(), currentUser(c), c.Param("order_no"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *HTTPHandler) UseGrant(c *gin.Context) {
	out, err := h.svc.UseGrant(c.Request.Context(), currentUser(c), c.Param("grant_no"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

// ---------- 余额与充值 ----------

func (h *HTTPHandler) Balance(c *gin.Context) {
	out, err := h.svc.Balance(c.Request.Context(), currentUser(c))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) Flows(c *gin.Context) {
	page, size := pageParams(c)
	out, err := h.svc.Flows(c.Request.Context(), currentUser(c), page, size)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) CreateRecharge(c *gin.Context) {
	var cmd dto.RechargeCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		badBody(c, err)
		return
	}
	cmd.UserID = currentUser(c)
	out, err := h.svc.CreateRecharge(c.Request.Context(), cmd)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *HTTPHandler) RechargeNotify(c *gin.Context) {
	var cmd dto.RechargeNotifyCommand
	if err := c.ShouldBindJSON(&cmd); err != nil {
		badBody(c, err)
		return
	}
	out, err := h.svc.RechargeNotify(c.Request.Context(), cmd)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) Recharges(c *gin.Context) {
	page, size := pageParams(c)
	out, err := h.svc.Recharges(c.Request.Context(), currentUser(c), page, size)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

// ---------- 个人中心 ----------

func (h *HTTPHandler) Profile(c *gin.Context) {
	out, err := h.svc.Profile(c.Request.Context(), currentUser(c))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) Coupons(c *gin.Context) {
	out, err := h.svc.Coupons(c.Request.Context(), currentUser(c), c.DefaultQuery("status", "all"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

// ---------- 消息通知 ----------

func (h *HTTPHandler) Notices(c *gin.Context) {
	unread := c.Query("unread_only") == "true" || c.Query("unread_only") == "1"
	out, err := h.svc.Notices(c.Request.Context(), currentUser(c), unread)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *HTTPHandler) ReadNotice(c *gin.Context) {
	out, err := h.svc.ReadNotice(c.Request.Context(), currentUser(c), c.Param("id"))
	if err != nil {
		httpx.Error(c, err)
		return
	}
	httpx.OK(c, out)
}

// Healthy 提供模块级健康检查，便于 gateway 启动自检。
func (h *HTTPHandler) Healthy(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"module": "locker", "status": "ok"})
}
