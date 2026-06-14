---
title: Calendar
description: Calendar management with CalDAV sync
---

# Calendar

Manage events, calendars, and reminders with CalDAV synchronization support.

## Events

Create, update, and delete calendar events. Each event has a title, start time, end time, location, and description.

**GET /api/events** - List events (filter by date range with `start` and `end` query params)
**POST /api/events** - Create a new event
**GET /api/events/{id}** - Get a single event
**PUT /api/events/{id}** - Update an event
**DELETE /api/events/{id}** - Delete an event

## Calendars

Organize events into separate calendars. Each calendar has a name and color.

**GET /api/calendars** - List all calendars
**PUT /api/calendars/{id}** - Update calendar properties

## Reminders

Attach reminders to events to receive notifications before an event starts.

**POST /api/events/{id}/reminders** - Add a reminder to an event
**GET /api/events/{id}/reminders** - List reminders for an event
**PUT /api/events/{id}/reminders** - Replace all reminders on an event
**DELETE /api/reminders/{rid}** - Delete a specific reminder
**GET /api/reminders/pending** - List all pending reminders
**POST /api/reminders/check** - Trigger reminder evaluation

## CalDAV Sync

Connect external CalDAV accounts (Google Calendar, iCloud, etc.) to sync events bidirectionally. Google OAuth is supported for authentication.

**GET /api/accounts** - List CalDAV accounts
**POST /api/accounts** - Add a CalDAV account
**POST /api/accounts/{id}/sync** - Sync a single account
**POST /api/sync** - Sync all accounts

## CalDAV Server

The app exposes a CalDAV server at `/caldav/` for external calendar clients. The well-known endpoint `/.well-known/caldav` redirects to it.

## Search

**GET /api/search** - Full-text search across events
**GET /api/parse-date** - Parse natural language date expressions
**GET /api/conflicts** - Detect scheduling conflicts

## Build & Deploy

### Version

```bash
./calendar-server --version
```

### Build from source

```bash
# Development (native)
cd apps/calendar && go build -o bin/calendar-server ./cmd/calendar-server

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath -o bin/calendar-server-linux-amd64 ./cmd/calendar-server
```

### Docker

Build a Docker image directly from the binary:

```bash
# Default base image (debian:12-slim)
./calendar-server docker-build

# Custom base image
./calendar-server docker-build --base ubuntu:24.04

# Custom Dockerfile
./calendar-server docker-build --dockerfile ./my.Dockerfile

# Tag and push to registry
./calendar-server docker-build --tag ghcr.io/localitas/calendar:latest --push
```

The `docker-build` command requires a Linux amd64 binary in the same directory. Run `make deploy-build` from the project root first.

### Download

Pre-built binaries are available on the [GitHub releases page](https://github.com/localitas/localitas/releases).

Each release includes three builds per app:
- `calendar-server-darwin-arm64` (macOS Apple Silicon)
- `calendar-server-linux-amd64` (Linux x86_64)
- `calendar-server-linux-arm64` (Linux ARM64)

Download with the GitHub CLI:

    gh release download --repo localitas/localitas --pattern 'calendar-server-*'

### Release

All app binaries are published to GitHub releases as part of `make deploy-upload-image`.
