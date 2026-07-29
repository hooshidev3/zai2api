.PHONY: sync build run test vet clean docker

# Default target
all: build

# ─── Sync upstream repos (clone + apply patches as commits) ─────────
sync:
	@bash scripts/sync-glm.sh
	@bash scripts/sync-mimo.sh

# Force re-sync (reset branch and reapply patches)
sync-force:
	@bash scripts/sync-glm.sh --force
	@bash scripts/sync-mimo.sh --force

# ─── Build ──────────────────────────────────────────────────────────
build:
	go build -o bin/zai2api ./cmd/server

build-glm:
	cd glm && go build ./...

build-mimo:
	cd mimo && go build ./...

# ─── Run ────────────────────────────────────────────────────────────
run: build
	./bin/zai2api

# Run with specific env
run-dev: build
	GATEWAY_TOKEN=dev-token VERBOSE=1 ./bin/zai2api

# ─── Test ───────────────────────────────────────────────────────────
test:
	go test ./gateway/...
	cd mimo && go test ./...
	cd glm && go test ./...

test-spike:
	cd spike && bash run.sh || true

# ─── Vet ────────────────────────────────────────────────────────────
vet:
	go vet ./...
	cd mimo && go vet ./...
	cd glm && go vet ./...

# ─── Clean ──────────────────────────────────────────────────────────
clean:
	rm -rf bin/ data/
	cd mimo && go clean -cache 2>/dev/null || true
	cd glm && go clean -cache 2>/dev/null || true

# ─── Docker ─────────────────────────────────────────────────────────
docker-build:
	docker build -t zai2api .

docker-run:
	docker run -p 8080:8080 -v $(PWD)/data:/app/data zai2api

# ─── Git helpers ────────────────────────────────────────────────────
git-log-glm:
	cd glm && git log --oneline origin/main..merged-patches

git-log-mimo:
	cd mimo && git log --oneline origin/main..merged-patches

# Show what our patches changed vs upstream
diff-glm:
	cd glm && git diff origin/main..merged-patches --stat

diff-mimo:
	cd mimo && git diff origin/main..merged-patches --stat
