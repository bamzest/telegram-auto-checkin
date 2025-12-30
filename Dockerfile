# syntax=docker/dockerfile:1.7
# Multi-arch minimal runtime image for Telegram Auto Check-in
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

ARG TARGETPLATFORM
# Binary will be injected by GoReleaser per-arch build context (dockers_v2)
COPY $TARGETPLATFORM/telegram-auto-checkin /app/telegram-auto-checkin
# Optional: include default config and locales for quick start
COPY config.yaml /app/config.yaml
COPY locales /app/locales

USER nonroot:nonroot
ENTRYPOINT ["/app/telegram-auto-checkin"]
