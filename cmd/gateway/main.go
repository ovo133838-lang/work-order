package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/client"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/router"
	"github.com/BwCloudWeGo/bw-cli/internal/locker/bootstrap"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/logger"
	"github.com/BwCloudWeGo/bw-cli/pkg/observability"
)

func main() {
	// 构建依赖前先加载运行时配置。
	if err := config.InitGlobal("configs/config.yaml"); err != nil {
		panic(err)
	}
	cfg := config.MustGlobal()
	applyEnvOverrides(cfg)
	gatewayServiceName := cfg.ServiceName("gateway")
	cfg.Log.Service = gatewayServiceName
	cfg.Log = logger.WithDailyFileName(cfg.Log, time.Now())

	log, err := logger.New(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer log.Sync()
	observability.Register(gatewayServiceName, log)
	config.PrintSourceNotice(cfg, os.Stdout)

	clients, err := client.New(cfg, log)
	if err != nil {
		log.Fatal("initialize grpc clients failed", zap.Error(err))
	}
	defer clients.Close()

	// 装配智能快寄柜业务模块（MySQL + Redis），失败直接退出避免半启动。
	lockerModule, err := bootstrap.New(cfg, log)
	if err != nil {
		log.Fatal("initialize locker module failed", zap.Error(err))
	}
	defer lockerModule.Close()

	engine := router.New(clients, lockerModule.Handler, log, cfg.Middleware)
	addr := fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      engine,
		ReadTimeout:  time.Duration(cfg.HTTP.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.HTTP.WriteTimeoutSeconds) * time.Second,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		printStartupFailure(addr, err)
		log.Fatal("gateway listen failed", zap.String("addr", addr), zap.Error(err))
	}
	printStartupSummary(cfg, addr)

	go func() {
		log.Info("gateway listening", zap.String("addr", addr))
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatal("gateway stopped unexpectedly", zap.Error(err))
		}
	}()

	waitForShutdown(server, log)
}

// applyEnvOverrides 允许通过环境变量覆盖本地数据库与 Redis 连接，避免把真实密码写进配置文件。
func applyEnvOverrides(cfg *config.Config) {
	if v := os.Getenv("BW_MYSQL_DSN"); v != "" {
		cfg.Database.DSN = v
		cfg.MySQL.DSN = v
	}
	if v := os.Getenv("BW_DB_DRIVER"); v != "" {
		cfg.Database.Driver = v
	}
	if v := os.Getenv("BW_REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("BW_REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
}

func printStartupFailure(addr string, err error) {
	fmt.Fprintf(os.Stderr, "\n[Gateway Start Failed]\n")
	fmt.Fprintf(os.Stderr, "  listen: %s\n", addr)
	fmt.Fprintf(os.Stderr, "  error: %v\n\n", err)
}

func printStartupSummary(cfg *config.Config, addr string) {
	host := cfg.HTTP.Host
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	baseURL := fmt.Sprintf("http://%s:%d", host, cfg.HTTP.Port)
	fmt.Fprintf(os.Stdout, "\n[Gateway Started]\n")
	fmt.Fprintf(os.Stdout, "  service: %s\n", cfg.ServiceName("gateway"))
	fmt.Fprintf(os.Stdout, "  env: %s\n", cfg.App.Env)
	fmt.Fprintf(os.Stdout, "  listen: %s\n", addr)
	fmt.Fprintf(os.Stdout, "  http: %s\n", baseURL)
	fmt.Fprintf(os.Stdout, "  health: %s/healthz\n", baseURL)
	fmt.Fprintf(os.Stdout, "  api: %s/api/v1\n\n", baseURL)
}

func waitForShutdown(server *http.Server, log *zap.Logger) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Info("gateway shutting down")
	if err := server.Shutdown(ctx); err != nil {
		log.Error("gateway shutdown failed", zap.Error(err))
	}
}
