# ongrid

> **给每台主机装上一个轻量 agent，然后用自然语言排障 —— 告警、日志、指标、链路、拓扑、源码，交给云端的 AIOps Agent 一起分析**

[![Go Report Card](https://goreportcard.com/badge/github.com/ongridio/ongrid)](https://goreportcard.com/report/github.com/ongridio/ongrid)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![技术栈](https://img.shields.io/badge/Tech-Go%20%7C%20TypeScript%20%7C%20React-blue)](#%EF%B8%8F-技术栈)
[![版本](https://img.shields.io/badge/Version-v0.7.138-green)](#)

简体中文 | [English](./README_en.md)

[快速开始](#-快速开始) • [项目简介](#-项目简介) • [架构](#%EF%B8%8F-架构) • [贡献](#-贡献)

---

## 📖 项目简介

ongrid 是一个开源的 AIOps 平台。在每台主机上装一个轻量的 `ongrid-edge` agent，它通过一条多路复用的出站隧道把指标、日志、链路上报到云端 —— 不需要在主机上开任何入站端口。云端是一个 LLM 驱动的运维 Agent：你用自然语言提问，它自己去查 PromQL / LogQL / TraceQL、走业务拓扑、检索知识库、读源码、调用主机上的只读巡检工具，然后给出有依据的回答。

它主要解决这些问题：

- **排障门槛高**：把"看哪个指标、查哪段日志、跑哪条 PromQL"交给 Agent，运维用自然语言描述现象即可（"这台机器为什么 load 飙了"、"谁在丢包"、"哪个进程在吃内存"）。
- **告警与根因脱节**：告警触发后，Agent 能顺着拓扑做爆炸半径分析、关联日志/链路、并定位到**源码位置**，把"告了什么"接到"为什么"。
- **数据散落**：指标（Prometheus）、日志（Loki）、链路（Tempo）、知识库（向量检索）、源码仓库统一接入，一个会话里联合分析。
- **不暴露内网**：edge 主动外拨，主机侧零入站端口；遥测数据面与控制面分离。
- **可私有化**：自管理、自托管，一键 docker compose 起一套；模型可对接 OpenAI 兼容的任意 endpoint。

## ✨ 核心能力

- **自然语言排障**：云端协调者 Agent 把问题拆解、分派给专家子 Agent 和一组工具，给出带证据链的结论。
- **全栈遥测**：内置 Prometheus + Loki + Tempo + Grafana，edge 采集主机指标 / 日志 / 链路。
- **代码感知分析**：注册代码仓库后，Agent 可 `list_repo_sources` / `read_source` / `grep_source`，把日志里的代码位置、栈帧关联到真实源码。
- **业务拓扑**：在拓扑图谱里 BFS 爆炸半径、定位节点，理解告警的影响面。
- **知识库（RAG）**：内置运维 playbook 基线 + 组织自有文档上传 + 向量检索；离线 ONNX embedder，无需外发。
- **告警引擎**：主机阈值告警 + 基于日志 / 链路的规则（log_match / log_volume / trace_latency / trace_error_rate）。
- **只读主机工具**：edge 暴露受策略约束的只读巡检能力（进程 / 网络 / 磁盘 / 连接 …），由 Agent 按需调用。
- **自管理 RBAC**：admin / user / viewer 三角色，无公网注册、无中心鉴权，首个管理员由环境变量种子化。

## 🚀 快速开始

### 从源码构建

```bash
# 云端二进制 → bin/ongrid，边端 → bin/ongrid-edge
make build            # 或 make build-ongrid / build-ongrid-edge

# 前端 SPA
cd web && npm ci && npm run build
```

> 云端内嵌了一个本地 ONNX embedder（CGO），所以 `ongrid` 以 `CGO_ENABLED=1` 构建。离线 RAG（`ONGRID_EMBEDDING_PROVIDER=local`）需先跑一次 `make fetch-embedding-model` 拉 BGE 模型，否则打包/运行会回退到联网下载。`make help` 列出全部 target。

### 本地起一套（Docker Compose）

```bash
cp deploy/.env.example deploy/.env   # 按需编辑（管理员账号、模型 key 等）
make compose-up                      # docker compose -f deploy/docker-compose.yml up -d
make compose-down                    # 停止
```

compose 会拉起 `mysql` / `ongrid` / `frontier`（上游隧道 broker）/ `nginx` / `prometheus` / `grafana`。首个管理员从 `.env` 的 `ONGRID_ADMIN_EMAIL` / `ONGRID_ADMIN_PASSWORD` 种子化。细节见 [`deploy/README.md`](deploy/README.md)。

### 生产部署

用发布包 + `deploy/install/`（支持 docker-compose / systemd 两种形态、TLS、升级、卸载），见 [`deploy/install/README.md`](deploy/install/README.md)。

### 在主机上装 edge

edge 是单个出站 agent，安装时把 `ONGRID_CLOUD_ADDR` 指向云端的隧道地址即可；它主动外拨、不监听入站端口。

## 🏗️ 架构

```
  主机 ─┐
        │  ongrid-edge（每台一个）
        │  · 采集 metrics / logs / traces
        │  · 暴露只读主机巡检工具
        ▼
   ┌───────────── 出站多路复用隧道 ─────────────┐
   ▼                                            ▼
ongrid（云端）
  ├─ manager      边端管理 + 遥测接入 + AIOps Agent
  │    └─ 协调者 Agent ──分派──► 专家子 Agent + 工具
  │         PromQL · LogQL · TraceQL · 拓扑 · RAG 检索 · 源码阅读 · 主机只读工具
  ├─ 遥测栈        Prometheus（指标）· Loki（日志）· Tempo（链路）· Grafana
  ├─ 知识库        向量检索（内置 playbook + 组织文档）· 离线 ONNX embedder
  └─ web UI        对话 + 仪表盘
```

### 核心组件

- **edge（`ongrid-edge`）**：每台主机一个，纯 Go、单二进制；采集遥测并通过隧道暴露只读巡检工具。主动外拨，主机侧零入站端口。
- **cloud（`ongrid`）**：manager + LLM 协调者。协调者把问题分派给专家子 Agent 和工具（PromQL / LogQL / TraceQL / 拓扑 / 知识库检索 / 源码阅读），联合给出结论。
- **web**：React SPA，对话式排障 + 仪表盘。

## 📦 仓库结构

```
cmd/        # ongrid（云端）+ ongrid-edge 入口
api/        # proto 定义，按限界上下文分组
internal/
  iam/        # 鉴权 / JWT / 组织 / 用户
  manager/    # 边端 + 遥测 + aiops 子域
  edgeagent/  # 主机采集与只读工具处理
  pkg/        # 共享：隧道 / llm / prom / log / conf …
web/        # React SPA（对话 + 仪表盘）
agents/     # LLM Agent persona 定义
skills/     # Agent 技能包
deploy/     # Dockerfile / docker-compose / 安装包（install/）
dist/       # 发布打包脚本
```

## 🛠️ 技术栈

| 层 | 选型 |
|---|---|
| 云端 | Go · [eino](https://github.com/cloudwego/eino) Agent 框架 · GORM · [geminio](https://github.com/singchia/geminio) 隧道 · 本地 ONNX embedder（CGO） |
| 边端 | Go（纯 Go，单二进制，跨平台） |
| 前端 | TypeScript · React |
| 存储 / 遥测 | MySQL / SQLite · Prometheus · Loki · Tempo · Grafana · qdrant |
| 模型 | OpenAI 兼容的任意 endpoint |

## 🤝 贡献

欢迎 issue 和 PR。提交前请确保 `make build`、`make test`、`make arch-lint`（校验限界上下文边界）都通过。

## 📄 许可证

[Apache-2.0](LICENSE)。
