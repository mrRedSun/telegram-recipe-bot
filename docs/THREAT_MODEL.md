# Threat model

## Boundaries

Telegram sender authorization, public source providers, local yt-dlp/FFmpeg/OCR
subprocesses, GLM inference, Docker/LXC administration, and GitHub/GHCR are distinct
trust boundaries. Tokens, source/evidence text, identities, conversation context,
and model output/reasoning are private transient material.

## Controls

- Numeric sender allowlist; exact supported source host/path validation before
  extraction; commands for other bots ignored; explicit group assistant triggers.
- One shared recipe pipeline for YouTube and Instagram; no raw media upload API
  or standalone delivery path. Reject missing/multiple videos and non-finite,
  zero, oversized, or overly long media.
- No source cookies/accounts, private access, or access-control bypass. Anonymous
  yt-dlp config/cache disabled; subprocess arguments use arrays without a shell.
- Bounded metadata/command output, stderr, source bytes, duration, frames, queue,
  workers, retries, and job timeouts. Actual duration checked with ffprobe when
  source metadata is absent. Analysis protocols restricted to local file/pipe.
- Source evidence is untrusted data under a fixed structured GLM prompt. Returned
  recipes are validated and all model-derived text is escaped for Telegram.
- Job diagnostics contain only anonymous IDs, platform, fixed stages/reasons,
  exit/HTTP codes, bounded counts, and timing. Raw stderr is classified in bounded
  memory, then discarded; URLs, source text, identities, credentials, response
  contents, and reasoning are excluded.
- Conversation keys include chat/topic, history and scope counts are bounded,
  request order is preserved, and reset cancels work and invalidates generations.
- Private tmpfs workspace cleanup on every job/probe exit. No database, source
  cache, Telegram file IDs, analytics, or persisted conversation.
- Non-root container, read-only root, dropped capabilities, no-new-privileges,
  health checking, no published app port, pinned bases/dependencies, exact-image
  scanning, and CI-only GHCR publication.

## Residual risks

Public providers may block/rate-limit the host or change extraction behavior.
Missing speech captions/comments limit evidence; visual OCR and inference are
probabilistic. Image-only and multi-video sources are unsupported. Trusted source
extractors select CDN requests; changes in extractor/network behavior require
review. Third parties receive data under their own policies. Privileged host
administrators can inspect runtime credentials. Another token-sharing poller can
conflict. Hardware/storage failures require the existing external backup policy.
