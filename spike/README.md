# Spike — authctx + per-account proxy proof

این spike اثبات می‌کند که معماری per-account proxy برای MiMo واقعاً کار می‌کند،
بدون ایجاد import cycle یا باگ بی‌صدا.

## اجرا

```bash
bash run.sh
```

## ساختار

```
spike/
├── run.sh                           # اسکریپت اجرا
├── subengine/                       # شبیه mimo (module subengine)
│   ├── go.mod
│   └── pkg/
│       ├── authctx/                 # پکیج مشترک (leaf، بدون وابستگی)
│       │   ├── authctx.go
│       │   └── authctx_test.go
│       └── services/                # شبیه mimoproxy/pkg/services
│           ├── services.go          # HandleChat با ResolveClient
│           └── services_test.go     # تست proxy واقعی
└── gateway/                         # شبیه merged-proxy-v2 (module gateway)
    ├── go.mod                       # replace subengine => ../subengine
    └── main.go
```

## چه چیزی اثبات می‌شود

| تست | چه چیزی را اثبات می‌کند |
|-----|------------------------|
| `TestKeyIdentity` | هویت کلید context درست کار می‌کند |
| `TestFallbackWhenNotSet` | fallback به env-based کار می‌کند |
| `TestDirectContextWithValueDoesNotWork` | کلیدهای خارجی پذیرفته نمی‌شوند |
| `TestResolveClient` | helper از panic جلوگیری می‌کند |
| `TestHandleChatUsesInjectedClient` | **proxy واقعاً استفاده می‌شود** (مهم‌ترین) |
| `TestHandleChatFallbackToGlobalClient` | backward compatibility |
| curl `/test` (round-robin) | gateway auth را تزریق می‌کند |
| curl `/auth-info` | fallback به env-token |

## ⚠️ محدودیت spike

هیچ تست end-to-end ای proxy را از طریق gateway تست نمی‌کند:

| تست | اثبات می‌کند | اثبات نمی‌کند |
|-----|-------------|---------------|
| `TestHandleChatUsesInjectedClient` | services.HandleChat از client تزریق‌شده استفاده می‌کند | gateway client را درست تزریق می‌کند |
| curl `/test` (round-robin) | gateway auth را تزریق می‌کند و sub-engine دریافت می‌کند | proxy استفاده می‌شود |

تست end-to-end (gateway + proxy + services) در فاز ۳ پوشش داده می‌شود.

## معیار موفقیت

اگر `run.sh` با exit code 0 تمام شود:

- ✅ build هر module بدون خطا
- ✅ cross-module import کار می‌کند (gateway می‌تواند subengine را import کند)
- ✅ هیچ import cycle ای وجود ندارد
- ✅ هویت کلید context درست است
- ✅ per-account proxy واقعاً استفاده می‌شود (TestHandleChatUsesInjectedClient)
- ✅ fallback به env-based کار می‌کند
- ✅ round-robin auth کار می‌کند

→ **آماده‌ی شروع فاز ۱**

اگر شکست خورد:

- `import cycle not allowed` → طراحی مجدد
- `proxyRequests == 0` → per-account proxy شکسته، طراحی مجدد
- `cd gateway && go build` شکست → cross-module import مشکل دارد
