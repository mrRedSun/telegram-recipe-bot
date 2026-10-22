# syntax=docker/dockerfile:1.19
FROM golang:1.26.10-alpine3.23@sha256:107301490755aa2ea1b0b43788b6e62cff2faae45a538028b193becc06aaffba AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/recipebot ./cmd/recipebot \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/recipeprobe ./cmd/recipeprobe

FROM python:3.13.17-alpine3.23@sha256:a183eaab9bc16fc01807204df7ed80291550a525662753ad166da45a078db18f AS runtime
COPY requirements-extractors.txt /tmp/requirements-extractors.txt
RUN apk add --no-cache \
      ca-certificates deno ffmpeg tesseract-ocr \
      tesseract-ocr-data-eng tesseract-ocr-data-rus tesseract-ocr-data-ukr \
    && apk upgrade --no-cache \
    && python -m venv /opt/venv \
    && /opt/venv/bin/pip install --no-cache-dir -r /tmp/requirements-extractors.txt \
    && addgroup -g 10001 recipebot \
    && adduser -D -H -u 10001 -G recipebot recipebot \
    && mkdir -p /tmp/recipebot \
    && chown recipebot:recipebot /tmp/recipebot
COPY --from=build --chown=10001:10001 /out/recipebot /usr/local/bin/recipebot
COPY --from=build --chown=10001:10001 /out/recipeprobe /usr/local/bin/recipeprobe
ENV PATH="/opt/venv/bin:$PATH"
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/recipebot"]
