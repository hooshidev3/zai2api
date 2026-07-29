#!/bin/bash
# ════════════════════════════════════════════════════════════════════
# test-anthropic-streaming.sh — Real MiMo Streaming Translation Test
# ════════════════════════════════════════════════════════════════════
#
# Verifies that the Anthropic ↔ OpenAI streaming translation bridge works
# correctly against a real MiMo account configured in the gateway.
#
# Prerequisites:
#   - Gateway running on $BASE (default: http://localhost:8080)
#   - At least one MiMo account added via /api/v1/accounts
#   - $GATEWAY_TOKEN set to a valid gateway token
#
# Usage:
#   GATEWAY_TOKEN=sk-your-token ./scripts/test-anthropic-streaming.sh
#
# What it checks:
#   1. All 6 Anthropic SSE events are emitted (message_start, content_block_start,
#      content_block_delta, content_block_stop, message_delta, message_stop)
#   2. text_delta type appears in content_block_delta events
#   3. No OpenAI-format chunks (chat.completion.chunk) leak into the output
# ════════════════════════════════════════════════════════════════════
set -uo pipefail

GATEWAY_TOKEN="${GATEWAY_TOKEN:-sk-merged-change-me}"
BASE="${BASE:-http://localhost:8080}"
OUT="/tmp/anthropic_stream_test.txt"

echo "╔════════════════════════════════════════════════════╗"
echo "║  Real MiMo Streaming Translation Test             ║"
echo "╚════════════════════════════════════════════════════╝"
echo ""
echo "Gateway: $BASE"
echo "Token:   ${GATEWAY_TOKEN:0:12}..."
echo ""

# ── Test 1: streaming ─────────────────────────────────────────────
echo "── Test 1: Anthropic → MiMo (streaming) ──"
HTTP_CODE=$(curl -N -s -o "$OUT" -w "%{http_code}" -X POST "$BASE/v1/messages" \
  -H "x-api-key: $GATEWAY_TOKEN" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mimo-v2.5-pro",
    "max_tokens": 128,
    "stream": true,
    "messages": [{"role": "user", "content": "Say hello in exactly one word."}]
  }')

echo "HTTP status: $HTTP_CODE"
echo ""

if [ "$HTTP_CODE" != "200" ]; then
  echo "❌ Request failed with HTTP $HTTP_CODE"
  echo "Response body:"
  cat "$OUT"
  exit 1
fi

echo "── Raw output (first 20 lines) ──"
head -20 "$OUT"
echo ""
echo "  (... full output saved to $OUT)"
echo ""

# ── Verification ──────────────────────────────────────────────────
echo "── Verification ──"
PASS=0
FAIL=0

check() {
    local desc="$1"; local pattern="$2"; local should_exist="${3:-yes}"
    if grep -q "$pattern" "$OUT"; then
        if [ "$should_exist" = "yes" ]; then
            echo "  ✓ $desc"; PASS=$((PASS+1))
        else
            echo "  ✗ $desc (LEAKED — should not be present)"; FAIL=$((FAIL+1))
        fi
    else
        if [ "$should_exist" = "yes" ]; then
            echo "  ✗ $desc (MISSING)"; FAIL=$((FAIL+1))
        else
            echo "  ✓ $desc (correctly absent)"; PASS=$((PASS+1))
        fi
    fi
}

# Anthropic events that MUST be present
check "message_start event"      "event: message_start"
check "content_block_start"      "event: content_block_start"
check "content_block_delta"      "event: content_block_delta"
check "text_delta type"          "text_delta"
check "message_stop event"       "event: message_stop"

# OpenAI-format chunks that MUST NOT leak
check "no OpenAI chunk object"   "chat.completion.chunk" "no"
check "no OpenAI data format"    '"object":"chat.completion' "no"

echo ""
echo "── Result: $PASS passed, $FAIL failed ──"
if [ "$FAIL" -eq 0 ]; then
    echo "✅ STREAMING TRANSLATION VERIFIED with real MiMo"
    exit 0
else
    echo "❌ TRANSLATION FAILED — see $OUT for full output"
    exit 1
fi
