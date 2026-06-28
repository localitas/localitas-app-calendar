# Calendar

Calendar management with CalDAV sync.

Part of the [Localitas](https://github.com/localitas) platform — a self-hosted, privacy-first personal computing system.

## Features

- Event CRUD with recurrence rules (RRULE)
- CalDAV server for native sync with macOS Calendar, iOS, Thunderbird
- Google Calendar OAuth sync
- Timezone-aware date handling
- Reminder support

## Installation

### Development (via Localitas core)

```bash
# Clone the repo
git clone https://github.com/localitas/localitas-app-calendar.git ~/localitas-app-calendar

# Start with the Localitas dev cluster (builds and runs in Docker automatically)
cd ~/localitas && make dev-core
```

### Standalone

```bash
cd ~/localitas-app-calendar

# Build and run locally
make build
./bin/calendar-server serve --listen :8000

# Or via launchd (macOS)
make start

# Or via Docker
make start-docker
```

## Exposing to the Internet

Localitas apps are accessible remotely through Localitas's built-in tunnel service, powered by FRP. No port forwarding or dynamic DNS required.

1. Sign up at [localitas.com](https://localitas.com) and connect your local Localitas core
2. The tunnel automatically exposes your core (and all apps) at `https://{your-subdomain}.localitas.com`
3. This app is available at `https://{your-subdomain}.localitas.com/apps/ext/calendar/`

All traffic is encrypted end-to-end. Authentication is handled by the Localitas core — only authorized users can access your apps.

## App Store

Install via the Localitas App Store (recommended):

```bash
localitas-core app-store add --name calendar --compose ./docker-compose.yml --port 9203
localitas-core app-store start calendar
```

Or open the App Store UI (package icon, top-right nav) and paste the `docker-compose.yml`.

The image is published to `ghcr.io/localitas/localitas-app-calendar:latest`. To publish a new version:

```bash
make docker-push   # runs tests, builds, and pushes to ghcr.io
```

## License

MIT
