# zai2api — Unified AI Gateway

> Single endpoint for **GLM (Z.AI)** and **MiMo (Xiaomi)** AI providers.
> One auth token, multi-account support, per-account proxy.

## What is this?

A unified HTTP gateway that combines two upstream AI proxy projects:

- **GLM-Free-API** (Z.AI) — OpenAI + Anthropic compatible, GLM-5/GLM-5.1 models
- **mimo-ai-proxy** (Xiaomi MiMo) — OpenAI compatible, mimo-v2.5 models

Instead of running two separate services with two endpoints and two auth tokens,
zai2api provides a single endpoint (`POST /v1/chat/completions`) that dispatches
to the right provider based on the `model` field in the request.

## Quick start

```bash
# Clone with submodules (important: --recursive)
git clone --recursive https://github.com/hooshidev3/zai2api.git
cd zai2api

# If you forgot --recursive:
git submodule update --init --recursive

# Build and run
make build
make run
```

The gateway starts on `http://localhost:8080`.

> ⚠️ `glm/` and `mimo/` are **git submodules** pointing to our forks:
> - `glm/` → [hooshidev3/GLM-Free-API](https://github.com/hooshidev3/GLM-Free-API) (branch: `merged-patches`)
> - `mimo/` → [hooshidev3/mimo-ai-proxy](https://github.com/hooshidev3/mimo-ai-proxy) (branch: `merged-patches`)
>
> Without `--recursive` or `make clone-init`, the build will fail.

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

### List models (aggregated GLM + MiMo)

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
| `GLM_CAPTCHA_DB` | `./data/tokens.sqlite` | Path to GLM captcha database |
| `VERBOSE` | `0` | Enable verbose logging |
| `AGENT_MODE` | `0` | Enable GLM agent mode |
| `DEFAULT_MODEL` | `glm-5` | Default model when not specified |
| `TRUSTED_PROXIES` | `127.0.0.1/32,::1/128` | Trusted proxy CIDRs |

### MiMo multi-account (env-based)

```bash
SERVICE_TOKENS="token1,token2,token3"
USER_IDS="id1,id2,id3"
XIAOMI_CHATBOT_PHS="ph1,ph2,ph3"
```

## Model routing

| Model prefix | Provider |
|--------------|----------|
| `glm-*`, `GLM-*`, `zai-*` | GLM (Z.AI) |
| `mimo*` | MiMo (Xiaomi) |
| (default) | GLM |

## Architecture

```
zai2api/
├── cmd/server/main.go              # Entry point
├── gateway/
│   ├── server/                     # Main gateway (dispatcher, handlers)
│   └── auth/                       # Timing-safe auth middleware
├── glm/                            # SUBMODULE → hooshidev3/GLM-Free-API
│   ├── main.go                     # package glm (func main() removed)
│   ├── provider.go                 # Provider struct
│   ├── exports.go                  # Public handler wrappers
│   └── cmd/token-collector/        # Separate binary (captcha collector)
├── mimo/                           # SUBMODULE → hooshidev3/mimo-ai-proxy
│   └── pkg/                        # internal/ → pkg/ for cross-module import
│       ├── authctx/                # Shared context package (no deps)
│       ├── services/               # GetAuthFromContext, ResolveClient
│       └── routes/                 # Per-account client threading
├── scripts/
│   ├── sync-submodules.sh          # Rebase submodules on latest upstream
│   ├── glm-patches/                # GLM patch scripts + templates
│   └── mimo-patches/               # MiMo patch scripts + templates
├── spike/                          # Prototype proving authctx works
├── AGENTS.md                       # Guide for AI agents
└── Makefile
```

## Syncing upstream updates

When upstream repos release updates:

```bash
make sync
```

This script:
1. Fetches latest from upstream (izaart95-jpg/GLM-Free-API, hugogadelha/mimo-ai-proxy)
2. Rebases our `merged-patches` branch on top
3. Force-pushes to our fork
4. Updates the submodule pointer in zai2api

If rebase conflicts occur, follow the on-screen instructions to resolve.

## Key design decisions

1. **Git submodules (fork-based)** — `glm/` and `mimo/` are submodules pointing to
   our forks. This is the industry-standard approach for vendoring patched dependencies.
   `git clone --recursive` brings everything.

2. **`authctx` package** — A leaf package (no dependencies) inside MiMo that breaks
   the import cycle between gateway and MiMo's services package.

3. **`ResolveClient` helper** — Exported function that prevents nil-pointer panics
   by falling back to `GlobalHTTPClient` then `http.DefaultClient`.

4. **Timing-safe auth** — Uses `crypto/subtle.ConstantTimeCompare` to prevent
   timing attacks on token validation.

5. **Per-account proxy ready** — HTTP client is threaded through all 9 MiMo
   functions via `authctx.WithClient()`, enabling per-account proxy in Phase 3.

## Submodule workflow (for maintainers)

### Updating submodules

```bash
# Check submodule status
make submodule-status

# Sync with upstream (rebase our patches)
make sync

# After sync, push the updated pointers
git push origin main
```

### Making changes to submodules

```bash
# Enter submodule
cd glm

# Make changes and commit
git add -A && git commit -m "patch(glm): description"

# Push to fork
git push origin merged-patches

# Return to parent repo and update pointer
cd ..
git add glm
git commit -m "chore: update glm submodule"
git push origin main
```

See [AGENTS.md](AGENTS.md) for detailed guidance.

## License

MIT — see [LICENSE](LICENSE).

## Acknowledgments

- [GLM-Free-API](https://github.com/izaart95-jpg/GLM-Free-API) by izaart95-jpg
- [mimo-ai-proxy](https://github.com/hugogadelha/mimo-ai-proxy) by hugogadelha
