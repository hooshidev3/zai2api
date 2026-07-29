# zai2api — Unified AI Gateway

> Single endpoint for **GLM (Z.AI)** and **MiMo (Xiaomi)** AI providers.
> One auth token, multi-account support, per-account proxy.

[![Build Status](https://img.shields.io/badge/build-passing-brightgreen)]()
[![Go Version](https://img.shields.io/badge/Go-1.25-blue)]()
[![License](https://img.shields.io/badge/license-MIT-green)]()

## What is this?

A unified HTTP gateway that combines two upstream AI proxy projects:

- **GLM-Free-API** (Z.AI) — OpenAI + Anthropic compatible, GLM-5/GLM-5.1 models
- **mimo-ai-proxy** (Xiaomi MiMo) — OpenAI compatible, mimo-v2.5 models

Instead of running two separate services with two endpoints and two auth tokens,
zai2api provides a single endpoint (`POST /v1/chat/completions`) that dispatches
to the right provider based on the `model` field in the request.

## Architecture

```
                    ┌─────────────────────────────────────┐
                    │     zai2api Gateway (:8080)         │
                    │                                     │
                    │  POST /v1/chat/completions          │
                    │    ├─ model="glm-*"  → GLM provider │
                    │    └─ model="mimo*"  → MiMo engine  │
                    │                                     │
                    │  POST /v1/messages (Anthropic)      │
                    │  GET  /v1/models (aggregated)       │
                    │  GET  /health                       │
                    │  /glm/*   → GLM-specific routes     │
                    │  /mimo/*  → MiMo-specific routes    │
                    └──────────┬──────────────┬───────────┘
                               │              │
                    ┌──────────▼────┐  ┌──────▼─────────┐
                    │ GLM Provider  │  │ MiMo Sub-engine│
                    │ (package glm) │  │ (gin.Engine)   │
                    └───────────────┘  └────────────────┘
```

## Key design decisions

### 1. Branch + Rebase (not patch files)

Each upstream provider is cloned into `glm/` and `mimo/` with a
`merged-patches` branch that contains our modifications as real git commits.
Syncing uses `git fetch + git rebase` instead of fragile unidiff patches.

```
glm/  (git clone, branch: merged-patches)
├── .git/
├── main.go          (package glm, func main() removed)
├── provider.go      (Provider struct for gateway integration)
├── exports.go       (public handler wrappers)
└── cmd/token-collector/  (separate module, heavy deps)

mimo/  (git clone, branch: merged-patches)
├── .git/
└── pkg/             (internal/ → pkg/ for cross-module import)
    ├── authctx/     (shared context package, no deps)
    ├── services/    (GetAuthFromContext, ResolveClient)
    └── routes/      (per-account client threading)
```

### 2. `authctx` package breaks import cycle

`authctx` is a leaf package (no dependencies) that lives inside MiMo.
Both gateway and MiMo import it to share context key identity without
creating an import cycle.

### 3. Per-account proxy (Phase 3)

Each account can have its own HTTP/SOCKS5 proxy. The gateway builds a
per-account `*http.Client` and injects it into the request context via
`authctx.WithClient()`. MiMo reads it via `services.GetAuthFromContext()`
and threads it through all downstream calls using `ResolveClient()`.

## Quick start

```bash
# 1. Clone this repo
git clone https://github.com/hooshidev3/zai2api.git
cd zai2api

# 2. Sync upstream repos (clone + apply patches as commits)
make sync

# 3. Build
make build

# 4. Configure (optional — defaults work for testing)
export GATEWAY_TOKEN="your-secret-token"
export GLM_CAPTCHA_DB="./data/tokens.sqlite"  # needed for GLM

# 5. Run
make run
```

The gateway starts on `http://localhost:8080`.

## Usage

### OpenAI-compatible

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer your-secret-token" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "glm-5",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": false
  }'
```

### Anthropic-compatible (GLM only)

```bash
curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer your-secret-token" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "glm-5",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### List models

```bash
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer your-secret-token"
```

### Health check

```bash
curl http://localhost:8080/health
```

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_ADDR` | `:8080` | Listen address |
| `GATEWAY_TOKEN` | `sk-merged-default` | Auth token for API endpoints |
| `DASHBOARD_TOKEN` | (empty) | Auth token for dashboard (empty = localhost only) |
| `GLM_CAPTCHA_DB` | `./data/tokens.sqlite` | Path to GLM captcha database |
| `VERBOSE` | `0` | Enable verbose logging |
| `AGENT_MODE` | `0` | Enable GLM agent mode (captcha background cache) |
| `DEFAULT_MODEL` | `glm-5` | Default model when not specified |
| `TRUSTED_PROXIES` | `127.0.0.1/32,::1/128` | Trusted proxy CIDRs |

### MiMo multi-account (env-based)

```bash
SERVICE_TOKENS="token1,token2,token3"
USER_IDS="id1,id2,id3"
XIAOMI_CHATBOT_PHS="ph1,ph2,ph3"
```

MiMo automatically rotates between these accounts (random selection).

## Model routing

The gateway dispatches based on the `model` field:

| Model prefix | Provider |
|--------------|----------|
| `glm-*`, `GLM-*`, `zai-*` | GLM (Z.AI) |
| `mimo*` | MiMo (Xiaomi) |
| (default) | GLM |

## Project structure

```
zai2api/
├── cmd/server/main.go              # Entry point
├── gateway/
│   ├── server/                     # Main gateway logic
│   ├── auth/                       # GatewayAuthMiddleware
│   └── proxy/                      # Per-account proxy (Phase 3)
├── glm/                            # GLM-Free-API (git clone, merged-patches branch)
├── mimo/                           # mimo-ai-proxy (git clone, merged-patches branch)
├── scripts/
│   ├── sync-glm.sh                 # Fetch + rebase GLM
│   ├── sync-mimo.sh                # Fetch + rebase MiMo
│   ├── glm-patches/
│   │   ├── apply-all.sh            # First-time patch application
│   │   ├── post-rebase.sh          # Mechanical fixes after rebase
│   │   ├── remove-func-main.py     # Removes func main() safely
│   │   └── templates/              # Template files for new code
│   └── mimo-patches/
│       ├── apply-all.sh
│       ├── post-rebase.sh
│       ├── add-getauthfromcontext.py
│       ├── thread-client.py
│       └── templates/
├── spike/                          # Prototype that proves authctx works
├── docs/                           # Architecture documents
├── Makefile
└── go.mod
```

## Syncing upstream updates

When upstream repos release updates:

```bash
# Fetch and rebase our patches on top of new upstream commits
make sync

# Or force a fresh reapply (if rebase gets stuck)
make sync-force
```

Our patches are preserved as git commits. View them:

```bash
make git-log-glm   # Show our GLM commits
make git-log-mimo  # Show our MiMo commits
make diff-glm      # Show diff vs upstream
make diff-mimo     # Show diff vs upstream
```

## Development

```bash
# Run all tests
make test

# Run vet
make vet

# Build
make build

# Run the spike (proves authctx + per-account proxy works)
make test-spike
```

## License

MIT — see [LICENSE](LICENSE).

## Acknowledgments

- [GLM-Free-API](https://github.com/izaart95-jpg/GLM-Free-API) by izaart95-jpg
- [mimo-ai-proxy](https://github.com/hugogadelha/mimo-ai-proxy) by hugogadelha
