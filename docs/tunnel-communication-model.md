# 隧道通信模型

本文解释 ongrid/ai-ops 的 edge tunnel 如何工作，重点覆盖端口拓扑、身份模型、RPC 与 stream 数据面、典型业务流、重连语义和排障边界。

## 设计目标

edge agent 部署在用户受管主机上，云端 manager 需要能读取主机状态、下发受控操作、推送插件配置、打开 WebSSH 会话，但受管主机通常位于 NAT、防火墙或私网之后。因此通信模型遵循以下原则：

- edge 只主动出站连接云端，不要求目标主机开放入站端口。
- 云端所有对 edge 的调用都复用这条已建立的出站隧道。
- tunnel 只承载边端控制与主机侧采集数据，不承载 Web UI/API 的普通 HTTPS 流量。
- manager 不直接暴露 tunnel servicebound 端口，避免外部绕过鉴权和路由层。
- 业务 payload 当前使用 JSON，方法名和结构体集中在 `internal/pkg/tunnel/messages.go`。

## 参与组件

| 组件 | 位置 | 职责 |
|---|---|---|
| `ongrid-edge` | 受管主机 | 主动拨号云端 tunnel endpoint，注册主机、心跳、推送指标，并注册 cloud -> edge RPC handler |
| `frontier` | 云端 | 上游 `singchia/frontier` broker，终止 geminio 连接并在 edge 与 manager 之间做路由 |
| `ongrid` manager | 云端 | 通过 frontier servicebound 连接注册生命周期回调和 RPC handler，并向指定 edge 发起反向调用 |
| `nginx` | 云端 | 处理 Web UI、REST API、Prometheus/Grafana/Loki/Tempo HTTP 路由；不参与 geminio tunnel 路由 |

## 端口拓扑

默认 Docker/安装包拓扑如下：

```text
edge host
  ongrid-edge
    |
    | TCP outbound, geminio, Meta={access_key,secret_key}
    v
cloud host public port
  <host>:40012
    |
    | docker publish: ${ONGRID_TUNNEL_PORT:-40012}:40012
    v
frontier container
  edgebound 0.0.0.0:40012
  servicebound 0.0.0.0:40011
    ^
    | docker network only, not published to host
    |
ongrid manager
  ONGRID_FRONTIER_ADDR=frontier:40011
```

关键点：

- `40012` 是 edge 侧公网入口，edge 安装命令中的 `--server-edge-addr` 或 `ONGRID_EDGE_CLOUD_ADDR` 指向这里。
- `40011` 是 manager 侧 servicebound 入口，只在 Docker 网络或内网进程间可见，不对公网发布。
- Web UI 和 REST API 走 nginx 的 HTTPS 入口，默认 `443`，与 tunnel 端口无关。
- 本地开发时可能把 HTTPS 改到 `8443`，但 tunnel 默认仍是 `40012`。

相关配置：

- `deploy/install/frontier.yaml`
- `deploy/docker-compose.yml`
- `deploy/install/docker-compose.yml`
- `docs/install/edge.md`

## 建连与身份绑定

edge 建连分两层：transport 层认证和业务层注册。

### 1. edge 拨号

`ongrid-edge` 启动后创建 `tunnel.Client`，读取：

- `ONGRID_EDGE_CLOUD_ADDR`
- `ONGRID_EDGE_ACCESS_KEY`
- `ONGRID_EDGE_SECRET_KEY`

然后向云端 `edgebound` 地址发起 TCP 连接。`internal/pkg/tunnel/client.go` 会把 access key 和 secret key 编码为 geminio Meta：

```json
{"access_key":"...","secret_key":"..."}
```

如果配置了 TLS CA，edge 会用 TLS 包住 TCP；当前安装文档默认 tunnel 端口是 plain TCP，HTTPS 只由 nginx 处理。

### 2. frontier 请求 manager 解析 edge id

manager 启动时通过 `internal/manager/service/frontierbound` 连接 `frontier:40011`，并注册三个生命周期回调：

- `GetEdgeID`
- `EdgeOnline`
- `EdgeOffline`

当 edge 连到 `40012` 后，frontier 把 Meta 交给 manager 的 `GetEdgeID`。manager 使用 `AccessKeyAuthenticator` 校验 access key/secret key，成功后返回 canonical `edge_id`。认证失败时 frontier 拒绝连接。

### 3. transport id 到 canonical edge id 的映射

frontier/geminio 内部会给连接分配 transport id。manager 需要把这个 transport id 映射到数据库里的 canonical `edges.id`：

```text
transport_edge_id <-> canonical edge_id
```

这个映射由 `frontierbound.Client` 维护：

- `bindEdgeTransport(transportID, edgeID)`
- `canonicalizeEdgeID(transportID)`
- `resolveTransportID(edgeID)`

这样业务代码始终使用数据库里的 `edge_id`，而不是泄露 broker 内部 transport id。这个约束非常重要，因为 Prometheus/Loki/Tempo 的 label 和 UI 查询都依赖稳定的业务 ID。

### 4. register_edge 业务注册

连接认证成功后，edge agent 立即调用 `register_edge`，把主机信息传给 manager：

- hostname
- OS/arch/kernel
- CPU/memory
- host fingerprint
- hardware fingerprint
- agent version

manager 在 `HandleRegister` 中更新 edge 状态，并建立 `edge_devices(type=host)` 关系。这个关系是 push 和 pull 路径之间的核心转换表：

```text
Push path:
  edge_id -> LookupHostDevice(edge_id) -> device_id -> write labels

Pull path:
  device_id -> LookupEdgeForDevice(device_id, type=host) -> edge_id -> tunnel call
```

## 数据面类型

tunnel 有两类数据面：方法型 RPC 和双向字节流。

## 方法型 RPC

方法型 RPC 是主要通信方式。所有方法名集中在 `internal/pkg/tunnel/messages.go`，payload 当前为 JSON。

### edge -> manager

edge 主动调用 manager，用于注册、心跳和数据推送：

| 方法 | 目的 |
|---|---|
| `register_edge` | 首次或重连后注册主机信息，获取 canonical edge id |
| `heartbeat` | 定期刷新 `last_seen_at`，并 piggyback 插件健康状态 |
| `push_host_metrics` | 旧版固定字段主机指标推送 |
| `push_prom_samples` | 开集 Prometheus samples 推送，manager 写入 Prometheus remote write |
| `get_plugin_configs` | edge 拉取自己的插件配置快照 |
| `shell_output` | WebSSH stdout/stderr chunk 推回 manager |
| `shell_exit` | WebSSH 会话结束事件推回 manager |

### manager -> edge

manager 对指定 edge 发起反向调用，用于查询主机状态、执行受控能力、推送配置变更：

| 方法 | 目的 |
|---|---|
| `get_host_load` | 查询 CPU、内存、磁盘等当前负载 |
| `get_process_list` | 查询进程列表 |
| `execute_skill` | 统一 skill dispatcher，按 skill key 调用 edge 本地 executor |
| `plugin_configs_changed` | 通知 edge 配置有变化，edge 收到后再调用 `get_plugin_configs` 拉取 |
| `write_database_metrics_secret` | 将数据库 exporter 所需 secret 写到 edge 本地 |
| `agent_upgrade` | 旧版单 binary 升级 |
| `fetch_package` | 下载并校验完整 edge release bundle |
| `apply_package` | ACK 后退出，让 systemd 执行 staged package 切换 |

manager 调用指定 edge 的过程：

```text
HTTP/API/AIOps tool
  -> manager biz/service
  -> frontierbound.Client.Call(ctx, edgeID, method, jsonBody)
  -> resolveTransportID(edgeID)
  -> frontier servicebound
  -> frontier broker
  -> edge geminio connection
  -> edge registered handler
  -> JSON response
```

edge 调用 manager 的过程：

```text
edge loop/plugin/webshell
  -> tunnel.Client.Call(ctx, method, req, resp)
  -> geminio end.Call
  -> frontier edgebound
  -> frontier servicebound
  -> manager registered handler
  -> manager biz/service
  -> JSON response
```

## 双向字节流

少数场景需要原始双向字节流，而不是一问一答的 JSON RPC。当前主要用于 WebSSH。

manager 使用 `frontierbound.Client.OpenStream(ctx, edgeID)` 打开到某个 edge 的 stream。stream 的 `Meta` 携带一个小 JSON 描述，例如目标地址：

```json
{"target":"127.0.0.1:22"}
```

edge 侧通过 `AcceptStream()` 接收 stream，解析 Meta，然后连接本地 `sshd`。边端本身不实现 SSH 协议、不创建 PTY、不保存 session map，只做字节转发：

```text
browser websocket
  -> manager webshell handler
  -> SSH client runs in manager
  -> frontier stream
  -> edge AcceptStream
  -> net.Dial("127.0.0.1:22")
  -> io.Copy both directions
```

这种设计让 SSH 凭据、审计、WebSocket 会话和权限判断集中在 manager，edge 只承担受控端口转发。

## 典型业务流

### 主机上线

```text
ongrid-edge starts
  -> register cloud->edge handlers before Dial
  -> Dial <cloud>:40012 with Meta credentials
  -> frontier asks manager GetEdgeID
  -> manager authenticates access_key/secret_key
  -> EdgeOnline lifecycle callback binds transport id to canonical edge_id
  -> edge calls register_edge with HostInfo
  -> manager upserts host device and edge_devices(type=host)
  -> edge starts heartbeat and metrics loops
```

### 心跳

edge 默认每 30 秒发送一次 `heartbeat`：

```text
edge heartbeat
  -> ts + edge_id + plugin health
  -> manager HandleHeartbeat
  -> update last_seen_at/status
  -> record in-memory plugin health
```

如果 tunnel 断开，`EdgeOffline` 会把 edge 状态持久化为 offline。告警层不依赖这个 callback 立即发通知，而是通过 `edge_last_seen_seconds_ago` 这类指标规则在下一轮评估中触发或恢复。

### 指标推送

edge 的采集有两个路径：

- `push_host_metrics`：固定字段 host metric point，兼容旧逻辑。
- `push_prom_samples`：开放 Prometheus samples，供插件和 exporter 使用。

manager 在写入时必须把 tunnel-side `edge_id` 转成 host `device_id`：

```text
edge_id
  -> edge_devices(type=host)
  -> device_id
  -> labels: device_id=...
  -> Prometheus/Loki/Tempo query path
```

如果 `device_id` 无法解析，manager 会丢弃该批数据，而不是把 `edge_id` 当 `device_id` 写入。原因是 Prometheus label 一旦写错，会污染历史时间序列，直到 retention 过期。

### AI 工具调用主机

当 AIOps tool 需要读取主机负载、列进程、查大文件或执行受控 skill 时：

```text
LLM tool call
  -> manager tool resolver maps device_id/name to edge_id
  -> frontierbound.Client.Call(edgeID, method, body)
  -> edge handler executes local collector/skill
  -> response returned to manager
  -> tool result returned to model/UI
```

对 mutating 操作，例如 restart service 或 agent upgrade，权限、审计、sandbox/policy 需要在 manager 和 edge 两侧同时约束：manager 决定是否允许发起，edge 决定本地命令是否允许执行。

### 插件配置更新

插件配置采用“通知 + 拉取”模型：

```text
user saves plugin config
  -> manager persists config
  -> manager NotifyPluginConfigsChanged(edgeID)
  -> edge receives plugin_configs_changed
  -> edge calls get_plugin_configs
  -> supervisor reconciles local plugin runtime
```

这样 `plugin_configs_changed` 不承载完整配置，避免 push payload 和配置快照格式强耦合。edge 还有周期性安全网拉取，即使通知丢失，配置也会在下一轮同步。

### WebSSH

WebSSH 是 stream 模型和 RPC 模型的组合：

- manager 打开 stream 到 edge，edge 把 stream 转发到本地 sshd。
- 用户输入、窗口大小、关闭等控制动作由 manager 侧 WebSocket/SSH 逻辑驱动。
- edge 输出通过 `shell_output` / `shell_exit` RPC 推回 manager 的 WebSocket router。

这让浏览器只连接 manager，edge 不暴露 SSH，也不需要从公网访问目标主机。

## 重连模型

edge 使用 geminio `RetryEnd` 维持长连接。首次 `Dial` 失败时指数退避：

```text
1s -> 2s -> ... -> 60s cap
```

首次连接成功后，底层会处理常规断线重连。除此之外，`tunnel.Client.Call` 会识别 broker route invalidation 类错误。典型场景是 manager 重启后 frontier 路由表清空，但 edge TCP 连接仍然存活。此时：

1. 当前 RPC 返回错误给调用方。
2. edge 异步关闭旧 end 并重新 Dial。
3. 重连成功后触发 `OnReconnect` callbacks。
4. edge agent 重新调用 `register_edge`。
5. manager 重新绑定 transport id 到 canonical edge id。

这个机制避免每个业务调用点都各自处理“连接看似存在但 broker 不知道路由”的状态。

## 故障边界

| 故障 | 影响 | 预期行为 |
|---|---|---|
| edge 到 `40012` 网络不通 | edge 无法上线 | edge 日志持续 `dial failed; will retry` |
| access key/secret key 错误 | 认证失败 | frontier 拒绝连接，edge 继续退避重试 |
| manager 到 `frontier:40011` 不通 | manager 启动失败，除非 `ONGRID_FRONTIER_DISABLED=true` | edge tunnel 功能不可用 |
| manager 重启 | edge 可能短暂 RPC 失败 | route invalidation 触发 edge 重连和重新 `register_edge` |
| edge 未完成 register | push 数据可能丢弃 | manager 不写入 transport id，避免污染 labels |
| `edge_devices(type=host)` 缺失 | 指标/log/trace 无法可靠归属 device | manager 丢弃 push batch，等待重新 register 修复 junction |
| plugin config push 失败 | 配置不会立即生效 | edge 周期性 `get_plugin_configs` 兜底 |
| WebSSH stream 断开 | 当前 shell 会话结束 | manager 审计会话结束，用户可重新打开 |

## 安全边界

- edge 凭据是 tunnel 身份凭据，不是用户登录凭据。
- edge 凭据在 geminio Meta 中发送，manager 只返回 canonical `edge_id`，不分配匿名身份。
- servicebound `40011` 不应发布到公网。
- 普通 Web/API 鉴权仍走 HTTPS + JWT/session，不走 tunnel。
- tunnel 中的 mutating 能力必须有 manager 侧授权、审计和 edge 侧 sandbox 双重保护。
- secret 类 payload，例如 `write_database_metrics_secret`，不得在 manager 侧持久化明文，也不得写入日志。

## 与 Node.js 迁移的关系

如果 manager 从 Go 迁移到 Node.js，tunnel contract 必须优先保持兼容：

- 保持 `frontier` 的 edgebound/servicebound 拓扑。
- 保持 Meta JSON：`access_key` / `secret_key`。
- 保持方法名和 JSON payload 结构。
- 保持 transport id 到 canonical edge id 的绑定语义。
- 保持 `edge_id -> device_id` 的 push path 解析，禁止把 transport id 或 edge id 误写为 device label。
- 如果 Node 侧 frontier/geminio SDK 不成熟，短期应保留 Go tunnel adapter sidecar，由 Node manager 通过 HTTP/gRPC 调用 adapter。

## 运维检查

云端：

```bash
cd /opt/ongrid
docker compose --env-file .env ps
docker logs ongrid-frontier -n 100
docker logs ongrid -n 200 | grep -i frontierbound
grep ONGRID_TUNNEL_PORT .env
```

edge 端：

```bash
journalctl -u ongrid-edge -n 100
journalctl -u ongrid-edge -f
grep ONGRID_EDGE_CLOUD_ADDR /etc/ongrid-edge/ongrid-edge.env
```

常见判断：

- edge 日志出现 `tunnel: connected` 说明 TCP + geminio 连接成功。
- manager 日志出现 `frontierbound: edge online` 说明生命周期回调成功。
- edge 日志出现 `agent: re-registered after tunnel reconnect` 说明 route invalidation 后恢复成功。
- UI 中 edge online 但指标为空时，优先检查 `edge_devices(type=host)` 关系和 `push_prom_samples` 相关日志。

