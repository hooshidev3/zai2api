#!/bin/bash
# scripts/mimo-patches/apply-all.sh
# Apply all MiMo patches as separate git commits (first-time setup).
# Each commit is self-contained and compiles independently.
set -euo pipefail

DIR="$(cd "$(dirname "$0")/../../" && pwd)"
MIMO_DIR="$DIR/mimo"
PATCHES_DIR="$(cd "$(dirname "$0")" && pwd)"

cd "$MIMO_DIR"

# ─── Commit 1: rename internal/ → pkg/, update imports, remove main.go ──
echo "📦 Commit 1: rename internal/ → pkg/ ..."

# Remove main.go (gateway provides its own)
git rm -f main.go 2>/dev/null || rm -f main.go

# Rename internal → pkg (only if internal exists)
if [ -d "internal" ]; then
    if [ -d "pkg" ]; then
        rm -rf pkg
    fi
    git mv internal pkg
fi

# Update all import paths mimoproxy/internal/ → mimoproxy/pkg/
find . -name '*.go' -not -path './.git/*' \
    -exec sed -i 's|mimoproxy/internal/|mimoproxy/pkg/|g' {} +

# Remove deprecated rand.Seed (Go 1.20+)
sed -i '/rand\.Seed(time\.Now().UnixNano())/d' pkg/services/mimo.go 2>/dev/null || true

git add -A
git commit -m "patch(mimo): rename internal/ → pkg/ for cross-module import

- Renamed internal/ to pkg/ so gateway module can import mimoproxy/pkg/*
- Updated all import paths from mimoproxy/internal/ to mimoproxy/pkg/
- Removed main.go (gateway provides its own main)
- Removed deprecated rand.Seed call (Go 1.20+)

This is required because Go's internal/ packages can only be imported
from within the same module. With replace directive in gateway/go.mod,
we need pkg/ to allow cross-module imports." --no-verify

echo "✓ Commit 1 done"

# ─── Commit 2: add authctx package + GetAuthFromContext ─────────────
echo "📦 Commit 2: add authctx package ..."

mkdir -p pkg/authctx
cp "$PATCHES_DIR/templates/authctx.go.tmpl" pkg/authctx/authctx.go
cp "$PATCHES_DIR/templates/authctx_test.go.tmpl" pkg/authctx/authctx_test.go

# Add GetAuthFromContext to services/mimo.go
python3 "$PATCHES_DIR/add-getauthfromcontext.py" pkg/services/mimo.go

git add -A
git commit -m "patch(mimo): add authctx package for context-based auth injection

- Added pkg/authctx/ leaf package (no external dependencies)
- Added GetAuthFromContext() to services that reads auth+client from
  request context, with fallback to env-based GetSelectedAuth()
- Added TestDirectContextWithValueDoesNotWork as safeguard against
  silent key-identity bugs

The authctx package is shared between gateway (which writes auth into
context) and services (which reads it). Because it is a leaf package
with no dependencies, no import cycle is created." --no-verify

echo "✓ Commit 2 done"

# ─── Commit 3: thread per-account HTTP client with ResolveClient ────
echo "📦 Commit 3: thread per-account HTTP client ..."

python3 "$PATCHES_DIR/thread-client.py" pkg/services/mimo.go pkg/routes/chat.go

git add -A
git commit -m "patch(mimo): thread per-account HTTP client through call chain

- Added exported ResolveClient() helper (returns non-nil client always,
  prevents nil-pointer panics)
- Changed signatures of HandleMimoChat, UploadToXiaomi,
  GetConversationHistory, CreateConversation to accept *http.Client
- Changed signatures of processStream, processNonStream,
  processAutoUploads, handleModels, handleDirectProxy to accept *http.Client
- Replaced GlobalHTTPClient.Do with client.Do in all 9 functions
- handleChatCompletions now reads client from request context and
  threads it through all downstream calls

This enables per-account proxy: gateway injects a per-account HTTP
client (with proxy) into request context, MiMo reads it and uses it
for all upstream calls. Without this, per-account proxy would be
silently broken (client read from context but discarded)." --no-verify

echo "✓ Commit 3 done"

echo ""
echo "✅ All MiMo patches applied as 3 commits."
