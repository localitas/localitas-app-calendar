FROM debian:12-slim

RUN apt-get update -qq && apt-get install -y -qq ca-certificates && rm -rf /var/lib/apt/lists/*
RUN useradd --system --no-create-home --shell /usr/sbin/nologin app || true

COPY calendar-server-linux-amd64 /usr/local/bin/app-server
RUN chmod +x /usr/local/bin/app-server

USER app

ENTRYPOINT ["/usr/local/bin/app-server"]
