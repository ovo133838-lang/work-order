# AGENTS.md · bw-cli 项目开发智能体规范

本文件是仓库根目录的智能体（Codex / Cursor / Trae / Claude Code 等）统一指令。任何 agent 在本仓库内生成、修改、审查代码前，必须先读本文件与 `docs/locker/` 下的设计文档，再动手。

## 1. 项目定位

bw-cli 是 Gin + gRPC + Gorm 的 Go 微服务脚手架，按 DDD 分层组织。当前业务落地的是「智能快寄柜系统」（寄存柜），业务域包名为 `locker`。

- 模块路径：`github.com/BwCloudWeGo/bw-cli`（如需改用自有仓库，需同步修改 `go.mod` 与全量 import）
- Go 版本：`go 1.25.0`（本地实测 1.27.1 可编译运行）
- 依赖：Gin、gRPC、Gorm、go-redis、zap、viper

## 2. 目录与分层约定

```
cmd/<service>/main.go        # 进程入口：加载配置 → 组装依赖 → 启动
internal/<domain>/
  dto/                       # 入参命令与出参 DTO，只放数据结构，不放业务判断
  entity/                    # 领域对象 + 领域错误 + 状态迁移规则（业务真相在这里）
  model/                     # Gorm 持久化模型，只描述表结构与索引
  repo/                     # 仓储实现：数据库、缓存、锁；不写业务分支
  service/                   # 用例编排：事务边界、幂等、状态校验、跨仓储协作
  handler/                   # 传输层适配：gRPC server 或 HTTP handler，只做协议转换
internal/gateway/
  client/                    # 下游 gRPC 客户端集合
  handler/                   # HTTP 入站适配
  router/                    # 路由注册，按 v1 分组
pkg/                         # 可跨域复用的基础组件（config、logger、mysqlx、redisx、httpx、errors…）
api/proto/... + api/gen/...  # 契约优先：先改 proto，再 make protogen
configs/config.yaml          # 运行时配置
```

硬性规则：

1. 依赖方向只能 `handler → service → repo → model`，禁止 handler 直接调 repo，禁止 repo 调 service。
2. `entity` 不依赖 Gin、Gorm、Redis 等任何框架类型。
3. 一个用例一个 service 方法，事务写在 service 层，不写在 repo 层。
4. 新增领域时按上面结构建包，并在 `internal/gateway/router` 注册路由、在 `cmd/<service>/main.go` 组装依赖。
5. 不引入新的第三方依赖，除非在 PR 说明里写清理由与体积影响。

## 3. 命名与错误约定

- 表名统一 `lk_` 前缀（locker 域），模型结构体单数命名，`TableName()` 显式声明。
- 金额一律用 `int64` 分（`Fen` 后缀），禁止 float 参与金额计算。
- 时间字段用 `time.Time`，对外输出 RFC3339（`2006-01-02T15:04:05Z07:00`）。
- 错误只从 `pkg/errors` 构造：`InvalidArgument / Unauthorized / NotFound / Conflict / Internal`，HTTP 状态码由 `apperrors.HTTPStatus` 统一映射。
- 领域错误定义在 `entity`，service 负责翻译成 `apperrors`，handler 只调 `httpx.Error`。
- 注释必须写明工单编号，例如：`// 工单：全栈开发-智能快寄柜系统-物品寄存任务工单`。这是十份工单共同的验收要求。

## 4. 数据与中间件

- 本地 MySQL：`127.0.0.1:3306`，库名 `bw_locker`，UTF8MB4。DSN 通过 `configs/config.yaml` 或环境变量 `BW_MYSQL_DSN` 注入，禁止把真实密码提交到仓库。
- 本地 Redis：`127.0.0.1:6379`，用于三件事：列表缓存（`lk:cache:*`）、柜门占用锁（`lk:cabinet:<id>`，SETNX + TTL）、回调幂等（`lk:idem:*`）。
- Redis 不可用时接口必须降级为直连数据库，不允许因缓存故障导致业务不可用。
- 表结构由 `AutoMigrate` 在启动时创建；改字段必须同时更新 `docs/locker/02-数据字典.md`。

## 5. 构建与运行

```bash
go build ./...                       # 全量编译
go run ./cmd/gateway                  # 启动网关（含 locker 域 HTTP 接口）
go test ./internal/locker/...         # 领域单测
```

本地联调步骤见 `docs/locker/05-设计说明书.md`。接口基线见 `docs/locker/04-接口文档.md`。

## 6. 测试要求

- 新增或修改接口，必须补充至少一组正向用例与一组异常用例。
- 每次交付前对**每个接口至少连续测试 3 遍**，记录状态码、耗时与响应体，写入 `docs/locker/test-evidence/`。
- 金额、状态迁移、幂等、并发占用这四类场景必须有针对性用例；仅"能返回 200"不算通过。
- 测试脚本：`scripts/locker-api-test.ps1`（三遍巡检 + 证据落盘）。

## 7. 禁止事项

- 禁止绕过 `service` 直接在 handler 里写 SQL 或 Redis 命令。
- 禁止在仓库内硬编码生产地址、密钥、AK/SK、支付私钥。
- 禁止删除或改写 `docs/` 下既有验收文档；修订需保留变更记录。
- 禁止把 `data/`、`logs/`、本地 `.env`、agent 私有文件提交进仓库。
- 禁止用 `git push --force` 覆盖 `main`；开发统一走 `dev` 分支，完成后再合并。
