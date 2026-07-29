#!/bin/bash
# scripts/glm-patches/post-rebase.sh
# Mechanical fixes after rebase.
set -euo pipefail

GLM_DIR="$(cd "$(dirname "$0")/../../glm" && pwd)"
PATCHES_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$GLM_DIR"

echo "🔧 Post-rebase mechanical fixes for GLM..."

# 1. Ensure all .go files (except cmd/) use package glm
find . -name '*.go' -not -path './cmd/*' -not -path './.git/*' \
    -exec grep -l '^package main$' {} \; | while read -r f; do
    echo "  - Fixing package in $f"
    sed -i 's/^package main$/package glm/' "$f"
done

# 2. Ensure captcha.go is in cmd/token-collector/ (in case upstream re-added it)
if [ -f "captcha.go" ]; then
    echo "  - Moving captcha.go to cmd/token-collector/main.go"
    mkdir -p cmd/token-collector
    mv captcha.go cmd/token-collector/main.go
fi

# 3. Ensure func main() is not in main.go (in case upstream changed it)
# We can't easily remove it again, so check and warn
if grep -q '^func main()' main.go 2>/dev/null; then
    echo "  - ⚠️  func main() found in main.go after rebase — needs manual review"
    echo "    Run: python3 $PATCHES_DIR/remove-func-main.py main.go"
fi

# 4. Ensure go.mod exists
if [ ! -f go.mod ]; then
    echo "  - Restoring go.mod"
    cat > go.mod <<'EOF'
module glm-free-api

go 1.25.0

require modernc.org/sqlite v1.34.5
EOF
fi

# 5. Ensure provider.go and exports.go exist
if [ ! -f provider.go ]; then
    echo "  - Restoring provider.go"
    cp "$PATCHES_DIR/templates/provider.go.tmpl" provider.go
fi
if [ ! -f exports.go ]; then
    echo "  - Restoring exports.go"
    cp "$PATCHES_DIR/templates/exports.go.tmpl" exports.go
fi

echo "✓ Post-rebase fixes done."
