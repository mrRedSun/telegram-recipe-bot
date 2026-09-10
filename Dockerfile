# syntax=docker/dockerfile:1.19
FROM golang:1.26.6-alpine3.23@sha256:e57c41c1d5864341031181b0db34b9a537bb5773eb6428e4e5bdaea0f9135406 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/recipebot ./cmd/recipebot

FROM python:3.13.11-alpine3.23@sha256:2f607129b1b915a949320bf0c4831a73d1c1b1be663c2b1d8c93aa35a5f44a95 AS runtime
RUN apk add --no-cache \
      ca-certificates deno ffmpeg tesseract-ocr \
      tesseract-ocr-data-eng tesseract-ocr-data-rus tesseract-ocr-data-ukr \
    && apk upgrade --no-cache \
    && python -m venv /opt/venv \
    && /opt/venv/bin/pip install --no-cache-dir yt-dlp==2026.8.19 yt-dlp-ejs==0.8.0 \
    && addgroup -g 10001 recipebot \
    && adduser -D -H -u 10001 -G recipebot recipebot \
    && mkdir -p /tmp/recipebot \
    && chown recipebot:recipebot /tmp/recipebot
COPY --from=build --chown=10001:10001 /out/recipebot /usr/local/bin/recipebot
ENV PATH="/opt/venv/bin:$PATH"
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/recipebot"]
