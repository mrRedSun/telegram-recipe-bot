# Operations

## Deployment

Use Bot API 10.3+ for Rich Message recipe cards. Preserve the sender allowlist,
model endpoint/key, non-root user, read-only filesystem, capabilities, and tmpfs.

1. Preserve exact Compose and the mode-0600 runtime environment under `.backups`.
2. Pin the intended immutable main-commit image, then run
   `docker compose config --quiet` and `docker compose up -d --no-build`.
3. Check Docker health and internal `/healthz`, startup/polling diagnostics, and
   the running image identity. Do not print resolved Compose environment values.
4. Verify recipe extraction using a known source and `recipeprobe`; test Telegram
   delivery from an allowlisted account separately.

Rollback restores the previous Compose image pin and recreates the bot, then
checks health. Keep old images and exact runtime/config backups. Restart loses
conversation context and queued work by design; no database migration is needed.
Images are published only through GitHub Actions. Runtime updates use new images,
not package installation inside the container.

## Recipe probe

Both source types exercise the same extraction and GLM recipe path:

```sh
docker compose exec recipebot recipeprobe 'https://www.instagram.com/reel/SHORTCODE/'
docker compose exec recipebot recipeprobe 'https://youtube.com/shorts/VIDEO_ID'
```

The probe reads the existing runtime configuration but sends no Telegram message.
It runs the actual source pipeline and inference, validates the recipe, prints
only recipe counts/confidence and evidence size/stage metrics, and removes its
private tmpfs workspace. A successful download alone does not establish a recipe
success: require `recipe_validated=true` and `probe cleanup completed`.

## Diagnostics

Use anonymous job IDs to correlate `recipe source progressed`, recipe progress,
evidence counts, GLM finish/token diagnostics, inference completion, and delivery.
Backend/stage/fixed reason distinguish metadata/download, captions, comments,
media inspection, frames, OCR, model, and Telegram failures. Never add raw stderr,
metadata, source URLs/content, identities, credentials, or model reasoning to logs.

Instagram duration can be absent in metadata; actual ffprobe duration is required
before sampling frames. Instagram creator captions are preserved from the selected
entry of playlist-shaped metadata. Missing subtitles/comments are optional evidence.

Private/login-gated, removed, rate-limited, unsupported multi-video/image-only,
oversized, or overly long sources fail safely with bounded diagnostics. No cookie,
login, or account fallback is permitted. GLM and extractor drift/quota issues need
reviewed image/config updates. The recipe bot sends recipe cards; there is no media
upload command or alternate Instagram download workflow.

## Secrets

Keep runtime `.env` mode 0600. Rotate at the provider, update it privately, and
recreate only when rotation is explicitly authorized. CI needs no runtime secrets.

### Optional local recipe catalogue

See [CATALOG.md](CATALOG.md) for data provenance, read-only mounting, configuration,
query/resource limits, fallback, attribution, and source-quality limits. Catalogue
text is untrusted reference material. It cannot override explicit video details,
introduce reference-only ingredients, or establish high-confidence fields. Only
used sources receive application-bound citations. No Telegram history or source
URLs are added to the catalogue, and runtime search phrases are never persisted.
