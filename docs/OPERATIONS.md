# Operations

## First deployment

The hosted `https://api.telegram.org` endpoint supports the required Bot API
10.3 Rich Messages and native media methods. A custom or self-hosted
`TELEGRAM_API_BASE` must run Bot API 10.3 or newer.

1. Copy `.env.example` to `.env`, set an immutable `IMAGE_TAG`, BotFather token,
   permitted numeric Telegram IDs, and Z.AI API key; then `chmod 600 .env`.
2. `docker compose config --quiet` and `docker compose pull`.
3. `docker compose up -d` and `docker compose ps`.
4. Confirm recent logs contain startup metadata but no identities, messages,
   captions, URLs, source text, context, signed media URLs, or credentials.
5. Send `/status`, a normal DM question, `/reset`, one known public Instagram
   post/Reel, and a known public cooking Short.

For groups, `/recipe URL`, `/ask TEXT`, bot mentions, and replies work with
BotFather privacy mode enabled. Forum replies remain in the originating topic.
The application allowlist always restricts requests by sender ID.

## Upgrade and rollback

Record the deployed immutable tag. Change only `IMAGE_TAG`, pull, and recreate.
Verify health plus the three smoke paths. Roll back by restoring the prior tag
and running `docker compose up -d`. There is no persistent application state or
migration; rollback also discards in-memory conversation context and active jobs.

Images must be published through GitHub Actions. Do not install or update
`gallery-dl`, `yt-dlp`, or `yt-dlp-ejs` at container startup. Extractor changes
must arrive through a reviewed immutable image.

## Diagnostics

Use `docker compose ps`, privacy-filtered `docker compose logs --since=10m`, and
`docker inspect --format '{{json .State.Health}}'`. Logs contain anonymous job
sequence, workflow kind, stages, elapsed times, bounded item/evidence sizes, GLM
finish/token diagnostics, and failure categories. They must not contain raw
identities, messages, captions, URLs, context, source evidence, signed CDN URLs,
model output/reasoning, or secrets.

Common Instagram failures are login-gated/private/removed content, host rate
limits, changed CDN domains, unsupported media formats, Bot API file limits, and
transient Telegram upload failure. The bot never adds cookies or credentials as
a fallback. Common recipe failures remain unavailable video, extractor drift,
quota/auth errors, truncated structured output, and weak evidence.

When the queue is idle, `recipeprobe` still reproduces only the recipe path:

```sh
docker compose exec recipebot recipeprobe 'https://youtube.com/shorts/VIDEO_ID'
```

There is intentionally no Instagram probe that prints resolved URLs or media
metadata. Validate Instagram through an allowlisted Telegram account and inspect
only safe count/size diagnostics.

## Secret rotation

Rotate at BotFather or Z.AI, update the mode-0600 `.env`, then recreate the
container. Never paste secrets into Compose YAML, Git, issues, or CI variables;
CI does not need runtime credentials. Instagram support uses no secret.
