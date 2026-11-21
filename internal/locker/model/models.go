// Package model 定义智能快寄柜业务的 Gorm 模型，表名统一 lk_ 前缀。
// 全栈开发-智能快寄柜系统 / 物品寄存任务工单
package model

import "time"

// City 城市。
type City struct {
	ID       string `gorm:"column:id;type:varchar(32);primaryKey" json:"id"`
	Name     string `gorm:"column:name;type:varchar(64);index" json:"name"`
	Province string `gorm:"column:province;type:varchar(64)" json:"province"`
	Pinyin   string `gorm:"column:pinyin;type:varchar(32);index" json:"pinyin"`
}

func (City) TableName() string { return "lk_cities" }

// Station 寄存网点。
type Station struct {
	ID       string  `gorm:"column:id;type:varchar(32);primaryKey" json:"id"`
	Name     string  `gorm:"column:name;type:varchar(128)" json:"name"`
	CityID   string  `gorm:"column:city_id;type:varchar(32);index" json:"city_id"`
	CityName string  `gorm:"column:city_name;type:varchar(64)" json:"city_name"`
	Address  string  `gorm:"column:address;type:varchar(255)" json:"address"`
	Lat      float64 `gorm:"column:lat" json:"lat"`
	Lng      float64 `gorm:"column:lng" json:"lng"`
	OpenAt   string  `gorm:"column:open_at;type:varchar(8)" json:"open_at"`
	CloseAt  string  `gorm:"column:close_at;type:varchar(8)" json:"close_at"`
	Status   string  `gorm:"column:status;type:varchar(16);index" json:"status"`
}

func (Station) TableName() string { return "lk_stations" }

// Cabinet 柜门。
type Cabinet struct {
	ID             string    `gorm:"column:id;type:varchar(40);primaryKey" json:"id"`
	StationID      string    `gorm:"column:station_id;type:varchar(32);index" json:"station_id"`
	Code           string    `gorm:"column:code;type:varchar(32)" json:"code"`
	Size           string    `gorm:"column:size;type:varchar(8)" json:"size"`
	Status         string    `gorm:"column:status;type:varchar(16);index" json:"status"`
	CurrentOrderNo string    `gorm:"column:current_order_no;type:varchar(40);index" json:"current_order_no"`
	Version        int       `gorm:"column:version;default:0" json:"version"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Cabinet) TableName() string { return "lk_cabinets" }

// Order 寄存订单。
type Order struct {
	OrderNo     string     `gorm:"column:order_no;type:varchar(40);primaryKey" json:"order_no"`
	UserID      string     `gorm:"column:user_id;type:varchar(40);index" json:"user_id"`
	StationID   string     `gorm:"column:station_id;type:varchar(32);index" json:"station_id"`
	StationName string     `gorm:"column:station_name;type:varchar(128)" json:"station_name"`
	CabinetID   string     `gorm:"column:cabinet_id;type:varchar(40);index" json:"cabinet_id"`
	CabinetCode string     `gorm:"column:cabinet_code;type:varchar(32)" json:"cabinet_code"`
	Size        string     `gorm:"column:size;type:varchar(8)" json:"size"`
	Hours       int        `gorm:"column:hours" json:"hours"`
	RentFen     int64      `gorm:"column:rent_fen;type:bigint" json:"rent_fen"`
	DepositFen  int64      `gorm:"column:deposit_fen;type:bigint" json:"deposit_fen"`
	CouponID    string     `gorm:"column:coupon_id;type:varchar(40)" json:"coupon_id"`
	CouponFen   int64      `gorm:"column:coupon_fen;type:bigint" json:"coupon_fen"`
	TotalFen    int64      `gorm:"column:total_fen;type:bigint" json:"total_fen"`
	OverdueFen  int64      `gorm:"column:overdue_fen;type:bigint" json:"overdue_fen"`
	Status      string     `gorm:"column:status;type:varchar(16);index" json:"status"`
	StartAt     time.Time  `gorm:"column:start_at" json:"start_at"`
	EndAt       time.Time  `gorm:"column:end_at" json:"end_at"`
	PaidAt      *time.Time `gorm:"column:paid_at" json:"paid_at"`
	PickupAt    *time.Time `gorm:"column:pickup_at" json:"pickup_at"`
	CreatedAt   time.Time  `gorm:"column:created_at;index" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (Order) TableName() string { return "lk_orders" }

// OrderEvent 订单轨迹。
type OrderEvent struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	OrderNo   string    `gorm:"column:order_no;type:varchar(40);index" json:"order_no"`
	Node      string    `gorm:"column:node;type:varchar(32)" json:"node"`
	Note      string    `gorm:"column:note;type:varchar(255)" json:"note"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (OrderEvent) TableName() string { return "lk_order_events" }

// Payment 支付记录。
type Payment struct {
	PayNo          string    `gorm:"column:pay_no;type:varchar(40);primaryKey" json:"pay_no"`
	OrderNo        string    `gorm:"column:order_no;type:varchar(40);index" json:"order_no"`
	UserID         string    `gorm:"column:user_id;type:varchar(40);index" json:"user_id"`
	AmountFen      int64     `gorm:"column:amount_fen;type:bigint" json:"amount_fen"`
	Channel        string    `gorm:"column:channel;type:varchar(16)" json:"channel"`
	Status         string    `gorm:"column:status;type:varchar(16);index" json:"status"`
	IdempotencyKey string    `gorm:"column:idempotency_key;type:varchar(64);uniqueIndex" json:"idempotency_key"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Payment) TableName() string { return "lk_payments" }

// Balance 账户余额。
type Balance struct {
	UserID     string    `gorm:"column:user_id;type:varchar(40);primaryKey" json:"user_id"`
	BalanceFen int64     `gorm:"column:balance_fen;type:bigint" json:"balance_fen"`
	FrozenFen  int64     `gorm:"column:frozen_fen;type:bigint" json:"frozen_fen"`
	UpdatedAt  time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Balance) TableName() string { return "lk_balances" }

// BalanceFlow 余额明细。
type BalanceFlow struct {
	FlowNo    string    `gorm:"column:flow_no;type:varchar(40);primaryKey" json:"flow_no"`
	UserID    string    `gorm:"column:user_id;type:varchar(40);index" json:"user_id"`
	BizType   string    `gorm:"column:biz_type;type:varchar(16)" json:"biz_type"`
	AmountFen int64     `gorm:"column:amount_fen;type:bigint" json:"amount_fen"`
	BeforeFen int64     `gorm:"column:before_fen;type:bigint" json:"before_fen"`
	AfterFen  int64     `gorm:"column:after_fen;type:bigint" json:"after_fen"`
	RefNo     string    `gorm:"column:ref_no;type:varchar(40);index" json:"ref_no"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (BalanceFlow) TableName() string { return "lk_balance_flows" }

// Recharge 充值记录。
type Recharge struct {
	RechargeNo     string    `gorm:"column:recharge_no;type:varchar(40);primaryKey" json:"recharge_no"`
	UserID         string    `gorm:"column:user_id;type:varchar(40);index" json:"user_id"`
	AmountFen      int64     `gorm:"column:amount_fen;type:bigint" json:"amount_fen"`
	Channel        string    `gorm:"column:channel;type:varchar(16)" json:"channel"`
	Status         string    `gorm:"column:status;type:varchar(16);index" json:"status"`
	IdempotencyKey string    `gorm:"column:idempotency_key;type:varchar(64);uniqueIndex" json:"idempotency_key"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (Recharge) TableName() string { return "lk_recharges" }

// Coupon 优惠券。
type Coupon struct {
	ID        string    `gorm:"column:id;type:varchar(40);primaryKey" json:"id"`
	UserID    string    `gorm:"column:user_id;type:varchar(40);index" json:"user_id"`
	Title     string    `gorm:"column:title;type:varchar(64)" json:"title"`
	Scope     string    `gorm:"column:scope;type:varchar(16)" json:"scope"`
	FaceFen   int64     `gorm:"column:face_fen;type:bigint" json:"face_fen"`
	MinFen    int64     `gorm:"column:min_fen;type:bigint" json:"min_fen"`
	ValidFrom time.Time `gorm:"column:valid_from" json:"valid_from"`
	ValidTo   time.Time `gorm:"column:valid_to" json:"valid_to"`
	Status    string    `gorm:"column:status;type:varchar(16);index" json:"status"`
}

func (Coupon) TableName() string { return "lk_coupons" }

// Notice 站内信（消息通知工单）。
type Notice struct {
	ID        string     `gorm:"column:id;type:varchar(40);primaryKey" json:"id"`
	UserID    string     `gorm:"column:user_id;type:varchar(40);index" json:"user_id"`
	Scene     string     `gorm:"column:scene;type:varchar(32)" json:"scene"`
	Title     string     `gorm:"column:title;type:varchar(128)" json:"title"`
	Content   string     `gorm:"column:content;type:varchar(512)" json:"content"`
	Channel   string     `gorm:"column:channel;type:varchar(16)" json:"channel"`
	Status    string     `gorm:"column:status;type:varchar(16)" json:"status"`
	OrderNo   string     `gorm:"column:order_no;type:varchar(40);index" json:"order_no"`
	ReadAt    *time.Time `gorm:"column:read_at" json:"read_at"`
	CreatedAt time.Time  `gorm:"column:created_at" json:"created_at"`
}

func (Notice) TableName() string { return "lk_notices" }

// AccessGrant 临时开柜授权。
type AccessGrant struct {
	GrantNo   string     `gorm:"column:grant_no;type:varchar(40);primaryKey" json:"grant_no"`
	OrderNo   string     `gorm:"column:order_no;type:varchar(40);index" json:"order_no"`
	UserID    string     `gorm:"column:user_id;type:varchar(40)" json:"user_id"`
	ExpireAt  time.Time  `gorm:"column:expire_at" json:"expire_at"`
	Remain    int        `gorm:"column:remain" json:"remain"`
	Status    string     `gorm:"column:status;type:varchar(16);index" json:"status"`
	CreatedAt time.Time  `gorm:"column:created_at" json:"created_at"`
	UsedAt    *time.Time `gorm:"column:used_at" json:"used_at"`
}

func (AccessGrant) TableName() string { return "lk_access_grants" }

// AuditLog 审计日志。
type AuditLog struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Actor     string    `gorm:"column:actor;type:varchar(40)" json:"actor"`
	Action    string    `gorm:"column:action;type:varchar(32)" json:"action"`
	Target    string    `gorm:"column:target;type:varchar(64)" json:"target"`
	Result    string    `gorm:"column:result;type:varchar(16)" json:"result"`
	Detail    string    `gorm:"column:detail;type:varchar(255)" json:"detail"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AuditLog) TableName() string { return "lk_audit_logs" }

// Models 返回全部模型，供 AutoMigrate 使用。
func Models() []any {
	return []any{
		&City{}, &Station{}, &Cabinet{}, &Order{}, &OrderEvent{},
		&Payment{}, &Balance{}, &BalanceFlow{}, &Recharge{},
		&Coupon{}, &Notice{}, &AccessGrant{}, &AuditLog{},
	}
}
