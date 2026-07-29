.PHONY: all build test vet run clean clean-all sync smoke clone-init submodule-status test-spike

all: build

# ─── Clone with submodules (first-time setup) ───────────────────────
clone-init:
	git submodule update --init --recursive

# ─── Submodule status ───────────────────────────────────────────────
submodule-status:
	@git submodule status
	@echo ""
	@echo "GLM branch:  $$(cd glm 2>/dev/null && git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'N/A')"
	@echo "MiMo branch: $$(cd mimo 2>/dev/null && git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'N/A')"

# ─── Sync upstream (rebase our patches on latest upstream) ──────────
sync:
	@bash scripts/sync-submodules.sh

# ─── Submodule check (used as dependency by build/test) ─────────────
submodule-check:
	@if [ ! -f glm/go.mod ] || [ ! -f mimo/go.mod ]; then \
		echo "⚠️  Submodules missing. Run: git submodule update --init --recursive"; \
		exit 1; \
	fi

# ─── Build ──────────────────────────────────────────────────────────
build: submodule-check
	go build -o bin/zai2api ./cmd/server

# ─── Run ────────────────────────────────────────────────────────────
run: build
	./bin/zai2api

run-dev: build
	GATEWAY_TOKEN=dev-token VERBOSE=1 ./bin/zai2api

# ─── Test ───────────────────────────────────────────────────────────
test: submodule-check
	go test ./gateway/...
	cd mimo && go test ./pkg/authctx/... ./pkg/services/...
	cd glm && go test ./... 2>/dev/null || true

# ─── Spike test (proves authctx + per-account proxy works) ──────────
test-spike:
	@cd spike && bash run.sh

# ─── Vet ────────────────────────────────────────────────────────────
vet: submodule-check
	go vet ./...
	cd mimo && go vet ./...
	cd glm && go vet ./...

# ─── Clean (safe — only removes build artifacts) ────────────────────
clean:
	rm -rf bin/

# ─── Clean all (⚠️ removes data/ including DBs — use with caution!) ──
clean-all: clean
	rm -rf data/

# ─── Quick smoke test ───────────────────────────────────────────────
smoke: build
	@echo "=== Starting gateway for smoke test ==="
	@GATEWAY_TOKEN=smoke-test setsid ./bin/zai2api > /tmp/zai2api-smoke.log 2>&1 &
	@sleep 2
	@echo "=== Health ===" && curl -s http://localhost:8080/health | python3 -m json.tool
	@echo "=== Auth (no token → 401) ===" && curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/v1/models
	@echo "=== Auth (correct token → 200) ===" && curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer smoke-test" http://localhost:8080/v1/models
	@echo "=== Models count ===" && curl -s -H "Authorization: Bearer smoke-test" http://localhost:8080/v1/models | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('data',[])))"
	@pkill -f "bin/zai2api" 2>/dev/null; true
