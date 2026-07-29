#!/bin/bash
# scripts/sync-submodules.sh — Sync submodules with upstream + rebase our patches
#
# This script updates each submodule to the latest upstream commit, then
# rebases our 'merged-patches' branch on top of it.
#
# Prerequisites:
#   - Each submodule has 'origin' pointing to our fork (hooshidev3/...)
#   - Each submodule has 'upstream' pointing to the original repo
#   - You have push access to the forks
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"

log()  { echo -e "\033[1;34m[sync]\033[0m $*"; }
ok()   { echo -e "\033[1;32m[sync] ✓\033[0m $*"; }
err()  { echo -e "\033[1;31m[sync] ✗\033[0m $*" >&2; }

sync_submodule() {
    local name="$1" path="$2" upstream_url="$3"

    log "Syncing $name..."
    cd "$DIR/$path"

    # Verify we're on the right branch
    local branch
    branch=$(git rev-parse --abbrev-ref HEAD)
    if [ "$branch" != "merged-patches" ]; then
        err "$name is on '$branch', expected 'merged-patches'"
        err "Run: cd $path && git checkout merged-patches"
        return 1
    fi

    # Check working tree is clean
    if ! git diff --quiet || ! git diff --cached --quiet; then
        err "$name has uncommitted changes. Commit or stash first."
        return 1
    fi

    # Add upstream remote if missing
    if ! git remote get-url upstream >/dev/null 2>&1; then
        git remote add upstream "$upstream_url"
        log "Added upstream remote: $upstream_url"
    fi

    # Fetch upstream
    git fetch upstream main

    local base head
    base=$(git merge-base merged-patches upstream/main)
    head=$(git rev-parse upstream/main)

    if [ "$base" = "$head" ]; then
        ok "$name already up to date"
        cd "$DIR"
        return 0
    fi

    local our_commits new_commits
    our_commits=$(git log --oneline upstream/main..merged-patches 2>/dev/null | wc -l)
    new_commits=$(git rev-list --count merged-patches..upstream/main 2>/dev/null || echo "0")
    log "Rebasing $our_commits commits onto $new_commits new upstream commits..."

    # Rebase
    if git rebase upstream/main; then
        ok "$name rebase successful"
    else
        err "$name rebase conflict! Resolve manually:"
        err "  cd $path"
        err "  # edit conflicted files (KEEP OUR PATCHES!)"
        err "  git add <files> && git rebase --continue"
        err "  git push --force-with-lease origin merged-patches"
        cd "$DIR"
        return 1
    fi

    # Push to fork (force-with-lease because rebase rewrites history)
    log "Pushing $name to fork..."
    git push --force-with-lease origin merged-patches

    cd "$DIR"

    # Update submodule pointer in parent repo
    git add "$path"
    ok "$name synced"
}

# Sync both submodules
sync_submodule "GLM"  "glm"  "https://github.com/izaart95-jpg/GLM-Free-API.git"
sync_submodule "MiMo" "mimo" "https://github.com/hugogadelha/mimo-ai-proxy.git"

# Commit submodule pointer updates in parent repo
if ! git diff --cached --quiet; then
    git commit -m "chore: update submodules to latest upstream"
    ok "Submodule updates committed"
fi

echo ""
ok "All submodules synced"
echo ""
echo "Next steps:"
echo "  git push origin main  # push submodule pointer update"
