#!/bin/bash
# scripts/sync-glm.sh — sync GLM-Free-API with branch + rebase approach
#
# Instead of patch files (which break with unidiff when upstream changes),
# we maintain a "merged-patches" branch with real git commits.
# Each sync: git fetch + git rebase (git intelligently merges our commits
# on top of new upstream commits).
#
# Usage:
#   bash scripts/sync-glm.sh            # sync (or first-time setup)
#   bash scripts/sync-glm.sh --force    # reset branch to upstream + reapply
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
GLM_DIR="$DIR/glm"
REPO="https://github.com/izaart95-jpg/GLM-Free-API.git"
BRANCH="merged-patches"
FORCE=false

if [ "${1:-}" = "--force" ]; then
    FORCE=true
fi

# ── 1. First-time clone + create branch ────────────────────────────
if [ ! -d "$GLM_DIR/.git" ]; then
    echo "📥 First-time clone of GLM-Free-API..."
    git clone "$REPO" "$GLM_DIR"
    cd "$GLM_DIR"
    git checkout -b "$BRANCH"
    echo "✓ Branch '$BRANCH' created."
    echo ""
    echo "🔧 Applying patches as commits..."
    bash "$DIR/scripts/glm-patches/apply-all.sh"
    echo ""
    echo "✅ Initial setup complete."
    git log --oneline origin/main.."$BRANCH" | sed 's/^/   /'
    exit 0
fi

cd "$GLM_DIR"

# ── 2. Safety: working tree must be clean ──────────────────────────
if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "❌ Working tree not clean in $GLM_DIR"
    echo "   Commit or stash your changes first:"
    echo "     cd $GLM_DIR && git status"
    exit 1
fi

# ── 3. Force mode: reset and reapply ───────────────────────────────
if [ "$FORCE" = "true" ]; then
    echo "⚠️  Force mode: resetting branch to upstream and reapplying patches..."
    git checkout main
    git fetch origin
    git reset --hard origin/main
    git branch -D "$BRANCH" 2>/dev/null || true
    git checkout -b "$BRANCH"
    bash "$DIR/scripts/glm-patches/apply-all.sh"
    echo "✅ Force reapply complete."
    exit 0
fi

# ── 4. Normal sync: fetch + rebase ─────────────────────────────────
echo "📥 Fetching upstream..."
git fetch origin main

LOCAL_BASE=$(git merge-base "$BRANCH" origin/main 2>/dev/null || echo "")
REMOTE_HEAD=$(git rev-parse origin/main)

if [ -n "$LOCAL_BASE" ] && [ "$LOCAL_BASE" = "$REMOTE_HEAD" ]; then
    echo "✅ Already up to date."
    exit 0
fi

# Switch to merged-patches if not already
git checkout "$BRANCH" 2>/dev/null || {
    echo "❌ Branch '$BRANCH' not found. Run with --force or reinit."
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
    echo "   Conflicted files:"
    git diff --name-only --diff-filter=U | sed 's/^/     - /'
    echo ""
    echo "   To resolve:"
    echo "     1. Edit conflicted files"
    echo "     2. git add <files>"
    echo "     3. git rebase --continue"
    echo ""
    echo "   To abort:"
    echo "     cd $GLM_DIR && git rebase --abort"
    exit 1
fi

# ── 5. Post-rebase mechanical fixes ────────────────────────────────
echo "🔧 Running post-rebase mechanical fixes..."
bash "$DIR/scripts/glm-patches/post-rebase.sh"

if ! git diff --quiet; then
    git add -A
    git commit --amend --no-edit --no-verify
    echo "✅ Mechanical fixes amended to last commit."
fi

echo ""
echo "✅ GLM synced successfully."
echo "   Branch: $BRANCH"
echo "   Our patches: $(git log --oneline origin/main..$BRANCH | wc -l) commits"
git log --oneline origin/main.."$BRANCH" | sed 's/^/     /'
