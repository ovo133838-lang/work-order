package entity

import (
	"errors"
	"time"
)

// 全栈开发-智能快寄柜系统 物品寄存任务工单 领域常量与规则。
// 金额单位统一为「分」。

const (
	OrderPendingPay = "pending_pay"
	OrderActive     = "active"
	OrderOverdue    = "overdue"
	OrderCompleted  = "completed"
	OrderCancelled  = "cancelled"

	CabinetFree     = "free"
	CabinetLocked   = "locked"
	CabinetOccupied = "occupied"
	CabinetAbnormal = "abnormal"

	PayProcessing = "processing"
	PaySuccess    = "success"
	PayFailed     = "failed"
	PayAbnormal   = "abnormal"

	GrantActive  = "active"
	GrantUsed    = "used"
	GrantExpired = "expired"

	CouponUnused  = "unused"
	CouponUsed    = "used"
	CouponExpired = "expired"

	NoticeSent = "sent"
	NoticeRead = "read"

	SizeS = "S"
	SizeM = "M"
	SizeL = "L"
)

// 领域错误，统一由 service 映射为 pkg/errors 的 Kind。
var (
	ErrOrderNotFound      = errors.New("order not found")
	ErrCabinetNotFound    = errors.New("cabinet not found")
	ErrStationNotFound    = errors.New("station not found")
	ErrCabinetUnavailable = errors.New("cabinet unavailable")
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidState       = errors.New("invalid order state")
	ErrCouponInvalid      = errors.New("coupon invalid")
	ErrAmountMismatch     = errors.New("amount mismatch")
	ErrGrantNotFound      = errors.New("grant not found")
	ErrGrantExpired       = errors.New("grant expired")
	ErrGrantExhausted     = errors.New("grant exhausted")
	ErrRechargeNotFound   = errors.New("recharge not found")
	ErrPaymentNotFound    = errors.New("payment not found")
	ErrNoticeNotFound     = errors.New("notice not found")
)

// Pricing 计费参数，来源于 config.LockerConfig。
type Pricing struct {
	PriceS   int64
	PriceM   int64
	PriceL   int64
	DepositS int64
	DepositM int64
	DepositL int64

	// OverdueRate 超时费率，默认 1.5（即超时按 1.5 倍小时价计费）。
	OverdueRate float64
}

// DefaultPricing 工单默认价目：S 3 元/时、M 4.5 元/时、L 6 元/时；押金 10/15/20 元。
func DefaultPricing() Pricing {
	return Pricing{
		PriceS: 300, PriceM: 450, PriceL: 600,
		DepositS: 1000, DepositM: 1500, DepositL: 2000,
		OverdueRate: 1.5,
	}
}

// Normalize 用默认值补齐零值字段，避免配置文件缺省导致计费为 0。
func (p Pricing) Normalize() Pricing {
	d := DefaultPricing()
	if p.PriceS <= 0 {
		p.PriceS = d.PriceS
	}
	if p.PriceM <= 0 {
		p.PriceM = d.PriceM
	}
	if p.PriceL <= 0 {
		p.PriceL = d.PriceL
	}
	if p.DepositS <= 0 {
		p.DepositS = d.DepositS
	}
	if p.DepositM <= 0 {
		p.DepositM = d.DepositM
	}
	if p.DepositL <= 0 {
		p.DepositL = d.DepositL
	}
	if p.OverdueRate <= 0 {
		p.OverdueRate = d.OverdueRate
	}
	return p
}

// UnitPrice 返回尺寸对应的小时单价。
func UnitPrice(p Pricing, size string) (int64, bool) {
	switch size {
	case SizeS:
		return p.PriceS, true
	case SizeM:
		return p.PriceM, true
	case SizeL:
		return p.PriceL, true
	default:
		return 0, false
	}
}

// Deposit 返回尺寸对应的押金。
func Deposit(p Pricing, size string) (int64, bool) {
	switch size {
	case SizeS:
		return p.DepositS, true
	case SizeM:
		return p.DepositM, true
	case SizeL:
		return p.DepositL, true
	default:
		return 0, false
	}
}

// Rent 按尺寸与小时数计算租金。
func Rent(p Pricing, size string, hours int) (int64, bool) {
	unit, ok := UnitPrice(p, size)
	if !ok || hours <= 0 {
		return 0, false
	}
	return unit * int64(hours), true
}

// OverdueFee 超时费用：按分钟向上取整小时，单价 × OverdueRate。
func OverdueFee(p Pricing, size string, minutes int) int64 {
	unit, ok := UnitPrice(p, size)
	if !ok || minutes <= 0 {
		return 0
	}
	hours := (minutes + 59) / 60
	fee := float64(hours) * float64(unit) * p.OverdueRate
	return int64(fee + 0.5)
}

// EffectiveStatus 读取时惰性判定超时：active 且已过 end_at 视为 overdue。
func EffectiveStatus(status string, endAt, now time.Time) string {
	if status == OrderActive && now.After(endAt) {
		return OrderOverdue
	}
	return status
}

// CouponDiscount 计算券可抵扣金额，返回 0 表示该券不可用于本次订单。
func CouponDiscount(face, min, rent int64) int64 {
	if face <= 0 {
		return 0
	}
	if rent < min {
		return 0
	}
	if face > rent {
		return rent
	}
	return face
}
