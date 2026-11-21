// Package dto 定义智能快寄柜业务的请求与响应结构。
// 金额字段单位为分，时间字段统一 RFC3339 字符串。
package dto

// ---------- 请求 ----------

// QuoteCommand 费用试算入参。
type QuoteCommand struct {
	UserID   string `json:"user_id"`
	Size     string `json:"size" binding:"required"`
	Hours    int    `json:"hours"`
	CouponID string `json:"coupon_id"`
}

// CreateOrderCommand 下单入参。
type CreateOrderCommand struct {
	UserID    string `json:"user_id"`
	StationID string `json:"station_id" binding:"required"`
	CabinetID string `json:"cabinet_id" binding:"required"`
	Hours     int    `json:"hours"`
	CouponID  string `json:"coupon_id"`
}

// PayNotifyCommand 支付回调入参。
type PayNotifyCommand struct {
	PayNo          string `json:"pay_no" binding:"required"`
	OrderNo        string `json:"order_no" binding:"required"`
	AmountFen      int64  `json:"amount_fen"`
	IdempotencyKey string `json:"idempotency_key"`
	Success        *bool  `json:"success"`
}

// RechargeCommand 发起充值入参。
type RechargeCommand struct {
	UserID    string `json:"user_id"`
	AmountFen int64  `json:"amount_fen" binding:"required"`
	Channel   string `json:"channel"`
}

// RechargeNotifyCommand 充值回调入参。
type RechargeNotifyCommand struct {
	RechargeNo     string `json:"recharge_no" binding:"required"`
	AmountFen      int64  `json:"amount_fen"`
	IdempotencyKey string `json:"idempotency_key"`
	Success        *bool  `json:"success"`
}

// ---------- 地点与柜门 ----------

type CityDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Province string `json:"province"`
	Pinyin   string `json:"pinyin"`
}

type CityListDTO struct {
	Items []CityDTO `json:"items"`
	Total int       `json:"total"`
}

type StationDTO struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	CityID       string   `json:"city_id"`
	CityName     string   `json:"city_name"`
	Address      string   `json:"address"`
	Lat          float64  `json:"lat"`
	Lng          float64  `json:"lng"`
	OpenAt       string   `json:"open_at"`
	CloseAt      string   `json:"close_at"`
	Status       string   `json:"status"`
	DistanceM    *float64 `json:"distance_m,omitempty"`
	FreeCount    int      `json:"free_count"`
	CabinetCount int      `json:"cabinet_count"`
}

type StationListDTO struct {
	Items    []StationDTO `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
}

type CabinetDTO struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Size   string `json:"size"`
	Status string `json:"status"`
}

type CabinetListDTO struct {
	StationID   string       `json:"station_id"`
	StationName string       `json:"station_name"`
	FreeCount   int          `json:"free_count"`
	Items       []CabinetDTO `json:"items"`
}

type GuideItemDTO struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type GuideDTO struct {
	Version string         `json:"version"`
	Items   []GuideItemDTO `json:"items"`
}

// ---------- 计费与订单 ----------

type QuoteDTO struct {
	Size         string `json:"size"`
	Hours        int    `json:"hours"`
	UnitPriceFen int64  `json:"unit_price_fen"`
	RentFen      int64  `json:"rent_fen"`
	DepositFen   int64  `json:"deposit_fen"`
	CouponFen    int64  `json:"coupon_fen"`
	CouponTitle  string `json:"coupon_title,omitempty"`
	TotalFen     int64  `json:"total_fen"`
}

type PayDTO struct {
	PayNo          string `json:"pay_no"`
	AmountFen      int64  `json:"amount_fen"`
	Channel        string `json:"channel"`
	IdempotencyKey string `json:"idempotency_key"`
}

type OrderDTO struct {
	OrderNo     string  `json:"order_no"`
	StationID   string  `json:"station_id"`
	StationName string  `json:"station_name"`
	CabinetCode string  `json:"cabinet_code"`
	Size        string  `json:"size"`
	Hours       int     `json:"hours"`
	RentFen     int64   `json:"rent_fen"`
	DepositFen  int64   `json:"deposit_fen"`
	CouponFen   int64   `json:"coupon_fen"`
	TotalFen    int64   `json:"total_fen"`
	Status      string  `json:"status"`
	StartAt     string  `json:"start_at"`
	EndAt       string  `json:"end_at"`
	Pay         *PayDTO `json:"pay,omitempty"`
}

type PayNotifyDTO struct {
	OrderNo    string `json:"order_no"`
	Status     string `json:"status"`
	PaidAt     string `json:"paid_at"`
	Duplicated bool   `json:"duplicated"`
}

type OrderSummaryDTO struct {
	OrderNo     string `json:"order_no"`
	StationName string `json:"station_name"`
	CabinetCode string `json:"cabinet_code"`
	Size        string `json:"size"`
	Status      string `json:"status"`
	TotalFen    int64  `json:"total_fen"`
	OverdueFen  int64  `json:"overdue_fen"`
	EndAt       string `json:"end_at"`
	HoursLeft   int    `json:"hours_left"`
}

type OrderStatsDTO struct {
	Active     int `json:"active"`
	Overdue    int `json:"overdue"`
	PendingPay int `json:"pending_pay"`
	Completed  int `json:"completed"`
}

type OrderListDTO struct {
	Items    []OrderSummaryDTO `json:"items"`
	Total    int64             `json:"total"`
	Stats    OrderStatsDTO     `json:"stats"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

type EventDTO struct {
	Node string `json:"node"`
	Note string `json:"note"`
	At   string `json:"at"`
}

type OrderDetailDTO struct {
	OrderSummaryDTO
	StationAddress string     `json:"station_address"`
	CabinetSize    string     `json:"cabinet_size"`
	RentFen        int64      `json:"rent_fen"`
	DepositFen     int64      `json:"deposit_fen"`
	CouponFen      int64      `json:"coupon_fen"`
	StartAt        string     `json:"start_at"`
	PaidAt         string     `json:"paid_at,omitempty"`
	PickupAt       string     `json:"pickup_at,omitempty"`
	Actions        []string   `json:"actions"`
	Events         []EventDTO `json:"events"`
}

type PickupDTO struct {
	OrderNo       string `json:"order_no"`
	Status        string `json:"status"`
	PickupAt      string `json:"pickup_at"`
	CabinetCode   string `json:"cabinet_code"`
	CabinetStatus string `json:"cabinet_status"`
}

type OverduePayDTO struct {
	OrderNo      string `json:"order_no"`
	Status       string `json:"status"`
	OverdueHours int    `json:"overdue_hours"`
	OverdueFen   int64  `json:"overdue_fen"`
	EndAt        string `json:"end_at"`
}

type GrantDTO struct {
	GrantNo       string `json:"grant_no"`
	OrderNo       string `json:"order_no"`
	ExpireAt      string `json:"expire_at"`
	Remain        int    `json:"remain"`
	Status        string `json:"status"`
	CabinetStatus string `json:"cabinet_status,omitempty"`
}

// ---------- 余额与充值 ----------

type BalanceDTO struct {
	UserID     string `json:"user_id"`
	BalanceFen int64  `json:"balance_fen"`
	FrozenFen  int64  `json:"frozen_fen"`
}

type FlowDTO struct {
	FlowNo    string `json:"flow_no"`
	BizType   string `json:"biz_type"`
	AmountFen int64  `json:"amount_fen"`
	BeforeFen int64  `json:"before_fen"`
	AfterFen  int64  `json:"after_fen"`
	RefNo     string `json:"ref_no"`
	CreatedAt string `json:"created_at"`
}

type FlowListDTO struct {
	Items    []FlowDTO `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

type RechargeDTO struct {
	RechargeNo     string `json:"recharge_no"`
	AmountFen      int64  `json:"amount_fen"`
	Channel        string `json:"channel"`
	Status         string `json:"status"`
	IdempotencyKey string `json:"idempotency_key"`
	CreatedAt      string `json:"created_at,omitempty"`
}

type RechargeNotifyDTO struct {
	RechargeNo string `json:"recharge_no"`
	Status     string `json:"status"`
	BalanceFen int64  `json:"balance_fen"`
	FlowNo     string `json:"flow_no"`
	Duplicated bool   `json:"duplicated"`
}

type RechargeListDTO struct {
	Items    []RechargeDTO `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// ---------- 个人中心与卡券 ----------

type ServiceDTO struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Permission string `json:"permission"`
}

type CouponStatDTO struct {
	Usable  int `json:"usable"`
	Expired int `json:"expired"`
}

type ProfileDTO struct {
	UserID     string        `json:"user_id"`
	Nickname   string        `json:"nickname"`
	AvatarURL  string        `json:"avatar_url"`
	Services   []ServiceDTO  `json:"services"`
	Coupon     CouponStatDTO `json:"coupon"`
	OrderStats OrderStatsDTO `json:"order_stats"`
}

type CouponDTO struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Scope   string `json:"scope"`
	FaceFen int64  `json:"face_fen"`
	MinFen  int64  `json:"min_fen"`
	ValidTo string `json:"valid_to"`
	Status  string `json:"status"`
}

type CouponListDTO struct {
	Items []CouponDTO `json:"items"`
	Total int         `json:"total"`
}

// ---------- 消息通知 ----------

type NoticeDTO struct {
	ID        string `json:"id"`
	Scene     string `json:"scene"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Channel   string `json:"channel"`
	Status    string `json:"status"`
	OrderNo   string `json:"order_no"`
	ReadAt    string `json:"read_at,omitempty"`
	CreatedAt string `json:"created_at"`
}

type NoticeListDTO struct {
	Items  []NoticeDTO `json:"items"`
	Total  int         `json:"total"`
	Unread int         `json:"unread"`
}

type ReadNoticeDTO struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	ReadAt string `json:"read_at"`
}
