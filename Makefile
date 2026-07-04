-include .env
export

GOOS ?= linux
GOARCH ?= amd64

DEV_COMPOSE := docker compose -f deploy/compose/control-plane.dev.yml

.PHONY: dev dev-api dev-web test test-go test-smoke build build-local build-cli install-cli clean up up-build up-rebuild up-api down logs release

# ── Development ──────────────────────────────────────────────

dev: ## Start backend + frontend
	@echo "Starting control-plane + admin frontend..."
	@echo "  API  → http://$(CONTROL_PLANE_ADDR)"
	@echo "  Web  → http://localhost:5173"
	@echo "  Agent → embedded (in-process)"
	@echo ""
	@mkdir -p .data
	@# managed-user 镜像内置 sing-box，非 Linux host 直接跑无需 build。
	@trap 'kill $$CP_PID $$VITE_PID 2>/dev/null; wait' INT EXIT; \
		bash scripts/dev-backend.sh & CP_PID=$$!; \
		cd web/admin && pnpm dev & VITE_PID=$$!; \
		wait

dev-api: ## Start backend only with hot reload
	@mkdir -p .data
	@bash scripts/dev-backend.sh

dev-web: ## Start frontend only
	cd web/admin && pnpm dev

dev-all: dev ## Alias for 'make dev'

# ── Testing ──────────────────────────────────────────────────# ── Testing ──────────────────────────────────────────────────

test: test-go test-smoke ## Run all tests

test-go: ## Run Go tests (Phase 51 QUAL-07: -race -shuffle=on 默认；tests/e2e 跑 docker，不带 race)
	go test $$(go list ./... | grep -v '/tests/e2e$$') -race -shuffle=on -count=1
	go test ./tests/e2e/... -count=1

test-smoke: ## Run BATS bootstrap smoke tests
	pnpm exec bats tests/smoke/

e2e: ## Run e2e test suite (v4.0 Phase 56: lint + vet + test)
	@echo "=== e2e: lint-no-bare-sleep ==="
	bash scripts/lint-no-bare-sleep.sh tests/e2e/
	@echo "=== e2e: go vet ==="
	go vet -tags=e2e ./tests/e2e/...
	@echo "=== e2e: go test ==="
	go test -tags=e2e ./tests/e2e/... -count=1 -v -timeout=15m

phase53-smoke: ## Run Phase 53 image smoke tests (requires managed-user:v4-dev)
	bash tests/phase53/smoke.sh

# ── Images ────────────────────────────────────────────────────

MANAGED_USER_TAG := $(shell grep '^image_name:' deploy/docker/managed-user/image.lock | cut -d' ' -f2)

user-image: ## Build managed-user image (cloud desktop)
	docker build -t $(MANAGED_USER_TAG) -f deploy/docker/managed-user/Dockerfile .
	@echo "Built: $(MANAGED_USER_TAG)"

# ── Build ────────────────────────────────────────────────────

build: ## Build all artifacts for target platform
	cd web/admin && pnpm build && cp -r dist ../../cmd/control-plane/dist
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o bin/control-plane-$(GOOS)-$(GOARCH) ./cmd/control-plane
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o bin/host-agent-$(GOOS)-$(GOARCH) ./cmd/host-agent
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags "-s -w" -trimpath -o bin/claudedock-$(GOOS)-$(GOARCH) ./cmd/claudedock

build-local: ## Build for current platform
	cd web/admin && pnpm build && cp -r dist ../../cmd/control-plane/dist
	go build -o bin/control-plane ./cmd/control-plane
	go build -o bin/host-agent ./cmd/host-agent
	go build -ldflags "-s -w" -trimpath -o bin/claudedock ./cmd/claudedock

build-api: ## Build Go backend only
	@test -d cmd/control-plane/dist || (echo "dist not found, run 'cd web/admin && pnpm build && cp -r dist ../../cmd/control-plane/dist'" && exit 1)
	go build -o bin/control-plane ./cmd/control-plane

build-cli: ## Build claudedock CLI
	go build -ldflags "-s -w" -trimpath -o bin/claudedock ./cmd/claudedock

build-web: ## Build frontend only
	cd web/admin && pnpm build

install-cli: ## Install claudedock to /usr/local/bin
	go build -ldflags "-s -w" -trimpath -o /usr/local/bin/claudedock ./cmd/claudedock

# ── Production ───────────────────────────────────────────────

up: ## Start production stack (prefer prebuilt latest images)
	docker compose pull --policy always
	docker compose up -d

up-build: ## Start production stack from local source build
	docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build
	docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate

up-rebuild: ## Rebuild from scratch (no cache) and start
	docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
	docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate

up-api: ## Rebuild and restart control-plane only (fastest for backend changes)
	docker compose -f docker-compose.yml -f docker-compose.build.yaml build control-plane
	docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate --no-deps control-plane

down: ## Stop production stack
	docker compose down

logs: ## Tail production logs
	docker compose logs -f

release: ## Create and push release tag (usage make release VERSION=1.5.0)
	@test -n "$(VERSION)" || (echo "VERSION is required. Example: make release VERSION=1.5.0" && exit 1)
	@test -z "$$(git status --porcelain)" || (echo "Working tree is dirty. Commit/stash changes before release." && exit 1)
	git tag v$(VERSION)
	git push origin v$(VERSION)

# ── Setup ────────────────────────────────────────────────────

setup: ## First-time setup: install deps, copy .env
	cd web/admin && pnpm install
	@test -f .env || cp .env.example .env
	@echo "Done. Edit .env if needed, then run: make dev"
	@echo ""
	@echo "常用命令:"
	@echo "  make dev         一键启动后端 + 前端（推荐）"
	@echo "  make dev-api     只启动后端"
	@echo "  make dev-web     只启动前端"
# ── Utilities ────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf bin/ web/admin/dist/

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' Makefile | sed 's/:.*## /: /' | awk 'BEGIN {FS = ": "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

# ── Phase 34 Plan 03: claudedock doctor M14 终验闸门 ────────────────
# (ROADMAP §Phase 34 SC#3 / Plan 03 Task 3.7)

.PHONY: claudedock
claudedock: ## Build claudedock binary at repo root (used by ci-doctor-grep)
	go build -o ./claudedock ./cmd/claudedock

.PHONY: ci-doctor-grep
ci-doctor-grep: claudedock ## Run scripts/ci-doctor-grep.sh against built claudedock
	bash scripts/ci-doctor-grep.sh ./claudedock

.PHONY: ci-gate
ci-gate: ## CI gate: short go test + ci-doctor-grep + uat dry-run (Phase 51 QUAL-07: -race -shuffle=on)
	go test $$(go list ./... | grep -v '/tests/e2e$$') -race -shuffle=on -count=1 -short
	go test ./tests/e2e/... -count=1 -short
	$(MAKE) ci-doctor-grep
	bash tests/scripts/uat-v31-promotion.sh --dry-run
