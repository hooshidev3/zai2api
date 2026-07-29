# Provider Capabilities

## Agent Support

| Provider | Agent Loop | Tool Calling | Notes |
|----------|:---:|:---:|-------|
| GLM (Z.AI) | No | Yes | "Agent mode" is only tool-calling translation (OpenAI tools ↔ Z.AI text contract). No real agent loop. |
| MiMo (Xiaomi) | Yes | Yes | Full agent loop with planner, critic, executor in `pkg/agent/`. Uses `/v1/agent/*` endpoints. |

### GLM Agent Mode (Limitation)

GLM-Free-API has an "Agent mode" that is **not a real agent loop**:
- Translates OpenAI tools/function-calling into a text contract that Z.AI can understand
- Intercepts `<<<TOOL_CALL>>>` blocks from the model's output
- Rewrites them back into native `tool_calls` deltas

This is useful for using tools with Z.AI, but there is no planner/critic/executor.

### MiMo Agent Loop (Full)

MiMo has a real agent loop:
- `POST /v1/agent/run` — Start a new agent
- `GET /v1/agent/status/:id` — Agent status
- `GET /v1/agent/stream/:id` — SSE stream of agent progress

## Anthropic Messages API

| Provider | Native Anthropic | Translation | Notes |
|----------|:---:|:---:|-------|
| GLM (Z.AI) | Yes | — | GLM has a native `/v1/messages` handler |
| MiMo (Xiaomi) | No | Yes | Gateway translates Anthropic ↔ OpenAI automatically |

## File Upload

| Provider | Direct Upload | Alternative | Notes |
|----------|:---:|------------|-------|
| GLM (Z.AI) | No | `image_url` in messages | Use vision models (glm-4.6v, glm-4.5v) with image_url content type |
| MiMo (Xiaomi) | Yes | — | `POST /v1/files` forwards to `UploadToXiaomi` |

## Vision / Multimodal

| Provider | Vision Models | Input Format | Notes |
|----------|:---:|-------------|-------|
| GLM (Z.AI) | glm-4.5v, glm-4.6v, GLM-5v-Turbo | `image_url` in messages content array | `image_generation` is always forced to `false` |
| MiMo (Xiaomi) | Yes (via upload) | Upload file first, then reference in chat | Uses `UploadToXiaomi` + `multiMedias` in payload |
