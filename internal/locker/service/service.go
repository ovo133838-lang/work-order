// Package service 实现智能快寄柜业务用例：首页发现、寄存下单、支付回调、订单与取出、余额充值、个人中心与消息通知。
// 对应工单：01 消息通知 / 02 支付宝支付 / 03 首页 / 04 物品寄存 / 05 订单查看与取出 / 06 个人中心。
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/locker/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/locker/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/locker/model"
	"github.com/BwCloudWeGo/bw-cli/internal/locker/repo"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// Config 寄存业务的运行参数。
type Config struct {
	Price          entity.Pricing
	CabinetLockTTL time.Duration
	CacheTTL       time.Duration
	GrantTTL       time.Duration
	GrantMaxUses   int
	PageSizeMax    int
}

// DefaultConfig 返回可直接运行的默认参数。
func DefaultConfig() Config {
	return Config{
		Price:          entity.DefaultPricing(),
		CabinetLockTTL: 20 * time.Second,
		CacheTTL:       60 * time.Second,
		GrantTTL:       15 * time.Minute,
		GrantMaxUses:   2,
		PageSizeMax:    100,
	}
}

// Normalize 补齐零值。
func (c Config) Normalize() Config {
	d := DefaultConfig()
	c.Price = c.Price.Normalize()
	if c.CabinetLockTTL <= 0 {
		c.CabinetLockTTL = d.CabinetLockTTL
	}
	if c.CacheTTL <= 0 {
		c.CacheTTL = d.CacheTTL
	}
	if c.GrantTTL <= 0 {
		c.GrantTTL = d.GrantTTL
	}
	if c.GrantMaxUses <= 0 {
		c.GrantMaxUses = d.GrantMaxUses
	}
	if c.PageSizeMax <= 0 {
		c.PageSizeMax = d.PageSizeMax
	}
	return c
}

// Service 寄存业务用例集合。
type Service struct {
	repo *repo.Repo
	log  *zap.Logger
	cfg  Config
}

// New 创建业务服务。
func New(r *repo.Repo, log *zap.Logger, cfg Config) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{repo: r, log: log, cfg: cfg.Normalize()}
}

// ---------- 通用 ----------

func ts(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func newNo(prefix string) string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return prefix + time.Now().Format("20060102150405") + strings.ToUpper(hex.EncodeToString(b[:]))
}

func pageOf(page, size, max int) (int, int, int) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > max {
		size = max
	}
	return page, size, (page - 1) * size
}

// fail 把领域错误映射为稳定的应用错误，未知错误统一落 500 并记录日志。
func (s *Service) fail(err error) error {
	if err == nil {
		return nil
	}
	var appErr *apperrors.AppError
	if stderrors.As(err, &appErr) {
		return err
	}
	switch {
	case stderrors.Is(err, entity.ErrOrderNotFound):
		return apperrors.NotFound("order_not_found", "order not found")
	case stderrors.Is(err, entity.ErrCabinetNotFound):
		return apperrors.NotFound("cabinet_not_found", "cabinet not found")
	case stderrors.Is(err, entity.ErrStationNotFound):
		return apperrors.NotFound("station_not_found", "station not found")
	case stderrors.Is(err, entity.ErrPaymentNotFound):
		return apperrors.NotFound("payment_not_found", "payment not found")
	case stderrors.Is(err, entity.ErrRechargeNotFound):
		return apperrors.NotFound("recharge_not_found", "recharge not found")
	case stderrors.Is(err, entity.ErrGrantNotFound):
		return apperrors.NotFound("grant_not_found", "grant not found")
	case stderrors.Is(err, entity.ErrNoticeNotFound):
		return apperrors.NotFound("notice_not_found", "notice not found")
	case stderrors.Is(err, entity.ErrCabinetUnavailable):
		return apperrors.Conflict("cabinet_unavailable", "cabinet is occupied or locked")
	case stderrors.Is(err, entity.ErrAmountMismatch):
		return apperrors.Conflict("amount_mismatch", "payment amount does not match order")
	case stderrors.Is(err, entity.ErrCouponInvalid):
		return apperrors.Conflict("coupon_invalid", "coupon is unusable for this order")
	case stderrors.Is(err, entity.ErrGrantExpired):
		return apperrors.Conflict("grant_expired", "grant expired")
	case stderrors.Is(err, entity.ErrGrantExhausted):
		return apperrors.Conflict("grant_exhausted", "grant has no remaining use")
	case stderrors.Is(err, entity.ErrInvalidState):
		return apperrors.Conflict("invalid_order_state", "order state does not allow this action")
	case stderrors.Is(err, entity.ErrInvalidInput):
		return apperrors.InvalidArgument("invalid_argument", "invalid argument")
	default:
		s.log.Error("locker service internal error", zap.Error(err))
		return apperrors.Internal("internal_error", "internal error")
	}
}

// ---------- 首页与网点 ----------

// Cities 城市列表（工单 03 城市定位与选择）。
func (s *Service) Cities(ctx context.Context, keyword string) (*dto.CityListDTO, error) {
	items, err := s.repo.ListCities(ctx, strings.TrimSpace(keyword))
	if err != nil {
		return nil, s.fail(err)
	}
	out := &dto.CityListDTO{Items: make([]dto.CityDTO, 0, len(items)), Total: len(items)}
	for _, c := range items {
		out.Items = append(out.Items, dto.CityDTO{ID: c.ID, Name: c.Name, Province: c.Province, Pinyin: c.Pinyin})
	}
	return out, nil
}

// Stations 网点列表，提供经纬度时按距离升序。
func (s *Service) Stations(ctx context.Context, cityID, keyword string, lat, lng *float64, page, size int) (*dto.StationListDTO, error) {
	items, err := s.repo.ListStations(ctx, cityID, strings.TrimSpace(keyword))
	if err != nil {
		return nil, s.fail(err)
	}
	out := &dto.StationListDTO{Items: make([]dto.StationDTO, 0, len(items)), Total: len(items)}
	for _, st := range items {
		cabinets, err := s.repo.ListCabinets(ctx, st.ID)
		if err != nil {
			return nil, s.fail(err)
		}
		free := 0
		for _, c := range cabinets {
			if c.Status == entity.CabinetFree {
				free++
			}
		}
		d := dto.StationDTO{
			ID: st.ID, Name: st.Name, CityID: st.CityID, CityName: st.CityName,
			Address: st.Address, Lat: st.Lat, Lng: st.Lng, OpenAt: st.OpenAt, CloseAt: st.CloseAt,
			Status: st.Status, FreeCount: free, CabinetCount: len(cabinets),
		}
		if lat != nil && lng != nil {
			dist := float64(distanceMeters(*lat, *lng, st.Lat, st.Lng))
			d.DistanceM = &dist
		}
		out.Items = append(out.Items, d)
	}
	if lat != nil && lng != nil {
		sort.SliceStable(out.Items, func(i, j int) bool {
			if out.Items[i].DistanceM == nil || out.Items[j].DistanceM == nil {
				return false
			}
			return *out.Items[i].DistanceM < *out.Items[j].DistanceM
		})
	}
	_, size, offset := pageOf(page, size, s.cfg.PageSizeMax)
	out.Page = page
	out.PageSize = size
	if offset >= len(out.Items) {
		out.Items = []dto.StationDTO{}
	} else {
		out.Items = out.Items[offset:]
		if len(out.Items) > size {
			out.Items = out.Items[:size]
		}
	}
	return out, nil
}

func distanceMeters(lat1, lng1, lat2, lng2 float64) int {
	const r = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return int(2 * r * math.Asin(math.Sqrt(a)))
}

// Cabinets 网点柜门列表（工单 04 柜门查询）。
func (s *Service) Cabinets(ctx context.Context, stationID string) (*dto.CabinetListDTO, error) {
	cacheKey := "lk:cache:station:" + stationID + ":cabinets"
	var cached dto.CabinetListDTO
	if s.repo.CacheGet(ctx, cacheKey, &cached) {
		return &cached, nil
	}
	st, err := s.repo.GetStation(ctx, stationID)
	if err != nil {
		return nil, s.fail(err)
	}
	cabinets, err := s.repo.ListCabinets(ctx, stationID)
	if err != nil {
		return nil, s.fail(err)
	}
	out := &dto.CabinetListDTO{StationID: st.ID, StationName: st.Name, Items: make([]dto.CabinetDTO, 0, len(cabinets))}
	for _, c := range cabinets {
		if c.Status == entity.CabinetFree {
			out.FreeCount++
		}
		out.Items = append(out.Items, dto.CabinetDTO{ID: c.ID, Code: c.Code, Size: c.Size, Status: c.Status})
	}
	s.repo.CacheSet(ctx, cacheKey, out, s.cfg.CacheTTL)
	return out, nil
}

// Guide 使用引导。
func (s *Service) Guide(_ context.Context) *dto.GuideDTO {
	return &dto.GuideDTO{
		Version: "v1.0",
		Items: []dto.GuideItemDTO{
			{Title: "寄存流程", Content: "选择网点与柜门 → 选择寄存时长 → 在线支付 → 生成寄存凭证 → 到场输入凭证开柜"},
			{Title: "计费说明", Content: fmt.Sprintf("按小时计费：S %d 分/时、M %d 分/时、L %d 分/时；押金 S %d 分、M %d 分、L %d 分，取件后原路退回。",
				s.cfg.Price.PriceS, s.cfg.Price.PriceM, s.cfg.Price.PriceL,
				s.cfg.Price.DepositS, s.cfg.Price.DepositM, s.cfg.Price.DepositL)},
			{Title: "超时规则", Content: fmt.Sprintf("超出寄存时长按 %.1f 倍小时价计收超时费，需补缴后方可取件。", s.cfg.Price.OverdueRate)},
			{Title: "注意事项", Content: "贵重物品请自行保管；柜门关闭后请确认锁扣到位；异常请联系网点工作人员。"},
		},
	}
}

// ---------- 计费 ----------

// Quote 费用试算（工单 04 计费规则）。
func (s *Service) Quote(ctx context.Context, cmd dto.QuoteCommand) (*dto.QuoteDTO, error) {
	size := strings.ToUpper(strings.TrimSpace(cmd.Size))
	if _, ok := entity.UnitPrice(s.cfg.Price, size); !ok {
		return nil, apperrors.InvalidArgument("size_invalid", "size must be S, M or L")
	}
	hours := cmd.Hours
	if hours <= 0 {
		hours = 1
	}
	if hours > 24*7 {
		return nil, apperrors.InvalidArgument("hours_invalid", "hours must be within 1..168")
	}
	rent, _ := entity.Rent(s.cfg.Price, size, hours)
	deposit, _ := entity.Deposit(s.cfg.Price, size)
	unit, _ := entity.UnitPrice(s.cfg.Price, size)

	var couponFen int64
	var couponTitle string
	if strings.TrimSpace(cmd.CouponID) != "" {
		coupon, err := s.repo.GetCoupon(ctx, cmd.CouponID)
		if err != nil {
			return nil, s.fail(err)
		}
		face, title, err := s.couponDiscount(ctx, coupon, cmd.UserID, size, rent)
		if err != nil {
			return nil, s.fail(err)
		}
		couponFen, couponTitle = face, title
	}

	return &dto.QuoteDTO{
		Size: size, Hours: hours, UnitPriceFen: unit, RentFen: rent, DepositFen: deposit,
		CouponFen: couponFen, CouponTitle: couponTitle,
		TotalFen: rent + deposit - couponFen,
	}, nil
}

// couponDiscount 校验券可用性并返回抵扣金额。
func (s *Service) couponDiscount(ctx context.Context, coupon *model.Coupon, userID, size string, rent int64) (int64, string, error) {
	now := time.Now()
	if userID != "" && coupon.UserID != userID {
		return 0, "", entity.ErrCouponInvalid
	}
	if coupon.Status != entity.CouponUnused {
		return 0, "", entity.ErrCouponInvalid
	}
	if now.Before(coupon.ValidFrom) || now.After(coupon.ValidTo) {
		return 0, "", entity.ErrCouponInvalid
	}
	if coupon.Scope != "" && coupon.Scope != "all" && coupon.Scope != size {
		return 0, "", entity.ErrCouponInvalid
	}
	face := entity.CouponDiscount(coupon.FaceFen, coupon.MinFen, rent)
	if face <= 0 {
		return 0, "", entity.ErrCouponInvalid
	}
	return face, coupon.Title, nil
}

// ---------- 下单与支付 ----------

// CreateOrder 下单并锁柜（工单 04）。
func (s *Service) CreateOrder(ctx context.Context, cmd dto.CreateOrderCommand) (*dto.OrderDTO, error) {
	hours := cmd.Hours
	if hours <= 0 {
		hours = 1
	}
	if hours > 24*7 {
		return nil, apperrors.InvalidArgument("hours_invalid", "hours must be within 1..168")
	}
	st, err := s.repo.GetStation(ctx, cmd.StationID)
	if err != nil {
		return nil, s.fail(err)
	}
	cabinet, err := s.repo.GetCabinet(ctx, cmd.CabinetID)
	if err != nil {
		return nil, s.fail(err)
	}
	if cabinet.StationID != st.ID {
		return nil, apperrors.InvalidArgument("cabinet_station_mismatch", "cabinet does not belong to station")
	}
	if cabinet.Status != entity.CabinetFree {
		return nil, apperrors.Conflict("cabinet_unavailable", "cabinet is occupied or locked")
	}

	// 先抢 Redis 柜门锁，降低并发下的无谓事务争用。
	if !s.repo.TryLockCabinet(ctx, cabinet.ID, s.cfg.CabinetLockTTL) {
		return nil, apperrors.Conflict("cabinet_unavailable", "cabinet is being locked by another request")
	}
	defer s.repo.UnlockCabinet(ctx, cabinet.ID)

	size := cabinet.Size
	rent, _ := entity.Rent(s.cfg.Price, size, hours)
	deposit, _ := entity.Deposit(s.cfg.Price, size)

	var couponFen int64
	couponID := ""
	if strings.TrimSpace(cmd.CouponID) != "" {
		coupon, err := s.repo.GetCoupon(ctx, cmd.CouponID)
		if err != nil {
			return nil, s.fail(err)
		}
		face, _, err := s.couponDiscount(ctx, coupon, cmd.UserID, size, rent)
		if err != nil {
			return nil, s.fail(err)
		}
		// 条件更新保证一张券只会被一笔订单占用。
		used, err := s.repo.UseCoupon(ctx, coupon.ID)
		if err != nil {
			return nil, s.fail(err)
		}
		if !used {
			return nil, apperrors.Conflict("coupon_invalid", "coupon has been used by another order")
		}
		couponFen, couponID = face, coupon.ID
	}

	now := time.Now()
	orderNo := newNo("LK")
	order := &model.Order{
		OrderNo: orderNo, UserID: cmd.UserID, StationID: st.ID, StationName: st.Name,
		CabinetID: cabinet.ID, CabinetCode: cabinet.Code, Size: size, Hours: hours,
		RentFen: rent, DepositFen: deposit, CouponID: couponID, CouponFen: couponFen,
		TotalFen: rent + deposit - couponFen, Status: entity.OrderPendingPay,
		StartAt: now, EndAt: now.Add(time.Duration(hours) * time.Hour),
		CreatedAt: now, UpdatedAt: now,
	}

	// 数据库条件更新锁定柜门，失败说明已被并发请求抢占。
	locked, err := s.repo.LockCabinet(ctx, cabinet.ID, entity.CabinetFree, orderNo)
	if err != nil {
		return nil, s.fail(err)
	}
	if !locked {
		return nil, apperrors.Conflict("cabinet_unavailable", "cabinet is occupied or locked")
	}
	if err := s.repo.CreateOrder(ctx, order); err != nil {
		_, _ = s.repo.FreeCabinet(ctx, cabinet.ID)
		return nil, s.fail(err)
	}

	payNo := newNo("PAY")
	payment := &model.Payment{
		PayNo: payNo, OrderNo: orderNo, UserID: cmd.UserID, AmountFen: order.TotalFen,
		Channel: "alipay", Status: entity.PayProcessing, IdempotencyKey: payNo,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.CreatePayment(ctx, payment); err != nil {
		return nil, s.fail(err)
	}

	_ = s.repo.AddEvent(ctx, orderNo, "created", fmt.Sprintf("下单锁柜 %s，%d 小时", cabinet.Code, hours))
	s.notify(ctx, cmd.UserID, "pickup_reminder", "寄存凭证已生成", fmt.Sprintf("订单 %s 已锁定柜门 %s，到场输入凭证开柜寄存。", orderNo, cabinet.Code), orderNo)
	s.repo.Audit(ctx, cmd.UserID, "create_order", orderNo, "success", fmt.Sprintf("cabinet=%s total=%d", cabinet.ID, order.TotalFen))
	s.repo.CacheDel(ctx, "lk:cache:station:"+st.ID+":cabinets")

	return &dto.OrderDTO{
		OrderNo: orderNo, StationID: st.ID, StationName: st.Name, CabinetCode: cabinet.Code,
		Size: size, Hours: hours, RentFen: rent, DepositFen: deposit, CouponFen: couponFen,
		TotalFen: order.TotalFen, Status: order.Status,
		StartAt: ts(order.StartAt), EndAt: ts(order.EndAt),
		Pay: &dto.PayDTO{PayNo: payNo, AmountFen: order.TotalFen, Channel: "alipay", IdempotencyKey: payNo},
	}, nil
}

// PayNotify 支付回调（工单 02）：幂等 + 金额校验 + 驱动订单与柜门状态。
func (s *Service) PayNotify(ctx context.Context, cmd dto.PayNotifyCommand) (*dto.PayNotifyDTO, error) {
	payment, err := s.repo.GetPayment(ctx, cmd.PayNo)
	if err != nil {
		return nil, s.fail(err)
	}
	if payment.OrderNo != cmd.OrderNo {
		return nil, apperrors.Conflict("order_mismatch", "payment does not belong to order")
	}
	if cmd.AmountFen > 0 && cmd.AmountFen != payment.AmountFen {
		s.repo.Audit(ctx, payment.UserID, "pay_notify", cmd.OrderNo, "failed", "amount mismatch")
		return nil, s.fail(entity.ErrAmountMismatch)
	}

	order, err := s.repo.GetOrder(ctx, cmd.OrderNo)
	if err != nil {
		return nil, s.fail(err)
	}

	// 幂等：同一支付单的成功回调重复到达时直接返回当前结果。
	if payment.Status == entity.PaySuccess {
		return &dto.PayNotifyDTO{
			OrderNo: order.OrderNo, Status: order.Status, PaidAt: ts(derefTime(order.PaidAt)), Duplicated: true,
		}, nil
	}
	idemKey := strings.TrimSpace(cmd.IdempotencyKey)
	if idemKey == "" {
		idemKey = cmd.PayNo
	}
	if !s.repo.MarkIdempotent(ctx, "pay", idemKey, 24*time.Hour) {
		return &dto.PayNotifyDTO{
			OrderNo: order.OrderNo, Status: order.Status, PaidAt: ts(derefTime(order.PaidAt)), Duplicated: true,
		}, nil
	}

	success := cmd.Success == nil || *cmd.Success
	now := time.Now()
	if !success {
		payment.Status = entity.PayFailed
		if err := s.repo.SavePayment(ctx, payment); err != nil {
			return nil, s.fail(err)
		}
		if order.Status == entity.OrderPendingPay {
			_ = s.repo.UpdateOrderFields(ctx, order.OrderNo, map[string]any{"status": entity.OrderCancelled})
			_, _ = s.repo.FreeCabinet(ctx, order.CabinetID)
		}
		_ = s.repo.AddEvent(ctx, order.OrderNo, "pay_failed", "支付失败，柜门已释放")
		s.repo.Audit(ctx, payment.UserID, "pay_notify", order.OrderNo, "failed", "channel reported failure")
		return &dto.PayNotifyDTO{OrderNo: order.OrderNo, Status: entity.OrderCancelled, PaidAt: "", Duplicated: false}, nil
	}

	payment.Status = entity.PaySuccess
	if err := s.repo.SavePayment(ctx, payment); err != nil {
		return nil, s.fail(err)
	}

	occupied, err := s.repo.UpdateCabinetStatus(ctx, order.CabinetID, entity.CabinetLocked, entity.CabinetOccupied, order.OrderNo)
	if err != nil {
		return nil, s.fail(err)
	}
	if !occupied {
		s.log.Warn("cabinet state changed before payment success", zap.String("order_no", order.OrderNo))
	}

	paidAt := now
	if err := s.repo.UpdateOrderFields(ctx, order.OrderNo, map[string]any{
		"status": entity.OrderActive, "paid_at": paidAt,
	}); err != nil {
		return nil, s.fail(err)
	}
	order.Status = entity.OrderActive
	order.PaidAt = &paidAt

	_ = s.repo.AddEvent(ctx, order.OrderNo, "paid", fmt.Sprintf("支付成功 %d 分", payment.AmountFen))
	s.notify(ctx, payment.UserID, "order_paid", "支付成功", fmt.Sprintf("订单 %s 支付成功，柜门 %s 已可使用。", order.OrderNo, order.CabinetCode), order.OrderNo)
	s.repo.Audit(ctx, payment.UserID, "pay_notify", order.OrderNo, "success", fmt.Sprintf("amount=%d", payment.AmountFen))
	s.repo.CacheDel(ctx, "lk:cache:station:"+order.StationID+":cabinets")

	return &dto.PayNotifyDTO{OrderNo: order.OrderNo, Status: order.Status, PaidAt: ts(paidAt), Duplicated: false}, nil
}

// ---------- 订单查看与取出 ----------

// ListOrders 当前订单与历史订单（工单 05），读取前惰性修正超时订单。
func (s *Service) ListOrders(ctx context.Context, userID, status string, page, size int) (*dto.OrderListDTO, error) {
	if _, err := s.repo.ExpireActiveOrders(ctx, userID, time.Now()); err != nil {
		return nil, s.fail(err)
	}
	page, size, offset := pageOf(page, size, s.cfg.PageSizeMax)
	items, total, err := s.repo.ListOrders(ctx, userID, status, offset, size)
	if err != nil {
		return nil, s.fail(err)
	}
	stats, err := s.repo.OrderStats(ctx, userID)
	if err != nil {
		return nil, s.fail(err)
	}
	now := time.Now()
	out := &dto.OrderListDTO{
		Items: make([]dto.OrderSummaryDTO, 0, len(items)), Total: total, Page: page, PageSize: size,
		Stats: dto.OrderStatsDTO{
			Active: int(stats[entity.OrderActive]), Overdue: int(stats[entity.OrderOverdue]),
			PendingPay: int(stats[entity.OrderPendingPay]), Completed: int(stats[entity.OrderCompleted]),
		},
	}
	for _, o := range items {
		out.Items = append(out.Items, summaryOf(o, now))
	}
	return out, nil
}

func summaryOf(o model.Order, now time.Time) dto.OrderSummaryDTO {
	hoursLeft := 0
	if o.Status == entity.OrderActive {
		left := int(math.Ceil(o.EndAt.Sub(now).Hours()))
		if left > 0 {
			hoursLeft = left
		}
	}
	return dto.OrderSummaryDTO{
		OrderNo: o.OrderNo, StationName: o.StationName, CabinetCode: o.CabinetCode, Size: o.Size,
		Status: o.Status, TotalFen: o.TotalFen, OverdueFen: o.OverdueFen,
		EndAt: ts(o.EndAt), HoursLeft: hoursLeft,
	}
}

// OrderDetail 订单详情（工单 05 查看详细）。
func (s *Service) OrderDetail(ctx context.Context, userID, orderNo string) (*dto.OrderDetailDTO, error) {
	order, err := s.repo.GetOrder(ctx, orderNo)
	if err != nil {
		return nil, s.fail(err)
	}
	if userID != "" && order.UserID != userID {
		return nil, apperrors.NotFound("order_not_found", "order not found")
	}
	now := time.Now()
	eff := entity.EffectiveStatus(order.Status, order.EndAt, now)
	if eff == entity.OrderOverdue && order.Status == entity.OrderActive {
		_ = s.repo.UpdateOrderFields(ctx, order.OrderNo, map[string]any{"status": entity.OrderOverdue})
		_ = s.repo.AddEvent(ctx, order.OrderNo, "overdue", "已超时，需补缴后取件")
		s.notify(ctx, order.UserID, "order_overdue", "寄存超时", fmt.Sprintf("订单 %s 已超时，请补缴超时费后取件。", order.OrderNo), order.OrderNo)
		order.Status = entity.OrderOverdue
	}

	events, err := s.repo.ListEvents(ctx, orderNo)
	if err != nil {
		return nil, s.fail(err)
	}
	address := ""
	if st, err := s.repo.GetStation(ctx, order.StationID); err == nil {
		address = st.Address
	}

	out := &dto.OrderDetailDTO{
		OrderSummaryDTO: summaryOf(*order, now),
		StationAddress:  address,
		CabinetSize:     order.Size,
		RentFen:         order.RentFen,
		DepositFen:      order.DepositFen,
		CouponFen:       order.CouponFen,
		StartAt:         ts(order.StartAt),
		PaidAt:          ts(derefTime(order.PaidAt)),
		PickupAt:        ts(derefTime(order.PickupAt)),
		Actions:         []string{},
		Events:          make([]dto.EventDTO, 0, len(events)),
	}
	switch order.Status {
	case entity.OrderActive:
		out.Actions = append(out.Actions, "pickup", "grant")
	case entity.OrderOverdue:
		out.Actions = append(out.Actions, "overdue_pay")
	case entity.OrderPendingPay:
		out.Actions = append(out.Actions, "pay")
	}
	for _, e := range events {
		out.Events = append(out.Events, dto.EventDTO{Node: e.Node, Note: e.Note, At: ts(e.CreatedAt)})
	}
	return out, nil
}

// Pickup 取出物品（工单 05）：释放柜门并把订单置为 completed。
func (s *Service) Pickup(ctx context.Context, userID, orderNo string) (*dto.PickupDTO, error) {
	order, err := s.repo.GetOrder(ctx, orderNo)
	if err != nil {
		return nil, s.fail(err)
	}
	if userID != "" && order.UserID != userID {
		return nil, apperrors.NotFound("order_not_found", "order not found")
	}
	now := time.Now()
	if entity.EffectiveStatus(order.Status, order.EndAt, now) == entity.OrderOverdue {
		return nil, apperrors.Conflict("order_overdue", "order is overdue, pay overdue fee before pickup")
	}
	if order.Status != entity.OrderActive {
		return nil, s.fail(entity.ErrInvalidState)
	}

	freed, err := s.repo.FreeCabinet(ctx, order.CabinetID)
	if err != nil {
		return nil, s.fail(err)
	}
	if !freed {
		s.log.Warn("cabinet already released", zap.String("order_no", orderNo))
	}
	pickupAt := now
	if err := s.repo.UpdateOrderFields(ctx, order.OrderNo, map[string]any{
		"status": entity.OrderCompleted, "pickup_at": pickupAt,
	}); err != nil {
		return nil, s.fail(err)
	}
	_ = s.repo.AddEvent(ctx, order.OrderNo, "pickup", "取件成功，柜门释放")
	s.notify(ctx, order.UserID, "order_completed", "寄存完成", fmt.Sprintf("订单 %s 已取件，押金 %d 分原路退回。", order.OrderNo, order.DepositFen), order.OrderNo)
	s.repo.Audit(ctx, order.UserID, "pickup", order.OrderNo, "success", "cabinet released")
	s.repo.CacheDel(ctx, "lk:cache:station:"+order.StationID+":cabinets")

	return &dto.PickupDTO{
		OrderNo: order.OrderNo, Status: entity.OrderCompleted, PickupAt: ts(pickupAt),
		CabinetCode: order.CabinetCode, CabinetStatus: entity.CabinetFree,
	}, nil
}

// OverduePay 超时补缴（工单 05 超时计费规则）。
func (s *Service) OverduePay(ctx context.Context, userID, orderNo string) (*dto.OverduePayDTO, error) {
	order, err := s.repo.GetOrder(ctx, orderNo)
	if err != nil {
		return nil, s.fail(err)
	}
	if userID != "" && order.UserID != userID {
		return nil, apperrors.NotFound("order_not_found", "order not found")
	}
	now := time.Now()
	if !now.After(order.EndAt) {
		return nil, apperrors.Conflict("order_not_overdue", "order has not exceeded reserved hours")
	}
	if order.Status == entity.OrderCompleted {
		return nil, s.fail(entity.ErrInvalidState)
	}
	minutes := int(now.Sub(order.EndAt).Minutes())
	if minutes < 1 {
		minutes = 1
	}
	fee := entity.OverdueFee(s.cfg.Price, order.Size, minutes)
	hours := (minutes + 59) / 60

	_, err = s.repo.FreeCabinet(ctx, order.CabinetID)
	if err != nil {
		return nil, s.fail(err)
	}
	pickupAt := now
	if err := s.repo.UpdateOrderFields(ctx, order.OrderNo, map[string]any{
		"status": entity.OrderCompleted, "overdue_fen": order.OverdueFen + fee, "pickup_at": pickupAt,
	}); err != nil {
		return nil, s.fail(err)
	}
	_ = s.repo.AddEvent(ctx, order.OrderNo, "overdue_pay", fmt.Sprintf("超时 %d 小时，补缴 %d 分", hours, fee))
	s.notify(ctx, order.UserID, "order_completed", "超时费已补缴", fmt.Sprintf("订单 %s 补缴 %d 分，柜门已释放。", order.OrderNo, fee), order.OrderNo)
	s.repo.Audit(ctx, order.UserID, "overdue_pay", order.OrderNo, "success", fmt.Sprintf("fee=%d", fee))

	return &dto.OverduePayDTO{
		OrderNo: order.OrderNo, Status: entity.OrderCompleted, OverdueHours: hours,
		OverdueFen: order.OverdueFen + fee, EndAt: ts(now),
	}, nil
}

// CreateGrant 临时开柜授权（工单 05 授权他人取件）。
func (s *Service) CreateGrant(ctx context.Context, userID, orderNo string) (*dto.GrantDTO, error) {
	order, err := s.repo.GetOrder(ctx, orderNo)
	if err != nil {
		return nil, s.fail(err)
	}
	if userID != "" && order.UserID != userID {
		return nil, apperrors.NotFound("order_not_found", "order not found")
	}
	eff := entity.EffectiveStatus(order.Status, order.EndAt, time.Now())
	if eff != entity.OrderActive && eff != entity.OrderOverdue {
		return nil, s.fail(entity.ErrInvalidState)
	}
	now := time.Now()
	grant := &model.AccessGrant{
		GrantNo: newNo("GR"), OrderNo: order.OrderNo, UserID: order.UserID,
		ExpireAt: now.Add(s.cfg.GrantTTL), Remain: s.cfg.GrantMaxUses,
		Status: entity.GrantActive, CreatedAt: now,
	}
	if err := s.repo.CreateGrant(ctx, grant); err != nil {
		return nil, s.fail(err)
	}
	_ = s.repo.AddEvent(ctx, order.OrderNo, "granted", fmt.Sprintf("生成授权 %s，%d 次", grant.GrantNo, grant.Remain))
	s.repo.Audit(ctx, order.UserID, "grant", order.OrderNo, "success", grant.GrantNo)

	return &dto.GrantDTO{
		GrantNo: grant.GrantNo, OrderNo: grant.OrderNo, ExpireAt: ts(grant.ExpireAt),
		Remain: grant.Remain, Status: grant.Status,
	}, nil
}

// UseGrant 授权核销（工单 05 临时开柜）。
func (s *Service) UseGrant(ctx context.Context, userID, grantNo string) (*dto.GrantDTO, error) {
	grant, err := s.repo.GetGrant(ctx, grantNo)
	if err != nil {
		return nil, s.fail(err)
	}
	if userID != "" && grant.UserID != userID {
		return nil, apperrors.NotFound("grant_not_found", "grant not found")
	}
	now := time.Now()
	if now.After(grant.ExpireAt) || grant.Status == entity.GrantExpired {
		return nil, s.fail(entity.ErrGrantExpired)
	}
	if grant.Remain <= 0 || grant.Status == entity.GrantUsed {
		return nil, s.fail(entity.ErrGrantExhausted)
	}
	ok, err := s.repo.ConsumeGrant(ctx, grant.GrantNo, now)
	if err != nil {
		return nil, s.fail(err)
	}
	if !ok {
		return nil, s.fail(entity.ErrGrantExhausted)
	}
	grant.Remain--
	if grant.Remain <= 0 {
		grant.Status = entity.GrantUsed
	}
	_ = s.repo.AddEvent(ctx, grant.OrderNo, "grant_used", fmt.Sprintf("授权 %s 已使用，剩余 %d 次", grant.GrantNo, grant.Remain))
	s.notify(ctx, grant.UserID, "grant_used", "临时开柜已使用", fmt.Sprintf("授权 %s 已开柜，剩余 %d 次。", grant.GrantNo, grant.Remain), grant.OrderNo)
	s.repo.Audit(ctx, grant.UserID, "grant_use", grant.GrantNo, "success", fmt.Sprintf("remain=%d", grant.Remain))

	return &dto.GrantDTO{
		GrantNo: grant.GrantNo, OrderNo: grant.OrderNo, ExpireAt: ts(grant.ExpireAt),
		Remain: grant.Remain, Status: grant.Status, CabinetStatus: "opening",
	}, nil
}

// ---------- 余额与充值（工单 02） ----------

// Balance 账户余额。
func (s *Service) Balance(ctx context.Context, userID string) (*dto.BalanceDTO, error) {
	b, err := s.repo.EnsureBalance(ctx, userID)
	if err != nil {
		return nil, s.fail(err)
	}
	return &dto.BalanceDTO{UserID: b.UserID, BalanceFen: b.BalanceFen, FrozenFen: b.FrozenFen}, nil
}

// Flows 余额明细。
func (s *Service) Flows(ctx context.Context, userID string, page, size int) (*dto.FlowListDTO, error) {
	page, size, offset := pageOf(page, size, s.cfg.PageSizeMax)
	items, total, err := s.repo.ListFlows(ctx, userID, offset, size)
	if err != nil {
		return nil, s.fail(err)
	}
	out := &dto.FlowListDTO{Items: make([]dto.FlowDTO, 0, len(items)), Total: total, Page: page, PageSize: size}
	for _, f := range items {
		out.Items = append(out.Items, dto.FlowDTO{
			FlowNo: f.FlowNo, BizType: f.BizType, AmountFen: f.AmountFen,
			BeforeFen: f.BeforeFen, AfterFen: f.AfterFen, RefNo: f.RefNo, CreatedAt: ts(f.CreatedAt),
		})
	}
	return out, nil
}

// CreateRecharge 发起充值。
func (s *Service) CreateRecharge(ctx context.Context, cmd dto.RechargeCommand) (*dto.RechargeDTO, error) {
	if cmd.AmountFen <= 0 {
		return nil, apperrors.InvalidArgument("amount_invalid", "amount must be positive")
	}
	if cmd.AmountFen > 100000 {
		return nil, apperrors.InvalidArgument("amount_invalid", "single recharge must not exceed 1000 yuan")
	}
	channel := cmd.Channel
	if channel == "" {
		channel = "alipay"
	}
	now := time.Now()
	no := newNo("RC")
	rc := &model.Recharge{
		RechargeNo: no, UserID: cmd.UserID, AmountFen: cmd.AmountFen, Channel: channel,
		Status: entity.PayProcessing, IdempotencyKey: no, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.CreateRecharge(ctx, rc); err != nil {
		return nil, s.fail(err)
	}
	s.repo.Audit(ctx, cmd.UserID, "create_recharge", no, "success", fmt.Sprintf("amount=%d", cmd.AmountFen))
	return &dto.RechargeDTO{
		RechargeNo: no, AmountFen: rc.AmountFen, Channel: rc.Channel,
		Status: rc.Status, IdempotencyKey: rc.IdempotencyKey, CreatedAt: ts(rc.CreatedAt),
	}, nil
}

// RechargeNotify 充值回调入账：幂等 + 事务写余额与流水。
func (s *Service) RechargeNotify(ctx context.Context, cmd dto.RechargeNotifyCommand) (*dto.RechargeNotifyDTO, error) {
	rc, err := s.repo.GetRecharge(ctx, cmd.RechargeNo)
	if err != nil {
		return nil, s.fail(err)
	}
	if cmd.AmountFen > 0 && cmd.AmountFen != rc.AmountFen {
		return nil, s.fail(entity.ErrAmountMismatch)
	}
	if rc.Status == entity.PaySuccess {
		b, err := s.repo.EnsureBalance(ctx, rc.UserID)
		if err != nil {
			return nil, s.fail(err)
		}
		return &dto.RechargeNotifyDTO{
			RechargeNo: rc.RechargeNo, Status: entity.PaySuccess, BalanceFen: b.BalanceFen, Duplicated: true,
		}, nil
	}
	idemKey := strings.TrimSpace(cmd.IdempotencyKey)
	if idemKey == "" {
		idemKey = rc.RechargeNo
	}
	if !s.repo.MarkIdempotent(ctx, "recharge", idemKey, 24*time.Hour) {
		b, err := s.repo.EnsureBalance(ctx, rc.UserID)
		if err != nil {
			return nil, s.fail(err)
		}
		return &dto.RechargeNotifyDTO{
			RechargeNo: rc.RechargeNo, Status: rc.Status, BalanceFen: b.BalanceFen, Duplicated: true,
		}, nil
	}

	success := cmd.Success == nil || *cmd.Success
	if !success {
		rc.Status = entity.PayFailed
		if err := s.repo.SaveRecharge(ctx, rc); err != nil {
			return nil, s.fail(err)
		}
		s.repo.Audit(ctx, rc.UserID, "recharge_notify", rc.RechargeNo, "failed", "channel reported failure")
		return &dto.RechargeNotifyDTO{RechargeNo: rc.RechargeNo, Status: entity.PayFailed, Duplicated: false}, nil
	}

	flowNo := newNo("FL")
	before, after, err := s.repo.ApplyBalanceChange(ctx, rc.UserID, "recharge", rc.RechargeNo, rc.AmountFen, flowNo)
	if err != nil {
		return nil, s.fail(err)
	}
	rc.Status = entity.PaySuccess
	if err := s.repo.SaveRecharge(ctx, rc); err != nil {
		return nil, s.fail(err)
	}
	_ = before
	s.repo.Audit(ctx, rc.UserID, "recharge_notify", rc.RechargeNo, "success", fmt.Sprintf("before=%d after=%d", before, after))

	return &dto.RechargeNotifyDTO{
		RechargeNo: rc.RechargeNo, Status: entity.PaySuccess, BalanceFen: after, FlowNo: flowNo, Duplicated: false,
	}, nil
}

// Recharges 充值记录。
func (s *Service) Recharges(ctx context.Context, userID string, page, size int) (*dto.RechargeListDTO, error) {
	page, size, offset := pageOf(page, size, s.cfg.PageSizeMax)
	items, total, err := s.repo.ListRecharges(ctx, userID, offset, size)
	if err != nil {
		return nil, s.fail(err)
	}
	out := &dto.RechargeListDTO{Items: make([]dto.RechargeDTO, 0, len(items)), Total: total, Page: page, PageSize: size}
	for _, rc := range items {
		out.Items = append(out.Items, dto.RechargeDTO{
			RechargeNo: rc.RechargeNo, AmountFen: rc.AmountFen, Channel: rc.Channel,
			Status: rc.Status, IdempotencyKey: rc.IdempotencyKey, CreatedAt: ts(rc.CreatedAt),
		})
	}
	return out, nil
}

// ---------- 个人中心与卡券（工单 06） ----------

// Profile 个人中心。
func (s *Service) Profile(ctx context.Context, userID string) (*dto.ProfileDTO, error) {
	b, err := s.repo.EnsureBalance(ctx, userID)
	if err != nil {
		return nil, s.fail(err)
	}
	_ = b
	coupons, err := s.repo.ListCoupons(ctx, userID)
	if err != nil {
		return nil, s.fail(err)
	}
	now := time.Now()
	usable, expired := 0, 0
	for _, c := range coupons {
		switch effectiveCouponStatus(c, now) {
		case entity.CouponUnused:
			usable++
		case entity.CouponExpired:
			expired++
		}
	}
	stats, err := s.repo.OrderStats(ctx, userID)
	if err != nil {
		return nil, s.fail(err)
	}
	return &dto.ProfileDTO{
		UserID: userID, Nickname: nickname(userID), AvatarURL: "",
		Services: []dto.ServiceDTO{
			{Name: "我的寄存", Path: "/pages/orders/index", Permission: "order:read"},
			{Name: "我的卡券", Path: "/pages/coupons/index", Permission: "coupon:read"},
			{Name: "余额与充值", Path: "/pages/wallet/index", Permission: "wallet:read"},
			{Name: "消息通知", Path: "/pages/notices/index", Permission: "notice:read"},
		},
		Coupon: dto.CouponStatDTO{Usable: usable, Expired: expired},
		OrderStats: dto.OrderStatsDTO{
			Active: int(stats[entity.OrderActive]), Overdue: int(stats[entity.OrderOverdue]),
			PendingPay: int(stats[entity.OrderPendingPay]), Completed: int(stats[entity.OrderCompleted]),
		},
	}, nil
}

func nickname(userID string) string {
	if userID == "u_1001" {
		return "宿迁用户"
	}
	return "用户" + userID
}

func effectiveCouponStatus(c model.Coupon, now time.Time) string {
	if c.Status == entity.CouponUsed {
		return entity.CouponUsed
	}
	if now.After(c.ValidTo) || c.Status == entity.CouponExpired {
		return entity.CouponExpired
	}
	return entity.CouponUnused
}

// Coupons 卡券列表。
func (s *Service) Coupons(ctx context.Context, userID, status string) (*dto.CouponListDTO, error) {
	coupons, err := s.repo.ListCoupons(ctx, userID)
	if err != nil {
		return nil, s.fail(err)
	}
	now := time.Now()
	out := &dto.CouponListDTO{Items: make([]dto.CouponDTO, 0, len(coupons))}
	for _, c := range coupons {
		st := effectiveCouponStatus(c, now)
		if status != "" && status != "all" {
			want := status
			if want == "usable" {
				want = entity.CouponUnused
			}
			if st != want {
				continue
			}
		}
		out.Items = append(out.Items, dto.CouponDTO{
			ID: c.ID, Title: c.Title, Scope: c.Scope, FaceFen: c.FaceFen, MinFen: c.MinFen,
			ValidTo: ts(c.ValidTo), Status: st,
		})
	}
	out.Total = len(out.Items)
	return out, nil
}

// ---------- 消息通知（工单 01） ----------

func (s *Service) notify(ctx context.Context, userID, scene, title, content, orderNo string) {
	n := &model.Notice{
		ID: newNo("NT"), UserID: userID, Scene: scene, Title: title, Content: content,
		Channel: "inbox", Status: entity.NoticeSent, OrderNo: orderNo, CreatedAt: time.Now(),
	}
	if err := s.repo.CreateNotice(ctx, n); err != nil {
		s.log.Warn("create notice failed", zap.String("scene", scene), zap.Error(err))
	}
}

// Notices 站内信列表。
func (s *Service) Notices(ctx context.Context, userID string, unreadOnly bool) (*dto.NoticeListDTO, error) {
	items, err := s.repo.ListNotices(ctx, userID, unreadOnly, 50)
	if err != nil {
		return nil, s.fail(err)
	}
	unread, err := s.repo.CountUnreadNotices(ctx, userID)
	if err != nil {
		return nil, s.fail(err)
	}
	out := &dto.NoticeListDTO{Items: make([]dto.NoticeDTO, 0, len(items)), Total: len(items), Unread: int(unread)}
	for _, n := range items {
		out.Items = append(out.Items, dto.NoticeDTO{
			ID: n.ID, Scene: n.Scene, Title: n.Title, Content: n.Content, Channel: n.Channel,
			Status: n.Status, OrderNo: n.OrderNo, ReadAt: ts(derefTime(n.ReadAt)), CreatedAt: ts(n.CreatedAt),
		})
	}
	return out, nil
}

// ReadNotice 标记站内信已读。
func (s *Service) ReadNotice(ctx context.Context, userID, id string) (*dto.ReadNoticeDTO, error) {
	now := time.Now()
	ok, err := s.repo.ReadNotice(ctx, id, userID, now)
	if err != nil {
		return nil, s.fail(err)
	}
	if !ok {
		if _, err := s.repo.GetNotice(ctx, id, userID); err != nil {
			return nil, s.fail(err)
		}
		// 已读状态重复提交视为成功（幂等）。
		g, err := s.repo.GetNotice(ctx, id, userID)
		if err != nil {
			return nil, s.fail(err)
		}
		return &dto.ReadNoticeDTO{ID: g.ID, Status: g.Status, ReadAt: ts(derefTime(g.ReadAt))}, nil
	}
	return &dto.ReadNoticeDTO{ID: id, Status: entity.NoticeRead, ReadAt: ts(now)}, nil
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
