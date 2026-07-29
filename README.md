# zai2api — Unified AI Gateway

یک gateway واحد که دو provider هوش مصنوعی (GLM/Z.AI و MiMo/Xiaomi) را پشت یک endpoint واحد سرویس می‌دهد.

## Badges

![Build](https://github.com/hooshidev3/zai2api/actions/workflows/build.yml/badge.svg)
![Docker](https://img.shields.io/badge/docker-ghcr.io-blue)
[![Go Report Card](https://goreportcard.com/badge/github.com/hooshidev3/zai2api)](https://goreportcard.com/report/github.com/hooshidev3/zai2api)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

## ویژگی‌ها

- ✅ **یک endpoint، یک توکن** — کلاینت فقط یک آدرس و یک توکن تنظیم می‌کند
- ✅ **Multi-Account** — برای هر دو GLM و MiMo با round-robin / least-used / random
- ✅ **Per-Account Proxy** — HTTP/HTTPS/SOCKS5 با auth، رمزنگاری AES-256-GCM
- ✅ **داشبورد یکپارچه** — ۷ تب، SSE real-time، dark theme، RTL
- ✅ **Anthropic Translation** — مترجم دوطرفه برای MiMo
- ✅ **Per-Model Features** — مدیریت feature‌های GLM از داشبورد
- ✅ **Model Aliases** — alias‌های قابل تنظیم (مثلاً `fast` → `glm-4.5-air`)
- ✅ **Rate Limiting** — per-model RPM/TPM با token bucket
- ✅ **Production-ready** — WAL، retention، healthcheck، Docker

## Provider Capabilities

| قابلیت | GLM (Z.AI) | MiMo (Xiaomi) |
|--------|:---:|:---:|
| OpenAI Chat | ✅ | ✅ |
| Streaming | ✅ | ✅ |
| Anthropic API | ✅ | ✅ (با ترجمه) |
| Agent | ❌ | ✅ |
| History | ❌ | ✅ |
| File Upload | ❌ | ✅ |
| Vision Input | ✅ | 🟡 |
| Per-Model Features | ✅ | ❌ |

جزئیات کامل: [docs/PROVIDERS.md](docs/PROVIDERS.md)

## نصب سریع

### Automatic (توصیه‌شده)

**Linux/macOS:**
```bash
curl -fsSL https://raw.githubusercontent.com/hooshidev3/zai2api/main/install.sh | bash
# با سرویس و autostart:
curl -fsSL .../install.sh | bash -s -- --service --autostart
```

**Windows (PowerShell as Admin):**
```powershell
iwr -useb https://raw.githubusercontent.com/hooshidev3/zai2api/main/install.ps1 | iex
# با سرویس:
.\install.ps1 -Service -Autostart
```

### Docker

```bash
docker pull ghcr.io/hooshidev3/zai2api:latest
docker run -d -p 8080:8080 -v zai2api-data:/app/data ghcr.io/hooshidev3/zai2api
```

یا با docker-compose:
```bash
git clone --recursive https://github.com/hooshidev3/zai2api.git
cd zai2api
docker compose up -d
```

### From Source

```bash
git clone --recursive https://github.com/hooshidev3/zai2api.git
cd zai2api
make build
make run
```

> ⚠️ `--recursive` حیاتی است. بدون آن، submodule‌های `glm/` و `mimo/` خالی هستند.

## استفاده

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="sk-merged-xxx"  # GATEWAY_TOKEN
)

# مدل GLM
response = client.chat.completions.create(
    model="glm-5.1",
    messages=[{"role": "user", "content": "Hello"}]
)

# مدل MiMo — همان کلاینت، همان توکن
response = client.chat.completions.create(
    model="mimo-v2.5-pro",
    messages=[{"role": "user", "content": "Hello"}]
)

# با alias
response = client.chat.completions.create(
    model="fast",  # → glm-4.5-air
    messages=[{"role": "user", "content": "Hello"}]
)

# لیست همه مدل‌ها
models = client.models.list()
```

### Anthropic SDK

```python
import anthropic

client = anthropic.Anthropic(
    base_url="http://localhost:8080",
    api_key="sk-merged-xxx"
)

# GLM (native)
message = client.messages.create(
    model="glm-5.1",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Hello"}]
)

# MiMo (با ترجمه خودکار)
message = client.messages.create(
    model="mimo-v2.5-pro",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Hello"}]
)
```

## پیکربندی

متغیرهای env (در `.env`):

```bash
# Gateway
PORT=8080
GATEWAY_TOKEN=sk-merged-change-me
DASHBOARD_TOKEN=              # برای دسترسی remote به داشبورد

# GLM (Z.AI)
GLM_CAPTCHA_DB=./data/tokens.sqlite
ZAI_STRATEGY=round-robin      # round-robin | least-used | random

# MiMo (Xiaomi)
# SERVICE_TOKENS=             # comma-separated (یا از داشبورد اضافه کنید)
# USER_IDS=
# XIAOMI_CHATBOT_PHS=

# Encryption
PROXY_ENCRYPTION_KEY=         # generate: openssl rand -hex 32
EXPORT_PASSWORD=              # برای export اکانت‌ها

# Data
ACCOUNTS_DB=./data/accounts.sqlite
```

## داشبورد

داشبورد در `http://localhost:8080/` با ۷ تب:

| تب | محتوا |
|----|-------|
| Overview | KPI cards + نمودارهای زنده + recent requests |
| Providers | وضعیت هر provider + uptime + health |
| Accounts | مدیریت اکانت‌های GLM و MiMo + پروکسی |
| Models | لیست مدل‌ها + per-model feature config (GLM) |
| Agents | agent loop‌های MiMo + history |
| Stats | نمودارهای دقیق + export CSV |
| Settings | پیکربندی gateway + aliases + rate limits |

## مستندات

- [AGENTS.md](AGENTS.md) — راهنمای کامل برای AI agents و توسعه‌دهندگان
- [docs/API.md](docs/API.md) — مرجع کامل API
- [docs/PROVIDERS.md](docs/PROVIDERS.md) — matrix قابلیت‌ها و جزئیات provider‌ها
- [docs/INSTALL.md](docs/INSTALL.md) — راهنمای نصب
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — Docker و production

## توسعه

```bash
# Build
make build

# Test
make test

# Smoke test
make smoke

# Sync submodules
make sync

# Docker
make docker-build
make docker-up
```

## معماری

```
Client (یک توکن)
     │
     ▼
┌─────────────────────────────────────────────┐
│  Gateway (:8080)                            │
│  1. GatewayAuth                             │
│  2. RateLimiter (per-model)                 │
│  3. Alias resolution                        │
│  4. Route by model:                         │
│     ├─ glm-*  → GLM Provider                │
│     └─ mimo-* → MiMo Sub-Engine (authctx)   │
└─────────────────────────────────────────────┘
         │                    │
         ▼                    ▼
   chat.z.ai        aistudio.xiaomimimo.com
```

## License

MIT
