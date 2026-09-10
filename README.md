# Telegram Recipe Bot

A private-by-default Telegram bot that reconstructs a recipe from one public
YouTube video or Short. It downloads the source into ephemeral storage, extracts
captions, OCR, metadata, uploader-authored comments, and sampled frames, then asks Z.AI GLM 5.3 to return a
validated recipe with uncertainty made explicit.

## Why two GLM modes?

Z.AI currently documents `glm-5.3` as text-only and `glm-5.3-flash` as native
multimodal. The default is `glm-5.3-flash`, which inspects sampled frames plus
text evidence. Set `GLM_MODEL=glm-5.3` and `GLM_VISION_ENABLED=false` to use the
plain model with title, description, uploader-authored comments, captions, and OCR only.

## Architecture

```text
Telegram long poll -> sender allowlist -> bounded queue
  -> exact YouTube URL policy -> yt-dlp (one bounded video)
  -> captions + metadata + bounded uploader comments + Tesseract OCR + 12 FFmpeg frames
  -> GLM JSON recipe -> semantic validation -> HTML-safe Telegram result
  -> unconditional temporary-directory cleanup
```

The service stores no history or analytics. See [the full specification](docs/SPEC.md),
[threat model](docs/THREAT_MODEL.md), and [operations guide](docs/OPERATIONS.md).

## Local development

Requirements: Go 1.26.6+, Docker, and (for running outside Docker) `yt-dlp`,
FFmpeg, and Tesseract.

```sh
make fmt vet test race
docker build -t telegram-recipe-bot:dev .
```

Tests use local fakes and do not require Telegram, YouTube, or Z.AI credentials.
An opt-in `integration` build-tag test accepts `TEST_YOUTUBE_URL` for live
extractor verification; it is intentionally excluded from deterministic CI.

## Configuration

Copy `.env.example` to `.env`. Required values are:

- `TELEGRAM_BOT_TOKEN`: BotFather token.
- `TELEGRAM_ALLOWED_USER_IDS`: comma-separated positive numeric sender IDs.
- `GLM_API_KEY`: Z.AI API/Coding Plan key accepted by the configured endpoint.
- `IMAGE_TAG`: immutable `sha-*` tag produced by CI when using Compose.

All limits and model options are documented in `.env.example`. The service
fails closed on missing secrets, an empty allowlist, malformed limits, or an
invalid reasoning level.

## Docker deployment

Images are built and published only by GitHub Actions. After CI publishes the
private GHCR package:

```sh
cp .env.example .env
chmod 600 .env
# Edit .env without committing it.
docker login ghcr.io
docker compose config --quiet
docker compose pull
docker compose up -d
docker compose ps
```

The container runs as UID 10001 with a read-only filesystem, no Linux
capabilities, and a 512 MiB tmpfs. No health port is published to the host.

## Limitations

Private/DRM/age-restricted/live content and playlists are unsupported. YouTube
may occasionally require a reviewed `yt-dlp` update. Disabled or inaccessible
comments are skipped, and a bounded top-comment sample can miss a buried creator
reply. Recipe output remains
probabilistic: always use food-safety judgment, especially for meat, eggs,
allergies, and uncertain cooking temperatures.
