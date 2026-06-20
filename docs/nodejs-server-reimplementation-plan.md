# Node.js 服务端重构技术方案

## 背景

当前 ai-ops 服务端由 Go 实现，入口为 `cmd/ongrid/main.go`，运行时由 `deploy/docker-compose.yml` 中的 `ongrid` 容器承载。服务端职责集中在一个 manager 进程内：

- HTTP API：认证、用户、组织、设备、边端、告警、AIOps Chat、知识库、监控、日志、Trace、报表、插件、Marketplace、WebShell 等。
- 数据存储：MySQL 为默认后端，启动时通过 GORM AutoMigrate 管理 schema；SQLite 仅作为本地调试选项。
- 外部依赖：Prometheus、Loki、Tempo、Grafana、Qdrant、Frontier tunnel broker、LLM Provider、通知渠道、IM Provider。
- 后台任务：告警评估、RCA 调查、Grafana 同步、审计保留、报表调度、边端状态修复、插件配置推送等。

目标是使用 Node.js 重新实现 manager 服务端逻辑，并在迁移期间保持现有 Web 前端、数据库、边端 agent、部署栈和 API 合约可用。

## 目标

- 保持现有 REST API 路径、请求体、响应体、鉴权语义和错误码兼容。
- 复用现有 MySQL 数据，不要求用户清库或重新注册边端。
- 保持 `ongrid-nginx` 到后端 `/api` 的代理形态不变。
- 保持边端与 manager 的 tunnel wire contract 兼容。
- 保持 Prometheus/Loki/Tempo/Grafana/Qdrant 等外部集成能力。
- 提供可灰度、可回滚的迁移路径，允许 Go 与 Node.js 在一段时间内并行运行。

## 非目标

- 不重写前端 `web/`。
- 不重写 edge agent `cmd/ongrid-edge`。
- 不改变 MySQL、Prometheus、Loki、Tempo、Grafana、Qdrant、Frontier 的基础部署形态。
- 不在第一阶段重构业务流程或重新设计权限模型。
- 不一次性替换所有 AIOps agent/tool 细节；优先做到 API 与核心流程兼容。

## 推荐技术栈

- Runtime：Node.js 22 LTS。
- Language：TypeScript，开启 `strict`。
- HTTP Framework：NestJS。
- API 校验：Zod 或 class-validator。建议内部 DTO 使用 Zod，OpenAPI 生成可通过 `@nestjs/swagger` 补齐。
- ORM：Prisma。
- DB：MySQL 8.0，连接池走 `mysql2`。
- Migration：Prisma Migrate 仅管理 Node 新增表；已有表先 introspection，不直接重写历史 schema。
- Auth：`jose` 处理 JWT，`@node-rs/argon2` 或 `argon2` 兼容当前 argon2id 密码哈希。
- RBAC：Casbin Node adapter，复用当前 `casbin_rule` 表和策略语义。
- Queue/Jobs：BullMQ + Redis。若暂不引入 Redis，第一阶段可用进程内 scheduler，但生产替换应引入持久队列。
- SSE/WebSocket：NestJS 原生 HTTP streaming + `ws`。
- Observability：OpenTelemetry JS SDK + Prometheus metrics endpoint。
- Testing：Vitest/Jest 单元测试，Supertest API 测试，保留现有 e2e 目录的 API 用例作为兼容验收。

## 代码组织

建议新增 `server-node/`，避免与现有 Go 代码混杂：

```text
server-node/
  src/
    main.ts
    app.module.ts
    config/
    common/
      auth/
      errors/
      logging/
      metrics/
      tenant/
    modules/
      iam/
      orgs/
      settings/
      audit/
      edges/
      devices/
      topology/
      alerts/
      aiops/
      knowledge/
      monitor/
      observability/
      integrations/
      reports/
      skills/
      marketplace/
      webshell/
      tunnel/
    workers/
    clients/
      prometheus/
      loki/
      tempo/
      grafana/
      qdrant/
      llm/
      frontier/
  prisma/
    schema.prisma
  test/
  Dockerfile
```

模块分层保持与当前 Go 代码一致：

- `controller`：HTTP 路由与 DTO。
- `service`：业务编排。
- `repository`：数据库访问。
- `clients`：外部系统适配。
- `workers`：后台任务。

## 现有模块映射

| 当前 Go 模块 | Node.js 模块 | 迁移优先级 |
|---|---|---|
| `internal/iam` | `modules/iam`, `modules/orgs` | P0 |
| `internal/manager/biz/setting` | `modules/settings` | P0 |
| `internal/manager/biz/audit` | `modules/audit` | P0 |
| `internal/manager/biz/device`, `edge` | `modules/devices`, `modules/edges` | P0 |
| `internal/manager/biz/alert` | `modules/alerts`, `workers/alert-evaluator` | P0 |
| `internal/manager/biz/aiops` | `modules/aiops` | P1 |
| `internal/manager/biz/knowledge` | `modules/knowledge` | P1 |
| `internal/manager/biz/monitor` | `modules/monitor` | P1 |
| logs/prom/traces server | `modules/observability` | P1 |
| `internal/manager/biz/report` | `modules/reports` | P2 |
| `internal/manager/biz/imbridge` | `modules/integrations/im` | P2 |
| `internal/manager/biz/marketplace`, `skill` | `modules/marketplace`, `modules/skills` | P2 |
| `internal/manager/biz/webshell` | `modules/webshell` | P2 |
| Frontier/tunnel handlers | `modules/tunnel` | P0/P1 |

## API 兼容范围

第一阶段必须保持以下 P0 API 可用：

- Auth/User/Org：`/v1/auth/login`, `/v1/auth/refresh`, `/v1/self`, `/v1/me`, `/v1/users`, `/v1/orgs`。
- Edge/Device：`/v1/edges`, `/v1/edges/{id}`, `/v1/edges/{id}/rotate-secret`, `/v1/edges/{id}/plugins/{name}`, `/v1/devices`。
- Alert：`/v1/alerts/incidents`, `/v1/alert-rules`, `/v1/notification-channels`, `/v1/alerts/runtime-info`。
- Settings/Integrations：`/v1/system-settings`, `/v1/integrations/*/test`。
- Observability read APIs：`/v1/edges/{id}/metrics`, `/v1/metrics/query_range`, `/v1/logs/query_range`, `/v1/traces/search`。
- Health/Metrics：`/healthz`、`/readyz`、`/metrics`。

P1/P2 API 可按模块逐步替换，但响应结构必须以当前前端调用为准。

## 数据兼容策略

使用 Prisma introspection 生成现有表模型，保留表名和列名：

- IAM：`users`, `orgs`, `org_memberships`, `casbin_rule`。
- Edge/Device：`edges`, `devices`, `edge_devices`, `edge_plugin_configs`。
- Alert：`alert_incidents`, `alert_events`, `alert_silences`, `alert_rules`, `notification_channels`, `notification_deliveries`, `investigation_reports`。
- AIOps：`chat_sessions`, `chat_messages`, `chat_tool_calls`, `chat_mutating_proposals`, `user_agents`。
- Knowledge：`knowledge_repos`, `ssh_identities`, knowledge docs 相关表。
- Settings/Audit：`system_settings`, `audit_logs`。
- Topology/Monitor/Report/Marketplace/WebShell：保持现有表名。

原则：

- 先只读 introspection，不让 Prisma 自动改历史表。
- 新增 Node.js 专用表必须加明确前缀或通过迁移脚本评审。
- 敏感字段继续按当前 `system_settings` 和通知渠道的加密/遮蔽语义处理。
- `pass_hash` 必须兼容当前 argon2id 编码，确保已有用户可直接登录。
- 时间字段保持 MySQL `datetime(3)` 精度，API 输出格式与当前 Go 服务一致。

## 鉴权与权限

Node.js 实现必须复刻当前安全语义：

- JWT 使用同一 `ONGRID_JWT_SECRET`、access TTL、refresh TTL。
- 登录字段仍为 `email/password`。
- `users.role` 保持 `admin | user | viewer` 兼容。
- `users.is_superuser=true` 保持超级管理员短路能力。
- Casbin 策略继续从 DB 加载，组织成员关系继续同步到 g rules。
- 受保护 API 保持 Bearer token 认证。
- 审计中间件记录登录、管理操作、失败原因、调用方 IP。

## 配置兼容

Node.js 服务读取现有环境变量，至少覆盖：

- HTTP：`ONGRID_HTTP_ADDR`, `ONGRID_METRICS_ADDR`, `ONGRID_PUBLIC_URL`。
- DB：`ONGRID_DB_DIALECT`, `ONGRID_DB_DSN`。
- JWT/Admin：`ONGRID_JWT_SECRET`, `ONGRID_JWT_ACCESS_TTL`, `ONGRID_JWT_REFRESH_TTL`, `ONGRID_ADMIN_EMAIL`, `ONGRID_ADMIN_PASSWORD`。
- LLM：`ONGRID_OPENAI_*`, `ONGRID_LLM_*` provider 配置。
- Observability：`ONGRID_PROM_*`, `ONGRID_GRAFANA_*`, `ONGRID_QDRANT_*`, `ONGRID_EMBEDDING_*`。
- Alert/Notify：`ONGRID_ALERT_*`, `ONGRID_NOTIFY_*`。
- Frontier：`ONGRID_FRONTIER_ADDR`, `ONGRID_FRONTIER_SERVICE_NAME`。

建议用 Zod 定义 config schema，启动时失败要给出明确日志。

## 后台任务设计

Go 版本中大量逻辑随 manager 进程启动。Node.js 版本建议拆为可观测 worker：

- `bootstrap-worker`：admin seed、默认组织 seed、settings seed、内置规则 seed、stale edge 修复。
- `alert-evaluator-worker`：周期性 PromQL 规则评估，产生/恢复 incident。
- `notification-worker`：投递和重试通知。
- `investigation-worker`：RCA 报告生成。
- `grafana-sync-worker`：Monitor panel 到 Grafana dashboard 同步。
- `report-worker`：定时报表。
- `edge-status-worker`：边端心跳超时、插件健康汇总。

单进程部署时可内嵌 worker；生产建议 API 与 worker 进程分离，并通过 BullMQ/Redis 做任务持久化和重试。

## 外部系统适配

- Prometheus：实现 query、query_range、remote_write 或保留当前 Prometheus ingest 路径。
- Loki：实现 label、label values、query_range。
- Tempo：实现 search、tag values、trace detail。
- Grafana：实现 service account bootstrap、dashboard fetch、panel sync。
- Qdrant：实现文档 embedding 写入和向量检索。
- LLM：实现 OpenAI-compatible client、多 provider 路由、streaming、tool calling。
- Frontier：优先评估 Node 是否能稳定使用现有 frontier/geminio 协议；若 SDK 不成熟，短期保留 Go tunnel adapter 作为 sidecar。

## 迁移路线

### 阶段 0：合约冻结

- 导出现有 OpenAPI/路由清单。
- 固化现有 e2e 用例作为兼容基线。
- 对关键表执行 schema snapshot。
- 确认 nginx upstream 可按路径路由到 Go 或 Node。

### 阶段 1：Node.js 骨架

- 新增 `server-node/`。
- 实现 config、logger、error envelope、JWT middleware、DB connection、health、metrics。
- Dockerfile 和 compose 可选服务 `ongrid-node`。
- 不接管生产流量。

### 阶段 2：IAM 与基础设置

- 实现登录、refresh、self/me、users、orgs、memberships。
- 兼容 `pass_hash` 和 Casbin。
- 实现 `system_settings`、audit logs。
- 通过 B1/B2/B3 e2e。

### 阶段 3：设备、边端、告警 P0

- 实现 edges/devices CRUD、插件配置、notification channels、alert rules、incidents。
- 实现 PromQL evaluator 和通知投递。
- 保持 edge agent 兼容。
- 通过 edge/register、alert、notify 相关 P0 e2e。

### 阶段 4：可观测查询与集成

- 实现 Prom/Loki/Tempo/Grafana API。
- 实现 monitor panels 和 Grafana sync。
- 接入 integration test endpoints。

### 阶段 5：AIOps、Knowledge、Skill

- 实现 chat session/message/stream。
- 接入 LLM provider router、tool calling、query translate。
- 实现 knowledge repo/doc/search 与 Qdrant。
- Marketplace/skills 可先只读，后续补齐安装与执行。

### 阶段 6：切流与下线 Go manager

- nginx 按路径或按环境变量切换 upstream。
- 先 dev/staging 全量切 Node。
- 生产保留 Go manager 热备至少一个发布周期。
- 完成全量 e2e、备份恢复演练、回滚演练后移除 Go manager。

## 灰度策略

推荐路径级灰度：

- `/api/v1/auth/*`、`/api/v1/self` 先切 Node。
- 用户/组织稳定后切 `/api/v1/users`、`/api/v1/orgs`。
- 再切读多写少接口，如 settings、audit、devices list。
- 最后切后台任务强相关接口，如 alert、edge plugin、aiops。

灰度期间需要：

- Go 与 Node 共享同一个 MySQL。
- 同一时间只允许一个服务运行某类后台 worker，避免重复告警和重复通知。
- 所有写接口切流前确认幂等性和审计一致性。

## 回滚策略

- 保留 Go manager 镜像和 compose 服务定义。
- Node 阶段不修改历史表 schema；回滚不需要 DB downgrade。
- 新增字段必须允许 Go 忽略。
- Worker 切换通过环境变量控制，例如 `ONGRID_WORKER_ALERT_ENABLED=false`。
- 切流失败时 nginx upstream 切回 Go，Node 停止 worker。

## 风险与应对

| 风险 | 影响 | 应对 |
|---|---|---|
| tunnel/frontier Node SDK 不成熟 | 边端注册、心跳、远程执行失败 | 保留 Go tunnel adapter sidecar，Node 通过 HTTP/gRPC 调用 adapter |
| Prisma 误改历史 schema | 数据损坏或 Go 回滚失败 | 历史表只 introspection，禁止自动 migrate 旧表 |
| 密码哈希/JWT 不兼容 | 老用户无法登录 | 单元测试覆盖现有 hash 样本和 JWT payload |
| Casbin 同步不一致 | 权限绕过或误拒绝 | 复刻 seed policies，增加 RBAC e2e |
| 后台任务重复运行 | 重复告警、重复通知、重复 RCA | worker ownership 配置与分布式锁 |
| SSE/tool calling 行为差异 | AIOps Chat 前端异常 | 保持 event 格式，先做兼容 adapter |
| 时间格式差异 | 前端排序和过滤异常 | 统一 datetime parser/serializer |

## 验收标准

- 现有前端无需改动即可登录和完成核心操作。
- `tests/e2e` 中 P0 用例全部通过。
- 老 MySQL volume 直接启动 Node 服务后，admin 可登录，已有用户、设备、告警、设置可读取。
- Prom/Loki/Tempo/Grafana/Qdrant 集成测试通过。
- 边端 agent 不升级也能注册、心跳、上报插件状态。
- 告警不会重复触发，通知不会重复投递。
- 回滚到 Go manager 后数据仍可读写。

## 建议里程碑

| 周期 | 交付 |
|---|---|
| 第 1 周 | 合约冻结、Node 骨架、Dockerfile、health/metrics |
| 第 2 周 | IAM、Org、Settings、Audit、B1-B3 e2e |
| 第 3-4 周 | Edge/Device/Alert/Notify P0 |
| 第 5 周 | Prom/Loki/Tempo/Grafana/Monitor |
| 第 6-7 周 | AIOps Chat、LLM router、Knowledge/Qdrant |
| 第 8 周 | 灰度、压测、回滚演练、Go manager 下线评审 |

## 立即行动项

1. 生成当前 REST API 路由快照并纳入文档。
2. 为 MySQL schema 生成 snapshot，作为 Prisma introspection 基线。
3. 新增 `server-node/` 最小 NestJS 工程。
4. 实现 auth hash/JWT 兼容测试。
5. 修改 compose 增加可选 `ongrid-node` 服务，但默认不启用。
6. 为后台 worker 增加 ownership 开关，先从 Go 侧支持关闭重复 worker。
