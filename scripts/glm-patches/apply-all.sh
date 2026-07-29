#!/bin/bash
# scripts/glm-patches/apply-all.sh
# Apply all GLM patches as separate git commits (first-time setup).
set -euo pipefail

DIR="$(cd "$(dirname "$0")/../../" && pwd)"
GLM_DIR="$DIR/glm"
PATCHES_DIR="$(cd "$(dirname "$0")" && pwd)"

cd "$GLM_DIR"

# ─── Commit 1: rename package main → glm, remove func main() ───────
echo "📦 Commit 1: rename package main → glm + remove func main() ..."

# Rename package main → glm in main.go
sed -i 's|^package main$|package glm|' main.go

# Remove func main() from main.go using Python (handles nested braces)
python3 "$PATCHES_DIR/remove-func-main.py" main.go

# Move captcha.go to cmd/token-collector/main.go (keeps package main for separate binary)
mkdir -p cmd/token-collector
if [ -f captcha.go ]; then
    git mv captcha.go cmd/token-collector/main.go
fi

# Create go.mod for the main module (lightweight — only sqlite)
cat > go.mod <<'EOF'
module glm-free-api

go 1.25.0

require modernc.org/sqlite v1.34.5
EOF

# Create separate go.mod for cmd/token-collector (heavy deps: playwright, bubbletea)
cat > cmd/token-collector/go.mod <<'EOF'
module glm-free-api/cmd/token-collector

go 1.25.0

require (
	github.com/charmbracelet/bubbletea v1.3.4
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/mxschmitt/playwright-go v0.0.0-20240819155338-3c5eb7b16b0b
	modernc.org/sqlite v1.34.5
)
EOF

git add -A
git commit -m "patch(glm): rename package main → glm, move captcha.go to cmd/

- Renamed package main → glm in main.go (so gateway can import it)
- Removed func main() and CLI flag parsing (gateway provides main)
- Moved captcha.go to cmd/token-collector/main.go (separate binary,
  keeps package main for standalone use)
- Added go.mod for main module (only modernc.org/sqlite — lightweight)
- Added separate go.mod for cmd/token-collector (heavy deps: playwright,
  bubbletea — kept out of main module so gateway binary stays small)

The captcha collector is a separate tool that harvests Z.AI device
tokens. It is built and run independently:
  cd cmd/token-collector && go build -o token-collector ." --no-verify

echo "✓ Commit 1 done"

# ─── Commit 2: add Provider struct + per-account support ───────────
echo "📦 Commit 2: add Provider struct + multi-account stub ..."

# Add provider.go with Provider struct
cp "$PATCHES_DIR/templates/provider.go.tmpl" provider.go

# Add exports.go with public API
cp "$PATCHES_DIR/templates/exports.go.tmpl" exports.go

git add -A
git commit -m "patch(glm): add Provider struct and exports for gateway integration

- Added provider.go with Provider struct (holds config, db, http clients)
- Added exports.go with NewProvider(), Close(), and public handler wrappers
  (ChatCompletionsHandler, AnthropicMessagesHandler, ModelsHandler,
  DashboardHandler, StatsHandler) that gateway can mount

The Provider struct encapsulates state that was previously global
(session, config, db). This enables multi-account support in gateway:
gateway creates one Provider per Z.AI account and dispatches requests." --no-verify

echo "✓ Commit 2 done"

echo ""
echo "✅ All GLM patches applied as 2 commits."
echo "   Note: per-account client threading (zaiHTTPClient → per-account)
   will be done in a follow-up commit during Phase 3 implementation.
   For now, Provider uses the existing global zaiHTTPClient which is
   fine for single-account operation."
