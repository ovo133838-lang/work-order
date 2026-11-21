// Package repo 实现智能快寄柜业务的数据访问：MySQL 为唯一事实来源，Redis 承担缓存、柜门并发锁与回调幂等。
package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/BwCloudWeGo/bw-cli/internal/locker/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/locker/model"
)

// Repo 聚合 Gorm 与 Redis 的访问入口。
type Repo struct {
	db     *gorm.DB
	rdb    *redis.Client
	prefix string
}

// New 创建仓储实例。rdb 允许为 nil（Redis 故障时业务降级为纯 MySQL + 数据库条件更新）。
func New(db *gorm.DB, rdb *redis.Client, prefix string) *Repo {
	if prefix == "" {
		prefix = "lk"
	}
	return &Repo{db: db, rdb: rdb, prefix: prefix}
}

func (r *Repo) key(parts ...string) string {
	out := r.prefix + ":"
	for i, p := range parts {
		if i > 0 {
			out += ":"
		}
		out += p
	}
	return out
}

// ---------- Redis 辅助 ----------

func (r *Repo) cacheGet(ctx context.Context, key string, out any) bool {
	if r.rdb == nil {
		return false
	}
	raw, err := r.rdb.Get(ctx, key).Bytes()
	if err != nil || len(raw) == 0 {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

func (r *Repo) cacheSet(ctx context.Context, key string, v any, ttl time.Duration) {
	if r.rdb == nil {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = r.rdb.Set(ctx, key, raw, ttl).Err()
}

func (r *Repo) cacheDel(ctx context.Context, keys ...string) {
	if r.rdb == nil {
		return
	}
	_ = r.rdb.Del(ctx, keys...).Err()
}

// CacheGet 读取并反序列化缓存；Redis 不可用时返回 false，业务自动回源 MySQL。
func (r *Repo) CacheGet(ctx context.Context, key string, out any) bool {
	return r.cacheGet(ctx, key, out)
}

// CacheSet 写入缓存，失败静默忽略。
func (r *Repo) CacheSet(ctx context.Context, key string, v any, ttl time.Duration) {
	r.cacheSet(ctx, key, v, ttl)
}

// CacheDel 删除缓存。
func (r *Repo) CacheDel(ctx context.Context, keys ...string) {
	r.cacheDel(ctx, keys...)
}

// TryLockCabinet 用 Redis SETNX 抢占柜门锁，ttl 到期自动释放。
func (r *Repo) TryLockCabinet(ctx context.Context, cabinetID string, ttl time.Duration) bool {
	if r.rdb == nil {
		return true // Redis 不可用时退化为数据库条件更新
	}
	ok, err := r.rdb.SetNX(ctx, r.key("cabinet", cabinetID), "1", ttl).Result()
	if err != nil {
		return true
	}
	return ok
}

// UnlockCabinet 释放柜门锁。
func (r *Repo) UnlockCabinet(ctx context.Context, cabinetID string) {
	r.cacheDel(ctx, r.key("cabinet", cabinetID))
}

// MarkIdempotent 注册幂等键，返回 true 表示首次出现。
func (r *Repo) MarkIdempotent(ctx context.Context, scope, key string, ttl time.Duration) bool {
	if r.rdb == nil {
		return true
	}
	ok, err := r.rdb.SetNX(ctx, r.key("idem", scope, key), "1", ttl).Result()
	if err != nil {
		return true
	}
	return ok
}

// ---------- 城市 / 网点 / 柜门 ----------

func (r *Repo) ListCities(ctx context.Context, keyword string) ([]model.City, error) {
	var out []model.City
	q := r.db.WithContext(ctx).Order("id asc")
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name LIKE ? OR pinyin LIKE ?", like, like)
	}
	err := q.Find(&out).Error
	return out, err
}

func (r *Repo) GetStation(ctx context.Context, id string) (*model.Station, error) {
	var st model.Station
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&st).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, entity.ErrStationNotFound
		}
		return nil, err
	}
	return &st, nil
}

func (r *Repo) ListStations(ctx context.Context, cityID, keyword string) ([]model.Station, error) {
	var out []model.Station
	q := r.db.WithContext(ctx).Where("status = ?", "open").Order("id asc")
	if cityID != "" {
		q = q.Where("city_id = ?", cityID)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("name LIKE ? OR address LIKE ?", like, like)
	}
	err := q.Find(&out).Error
	return out, err
}

func (r *Repo) ListCabinets(ctx context.Context, stationID string) ([]model.Cabinet, error) {
	var out []model.Cabinet
	err := r.db.WithContext(ctx).Where("station_id = ?", stationID).Order("code asc").Find(&out).Error
	return out, err
}

func (r *Repo) GetCabinet(ctx context.Context, id string) (*model.Cabinet, error) {
	var cab model.Cabinet
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&cab).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, entity.ErrCabinetNotFound
		}
		return nil, err
	}
	return &cab, nil
}

// LockCabinet 用条件更新把柜门从 expected 状态改为 locked 并绑定订单，返回是否成功。
func (r *Repo) LockCabinet(ctx context.Context, cabinetID, expected, orderNo string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Cabinet{}).
		Where("id = ? AND status = ? AND version = ?", cabinetID, expected, 0).
		Updates(map[string]any{
			"status":           entity.CabinetLocked,
			"current_order_no": orderNo,
			"version":          gorm.Expr("version + 1"),
			"updated_at":       time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		// 允许柜门已被使用过（version > 0）但当前为空闲的情况，再做一次不带 version 的条件更新。
		res = r.db.WithContext(ctx).Model(&model.Cabinet{}).
			Where("id = ? AND status = ?", cabinetID, expected).
			Updates(map[string]any{
				"status":           entity.CabinetLocked,
				"current_order_no": orderNo,
				"version":          gorm.Expr("version + 1"),
				"updated_at":       time.Now(),
			})
		if res.Error != nil {
			return false, res.Error
		}
	}
	return res.RowsAffected > 0, nil
}

// UpdateCabinetStatus 条件更新柜门状态。
func (r *Repo) UpdateCabinetStatus(ctx context.Context, cabinetID, expected, next, orderNo string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Cabinet{}).
		Where("id = ? AND status = ?", cabinetID, expected).
		Updates(map[string]any{
			"status":           next,
			"current_order_no": orderNo,
			"version":          gorm.Expr("version + 1"),
			"updated_at":       time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// FreeCabinet 释放柜门。
func (r *Repo) FreeCabinet(ctx context.Context, cabinetID string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Cabinet{}).
		Where("id = ?", cabinetID).
		Updates(map[string]any{
			"status":           entity.CabinetFree,
			"current_order_no": "",
			"version":          gorm.Expr("version + 1"),
			"updated_at":       time.Now(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ---------- 订单 ----------

func (r *Repo) CreateOrder(ctx context.Context, o *model.Order) error {
	return r.db.WithContext(ctx).Create(o).Error
}

func (r *Repo) GetOrder(ctx context.Context, orderNo string) (*model.Order, error) {
	var o model.Order
	if err := r.db.WithContext(ctx).Where("order_no = ?", orderNo).First(&o).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, entity.ErrOrderNotFound
		}
		return nil, err
	}
	return &o, nil
}

func (r *Repo) SaveOrder(ctx context.Context, o *model.Order) error {
	return r.db.WithContext(ctx).Save(o).Error
}

// UpdateOrderFields 局部更新订单字段。
func (r *Repo) UpdateOrderFields(ctx context.Context, orderNo string, fields map[string]any) error {
	fields["updated_at"] = time.Now()
	return r.db.WithContext(ctx).Model(&model.Order{}).Where("order_no = ?", orderNo).Updates(fields).Error
}

func (r *Repo) ListOrders(ctx context.Context, userID, status string, offset, limit int) ([]model.Order, int64, error) {
	var items []model.Order
	var total int64
	q := r.db.WithContext(ctx).Model(&model.Order{}).Where("user_id = ?", userID)
	if status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func (r *Repo) OrderStats(ctx context.Context, userID string) (map[string]int64, error) {
	type row struct {
		Status string
		Cnt    int64
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&model.Order{}).
		Select("status, count(*) as cnt").
		Where("user_id = ?", userID).
		Group("status").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[string]int64{
		entity.OrderPendingPay: 0,
		entity.OrderActive:     0,
		entity.OrderOverdue:    0,
		entity.OrderCompleted:  0,
	}
	for _, it := range rows {
		out[it.Status] = it.Cnt
	}
	return out, nil
}

// ExpireActiveOrders 把已过 end_at 的 active 订单批量转为 overdue，返回受影响条数。
func (r *Repo) ExpireActiveOrders(ctx context.Context, userID string, now time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Model(&model.Order{}).
		Where("user_id = ? AND status = ? AND end_at < ?", userID, entity.OrderActive, now).
		Updates(map[string]any{"status": entity.OrderOverdue, "updated_at": now})
	return res.RowsAffected, res.Error
}

// ---------- 订单轨迹 ----------

func (r *Repo) AddEvent(ctx context.Context, orderNo, node, note string) error {
	return r.db.WithContext(ctx).Create(&model.OrderEvent{
		OrderNo: orderNo, Node: node, Note: note, CreatedAt: time.Now(),
	}).Error
}

func (r *Repo) ListEvents(ctx context.Context, orderNo string) ([]model.OrderEvent, error) {
	var out []model.OrderEvent
	err := r.db.WithContext(ctx).Where("order_no = ?", orderNo).Order("id asc").Find(&out).Error
	return out, err
}

// ---------- 支付 ----------

func (r *Repo) CreatePayment(ctx context.Context, p *model.Payment) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *Repo) GetPayment(ctx context.Context, payNo string) (*model.Payment, error) {
	var p model.Payment
	if err := r.db.WithContext(ctx).Where("pay_no = ?", payNo).First(&p).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, entity.ErrPaymentNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *Repo) SavePayment(ctx context.Context, p *model.Payment) error {
	p.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(p).Error
}

// ---------- 余额 / 明细 / 充值 ----------

func (r *Repo) EnsureBalance(ctx context.Context, userID string) (*model.Balance, error) {
	var b model.Balance
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&b).Error
	if err == nil {
		return &b, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	b = model.Balance{UserID: userID, UpdatedAt: time.Now()}
	if err := r.db.WithContext(ctx).Create(&b).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

// ApplyBalanceChange 在单个事务内更新余额并写入流水，返回变动前后余额与流水号。
func (r *Repo) ApplyBalanceChange(ctx context.Context, userID, bizType, refNo string, amountFen int64, flowNo string) (int64, int64, error) {
	var before, after int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var b model.Balance
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(&b).Error
		if err == gorm.ErrRecordNotFound {
			b = model.Balance{UserID: userID, UpdatedAt: time.Now()}
			if err := tx.Create(&b).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		before = b.BalanceFen
		after = before + amountFen
		if after < 0 {
			return entity.ErrInvalidInput
		}
		if err := tx.Model(&model.Balance{}).Where("user_id = ?", userID).
			Updates(map[string]any{"balance_fen": after, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return tx.Create(&model.BalanceFlow{
			FlowNo: flowNo, UserID: userID, BizType: bizType, AmountFen: amountFen,
			BeforeFen: before, AfterFen: after, RefNo: refNo, CreatedAt: time.Now(),
		}).Error
	})
	return before, after, err
}

func (r *Repo) ListFlows(ctx context.Context, userID string, offset, limit int) ([]model.BalanceFlow, int64, error) {
	var items []model.BalanceFlow
	var total int64
	q := r.db.WithContext(ctx).Model(&model.BalanceFlow{}).Where("user_id = ?", userID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func (r *Repo) CreateRecharge(ctx context.Context, rc *model.Recharge) error {
	return r.db.WithContext(ctx).Create(rc).Error
}

func (r *Repo) GetRecharge(ctx context.Context, no string) (*model.Recharge, error) {
	var rc model.Recharge
	if err := r.db.WithContext(ctx).Where("recharge_no = ?", no).First(&rc).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, entity.ErrRechargeNotFound
		}
		return nil, err
	}
	return &rc, nil
}

func (r *Repo) SaveRecharge(ctx context.Context, rc *model.Recharge) error {
	rc.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(rc).Error
}

func (r *Repo) ListRecharges(ctx context.Context, userID string, offset, limit int) ([]model.Recharge, int64, error) {
	var items []model.Recharge
	var total int64
	q := r.db.WithContext(ctx).Model(&model.Recharge{}).Where("user_id = ?", userID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

// ---------- 卡券 ----------

func (r *Repo) GetCoupon(ctx context.Context, id string) (*model.Coupon, error) {
	var c model.Coupon
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, entity.ErrCouponInvalid
		}
		return nil, err
	}
	return &c, nil
}

func (r *Repo) ListCoupons(ctx context.Context, userID string) ([]model.Coupon, error) {
	var out []model.Coupon
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("valid_to asc").Find(&out).Error
	return out, err
}

// UseCoupon 条件更新券状态，避免同一张券被两笔订单同时使用。
func (r *Repo) UseCoupon(ctx context.Context, id string) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Coupon{}).
		Where("id = ? AND status = ?", id, entity.CouponUnused).
		Updates(map[string]any{"status": entity.CouponUsed})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// ---------- 站内信 ----------

func (r *Repo) CreateNotice(ctx context.Context, n *model.Notice) error {
	return r.db.WithContext(ctx).Create(n).Error
}

func (r *Repo) ListNotices(ctx context.Context, userID string, unreadOnly bool, limit int) ([]model.Notice, error) {
	var out []model.Notice
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if unreadOnly {
		q = q.Where("status = ?", entity.NoticeSent)
	}
	err := q.Order("created_at desc").Limit(limit).Find(&out).Error
	return out, err
}

func (r *Repo) CountUnreadNotices(ctx context.Context, userID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Notice{}).
		Where("user_id = ? AND status = ?", userID, entity.NoticeSent).Count(&n).Error
	return n, err
}

// ReadNotice 标记已读，返回是否命中。
func (r *Repo) ReadNotice(ctx context.Context, id, userID string, at time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.Notice{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{"status": entity.NoticeRead, "read_at": at})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *Repo) GetNotice(ctx context.Context, id, userID string) (*model.Notice, error) {
	var n model.Notice
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&n).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, entity.ErrNoticeNotFound
		}
		return nil, err
	}
	return &n, nil
}

// ---------- 临时授权 ----------

func (r *Repo) CreateGrant(ctx context.Context, g *model.AccessGrant) error {
	return r.db.WithContext(ctx).Create(g).Error
}

func (r *Repo) GetGrant(ctx context.Context, grantNo string) (*model.AccessGrant, error) {
	var g model.AccessGrant
	if err := r.db.WithContext(ctx).Where("grant_no = ?", grantNo).First(&g).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, entity.ErrGrantNotFound
		}
		return nil, err
	}
	return &g, nil
}

// ConsumeGrant 条件扣减授权次数。
func (r *Repo) ConsumeGrant(ctx context.Context, grantNo string, now time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&model.AccessGrant{}).
		Where("grant_no = ? AND status = ? AND remain > 0 AND expire_at > ?", grantNo, entity.GrantActive, now).
		Updates(map[string]any{
			"remain":  gorm.Expr("remain - 1"),
			"used_at": now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	// 次数用尽后置为 used
	_ = r.db.WithContext(ctx).Model(&model.AccessGrant{}).
		Where("grant_no = ? AND remain <= 0", grantNo).
		Updates(map[string]any{"status": entity.GrantUsed}).Error
	return true, nil
}

// ---------- 审计 ----------

func (r *Repo) Audit(ctx context.Context, actor, action, target, result, detail string) {
	_ = r.db.WithContext(ctx).Create(&model.AuditLog{
		Actor: actor, Action: action, Target: target, Result: result, Detail: detail, CreatedAt: time.Now(),
	}).Error
}

// ---------- 种子数据 ----------

// Seed 幂等写入演示数据（仅当城市表为空时执行）。
func (r *Repo) Seed(ctx context.Context) error {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.City{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	cities := []model.City{
		{ID: "city_512", Name: "宿迁", Province: "江苏", Pinyin: "sq"},
		{ID: "city_025", Name: "南京", Province: "江苏", Pinyin: "nj"},
		{ID: "city_516", Name: "徐州", Province: "江苏", Pinyin: "xz"},
	}
	stations := []model.Station{
		{ID: "st_001", Name: "宿迁学院寄存点", CityID: "city_512", CityName: "宿迁", Address: "宿迁职业技术学院北门", Lat: 33.9631, Lng: 118.2752, OpenAt: "08:00", CloseAt: "22:00", Status: "open"},
		{ID: "st_002", Name: "宿迁高铁站寄存点", CityID: "city_512", CityName: "宿迁", Address: "宿迁站东广场一层", Lat: 33.9580, Lng: 118.3100, OpenAt: "06:30", CloseAt: "23:00", Status: "open"},
		{ID: "st_003", Name: "南京新街口寄存点", CityID: "city_025", CityName: "南京", Address: "新街口地铁站 2 号口", Lat: 32.0415, Lng: 118.7844, OpenAt: "07:00", CloseAt: "22:30", Status: "open"},
	}
	cabinets := make([]model.Cabinet, 0, 22)
	sizes := []string{entity.SizeS, entity.SizeM, entity.SizeL}
	for i := 0; i < 10; i++ {
		size := sizes[i%3]
		cabinets = append(cabinets, model.Cabinet{
			ID: fmt.Sprintf("cab_001_%02d", i+1), StationID: "st_001",
			Code: fmt.Sprintf("A-%02d", i+1), Size: size, Status: entity.CabinetFree, UpdatedAt: time.Now(),
		})
	}
	for i := 0; i < 6; i++ {
		cabinets = append(cabinets, model.Cabinet{
			ID: fmt.Sprintf("cab_002_%02d", i+1), StationID: "st_002",
			Code: fmt.Sprintf("B-%02d", i+1), Size: sizes[i%3], Status: entity.CabinetFree, UpdatedAt: time.Now(),
		})
	}
	for i := 0; i < 6; i++ {
		cabinets = append(cabinets, model.Cabinet{
			ID: fmt.Sprintf("cab_003_%02d", i+1), StationID: "st_003",
			Code: fmt.Sprintf("C-%02d", i+1), Size: sizes[i%3], Status: entity.CabinetFree, UpdatedAt: time.Now(),
		})
	}
	now := time.Now()
	coupons := []model.Coupon{
		{ID: "cp_001", UserID: "u_1001", Title: "新人立减5元", Scope: "all", FaceFen: 500, MinFen: 1500, ValidFrom: now.Add(-24 * time.Hour), ValidTo: now.AddDate(1, 0, 0), Status: entity.CouponUnused},
		{ID: "cp_002", UserID: "u_1001", Title: "满20减8", Scope: "all", FaceFen: 800, MinFen: 2000, ValidFrom: now.Add(-24 * time.Hour), ValidTo: now.AddDate(1, 0, 0), Status: entity.CouponUnused},
		{ID: "cp_003", UserID: "u_1001", Title: "国庆寄存券", Scope: "M", FaceFen: 300, MinFen: 900, ValidFrom: now.AddDate(0, -3, 0), ValidTo: now.Add(-24 * time.Hour), Status: entity.CouponExpired},
	}
	balances := []model.Balance{{UserID: "u_1001", BalanceFen: 5000, FrozenFen: 0, UpdatedAt: now}}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&cities).Error; err != nil {
			return err
		}
		if err := tx.Create(&stations).Error; err != nil {
			return err
		}
		if err := tx.Create(&cabinets).Error; err != nil {
			return err
		}
		if err := tx.Create(&coupons).Error; err != nil {
			return err
		}
		return tx.Create(&balances).Error
	})
}
