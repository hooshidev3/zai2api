# AGENTS.md — راهنمای کار با zai2api برای AI Agents

> این فایل برای AI agentهایی نوشته شده که روی این پروژه کار می‌کنند.
> قبل از هر تغییر، این فایل را کامل بخوانید.

## ۱. معماری در یک نگاه

zai2api یک gateway واحد است که دو provider هوش مصنوعی را ترکیب می‌کند:

| Provider | Submodule | Upstream اصلی | Fork ما |
|----------|-----------|---------------|---------|
| GLM (Z.AI) | `glm/` | izaart95-jpg/GLM-Free-API | hooshidev3/GLM-Free-API |
| MiMo (Xiaomi) | `mimo/` | hugogadelha/mimo-ai-proxy | hooshidev3/mimo-ai-proxy |

**نکته کلیدی:** `glm/` و `mimo/` **git submodule** هستند، نه کد معمولی.
هر کدام یک repo مستقل با branch `merged-patches` در fork ما هستند.

### جریان درخواست

```
Client (یک توکن: GATEWAY_TOKEN)
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
```

## ۲. قوانین حیاتی (هرگز نقض نکنید)

### ❌ هرگز این کارها را نکنید:

1. **هرگز submodule‌ها را مستقیم ویرایش و بدون commit رها نکنید.**
   تغییرات در submodule باید commit و به fork push شوند، وگرنه گم می‌شوند.

2. **هرگز در submodule به upstream اصلی push نکنید.**
   فقط به fork (hooshidev3/...) push کنید.

3. **هرگز `git submodule update` را بدون درک وضعیت اجرا نکنید.**
   اگر در submodule تغییرات commit‌نشده دارید، `update` آن‌ها را پاک می‌کند.

4. **هرگز `internal/` را در MiMo دوباره نسازید.**
   ما آن را به `pkg/` تغییر دادیم تا cross-module import ممکن شود.

5. **هرگز `package main` را در GLM root برگردانید.**
   ما آن را به `package glm` تغییر دادیم. فقط `cmd/token-collector/` باید `package main` باشد.

6. **هرگز مستقیماً از `context.WithValue` برای auth استفاده نکنید.**
   همیشه از `authctx.WithAuth` / `authctx.FromContext` استفاده کنید.
   استفاده مستقیم باعث باگ بی‌صدای runtime می‌شود (context key identity).

7. **هرگز `GlobalHTTPClient` را مستقیم صدا نزنید.**
   همیشه از `ResolveClient(client)` استفاده کنید تا از panic جلوگیری شود.

### ✅ همیشه این کارها را بکنید:

1. قبل از کار روی submodule، مطمئن شوید روی branch `merged-patches` هستید:
   ```bash
   cd glm && git status  # باید بگوید: On branch merged-patches
   ```

2. بعد از تغییر در submodule، commit و push کنید:
   ```bash
   cd glm
   git add -A && git commit -m "patch(glm): توضیح تغییر"
   git push origin merged-patches
   cd ..
   git add glm  # ثبت commit SHA جدید در پروژه اصلی
   git commit -m "chore: update glm submodule"
   ```

3. برای HTTP client همیشه از `ResolveClient` استفاده کنید:
   ```go
   client = services.ResolveClient(client)  // ✅ همیشه غیر-nil
   ```

## ۳. ساختار پروژه

```
zai2api/
├── cmd/server/main.go          # Entry point
├── gateway/
│   ├── server/                 # Core gateway logic
│   │   ├── server.go           # Server struct + mountRoutes
│   │   ├── dispatcher.go       # Route by model + alias resolution
│   │   ├── accounts.go         # AccountManager (atomic.Bool, DTO, slice order)
│   │   ├── accounts_api.go     # Account CRUD endpoints
│   │   ├── accounts_crud.go    # CRUD logic
│   │   ├── proxy.go            # Per-account HTTP client (HTTP/HTTPS/SOCKS5)
│   │   ├── crypto.go           # AES-256-GCM for proxy passwords
│   │   ├── ratelimit.go        # Token bucket rate limiter
│   │   ├── ratelimit_api.go    # Rate limit CRUD
│   │   ├── aliases_api.go      # Model aliases CRUD
│   │   ├── stats.go            # StatsCollector + SSE
│   │   ├── stats_api.go        # Stats endpoints + CSV export
│   │   ├── dashboard.go        # Dashboard handlers
│   │   ├── models_api.go       # Aggregated models + features
│   │   ├── providers_api.go    # Provider status
│   │   ├── agents_api.go       # MiMo agent proxy
│   │   ├── files_api.go        # File upload (MiMo)
│   │   ├── anthropic_bridge.go # Anthropic translator for MiMo
│   │   ├── auth.go             # GatewayAuth + DashboardAuth
│   │   ├── db.go               # SQLite WAL + retention
│   │   └── test_connection.go  # Connection testing
│   └── auth/                   # Auth middleware
├── glm/                        # Submodule → hooshidev3/GLM-Free-API
│   ├── go.mod                  # module glm-free-api
│   ├── main.go                 # package glm (patched)
│   ├── provider.go             # Provider struct
│   ├── exports.go              # Exported functions (FetchModels, SetFeatureState)
│   └── cmd/token-collector/    # Separate module (playwright + bubbletea)
├── mimo/                       # Submodule → hooshidev3/mimo-ai-proxy
│   ├── go.mod                  # module mimoproxy
│   └── pkg/                    # internal → pkg (patched)
│       ├── authctx/            # Context-based auth injection (shared)
│       ├── services/           # GetAuthFromContext, ResolveClient, HandleMimoChat
│       ├── routes/             # Chat + Agent routes
│       ├── agent/              # Agent loop (planner, critic, executor)
│       ├── models/             # Data models
│       └── utils/              # Helpers
├── templates/                  # HTML templates
├── static/                     # CSS/JS/fonts
├── scripts/                    # Sync scripts
├── .github/workflows/          # CI/CD
├── docs/                       # Documentation
└── data/                       # SQLite databases (runtime)
```

## ۴. پکیج `authctx` — قلب معماری

`mimo/pkg/authctx/` یک پکیج مستقل است که import cycle بین gateway و MiMo را حل می‌کند.

**قرارداد حیاتی:** هرگز مستقیماً از `context.WithValue` برای auth استفاده نکنید.
همیشه از `authctx.WithAuth` / `authctx.FromContext` استفاده کنید.

```go
// ❌ اشتباه
ctx = context.WithValue(ctx, "auth", account)

// ✅ صحیح
ctx = authctx.WithAuth(ctx, authctx.InjectedAuth{...})
```

### جریان auth در MiMo

```
1. Gateway: انتخاب اکانت (AccountManager.Next)
2. Gateway: ساخت HTTP client per-account (با proxy)
3. Gateway: authctx.WithAuth + WithClient در request.Context()
4. Gateway: mimoEngine.ServeHTTP(c.Writer, c.Request)
5. MiMo: services.GetAuthFromContext(c.Request.Context())
6. MiMo: authctx.FromContext(ctx) → InjectedAuth + *http.Client
7. MiMo: ResolveClient(client) → استفاده در client.Do(req)
```

## ۵. Workflow آپدیت upstream (گام‌به‌گام)

وقتی می‌خواهید آخرین تغییرات upstream را دریافت کنید:

```bash
# برای GLM:
cd glm
git fetch upstream                    # upstream = izaart95-jpg/GLM-Free-API
git rebase upstream/main              # rebase تغییرات ما روی upstream جدید
# اگر conflict شد: حل کنید، git add، git rebase --continue
git push --force-with-lease origin merged-patches
cd ..
git add glm
git commit -m "chore: update glm to latest upstream"

# برای MiMo (همین الگو):
cd mimo
git fetch upstream                    # upstream = hugogadelha/mimo-ai-proxy
git rebase upstream/main
git push --force-with-lease origin merged-patches
cd ..
git add mimo
git commit -m "chore: update mimo to latest upstream"
```

### اگر rebase conflict داد:

1. `git status` — فایل‌های conflicted را ببینید
2. فایل را باز کنید و markerهای `<<<<<<<` را حل کنید
3. **مراقب باشید تغییرات ما را پاک نکنید:**
   - در GLM: `package glm` و حذف `func main()` باید حفظ شوند
   - در MiMo: `pkg/` (نه `internal/`) و `authctx` باید حفظ شوند
4. `git add <file>` و `git rebase --continue`

## ۶. لیست patch‌های اعمال‌شده

### در `glm/` (branch: merged-patches)

| Commit | تغییر | چرا |
|--------|-------|-----|
| 1 | `package main` → `package glm` | قابل import شدن به‌عنوان library |
| 2 | حذف `func main()` و flag parsing | gateway خودش entrypoint دارد |
| 3 | انتقال `captcha.go` → `cmd/token-collector/main.go` | جدا کردن وابستگی‌های سنگین |
| 4 | افزودن `go.mod` (module glm-free-api) | تعریف module |
| 5 | افزودن `provider.go` (Provider struct) | multi-account support |
| 6 | افزودن `exports.go` (FetchModels, SetFeatureState) | gateway integration |

### در `mimo/` (branch: merged-patches)

| Commit | تغییر | چرا |
|--------|-------|-----|
| 1 | `internal/` → `pkg/` + update imports | Go اجازه import `internal/` از خارج module را نمی‌دهد |
| 2 | حذف `main.go` | gateway خودش entrypoint دارد |
| 3 | افزودن `pkg/authctx/` | شکستن import cycle + تضمین context key identity |
| 4 | افزودن `GetAuthFromContext` در services | خواندن auth از request context |
| 5 | افزودن `ResolveClient` (exported) | جلوگیری از panic + استفاده در routes |
| 6 | thread client در ۹ تابع | per-account proxy |

## ۷. Database Schema

```sql
-- accounts: اکانت‌های GLM و MiMo
CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,           -- 'glm' | 'mimo'
    display_name TEXT NOT NULL,
    notes TEXT,
    zai_token TEXT,                   -- GLM
    service_token TEXT,               -- MiMo
    user_id TEXT,                     -- MiMo
    xiaomichatbot_ph TEXT,            -- MiMo
    proxy_type TEXT,                  -- 'http' | 'https' | 'socks5' | NULL
    proxy_host TEXT,
    proxy_port INTEGER,
    proxy_username TEXT,
    proxy_password TEXT,              -- AES-256-GCM encrypted
    enabled BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- model_features: per-model feature state (GLM)
CREATE TABLE model_features (
    model TEXT PRIMARY KEY,
    include_all BOOLEAN DEFAULT 0,
    overrides_json TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- model_aliases: alias‌های مدل
CREATE TABLE model_aliases (
    alias TEXT PRIMARY KEY,
    target_model TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- model_rate_limits: rate limit‌های per-model
CREATE TABLE model_rate_limits (
    model TEXT PRIMARY KEY,
    max_rpm INTEGER,
    max_tpm INTEGER,
    max_context INTEGER,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- request_log: لاگ درخواست‌ها (برای داشبورد)
CREATE TABLE request_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    provider TEXT NOT NULL,
    model TEXT NOT NULL,
    account_id TEXT,
    tokens_prompt INTEGER,
    tokens_completion INTEGER,
    tokens_total INTEGER,
    duration_ms INTEGER,
    status_code INTEGER,
    error_message TEXT,
    client_ip TEXT
);
```

## ۸. تست‌ها

```bash
# تست کل پروژه
make test

# تست authctx (مهم‌ترین)
cd mimo && go test ./pkg/authctx/...

# تست services
cd mimo && go test ./pkg/services/...

# تست gateway
go test ./gateway/...

# Smoke test
make smoke
```

## ۹. Troubleshooting

| مشکل | راه‌حل |
|------|--------|
| `go build` خطای `directory not found` برای glm/mimo | `git submodule update --init --recursive` |
| submodule در detached HEAD است | `cd glm && git checkout merged-patches` |
| تغییرات در submodule گم شد | همیشه commit + push قبل از `git add` در root |
| `import cycle not allowed` | بررسی کنید authctx به gateway اشاره نکند |
| `use of internal package` | MiMo را `internal/` → `pkg/` rename کنید |
| `panic: nil pointer` در client.Do | از `ResolveClient(client)` استفاده کنید |
| `converting NULL to string` در CSV export | از `sql.NullString` استفاده کنید |
| SSE stream قطع می‌شود | از `Context().Done()` استفاده کنید، نه `CloseNotify` |

## ۱۰. Adding a New Provider (Guide for Agents)

این بخش توضیح می‌دهد چطور یک provider سوم (مثلاً OpenAI, DeepSeek, Qwen) اضافه کنید.

### ۱۰.۱ تصمیم معماری

ابتدا الگوی integration را انتخاب کنید:

| الگو | کی استفاده شود | مثال |
|------|----------------|------|
| **Submodule** | Provider یک codebase Go پیچیده موجود دارد | GLM, MiMo |
| **Native** | Provider یک API wrapper ساده است (از صفر بنویسید) | OpenAI, DeepSeek |

برای یک provider API ساده، **Native** توصیه می‌شود (بدون پیچیدگی submodule).

### ۱۰.۲ Native Provider — گام‌به‌گام

#### گام ۱: ساخت پکیج provider

```
gateway/providers/
└── deepseek/
    ├── provider.go      # implements Provider interface
    └── provider_test.go
```

#### گام ۲: پیاده‌سازی Provider interface

هر provider باید این interface را پیاده کند (تعریف در `gateway/providers/provider.go`):

```go
package providers

import "net/http"

type Provider interface {
    // Name شناسه provider را برمی‌گرداند (مثلاً "deepseek")
    Name() string

    // ChatCompletion یک درخواست chat را handle می‌کند.
    // باید streaming (SSE) را وقتی req.Stream true است پشتیبانی کند.
    ChatCompletion(w http.ResponseWriter, r *http.Request, req ChatRequest, acct *Account) error

    // ListModels مدل‌های موجود این provider را برمی‌گرداند
    ListModels() []ModelInfo

    // TestConnection credentials یک اکانت را verify می‌کند
    TestConnection(acct *Account) error

    // Close منابع را آزاد می‌کند
    Close() error
}

type ChatRequest struct {
    Model    string
    Messages []Message
    Stream   bool
    // ... سایر فیلدهای OpenAI-compatible
}

type ModelInfo struct {
    ID       string `json:"id"`
    OwnedBy  string `json:"owned_by"`
    Provider string `json:"_provider"`
}
```

#### گام ۳: ثبت در dispatcher

در `gateway/server/dispatcher.go`، به `routeByModel` اضافه کنید:

```go
func routeByModel(model string) string {
    m := strings.ToLower(model)
    switch {
    case strings.HasPrefix(m, "mimo"):     return "mimo"
    case strings.HasPrefix(m, "glm"):      return "glm"
    case strings.HasPrefix(m, "deepseek"): return "deepseek"  // ← جدید
    default:                               return "glm"
    }
}
```

#### گام ۴: افزودن فیلدهای اکانت (در صورت نیاز)

اگر provider فیلدهای خاص اکانت نیاز دارد، به `Account` struct در `gateway/server/accounts.go` اضافه کنید و این‌ها را به‌روز کنید:
- `CreateAccountRequest` validation
- `saveToDB` / `loadFromDB` (افزودن ستون به schema)
- `ToDTO` (mask کردن فیلدهای حساس)

#### گام ۵: افزودن DB migration

ستون‌های جدید را به جدول `accounts` در `gateway/server/db.go` اضافه کنید:

```go
func migrateDB(db *sql.DB) error {
    // بررسی وجود ستون، افزودن در صورت نبود
    var count int
    db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('accounts')
                 WHERE name='deepseek_api_key'`).Scan(&count)
    if count == 0 {
        db.Exec(`ALTER TABLE accounts ADD COLUMN deepseek_api_key TEXT`)
    }
    return nil
}
```

#### گام ۶: افزودن UI داشبورد

در `templates/dashboard.html`، فیلدهای فرم اکانت provider را اضافه کنید
(الگوی GLM/MiMo موجود را دنبال کنید با نمایش شرطی).

#### گام ۷: افزودن تست‌ها

```go
func TestDeepSeekProvider(t *testing.T) {
    // از httptest.Server برای mock کردن API provider استفاده کنید
    mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
    }))
    defer mock.Close()

    p := NewDeepSeekProvider(mock.URL)
    // تست ChatCompletion, ListModels, TestConnection
}
```

#### گام ۸: به‌روزرسانی مستندات

- [ ] افزودن به جدول provider‌ها در README.md
- [ ] افزودن به بخش ۴ AGENTS.md (patch‌ها/integration)
- [ ] افزودن متغیرهای env به `.env.example`
- [ ] به‌روزرسانی `docs/API.md`
- [ ] به‌روزرسانی `docs/PROVIDERS.md` (matrix قابلیت‌ها)

### ۱۰.۳ Submodule Provider — گام‌به‌گام

برای یک codebase موجود پیچیده (مثل GLM/MiMo):

1. **Fork** کردن repo upstream به `hooshidev3/<name>`
2. **افزودن submodule**: `git submodule add -b merged-patches <fork-url> <name>/`
3. **ساخت branch `merged-patches`** در fork
4. **اعمال patch‌ها** (rename package, حذف main, افزودن exports)
5. **افزودن `replace` directive** در root `go.mod`: `replace <module> => ./<name>`
6. **Mount کردن routes** در `server.go` (الگوی sub-engine برای gin، یا handler wrapper)
7. **افزودن پشتیبانی authctx** اگر per-account proxy لازم است
8. **مستندسازی patch‌ها** در AGENTS.md بخش ۶

### ۱۰.۴ Matrix قابلیت‌های Provider

هنگام افزودن provider، این matrix را در `docs/PROVIDERS.md` پر کنید:

| قابلیت | Provider | توضیح |
|--------|----------|-------|
| OpenAI Chat | ✅/❌ | |
| Streaming | ✅/❌ | |
| Anthropic | ✅/❌ | |
| Agent | ✅/❌ | |
| History | ✅/❌ | |
| File Upload | ✅/❌ | |
| Vision Input | ✅/❌ | |
| Per-Model Features | ✅/❌ | |

### ۱۰.۵ اشتباهات رایج هنگام افزودن Provider

| اشتباه | چرا اشتباه است | رویکرد صحیح |
|--------|----------------|-------------|
| استفاده از `context.WithValue` برای auth | باگ بی‌صدای runtime | استفاده از `authctx.WithAuth` |
| فراموش کردن `ResolveClient` | panic با nil pointer | همیشه HTTP client را wrap کنید |
| blocking روی streaming response | SSE را می‌شکند | استفاده از `c.Writer.Flush()` |
| hardcode کردن لیست مدل‌ها | قدیمی می‌شود | fetch از API provider |
| ذخیره secrets به‌صورت plaintext | ریسک امنیتی | استفاده از AES-256-GCM (`crypto.go`) |
| import کردن gateway از provider | import cycle | Provider نباید gateway را import کند |

## ۱۱. CI/CD

### GitHub Actions

- `build.yml`: matrix build برای ۶ پلتفرم + Docker multi-arch + Release
- Docker Hub فقط اگر `DOCKERHUB_TOKEN` تنظیم شده باشد push می‌کند
- GHCR همیشه push می‌شود

### نصب خودکار

- `install.sh`: Linux (systemd) + macOS (launchd)
- `install.ps1`: Windows (Windows Service)
- هر دو `--service` و `--autostart` را پشتیبانی می‌کنند

## ۱۲. چک‌لیست قبل از هر PR

- [ ] submodule‌ها روی branch `merged-patches` هستند
- [ ] تغییرات submodule commit و push شده‌اند
- [ ] commit SHA جدید در root ثبت شده (`git add glm`)
- [ ] `make build` موفق است
- [ ] `make test` موفق است
- [ ] `cd mimo && go test ./pkg/authctx/...` موفق است
- [ ] مستندات به‌روز شده‌اند (README, AGENTS, API, PROVIDERS)
- [ ] `.env.example` متغیرهای جدید دارد
