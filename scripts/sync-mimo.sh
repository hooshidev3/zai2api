#!/bin/bash
# scripts/sync-mimo.sh — sync mimo-ai-proxy with branch + rebase approach
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
MIMO_DIR="$DIR/mimo"
REPO="https://github.com/hugogadelha/mimo-ai-proxy.git"
BRANCH="merged-patches"
FORCE=false

if [ "${1:-}" = "--force" ]; then
    FORCE=true
fi

# ── 1. First-time clone + create branch ────────────────────────────
if [ ! -d "$MIMO_DIR/.git" ]; then
    echo "📥 First-time clone of mimo-ai-proxy..."
    git clone "$REPO" "$MIMO_DIR"
    cd "$MIMO_DIR"
    git checkout -b "$BRANCH"
    echo "✓ Branch '$BRANCH' created."
    echo ""
    echo "🔧 Applying patches as commits..."
    bash "$DIR/scripts/mimo-patches/apply-all.sh"
    echo ""
    echo "✅ Initial setup complete."
    git log --oneline origin/main.."$BRANCH" | sed 's/^/   /'
    exit 0
fi

cd "$MIMO_DIR"

if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "❌ Working tree not clean in $MIMO_DIR"
    exit 1
fi

if [ "$FORCE" = "true" ]; then
    echo "⚠️  Force mode: resetting and reapplying..."
    git checkout main
    git fetch origin
    git reset --hard origin/main
    git branch -D "$BRANCH" 2>/dev/null || true
    git checkout -b "$BRANCH"
    bash "$DIR/scripts/mimo-patches/apply-all.sh"
    echo "✅ Force reapply complete."
    exit 0
fi

echo "📥 Fetching upstream..."
git fetch origin main

LOCAL_BASE=$(git merge-base "$BRANCH" origin/main 2>/dev/null || echo "")
REMOTE_HEAD=$(git rev-parse origin/main)

if [ -n "$LOCAL_BASE" ] && [ "$LOCAL_BASE" = "$REMOTE_HEAD" ]; then
    echo "✅ Already up to date."
    exit 0
fi

git checkout "$BRANCH" 2>/dev/null || {
    echo "❌ Branch '$BRANCH' not found. Run with --force."
    exit 1
}

OUR_COMMITS=$(git log --oneline origin/main.."$BRANCH" 2>/dev/null | wc -l)
NEW_COMMITS=$(git rev-list --count "$BRANCH"..origin/main 2>/dev/null || echo "0")
echo "🔄 Rebasing our $OUR_COMMITS commits onto $NEW_COMMITS new upstream commits..."

if git rebase origin/main; then
    echo "✅ Rebase successful."
else
    echo ""
    echo "⚠️  Rebase conflict detected!"
    git diff --name-only --diff-filter=U | sed 's/^/     - /'
    echo ""
    echo "   To resolve:"
    echo "     1. Edit conflicted files"
    echo "     2. git add <files>"
    echo "     3. git rebase --continue"
    exit 1
fi

echo "🔧 Running post-rebase mechanical fixes..."
bash "$DIR/scripts/mimo-patches/post-rebase.sh"

if ! git diff --quiet; then
    git add -A
    git commit --amend --no-edit --no-verify
    echo "✅ Mechanical fixes amended to last commit."
fi

echo ""
echo "✅ MiMo synced successfully."
echo "   Branch: $BRANCH"
echo "   Our patches: $(git log --oneline origin/main..$BRANCH | wc -l) commits"
git log --oneline origin/main.."$BRANCH" | sed 's/^/     /'
