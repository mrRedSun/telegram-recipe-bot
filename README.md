# Telegram Recipe Bot and Assistant

A private-by-default Telegram bot with three bounded workflows for allowlisted
users:

- reconstruct a recipe from one public YouTube video or Short;
- hold a general conversation through the existing Z.AI GLM provider;
- download and return media from one public Instagram post or Reel, including
  ordered carousel items.

Recipe extraction downloads the source into ephemeral storage, extracts
captions, OCR, metadata, uploader-authored comments, and sampled frames, then
asks Z.AI GLM 5.3 to return a validated recipe with uncertainty made explicit.
Instagram media is returned with Telegram-native photo, video, or album methods
and is never sent to GLM.

## Telegram interaction

In direct messages:

- send a YouTube URL for recipe extraction;
- send a public `instagram.com/p/...` or `instagram.com/reel/...` URL to receive
  its media;
- send ordinary text or a caption to talk to the assistant;
- use `/reset` to clear that DM's temporary conversation context.

In groups and supergroups, recipe requests continue to use
`/recipe <YouTube URL>` (including Telegram's `/recipe@BotName` form). General
assistant and Instagram requests require `/ask`, a mention of the bot, or a
reply to one of its messages, so the bot does not answer every group message and
remains compatible with BotFather privacy mode. Forum topics have independent
conversation contexts. An explicit `/recipe` always means YouTube recipe
extraction, never Instagram downloading.

## Privacy and context

The assistant keeps only the latest 20 user/assistant messages for each DM,
group, or forum topic. Message text and captions are bounded, held in process
memory only, omitted from logs and analytics, cleared by `/reset`, and lost on
restart. Reset also cancels queued or in-flight assistant requests in that scope and
suppresses their replies. Different chats and topics cannot share context.

Recipe and Instagram jobs use private per-job directories on tmpfs. Downloads,
frames, metadata, and uploads are removed after success, failure, cancellation,
or timeout. Logs use anonymous process-local job numbers and bounded diagnostics;
they never contain raw Telegram identities, messages, captions, URLs,
conversation context, source content, credentials, or model reasoning.

## Why two GLM modes?

Z.AI currently documents `glm-5.3` as text-only and `glm-5.3-flash` as native
multimodal. The default is `glm-5.3-flash`, which inspects sampled YouTube frames
plus text evidence for recipes. Set `GLM_MODEL=glm-5.3` and
`GLM_VISION_ENABLED=false` to use the plain model with title, description,
uploader-authored comments, captions, and OCR only. Both models support the
text-only conversational assistant.

## Architecture

```text
Telegram long poll -> sender allowlist -> interaction policy -> bounded queue
  recipe -> exact YouTube URL policy -> yt-dlp -> bounded evidence -> GLM JSON
  chat   -> per-chat/topic 20-message memory -> GLM plain text
  media  -> exact Instagram policy -> yt-dlp (Reels) / gallery-dl (posts) URL discovery
         -> bounded anonymous CDN downloads -> Telegram photo/video/albums
  all jobs -> timeouts, anonymous diagnostics, unconditional temp cleanup
```

Recipe acknowledgements are edited through seven emoji-labelled stages covering
download, captions, creator comments, frame sampling, OCR, GLM reconstruction,
and Rich Message formatting. Instagram jobs show bounded download/upload status.
Reels use yt-dlp’s anonymous video extraction path; post/carousel extraction
continues through gallery-dl.

## Local development

Requirements: Go 1.26.10+, Docker, and (for running outside Docker) `yt-dlp`,
`gallery-dl`, FFmpeg, and Tesseract.

```sh
make fmt vet test race
docker build -t telegram-recipe-bot:dev .
```

Python extractors and their transitive dependencies are exact-version pinned in
`requirements-extractors.txt`:
`gallery-dl==1.32.17`, `yt-dlp==2026.8.19`, and `yt-dlp-ejs==0.8.0`. Tests use
local fakes and do not require Telegram, Instagram, YouTube, or Z.AI credentials.
Opt-in integration tests remain separate from deterministic CI. Pip and its
bundled dependency copies are removed after the build; runtime upgrades must use
a new reviewed image.

## Configuration

Copy `.env.example` to `.env`. Required values are:

- `TELEGRAM_BOT_TOKEN`: BotFather token.
- `TELEGRAM_ALLOWED_USER_IDS`: comma-separated positive numeric sender IDs.
- `GLM_API_KEY`: Z.AI API/Coding Plan key accepted by the configured endpoint.
- `IMAGE_TAG`: immutable `sha-*` tag produced by CI when using Compose.

All limits and model options are documented in `.env.example`. The same bounded
worker pool and five-minute job deadline cover recipes, assistant replies, and
Instagram downloads. The service fails closed on missing secrets, an empty
allowlist, malformed limits, or an invalid reasoning level.

## Docker deployment

Images are built and published only by GitHub Actions. After CI publishes the
GHCR package:

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

Private/authenticated Instagram content, stories, profiles, highlights, and
access-control bypasses are unsupported. Anonymous Instagram extraction is
best-effort and can fail when Instagram login-gates or rate-limits an otherwise
public post. The bot stores no Instagram credentials or cookies. Telegram's
hosted Bot API limits each photo to 10 MiB, each video to 50 MiB, and each album
to 10 items; larger carousels are sent as multiple ordered batches.

Private/DRM/age-restricted/live YouTube content and playlists are unsupported.
YouTube and Instagram may require reviewed extractor updates. Recipe output
remains probabilistic: always use food-safety judgment, especially for meat,
eggs, allergens, and uncertain cooking temperatures.
