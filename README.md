<h1 align="center"><img src="web/public/ongrid-logo.svg" alt="" width="44" style="vertical-align: middle;" /> ongrid</h1>

<p align="center">
  <b>English</b> ·
  <a href="./README_ZH.md">简体中文</a> ·
  <a href="./README_JA.md">日本語</a> ·
  <a href="./README_KO.md">한국어</a> ·
  <a href="./README_ES.md">Español</a> ·
  <a href="./README_FR.md">Français</a> ·
  <a href="./README_DE.md">Deutsch</a> ·
  <a href="./README_PT.md">Português</a> ·
  <a href="./README_RU.md">Русский</a>
</p>

<p align="center">
  <a href="https://goreportcard.com/report/github.com/ongridio/ongrid"><img src="https://goreportcard.com/badge/github.com/ongridio/ongrid" alt="Go Report Card"></a>
  <a href="https://opensource.org/licenses/Apache-2.0"><img src="https://img.shields.io/badge/License-Apache_2.0-blue.svg" alt="License"></a>
  <img src="https://img.shields.io/badge/Tech-Go%20%7C%20TypeScript%20%7C%20React-blue" alt="Tech">
</p>

> **Put a lightweight agent on every host, then troubleshoot in natural language — alerts, logs, metrics, traces, topology, and source code, analyzed together by a cloud AIOps agent.**

<p align="center">
  <a href="#overview">Overview</a> • <a href="#quickstart">Quickstart</a> • <a href="#architecture">Architecture</a> • <a href="#tech-stack">Tech stack</a> • <a href="#contributing">Contributing</a>
</p>

---

## Overview

ongrid is an open-source, self-hostable AIOps platform. A lightweight `ongrid-edge` agent on each host ships metrics, logs, and traces to the cloud over a single **outbound** tunnel — no inbound ports on the host. The cloud is an LLM-driven ops agent: ask in natural language and it runs the PromQL / LogQL / TraceQL, walks the service topology, searches the knowledge base, reads source code, and calls read-only host tools to return a grounded, evidence-backed answer.

What it solves:

- **High troubleshooting bar** — describe the symptom ("why is load spiking?", "who's dropping packets?"); the agent figures out which metric, which logs, and which query to look at.
- **Alerts disconnected from root cause** — on an alert it walks the topology for blast radius, correlates logs/traces, and pins down the **source-code location** behind the "why".
- **Scattered signals** — metrics (Prometheus), logs (Loki), traces (Tempo), a knowledge base (vector search), and source repos are unified and analyzed in one session.
- **No exposed intranet** — edge dials out; zero inbound ports on hosts; the telemetry data plane is separated from the control plane.
- **Self-hostable** — one `docker compose` brings up the full stack; point the model at any OpenAI-compatible endpoint.

## Quickstart

```bash
# 1. configure: set the admin account + a model API key
cp deploy/.env.example deploy/.env

# 2. bring up the full stack (mysql / ongrid / frontier / nginx / prometheus / grafana)
make compose-up      # make compose-down to stop
```

Open `https://<host>` and sign in with the admin account seeded from `.env`. For a production release package (TLS, systemd, upgrade/uninstall) see [`deploy/install/`](deploy/install/README.md).

**Install edge on a host** — create the edge in the console, copy the one-line install command for the target platform, and run it. The edge dials out and listens on no inbound port.

> Building from source: `make build` (the cloud is `CGO_ENABLED=1` for the local ONNX embedder) and `cd web && npm ci && npm run build`. Run `make help` for all targets.

## Architecture

```
  hosts ─┐
         │  ongrid-edge (one per host)
         │  · collects metrics / logs / traces
         │  · exposes read-only host inspection tools
         ▼
   ┌──────── outbound multiplexed tunnel ────────┐
   ▼                                              ▼
ongrid (cloud)
  ├─ manager     edge mgmt + telemetry ingest + AIOps agent
  │   └─ coordinator agent ─dispatch─► specialist sub-agents + tools
  │        PromQL · LogQL · TraceQL · topology · RAG · source reading · host tools
  ├─ telemetry   Prometheus · Loki · Tempo · Grafana
  ├─ knowledge   vector search (built-in playbooks + org docs) · offline ONNX embedder
  └─ web UI      chat + dashboards
```

- **edge (`ongrid-edge`)** — one per host, pure-Go single binary; collects telemetry and exposes read-only inspection tools over the tunnel. Dials out, zero inbound ports.
- **cloud (`ongrid`)** — manager + LLM coordinator that dispatches to specialist sub-agents and tools (PromQL / LogQL / TraceQL / topology / knowledge search / source reading) and synthesizes the answer.
- **web** — React SPA: conversational troubleshooting + dashboards.

## Tech stack

| Layer | Choice |
|---|---|
| Cloud | Go · [eino](https://github.com/cloudwego/eino) agent framework · GORM · [geminio](https://github.com/singchia/geminio) tunnel · local ONNX embedder |
| Edge | Go — pure-Go single binary, cross-platform (Linux / macOS / Windows, x86_64 & ARM64) |
| Frontend | TypeScript · React (English / 简体中文) |
| Telemetry / storage | Prometheus · Loki · Tempo · Grafana · qdrant · MySQL / SQLite |
| Model | any OpenAI-compatible endpoint — OpenAI · Anthropic · Gemini · DeepSeek · Zhipu · Kimi · Ollama / vLLM / OpenRouter · … |

## Contributing

Issues and PRs are welcome. Before submitting, make sure `make build`, `make test`, and `make arch-lint` (which enforces bounded-context boundaries) all pass.

## License

[Apache-2.0](LICENSE).
