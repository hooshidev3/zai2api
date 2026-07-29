#!/bin/bash
# scripts/mimo-patches/post-rebase.sh
# Mechanical fixes that must run after every rebase (because upstream
# may have added new files that need the same treatment).
set -euo pipefail

MIMO_DIR="$(cd "$(dirname "$0")/../../mimo" && pwd)"
cd "$MIMO_DIR"

echo "🔧 Post-rebase mechanical fixes for MiMo..."

# 1. Ensure no main.go exists (gateway provides main)
if [ -f "main.go" ]; then
    echo "  - Removing main.go (gateway provides its own)"
    rm -f main.go
fi

# 2. Ensure internal/ is renamed to pkg/ (in case upstream added new files)
if [ -d "internal" ]; then
    echo "  - Renaming internal/ → pkg/ (upstream may have added new files)"
    # Move any new files from internal/ to pkg/
    if [ ! -d "pkg" ]; then
        mkdir -p pkg
    fi
    # Copy new files (don't overwrite existing pkg/ files)
    cd internal
    for item in *; do
        if [ -e "../pkg/$item" ]; then
            # Merge: copy new files into existing pkg/ subdir
            cp -rn "$item" "../pkg/" 2>/dev/null || true
        else
            mv "$item" "../pkg/"
        fi
    done
    cd ..
    rm -rf internal
fi

# 3. Update all import paths (catches any new files using old paths)
echo "  - Updating import paths mimoproxy/internal/ → mimoproxy/pkg/"
find . -name '*.go' -not -path './.git/*' \
    -exec sed -i 's|mimoproxy/internal/|mimoproxy/pkg/|g' {} +

# 4. Ensure rand.Seed deprecation is removed (in case upstream re-added it)
sed -i '/rand\.Seed(time\.Now().UnixNano())/d' pkg/services/mimo.go 2>/dev/null || true

# 5. Ensure authctx package exists (in case rebase removed it)
if [ ! -f "pkg/authctx/authctx.go" ]; then
    PATCHES_DIR="$(cd "$(dirname "$0")" && pwd)"
    mkdir -p pkg/authctx
    cp "$PATCHES_DIR/templates/authctx.go.tmpl" pkg/authctx/authctx.go
    cp "$PATCHES_DIR/templates/authctx_test.go.tmpl" pkg/authctx/authctx_test.go
    echo "  - Restored authctx package (was missing after rebase)"
fi

echo "✓ Post-rebase fixes done."
