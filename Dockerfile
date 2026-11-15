FROM debian:12-slim

RUN apt-get update -qq && apt-get install -y -qq ca-certificates && rm -rf /var/lib/apt/lists/*
RUN useradd --system --no-create-home --shell /usr/sbin/nologin app || true

COPY calendar-server-linux-amd64 /usr/local/bin/app-server
RUN chmod +x /usr/local/bin/app-server

USER app

HEALTHCHECK --interval=10s --timeout=10s --start-period=2m --start-interval=1s --retries=3 \
  CMD bash -c 'exec 3<>/dev/tcp/127.0.0.1/8000 && printf "GET /health.json HTTP/1.0\r\n\r\n" >&3 && IFS= read -r line <&3 && [[ $line == HTTP/*" 200"* ]]'

ENTRYPOINT ["/usr/local/bin/app-server"]
