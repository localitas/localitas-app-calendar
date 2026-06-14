.PHONY: build test install uninstall start stop restart status logs logs-err \
       build-docker start-docker stop-docker restart-docker logs-docker \
       lint strict-lint swagger

APP_NAME := calendar
PORT ?= 9203
BASE_PATH ?= /apps/ext/calendar/
PLIST_NAME := com.localitas.app.calendar
PLIST_FILE := $(HOME)/Library/LaunchAgents/$(PLIST_NAME).plist
LOG_DIR := $(HOME)/.localitas/logs/calendar
BIN_PATH := $(shell pwd)/bin/calendar-server
WORK_DIR := $(shell pwd)

# ── Build & Test ──────────────────────────────────────────────

build: lint

build-linux: lint
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath \
		-o calendar-server-linux-amd64 ./cmd/calendar-server
	@mkdir -p bin
	go build -o bin/calendar-server ./cmd/calendar-server

test: lint
	go test -v ./...

lint:
	@echo "Running gofmt..."
	@gofmt -w .
	@echo "Running go vet..."
	@go vet ./...

strict-lint: lint
	@echo "Running staticcheck..."
	@if ! command -v staticcheck > /dev/null 2>&1; then \
		echo "Installing staticcheck..."; \
		go install honnef.co/go/tools/cmd/staticcheck@latest; \
	fi
	@staticcheck ./...
	@echo "staticcheck passed"

swagger:
	@curl -s http://localhost:$(PORT)/swagger.json | python3 -m json.tool

# ── Native (launchd) ─────────────────────────────────────────

install: build
	@mkdir -p $(LOG_DIR)
	@sed 's|$${BIN_PATH}|$(BIN_PATH)|g; s|$${PORT}|$(PORT)|g; s|$${BASE_PATH}|$(BASE_PATH)|g; s|$${LOG_DIR}|$(LOG_DIR)|g; s|$${WORK_DIR}|$(WORK_DIR)|g' \
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
	docker build -t calendar:latest .

start-docker: build-docker stop-docker
	@docker run -d -p $(PORT):8000 --name calendar \
		--log-opt max-size=10m --log-opt max-file=7 \
		calendar:latest
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
