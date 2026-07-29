# API Reference — zai2api

## احراز هویت

همه endpoint‌ها (به‌جز `/health`) نیاز به header زیر دارند:

```
Authorization: Bearer <GATEWAY_TOKEN>
```

برای Anthropic API، از `x-api-key` هم پشتیبانی می‌شود:

```
x-api-key: <GATEWAY_TOKEN>
anthropic-version: 2023-06-01
```

## OpenAI-Compatible API

### POST /v1/chat/completions

Chat completions با dispatch خودکار بر اساس model.

**Request:**
```json
{
  "model": "glm-5.1",
  "messages": [{"role": "user", "content": "Hello"}],
  "stream": false,
  "temperature": 0.7,
  "max_tokens": 1024
}
```

**Response (non-streaming):**
```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": "..."},
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}
}
```

**Dispatch:**
- `glm-*`, `zai-*` → GLM Provider
- `mimo-*` → MiMo Sub-Engine
- Alias‌ها قبل از dispatch resolve می‌شوند

### GET /v1/models

لیست ترکیبی مدل‌ها از هر دو provider.

**Response:**
```json
{
  "object": "list",
  "data": [
    {"id": "glm-5.1", "object": "model", "owned_by": "zai", "_provider": "glm"},
    {"id": "mimo-v2.5-pro", "object": "model", "owned_by": "xiaomi", "_provider": "mimo"}
  ]
}
```

### POST /v1/files

Upload فایل (فقط MiMo).

**Request:** `multipart/form-data` با فیلد `file`.

**Response:**
```json
{
  "id": "file-xxx",
  "object": "file",
  "bytes": 12345,
  "filename": "image.png"
}
```

> **نکته:** GLM endpoint مستقیم file upload ندارد. برای مدل‌های vision GLM (glm-4.6v, glm-4.5v)، از `image_url` در آرایه content messages استفاده کنید.

## Anthropic-Compatible API

### POST /v1/messages

Messages API. برای GLM مستقیم forward می‌شود. برای MiMo به OpenAI ترجمه می‌شود.

**Request:**
```json
{
  "model": "glm-5.1",
  "max_tokens": 1024,
  "messages": [{"role": "user", "content": "Hello"}],
  "stream": false
}
```

**Response:**
```json
{
  "id": "msg_xxx",
  "type": "message",
  "role": "assistant",
  "model": "glm-5.1",
  "content": [{"type": "text", "text": "..."}],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 10, "output_tokens": 20}
}
```

**Translation برای MiMo:**
- Request: Anthropic → OpenAI (system prompt, messages, tools, images)
- Response: OpenAI → Anthropic (content blocks, tool_use, stop_reason)
- Streaming: SSE OpenAI → SSE Anthropic (message_start, content_block_delta, message_stop)

## Dashboard API

### GET /api/v1/accounts

لیست اکانت‌ها (با masked tokens).

**Query Parameters:**
- `provider`: فیلتر بر اساس `glm` یا `mimo`

**Response:**
```json
[
  {
    "id": "zai-1",
    "provider": "glm",
    "display_name": "Production #1",
    "zai_token_mask": "eyJhbG...xyz",
    "has_proxy": true,
    "proxy_type": "socks5",
    "proxy_host": "10.0.0.1",
    "proxy_port": 1080,
    "enabled": true,
    "req_count": 421,
    "err_count": 0,
    "avg_latency_ms": 843,
    "last_used": "2026-07-29T14:23:01Z"
  }
]
```

### POST /api/v1/accounts

افزودن اکانت جدید.

**Request (GLM):**
```json
{
  "id": "zai-2",
  "provider": "glm",
  "display_name": "Production #2",
  "zai_token": "eyJhbG...",
  "proxy": {
    "type": "socks5",
    "host": "10.0.0.1",
    "port": 1080,
    "username": "user",
    "password": "pass"
  }
}
```

**Request (MiMo):**
```json
{
  "id": "mimo-1",
  "provider": "mimo",
  "display_name": "MiMo #1",
  "service_token": "serviceToken123",
  "user_id": "12345",
  "xiaomichatbot_ph": "ph_xyz"
}
```

**Response:** `201 Created` با اکانت + نتیجه تست اتصال.

### GET /api/v1/accounts/:id

جزئیات یک اکانت.

### PUT /api/v1/accounts/:id

به‌روزرسانی اکانت.

### DELETE /api/v1/accounts/:id

حذف اکانت.

### POST /api/v1/accounts/:id/toggle

فعال/غیرفعال کردن اکانت.

**Request:**
```json
{"enabled": false}
```

### POST /api/v1/accounts/:id/test

تست اتصال (proxy + provider).

**Response:**
```json
{
  "account_id": "zai-1",
  "tested_at": "2026-07-29T14:23:01Z",
  "proxy_status": "ok",
  "proxy_latency_ms": 45,
  "provider_status": "ok",
  "provider_latency_ms": 230,
  "overall": "ok"
}
```

### GET /api/v1/accounts/export

Export اکانت‌ها (با tokens کامل). نیاز به `X-Confirm-Token` header با `EXPORT_PASSWORD` دارد.

### POST /api/v1/accounts/import

Import اکانت‌ها از JSON.

## Models API

### GET /api/v1/models

لیست ترکیبی مدل‌ها (همان `/v1/models` اما با جزئیات بیشتر).

### GET /api/v1/models/features

همه feature state‌ها (GLM).

### GET /api/v1/models/:id/features

Feature state یک مدل.

### PUT /api/v1/models/:id/features

تنظیم feature state.

**Request:**
```json
{
  "include_all": false,
  "overrides": {
    "enable_thinking": true,
    "web_search": false,
    "auto_web_search": false,
    "preview_mode": false
  }
}
```

### GET /api/v1/models/aliases

لیست alias‌ها.

### POST /api/v1/models/aliases

افزودن alias.

**Request:**
```json
{"alias": "fast", "target_model": "glm-4.5-air"}
```

### DELETE /api/v1/models/aliases/:name

حذف alias.

### GET /api/v1/models/rate-limits

لیست rate limit‌ها.

### PUT /api/v1/models/:id/rate-limit

تنظیم rate limit.

**Request:**
```json
{"max_rpm": 60, "max_tpm": 100000, "max_context": 128000}
```

### DELETE /api/v1/models/:id/rate-limit

حذف rate limit.

## Stats API

### GET /api/v1/stats/detailed

آمار دقیق per-model.

**Query Parameters:**
- `provider`: فیلتر (`glm` یا `mimo`)
- `range`: بازه زمانی (`1h`, `24h`, `7d`)

**Response:**
```json
{
  "range": "24h",
  "provider": "",
  "models": [
    {
      "model": "glm-5.1",
      "provider": "glm",
      "requests": 832,
      "tokens": 124531,
      "avg_latency_ms": 1200,
      "error_rate": 2.1
    }
  ],
  "generated": "2026-07-29T14:23:01Z"
}
```

### GET /api/v1/stats/export

Export CSV از request_log.

**Query Parameters:**
- `provider`: فیلتر
- `range`: بازه زمانی

**Response:** `text/csv` با ستون‌های:
`timestamp, provider, model, account_id, tokens_prompt, tokens_completion, tokens_total, duration_ms, status_code, error_message`

### GET /api/v1/stats/stream

SSE stream آمار زنده (هر ۲ ثانیه).

**Response:** `text/event-stream`
```
event: stats
data: {"timestamp":"...","kpis":{...},"accounts":{...},"recent_requests":[...]}
```

## Providers API

### GET /api/v1/providers/status

وضعیت هر provider.

**Response:**
```json
{
  "providers": [
    {
      "name": "GLM (Z.AI)",
      "status": "ready",
      "uptime": "3d 4h 23m",
      "account_count": 3,
      "active_count": 2,
      "total_requests": 12453,
      "total_errors": 42,
      "avg_latency_ms": 843
    },
    {
      "name": "MiMo (Xiaomi)",
      "status": "ready",
      "uptime": "3d 4h 23m",
      "account_count": 2,
      "active_count": 2,
      "total_requests": 5432,
      "total_errors": 12,
      "avg_latency_ms": 620
    }
  ]
}
```

## Agents API (MiMo)

### GET /api/v1/agents

لیست agent‌های MiMo.

### GET /api/v1/agents/:id

وضعیت یک agent.

### POST /api/v1/agents/run

اجرای agent جدید.

### GET /api/v1/agents/:id/stream

SSE stream پیشرفت agent.

## Health

### GET /health

سلامت gateway + provider‌ها. بدون auth.

**Response:**
```json
{
  "status": "ok",
  "providers": {
    "glm": "ready",
    "mimo": "ready"
  },
  "uptime_seconds": 293421
}
```

## Error Responses

همه خطاها فرمت OpenAI-compatible دارند:

```json
{
  "error": {
    "type": "authentication_error",
    "message": "Invalid gateway token"
  }
}
```

**انواع خطا:**
- `authentication_error` (401)
- `model_not_found` (404)
- `rate_limit_exceeded` (429)
- `no_available_account` (503)
- `glm_unavailable` (503)
- `mimo_unavailable` (503)
- `translation_error` (400)
- `invalid_request` (400)
