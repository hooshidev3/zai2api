# AGENTS.md — Guide for AI Agents Working on This Project

> This file is for AI agents (Claude, Cursor, Copilot) that work on this project.
> Read this file completely before making any changes.

## 1. Architecture Overview

This project is a unified gateway combining two AI providers:

| Provider | Submodule | Upstream | Our Fork |
|----------|-----------|----------|----------|
| Z.AI (GLM) | `glm/` | izaart95-jpg/GLM-Free-API | hooshidev3/GLM-Free-API |
| Xiaomi MiMo | `mimo/` | hugogadelha/mimo-ai-proxy | hooshidev3/mimo-ai-proxy |

**Key:** `glm/` and `mimo/` are **git submodules** pointing to our forks.
Each has a `merged-patches` branch with our integration patches.

## 2. Critical Rules (Never Violate)

### ❌ Never do these:

1. **Never push to upstream repos** (izaart95-jpg/..., hugogadelha/...).
   We don't have write access. Always push to our forks (hooshidev3/...).

2. **Never commit submodule changes without pushing the submodule first.**
   If you commit in `glm/` or `mimo/`, you MUST push to the fork before
   committing the pointer update in zai2api.

3. **Never change `package glm` back to `package main`** in `glm/` root files.
   Only `glm/cmd/token-collector/main.go` should be `package main`.

4. **Never recreate `internal/` in MiMo.** We renamed it to `pkg/` for
   cross-module import. Go's `internal/` packages can only be imported
   from within the same module.

5. **Never use `context.WithValue` directly for auth.** Always use
   `authctx.WithAuth`/`FromContext`. Direct use causes a silent runtime bug.

5. **Never remove `ResolveClient` calls.** They prevent nil-pointer panics.

### ✅ Always do these:

1. After cloning: `git submodule update --init --recursive`
2. After modifying a submodule: commit + push to fork, then update pointer in zai2api
3. After any change: `make build && make test`
4. Before PR: `make smoke` for quick verification

## 3. Submodule Workflow

### Making changes to glm/ or mimo/

```bash
# 1. Enter the submodule (ensure on correct branch)
cd glm
git checkout merged-patches

# 2. Make changes
# ... edit files ...

# 3. Commit and push to FORK (not upstream!)
git add -A
git commit -m "patch(glm): description of change"
git push origin merged-patches

# 4. Return to parent repo and update the submodule pointer
cd ..
git add glm
git commit -m "chore: update glm submodule"
git push origin main
```

### Syncing with upstream

```bash
# Use the sync script (handles fetch + rebase + push)
make sync

# Or manually:
cd glm
git fetch upstream main
git rebase upstream/main
# Resolve conflicts if any (KEEP OUR PATCHES!)
git push --force-with-lease origin merged-patches
cd ..
git add glm
git commit -m "chore: update glm to latest upstream"
```

## 4. Our Patches

### In `glm/` (branch: merged-patches)

| Commit | Change | Why |
|--------|--------|-----|
| 1 | `package main` → `package glm` | Importable as library |
| 2 | Removed `func main()` and `flag` import | Gateway provides entrypoint |
| 3 | Moved `captcha.go` → `cmd/token-collector/main.go` | Separate module for heavy deps |
| 4 | Added `go.mod` (module glm-free-api) | Module definition |
| 5 | Added `provider.go` (Provider struct) | Multi-account support scaffold |
| 6 | Added `exports.go` (public handler wrappers) | Gateway can mount handlers |

### In `mimo/` (branch: merged-patches)

| Commit | Change | Why |
|--------|--------|-----|
| 1 | `internal/` → `pkg/` + updated imports | Cross-module import (Go restriction) |
| 2 | Removed `main.go` | Gateway provides entrypoint |
| 3 | Added `pkg/authctx/` (leaf package) | Breaks import cycle, shared context key identity |
| 4 | Added `GetAuthFromContext` in services | Read auth+client from request context |
| 5 | Added `ResolveClient` (exported) | Prevents nil panic, used by routes too |
| 6 | Threaded client in 9 functions | Per-account proxy support |

## 5. The `authctx` Package — Heart of the Architecture

`mimo/pkg/authctx/` is a leaf package (no dependencies) that solves the import cycle:

```
gateway → mimoproxy/pkg/services → mimoproxy/pkg/authctx
   │                                         ▲
   └─────────────────────────────────────────┘
        (no edge back to gateway)
```

**Contract:** Always use `authctx.WithAuth` / `authctx.FromContext`, never
`context.WithValue` directly.

## 6. Testing

```bash
# Full project test
make test

# Authctx tests (most critical)
cd mimo && go test -v ./pkg/authctx/...

# Build verification
make build

# Smoke test (starts gateway, tests health + auth)
make smoke
```

## 7. Dispatch Logic

```
POST /v1/chat/completions
  → read body, peek "model" field (json.Unmarshal)
  → routeByModel:
      glm-* / zai-*  → glm.Provider.ChatCompletionsHandler
      mimo*          → forwardToMiMo (strips /mimo prefix, injects authctx)
      (default)      → GLM
```

## 8. Common Issues

| Issue | Solution |
|-------|----------|
| `go build` fails with `directory not found` | `git submodule update --init --recursive` |
| Submodule in detached HEAD | `cd glm && git checkout merged-patches` |
| Changes in submodule lost | Always commit + push BEFORE `git add` in parent |
| `import cycle not allowed` | Check that authctx doesn't import gateway |
| `use of internal package` | Ensure MiMo uses `pkg/` not `internal/` |
| `nil pointer dereference` in HTTP client | Check `ResolveClient` is called |
| Cannot push to upstream | Push to fork (hooshidev3/...), not upstream |

## 9. Pre-PR Checklist

- [ ] Submodules on `merged-patches` branch
- [ ] Submodule changes committed and pushed to forks
- [ ] Submodule pointers updated in zai2api (`git add glm mimo`)
- [ ] `make build` succeeds
- [ ] `make test` succeeds
- [ ] `make smoke` passes
- [ ] No token/secret in committed code
