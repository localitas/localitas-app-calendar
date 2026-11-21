.PHONY: build test check-core install uninstall start stop restart status logs logs-err \
       build-docker start-docker stop-docker restart-docker logs-docker \
       docker-push lint strict-lint swagger

APP_NAME := calendar
PORT ?= 9203
BASE_PATH ?= /apps/ext/calendar/
PLIST_NAME := com.localitas.app.calendar
PLIST_FILE := $(HOME)/Library/LaunchAgents/$(PLIST_NAME).plist
LOG_DIR := $(HOME)/.localitas/logs/calendar
BIN_PATH := $(shell pwd)/bin/calendar-server
WORK_DIR := $(shell pwd)
# The home of the core this app reaches (its core.env: CORE_URL and the API
# token, written by core each time it serves). Another core: LOCALITAS_HOME=<dir>.
LOCALITAS_HOME ?= $(HOME)/.localitas
# The Docker daemon's architecture: the image's base is pulled for it, so the
# Linux binary is built for it too and runs natively, not under emulation.
LINUX_ARCH ?= $(strip $(shell docker version --format '{{.Server.Arch}}' 2>/dev/null || go env GOARCH))

# ── Build & Test ──────────────────────────────────────────────

build: lint
	@mkdir -p bin
	go build -o bin/calendar-server ./cmd/calendar-server

build-linux: lint
	GOOS=linux GOARCH=$(LINUX_ARCH) CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath \
		-o calendar-server-linux-$(LINUX_ARCH) ./cmd/calendar-server

test: lint
	CGO_ENABLED=1 go test -tags fts5 -v ./...

lint:
	@echo "Running gofmt..."
	@gofmt -w .
	@echo "Running go vet..."
	@go vet ./...

strict-lint: lint
	@echo "Running staticcheck..."
	@go tool staticcheck ./...
	@echo "staticcheck passed"

swagger:
	@curl -s http://localhost:$(PORT)/swagger.json | python3 -m json.tool

# ── Native (launchd) ─────────────────────────────────────────

check-core:
	@test -n "$(CORE_URL)" -o -f "$(LOCALITAS_HOME)/core.env" || { echo "No core: $(LOCALITAS_HOME)/core.env is missing. Start core, or set LOCALITAS_HOME to the home of the core to reach."; exit 1; }

install: build check-core
	@mkdir -p $(LOG_DIR)
	@sed 's|$${BIN_PATH}|$(BIN_PATH)|g; s|$${PORT}|$(PORT)|g; s|$${BASE_PATH}|$(BASE_PATH)|g; s|$${LOG_DIR}|$(LOG_DIR)|g; s|$${LOCALITAS_HOME}|$(LOCALITAS_HOME)|g; s|$${WORK_DIR}|$(WORK_DIR)|g' \
		plist.template > $(PLIST_FILE)
	@echo "Installed launchd service: $(PLIST_NAME)"

uninstall: stop
	@rm -f $(PLIST_FILE)
	@echo "Uninstalled launchd service: $(PLIST_NAME)"

start: install
	@launchctl load $(PLIST_FILE) 2>/dev/null || true
	@echo "Started $(PLIST_NAME) on port $(PORT)"

stop:
	@launchctl unload $(PLIST_FILE) 2>/dev/null || true
	@echo "Stopped $(PLIST_NAME)"

restart: stop start

status:
	@launchctl list | grep $(PLIST_NAME) || echo "$(PLIST_NAME) is not running"

logs:
	@tail -f $(LOG_DIR)/stdout.log

logs-err:
	@tail -f $(LOG_DIR)/stderr.log

# ── Docker ────────────────────────────────────────────────────

build-docker: build-linux
	docker build --platform linux/$(LINUX_ARCH) -t calendar:latest .

start-docker: build-docker stop-docker
	@docker run -d -p 127.0.0.1:$(PORT):8000 --name $(APP_NAME) \
		--log-opt max-size=10m --log-opt max-file=7 \
		--label localitas.app=true \
		-e LOCALITAS_API_TOKEN \
		--add-host host.docker.internal:host-gateway \
		$(APP_NAME):latest serve --listen :8000 --base-path $(BASE_PATH)
	@echo "Waiting for calendar to be ready..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		curl -sf http://localhost:$(PORT)/health.json > /dev/null 2>&1 && break; \
		sleep 1; \
	done
	@echo "calendar running in Docker on port $(PORT)"

stop-docker:
	@docker rm -f calendar 2>/dev/null || true

restart-docker: stop-docker start-docker

logs-docker:
	@docker logs -f calendar


# ── Release ───────────────────────────────────────────────────

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GHCR_IMAGE := ghcr.io/localitas/localitas-app-$(APP_NAME)

docker-push: test build-docker
	docker tag $(APP_NAME):latest $(GHCR_IMAGE):latest
	docker tag $(APP_NAME):latest $(GHCR_IMAGE):$(VERSION)
	docker push $(GHCR_IMAGE):latest
	docker push $(GHCR_IMAGE):$(VERSION)
	@echo "✅ Pushed $(GHCR_IMAGE):latest and $(GHCR_IMAGE):$(VERSION)"


build-release: lint
	@mkdir -p dist
	@echo "Building $(APP_NAME) $(VERSION) ($(COMMIT))..."
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)" -trimpath \
		-o dist/calendar-server-darwin-arm64 ./cmd/calendar-server
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)" -trimpath \
		-o dist/calendar-server-darwin-amd64 ./cmd/calendar-server
	@echo "Built: dist/calendar-server-darwin-arm64, dist/calendar-server-darwin-amd64"

release: build-release
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "dev" ]; then echo "Set VERSION=vX.Y.Z"; exit 1; fi
	@echo "Creating release $(VERSION) on GitHub..."
	gh release create $(VERSION) \
		dist/calendar-server-darwin-arm64 \
		dist/calendar-server-darwin-amd64 \
		--title "$(VERSION)" --generate-notes
	@echo "✅ Released $(VERSION)"
