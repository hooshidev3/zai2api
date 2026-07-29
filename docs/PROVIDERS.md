# Providers — zai2api

## Overview

zai2api دو provider هوش مصنوعی را پشت یک gateway واحد ترکیب می‌کند:

| Provider | ریپازیتوری | مدل‌ها |
|----------|-----------|--------|
| GLM (Z.AI) | izaart95-jpg/GLM-Free-API | glm-5.1, glm-5, glm-4.7, glm-4.6v, glm-4.5v |
| MiMo (Xiaomi) | hugogadelha/mimo-ai-proxy | mimo-v2.5-pro, mimo-v2.5, mimo-7b |

## Matrix قابلیت‌ها

| قابلیت | GLM (Z.AI) | MiMo (Xiaomi) | توضیح |
|--------|:---:|:---:|-------|
| **OpenAI Chat Completions** | ✅ | ✅ | هر دو |
| **Streaming (SSE)** | ✅ | ✅ | هر دو |
| **Anthropic Messages API** | ✅ | ✅ (با ترجمه) | GLM native؛ MiMo از طریق translator |
| **Agent Loop** | ❌ | ✅ | فقط MiMo (planner/critic/executor) |
| **Agent Monitoring** | ❌ | ✅ | فقط MiMo |
| **Conversation History** | ❌ | ✅ | GLM stateless است |
| **File Upload** | ❌ | ✅ | MiMo: UploadToXiaomi؛ GLM: فقط image_url |
| **Vision Input** | ✅ | 🟡 | GLM-4.6v/4.5v؛ MiMo محدود |
| **Image Generation** | ❌ | ❌ | GLM: force-disabled در upstream |
| **Web Search** | ✅ | ❌ | GLM: web_search feature |
| **Thinking/Reasoning** | ✅ | ✅ | GLM: enable_thinking؛ MiMo: reasoning models |
| **Per-Model Features** | ✅ | ❌ | فقط GLM (ModelFeatureState) |
| **Multi-Account (ذاتی)** | ❌ | ✅ | MiMo: SERVICE_TOKENS comma-separated |
| **Multi-Account (gateway)** | ✅ | ✅ | هر دو از طریق AccountManager |
| **Per-Account Proxy** | ✅ | ✅ | هر دو از طریق authctx + ResolveClient |
| **Captcha Handling** | ✅ | ❌ | GLM: Aliyun captcha + cache |

## GLM (Z.AI) — جزئیات

### مدل‌ها

| مدل | نوع | Context | توضیح |
|-----|-----|---------|-------|
| glm-5.1 | Reasoning | 128K | پرچمدار، thinking پیش‌فرض |
| glm-5 | Reasoning | 128K | |
| glm-4.7 | Chat | 128K | |
| glm-4.6v | Vision | 128K | درک تصویر |
| glm-4.5v | Vision | 128K | درک تصویر |
| glm-4.5-air | Fast | 128K | سریع، ارزان |

### Per-Model Features

GLM یک سیستم per-model feature override دارد (`ModelFeatureState`):

| Feature | نوع | پیش‌فرض | توضیح |
|---------|-----|---------|-------|
| enable_thinking | bool | true | فعال‌سازی reasoning |
| web_search | bool | false | جستجوی وب (درخواستی) |
| auto_web_search | bool | false | جستجوی خودکار وب |
| preview_mode | bool | false | حالت پیش‌نمایش |
| image_generation | bool | false (force) | تولید تصویر (همیشه off) |

این features از طریق داشبورد (تب Models) یا API قابل تنظیم هستند:

```bash
curl -X PUT http://localhost:8080/api/v1/models/glm-5.1/features \
  -H "Authorization: Bearer $GATEWAY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"include_all":false,"overrides":{"enable_thinking":true,"web_search":true}}'
```

### محدودیت‌ها

1. **File Upload:** GLM endpoint مستقیم upload ندارد. برای vision، از `image_url` در messages استفاده کنید:
   ```json
   {
     "model": "glm-4.6v",
     "messages": [{"role":"user","content":[
       {"type":"image_url","image_url":{"url":"https://example.com/img.png"}},
       {"type":"text","text":"توضیح بده"}
     ]}]
   }
   ```

2. **Agent:** GLM «Agent mode» دارد که فقط ترجمه tool-calling است (OpenAI tools ↔ Z.AI text contract). agent loop واقعی (planner/critic/executor) ندارد.

3. **History:** GLM stateless است. تاریخچه مکالمه ذخیره نمی‌شود.

4. **Image Generation:** در upstream همیشه `false` است و قابل فعال‌سازی نیست.

### Captcha

GLM از Aliyun captcha برای احراز هویت با Z.AI استفاده می‌کند:
- `captchaCache` با TTL ۷۵ ثانیه و pool size ۲
- `cmd/token-collector/` برای تولید token‌های جدید (با playwright)

## MiMo (Xiaomi) — جزئیات

### مدل‌ها

| مدل | نوع | Context | توضیح |
|-----|-----|---------|-------|
| mimo-v2.5-pro | Advanced | 128K | پرچمدار |
| mimo-v2.5 | Reasoning | 128K | |
| mimo-7b | Fast | 32K | سریع، سبک |

### Agent Loop

MiMo یک agent loop کامل دارد (`pkg/agent/`):
- **Planner:** برنامه‌ریزی مراحل
- **Critic:** ارزیابی پیشرفت
- **Executor:** اجرای اقدامات

Endpoint‌ها:
- `POST /v1/agent/run` — اجرای agent جدید
- `GET /v1/agent/status/:id` — وضعیت agent
- `GET /v1/agent/stream/:id` — SSE stream پیشرفت

### File Upload

MiMo از `UploadToXiaomi` در `services/mimo.go` استفاده می‌کند:
1. محاسبه MD5 فایل
2. دریافت upload URL از Xiaomi API
3. آپلود فایل
4. parse کردن نتیجه

### Multi-Account ذاتی

MiMo از قبل multi-account است:
```bash
SERVICE_TOKENS="token1,token2,token3"
USER_IDS="user1,user2,user3"
XIAOMI_CHATBOT_PHS="ph1,ph2,ph3"
```
`GetSelectedAuth()` با random rotation یکی را انتخاب می‌کند.

### محدودیت‌ها

1. **Anthropic API:** MiMo ذاتاً Anthropic API ندارد. gateway یک translation layer دارد که request/response را بین Anthropic و OpenAI ترجمه می‌کند.

2. **Vision:** پشتیبانی محدود از vision input.

3. **Web Search:** پشتیبانی نمی‌شود.

## Anthropic Translation Layer

برای MiMo، gateway یک مترجم دوطرفه دارد (`anthropic_bridge.go`):

### Request Translation (Anthropic → OpenAI)

| Anthropic | OpenAI |
|-----------|--------|
| `system` (string یا blocks) | `messages[0]` با `role: system` |
| `messages[].content` (blocks) | `messages[].content` (parts) |
| `content[].type: "image"` | `content[].type: "image_url"` |
| `content[].type: "tool_use"` | `tool_calls[]` |
| `content[].type: "tool_result"` | `role: "tool"` message |
| `tools[].input_schema` | `tools[].function.parameters` |
| `stop_sequences` | `stop` |

### Response Translation (OpenAI → Anthropic)

| OpenAI | Anthropic |
|--------|-----------|
| `choices[0].message.content` | `content[0]` با `type: "text"` |
| `choices[0].message.tool_calls` | `content[]` با `type: "tool_use"` |
| `choices[0].finish_reason: "stop"` | `stop_reason: "end_turn"` |
| `choices[0].finish_reason: "length"` | `stop_reason: "max_tokens"` |
| `choices[0].finish_reason: "tool_calls"` | `stop_reason: "tool_use"` |
| `usage.prompt_tokens` | `usage.input_tokens` |
| `usage.completion_tokens` | `usage.output_tokens` |

### Streaming Translation

SSE OpenAI (`data: {...}`) به SSE Anthropic ترجمه می‌شود:
- `message_start`
- `content_block_start`
- `content_block_delta` (text_delta)
- `content_block_stop`
- `message_delta`
- `message_stop`

## افزودن Provider جدید

برای راهنمای کامل افزودن provider جدید، بخش ۱۰ `AGENTS.md` را ببینید.

خلاصه:
1. تصمیم: Submodule (codebase پیچیده) یا Native (API ساده)
2. پیاده‌سازی `Provider` interface
3. ثبت در `routeByModel` dispatcher
4. افزودن فیلدهای اکانت + DB migration
5. افزودن UI داشبورد
6. افزودن تست‌ها
7. به‌روزرسانی مستندات
