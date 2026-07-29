#!/bin/bash
# scripts/sync-submodules.sh — Sync submodules with upstream + rebase our patches
#
# This script updates each submodule to the latest upstream commit, then
# rebases our 'merged-patches' branch on top of it.
#
# Handles:
#   - Submodules in detached HEAD (common after `git submodule update`)
#   - Missing upstream remote (adds it automatically)
#   - Uncommitted changes (aborts with clear message)
#   - Rebase conflicts (shows resolution instructions)
#
# Prerequisites:
#   - Submodules initialized (git submodule update --init --recursive)
#   - Push access to forks (hooshidev3/...)
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"

log()  { echo -e "\033[1;34m[sync]\033[0m $*"; }
ok()   { echo -e "\033[1;32m[sync] ✓\033[0m $*"; }
err()  { echo -e "\033[1;31m[sync] ✗\033[0m $*" >&2; }

sync_submodule() {
    local name="$1" path="$2" upstream_url="$3"

    log "Syncing $name..."

    # Check if submodule directory exists and is a git repo
    # Use git rev-parse which works for both submodules (.git file) and
    # regular clones (.git directory)
    if ! git -C "$DIR/$path" rev-parse --git-dir >/dev/null 2>&1; then
        err "$name at '$path' is not a git repository."
        err "Run: git submodule update --init --recursive"
        return 1
    fi

    cd "$DIR/$path"

    # Submodules are often in detached HEAD after `git submodule update`.
    # Switch to merged-patches branch first.
    local branch
    branch=$(git rev-parse --abbrev-ref HEAD)
    if [ "$branch" != "merged-patches" ]; then
        if [ "$branch" = "HEAD" ]; then
            log "$name is in detached HEAD (normal after submodule init). Switching to merged-patches..."
        else
            log "$name is on '$branch', switching to merged-patches..."
        fi
        if ! git checkout merged-patches 2>/dev/null; then
            err "Cannot checkout merged-patches in $name."
            err "The branch may not exist. Run:"
            err "  cd $path && git fetch origin && git checkout merged-patches"
            cd "$DIR"
            return 1
        fi
        ok "Switched to merged-patches"
    fi

    # Check working tree is clean
    if ! git diff --quiet || ! git diff --cached --quiet; then
        err "$name has uncommitted changes. Commit or stash first:"
        err "  cd $path && git status"
        cd "$DIR"
        return 1
    fi

    # Add upstream remote if missing
    if ! git remote get-url upstream >/dev/null 2>&1; then
        git remote add upstream "$upstream_url"
        log "Added upstream remote: $upstream_url"
    fi

    # Fetch upstream
    log "Fetching upstream..."
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
    log "Rebasing $our_commits commit(s) onto $new_commits new upstream commit(s)..."

    # Rebase
    if git rebase upstream/main; then
        ok "$name rebase successful"
    else
        err "$name rebase conflict! Resolve manually:"
        err ""
        err "  cd $path"
        err "  # Edit conflicted files (KEEP OUR PATCHES!)"
        err "  git add <files>"
        err "  git rebase --continue"
        err "  git push --force-with-lease origin merged-patches"
        err ""
        err "  To abort: git rebase --abort"
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
