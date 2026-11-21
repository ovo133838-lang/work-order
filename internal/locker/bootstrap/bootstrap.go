// Package bootstrap 负责把寄存业务模块装配到网关进程：打开数据库、迁移表结构、建立 Redis 客户端、注入种子数据。
package bootstrap

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/BwCloudWeGo/bw-cli/internal/locker/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/locker/handler"
	"github.com/BwCloudWeGo/bw-cli/internal/locker/model"
	"github.com/BwCloudWeGo/bw-cli/internal/locker/repo"
	"github.com/BwCloudWeGo/bw-cli/internal/locker/service"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/database"
	"github.com/BwCloudWeGo/bw-cli/pkg/redisx"
)

// Module 是寄存业务模块的运行期句柄。
type Module struct {
	Handler *handler.HTTPHandler
	Service *service.Service
	DB      *gorm.DB
	Redis   *redis.Client

	log *zap.Logger
}

// New 完成数据库迁移、Redis 连接、种子数据写入与依赖装配。
func New(cfg *config.Config, log *zap.Logger) (*Module, error) {
	if log == nil {
		log = zap.NewNop()
	}

	db, err := database.Open(cfg.Database, cfg.MySQL, cfg.PostgreSQL, log)
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(model.Models()...); err != nil {
		return nil, err
	}
	log.Info("locker schema migrated")

	rdb := redisx.NewClient(cfg.Redis)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := redisx.Ping(ctx, rdb); err != nil {
		// Redis 故障不阻断启动：柜门锁退化为数据库条件更新，缓存与幂等降级。
		log.Warn("locker redis unavailable, degraded to database-only mode", zap.String("addr", cfg.Redis.Addr), zap.Error(err))
	}

	r := repo.New(db, rdb, "lk")
	if err := r.Seed(context.Background()); err != nil {
		log.Warn("locker seed data skipped", zap.Error(err))
	}

	svcCfg := service.Config{
		Price: entity.Pricing{
			PriceS: cfg.Locker.PriceSFen, PriceM: cfg.Locker.PriceMFen, PriceL: cfg.Locker.PriceLFen,
			DepositS: cfg.Locker.DepositSFen, DepositM: cfg.Locker.DepositMFen, DepositL: cfg.Locker.DepositLFen,
			OverdueRate: cfg.Locker.OverdueRate,
		},
		CabinetLockTTL: time.Duration(cfg.Locker.CabinetLockTTLSeconds) * time.Second,
		CacheTTL:       time.Duration(cfg.Locker.CacheTTLSeconds) * time.Second,
		GrantTTL:       time.Duration(cfg.Locker.GrantTTLMinutes) * time.Minute,
		GrantMaxUses:   cfg.Locker.GrantMaxUses,
	}
	svc := service.New(r, log, svcCfg)

	return &Module{
		Handler: handler.New(svc),
		Service: svc,
		DB:      db,
		Redis:   rdb,
		log:     log,
	}, nil
}

// Close 释放 Redis / 数据库连接。
func (m *Module) Close() {
	if m == nil {
		return
	}
	if m.Redis != nil {
		_ = m.Redis.Close()
	}
	if m.DB != nil {
		if sqlDB, err := m.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}
