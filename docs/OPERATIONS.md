# Operations

## First deployment

1. Copy `.env.example` to `.env`, set an immutable `IMAGE_TAG`, BotFather token,
   permitted numeric Telegram IDs, and Z.AI API key; then `chmod 600 .env`.
2. `docker compose config --quiet` and `docker compose pull`.
3. `docker compose up -d` and `docker compose ps`.
4. Confirm `docker compose logs --tail=50` contains startup metadata but no
   secrets, then send `/status` and a known public cooking Short.

## Upgrade and rollback

Record the currently deployed immutable tag. Change only `IMAGE_TAG`, pull, and
recreate. Verify health and a smoke recipe. Roll back by restoring the prior tag
and running `docker compose up -d`. There is no persistent application state or
migration.

## Diagnostics

Use `docker compose ps`, `docker compose logs --since=10m`, and
`docker inspect --format '{{json .State.Health}}'`. Logs contain job sequence,
stage failures, and confidence, not URLs or identities. Common failures are a
private/unavailable video, YouTube extractor drift, missing subtitles with weak
visual evidence, quota/auth errors, and Telegram token reuse by another poller.

## Secret rotation

Rotate at BotFather or Z.AI, update the mode-0600 `.env`, then recreate the
container. Never paste secrets into Compose YAML, Git, issues, or CI variables;
CI does not need runtime credentials.

