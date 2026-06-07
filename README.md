# <img src="web/public/ongrid-logo.svg" alt="" width="40" align="absmiddle" style="vertical-align: middle;" /> Ongrid

> **An ops AI that understands, finds the root cause, and fixes things.** *Monitoring, remote execution, knowledge base, specialist agents, Bash, files, and more skills — issue commands directly from Slack, Telegram, or Lark.*

[![Go Report Card](https://goreportcard.com/badge/github.com/ongridio/ongrid)](https://goreportcard.com/report/github.com/ongridio/ongrid)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Tech](https://img.shields.io/badge/Tech-Go%20%7C%20TypeScript%20%7C%20React-blue)](#)

English | [简体中文](./README_ZH.md) | [日本語](./README_JA.md) | [한국어](./README_KO.md) | [Español](./README_ES.md) | [Français](./README_FR.md) | [Deutsch](./README_DE.md) | [Português](./README_PT.md) | [Русский](./README_RU.md)

[Features](#features) • [Install](#install) • [Integrations](#integrations) • [License](#license)

---

<p align="center">
  <img src="docs/assets/demo.gif" alt="Ongrid demo" width="100%" />
</p>
<p align="center"><sub><a href="https://github.com/ongridio/ongrid/releases/download/v0.7.169/Area2_hq.mp4">▶ Watch full demo in HD (MP4, 18 MB)</a></sub></p>

## Features

- 🤖 **Coordinator + Specialist agents** — coordinator dispatches to SRE / network / DB sub-agents
- 🚨 **Auto-investigate on alert** — investigator spawns an RCA worker, writes the cause back to chat
- 🔍 **Root-cause RCA** — walks topology, correlates m/l/t, pins the "why" to a source-code line
- 🔒 **Zero inbound ports** — edge dials out; no port 22 / 80 / 443 on hosts
- 💻 **Browser SSH** — reverse-tunnel shell into any host; no keys, no jumpbox, all audited
- 🐳 **Self-host in one command** — `docker compose up` brings up the full stack
- 📊 **Built-in observability** — Prometheus + Loki + Tempo + Grafana wired; the agent writes the queries
- 🧠 **Bring your own model** — Anthropic / OpenAI / GLM / DeepSeek / Gemini / Kimi, hot routing
- 💬 **Two-way IM channels** — Slack / Telegram / Larksuite / DingTalk / WeCom, per-channel locale
- 🛠️ **Read-only host tools** — bash sandbox + 26+ inspection tools; every call audited

## Install

Download the latest release, extract it, and run the installer (Ubuntu 22.04+, Debian 12+, RHEL/Rocky 9):

```bash
# 1. Download latest release (Ubuntu 22.04+, Debian 12+, RHEL/Rocky 9)
wget https://github.com/ongridio/ongrid/releases/download/v0.8.2/ongrid-v0.8.2-linux-amd64.tar.xz

# 2. Extract
tar -xf ongrid-v0.8.2-linux-amd64.tar.xz && cd ongrid-v0.8.2-linux-amd64

# 3. Install
sudo ./install.sh
```

**🇨🇳 Mainland China** — if GitHub is slow, download step 1 from the CDN mirror instead (everything else is the same):

```bash
wget https://ongrid.cloud/dl/ongrid-v0.8.2-linux-amd64.tar.xz
```

### Or run from source

Local dev: set the admin account + one model API key, then bring up the full stack.

```bash
cp deploy/.env.example deploy/.env
make compose-up    # make compose-down to stop
```

## Integrations

Drop-in for the observability, channel, and model stacks your team already uses.

| | |
|---|---|
| **Observability** | <img src="https://api.iconify.design/logos:prometheus.svg" alt="Prometheus" title="Prometheus" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="https://api.iconify.design/logos:grafana.svg" alt="Grafana" title="Grafana" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="docs/assets/integrations/loki.svg" alt="Loki" title="Loki" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="docs/assets/integrations/tempo.svg" alt="Tempo" title="Tempo" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="docs/assets/integrations/opentelemetry.svg" alt="OpenTelemetry" title="OpenTelemetry" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="https://api.iconify.design/logos:qdrant-icon.svg" alt="Qdrant" title="Qdrant" width="28" height="28" /> |
| **Channels** | <img src="https://api.iconify.design/logos:slack-icon.svg" alt="Slack" title="Slack" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="https://api.iconify.design/logos:telegram.svg" alt="Telegram" title="Telegram" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="docs/assets/integrations/larksuite.svg" alt="Larksuite" title="Larksuite" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="docs/assets/integrations/dingtalk.svg" alt="DingTalk" title="DingTalk" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="https://cdn.simpleicons.org/wechat" alt="WeCom" title="WeCom" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="https://api.iconify.design/logos:webhooks.svg" alt="Webhook" title="Webhook" width="28" height="28" /> |
| **Models** | <img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/claude-color.svg" alt="Anthropic" title="Anthropic" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="docs/assets/integrations/openai.svg" alt="OpenAI" title="OpenAI" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/gemini-color.svg" alt="Gemini" title="Gemini" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/deepseek-color.svg" alt="DeepSeek" title="DeepSeek" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="docs/assets/integrations/zhipu.svg" alt="Zhipu" title="Zhipu" width="28" height="28" />&nbsp;&nbsp;&nbsp;<img src="https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/kimi-color.svg" alt="Kimi" title="Kimi" width="28" height="28" /> |

## License

Apache 2.0 — see [LICENSE](LICENSE).

## Edge Agent

Edge agents connect managed hosts to the ongrid control plane. The agent dials **out** — no inbound ports required on the host.

### Build the edge binary (source installs only)

If you are running from source rather than a release tarball, the edge binary must be compiled and staged before hosts can install it:

```bash
# 1. Build the edge binary
make build-ongrid-edge

# 2. Stage it so nginx can serve it to installing hosts
cp bin/ongrid-edge bin/ongrid-edge-linux-amd64
```

> Release tarballs already include pre-built binaries under `bin/` — skip this step if you installed from a tarball.

### Register a host

1. Open the ongrid web UI and create a new Edge under **Infrastructure → Edges**.
2. Copy the generated install command — it contains a one-time `access-key` and `secret-key`.
3. Run the command on the target host:

```bash
curl -k -sSL https://<server>/install.sh | bash -s -- \
  --access-key=<access-key> \
  --secret-key=<secret-key> \
  --server-edge-addr=<server>:40012 \
  --server-http-addr=<server>
```

**Notes:**
- The `access-key` is generated by the control plane — arbitrary strings will be rejected with `unauthorized`.
- If the control plane and the edge agent run on the **same host**, use `127.0.0.1` (not the public IP) for `--server-edge-addr`, since hairpin NAT is typically blocked: `--server-edge-addr=127.0.0.1:40012`
- Port 40012 uses plain TCP (geminio protocol). TLS termination is on port 443 (nginx) only.
- The self-signed certificate warning on `curl` is expected — the `-k` flag suppresses it.
