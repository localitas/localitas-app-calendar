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
**POST /api/accounts/sync** - Sync all of your accounts

## CalDAV Server

The app exposes a CalDAV server at `/caldav/` for external calendar clients; it requires a signed-in user and shows only that user's calendars. The well-known endpoint `/.well-known/caldav` redirects to it.

## Search

**GET /api/search** - Full-text search across events
**GET /api/parse-date** - Parse natural language date expressions
**GET /api/conflicts** - Detect scheduling conflicts

## Build & Deploy

Run these in the app's repository, `~/localitas-app-calendar`.

### Version

```bash
bin/calendar-server --version
```

### Build from source

```bash
# Native binary (bin/calendar-server)
make build

# Linux binary the Docker image is built from, for the Docker daemon's
# architecture (calendar-server-linux-arm64 on Apple silicon); LINUX_ARCH=amd64 overrides
make build-linux
```

Both run `make lint` (gofmt and go vet) first; `make test` runs the lint step and then the tests.

### Run natively

```bash
make start   # builds bin/calendar-server, writes the launchd service com.localitas.app.calendar for bin/calendar-server and loads it on port 9203
make stop
make logs    # follows ~/.localitas/logs/calendar/stdout.log
```

The app announces itself to core when it starts, and core serves it at `/apps/ext/calendar/`.

### Docker

```bash
make build-docker   # builds the Linux binary and the calendar:latest image
make start-docker   # runs container "calendar" on port 9203, labelled localitas.app=true
make stop-docker
make logs-docker
```

Core registers containers labelled `localitas.app=true` once Docker reports them healthy.

### Publish

```bash
# Run the tests, build the image, push ghcr.io/localitas/localitas-app-calendar:latest and :<VERSION>
make docker-push

# Build dist/calendar-server-darwin-arm64 and dist/calendar-server-darwin-amd64
# and create GitHub release vX.Y.Z on this repository with them
make release VERSION=vX.Y.Z
```

`VERSION` defaults to `git describe --tags --always --dirty`.

### Download

Pre-built macOS binaries are on the [releases page](https://github.com/localitas/localitas-app-calendar/releases):
- `calendar-server-darwin-arm64` (Apple Silicon)
- `calendar-server-darwin-amd64` (Intel)

Every push to `main` publishes a release: `.github/workflows/release.yml` runs the tests and releases both binaries as `v0.1.<commit count>.<UTC timestamp>`. `make release` publishes one by hand.

Download with the GitHub CLI:

    gh release download --repo localitas/localitas-app-calendar --pattern 'calendar-server-*'
