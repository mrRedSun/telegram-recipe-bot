# Telegram Recipe Bot and Assistant

An allowlisted Telegram bot that reconstructs recipes from public YouTube
videos/Shorts and Instagram Reels, plus general GLM conversation.

Send either supported video link to receive a recipe card. Both sources use
one pipeline: anonymous yt-dlp metadata/download, creator caption/description,
available subtitles and creator comments, sampled frames, OCR, GLM inference,
recipe validation, and Telegram Rich Message rendering.

## Interaction

- Send a YouTube video/Short or Instagram Reel URL for a recipe.
- `/recipe URL` works for either source, including `/recipe@BotName` in groups.
- Ordinary DM text/captions are conversational. In groups, use `/ask`, a bot
  mention, or a reply to the bot for conversation. Bare supported video links
  trigger the same recipe workflow for either platform when Telegram delivers them.
- `/reset` clears the current chat/topic conversation and cancels its assistant
  work. `/help`, `/privacy`, and `/status` explain usage and service state.

The recipe bot returns recipes from Instagram links. Standalone photo, video,
and album delivery has been removed. Multi-video posts and image-only posts
are outside the recipe source contract. The separate video downloader remains
independent.

## Recipe evidence

The downloader handles a single public video. Instagram's one-video playlist
metadata is flattened so its creator caption is preserved. Actual duration is
verified with ffprobe, including when Instagram metadata omits it. Recipe
analysis samples up to 12 frames, runs OCR, and forwards bounded text plus frames
when the configured GLM model supports vision. Missing subtitles or creator
comments do not discard usable video/caption evidence.

GLM returns structured ingredients, steps, times, assumptions, warnings, and
confidence. Untrusted source instructions cannot change the fixed extraction
policy. Every model-derived field is escaped before Telegram formatting.

## Privacy and limits

Sender authorization uses the configured numeric allowlist. Conversation context
is in process memory only, scoped by chat/forum topic, capped at 20 messages of
4,000 runes across 256 resident scopes. Reset and restart remove context.

Source media, metadata, subtitles, and frames use private per-job tmpfs directories
and are removed after every job. Logs contain anonymous job numbers, source
platform, stage/backend/outcome, fixed failure reason, exit/HTTP codes, bounded
counts/sizes, and timing. They contain no messages, URLs, identities, captions,
source content, raw extractor output, credentials, or model reasoning.

Default bounds: two workers, eight queued jobs, 100 MiB source limit, four-minute
video limit, five-minute job deadline, 512 MiB shared tmpfs, 2 MiB metadata stdout,
and 64 KiB raw stderr retained temporarily in memory for fixed-code classification.

## Development and validation

Go 1.26.10+, Docker, yt-dlp, FFmpeg/ffprobe, Deno, and Tesseract are required.

```sh
make fmt vet test race
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
docker build -t telegram-recipe-bot:dev .
```

Python dependencies are pinned in `requirements-extractors.txt`; base images and
GitHub Actions are pinned by digest/SHA. Pip and ensurepip are build-only and
removed afterward. CI scans source secrets and the exact image, runs smoke checks,
and publishes immutable SHA tags only after checks pass.

## Configuration and deployment

Copy `.env.example` to `.env` and keep it mode 0600. Set
`TELEGRAM_BOT_TOKEN`, `TELEGRAM_ALLOWED_USER_IDS`, `GLM_API_KEY`, and an immutable
`IMAGE_TAG` for the example Compose file. Model/endpoint, reasoning, workers,
timeouts, and source limits are documented in that example.

```sh
docker compose config --quiet
docker compose pull
docker compose up -d
docker compose ps
```

Production uses a non-root, read-only container with no capabilities or published
application port. Upgrade/rollback uses immutable SHA images and exact Compose
plus runtime-environment backups. See [operations](docs/OPERATIONS.md) for the
metrics-only `recipeprobe`, deployment checks, and failure investigation.

Private/login-gated, DRM, live, story/profile, playlist, image-only, and multi-video
sources are unsupported. Extraction remains subject to provider rate limits and
extractor drift. Recipe reconstruction is probabilistic; check uncertain amounts,
timings, allergens, and food safety.

## Local recipe references

An optional read-only SQLite catalogue can retrieve similar recipes and supply
labelled reference estimates during video reconstruction. Used references are
linked in the recipe card; video evidence takes priority. The Food.com importer
and benchmark run offline on the development machine. See [catalogue setup,
source-quality limits, and rollback](docs/CATALOG.md).
