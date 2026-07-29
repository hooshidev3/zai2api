.PHONY: all build test vet run clean sync submodule-status smoke clone-init

all: build

# ─── Clone with submodules (first-time setup) ───────────────────────
clone-init:
	git submodule update --init --recursive

# ─── Submodule status ───────────────────────────────────────────────
submodule-status:
	@git submodule status
	@echo ""
	@echo "GLM branch:  $$(cd glm && git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'N/A')"
	@echo "MiMo branch: $$(cd mimo && git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'N/A')"

# ─── Sync upstream (rebase our patches on latest upstream) ──────────
sync:
	@bash scripts/sync-submodules.sh

# ─── Build ──────────────────────────────────────────────────────────
build:
	@if [ ! -f glm/go.mod ] || [ ! -f mimo/go.mod ]; then \
		echo "⚠️  Submodules missing. Running: git submodule update --init --recursive"; \
		git submodule update --init --recursive; \
	fi
	go build -o bin/zai2api ./cmd/server

# ─── Run ────────────────────────────────────────────────────────────
run: build
	./bin/zai2api

run-dev: build
	GATEWAY_TOKEN=dev-token VERBOSE=1 ./bin/zai2api

# ─── Test ───────────────────────────────────────────────────────────
test:
	go test ./gateway/...
	cd mimo && go test ./pkg/authctx/... ./pkg/services/...
	cd glm && go test ./... 2>/dev/null || true

# ─── Vet ────────────────────────────────────────────────────────────
vet:
	go vet ./...
	cd mimo && go vet ./...
	cd glm && go vet ./...

# ─── Clean ──────────────────────────────────────────────────────────
clean:
	rm -rf bin/ data/

# ─── Quick smoke test ───────────────────────────────────────────────
smoke: build
	@echo "=== Starting gateway for smoke test ==="
	@GATEWAY_TOKEN=smoke-test setsid ./bin/zai2api > /tmp/zai2api-smoke.log 2>&1 &
	@sleep 2
	@echo "=== Health ===" && curl -s http://localhost:8080/health | python3 -m json.tool
	@echo "=== Auth (wrong token → 401) ===" && curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/v1/models
	@echo "=== Auth (correct token → 200) ===" && curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer smoke-test" http://localhost:8080/v1/models
	@pkill -f zai2api 2>/dev/null; true
