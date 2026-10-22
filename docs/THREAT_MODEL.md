# Threat model and self-review

## Assets and trust boundaries

Secrets are the Telegram bot token and Z.AI API key. User messages, captions,
conversation context, recipe URLs/evidence, Instagram URLs, and downloaded media
are private transient data. Telegram, Instagram, YouTube, Z.AI, this container,
and the administrator-owned LXC/Docker daemon are separate trust boundaries.
GitHub/GHCR holds code and images, never runtime secrets.

## Controls

- Authentication: numeric Telegram sender allowlist, required at startup.
- Group behavior: ordinary conversational/media messages are ignored unless they
  use `/ask`, mention the bot, or reply to it. `/recipe` remains explicit.
- Context isolation: memory keys include chat and forum topic IDs; only the newest
  20 bounded user/assistant messages are retained; `/reset` clears history, erases queued prompt text, cancels in-flight scope
  requests, and invalidates old generations; restart removes everything.
- URL/SSRF policy: exact YouTube or Instagram hosts and path forms are parsed
  before subprocesses. Instagram CDN downloads accept HTTPS only from
  `cdninstagram.com` and `fbcdn.net`, including redirects. URL credentials,
  lookalikes, profiles, arbitrary redirects, and non-HTTP(S) schemes are rejected.
- Subprocesses: argument arrays only, never a shell. `gallery-dl` ignores user
  configuration, uses no cookies, emits bounded URL-only stdout, and discards
  stderr so signed URLs cannot reach application logs.
- Prompt injection: recipe evidence is explicitly untrusted and constrained by a
  fixed structured prompt. Conversation history is bounded and cannot alter
  application configuration or invoke tools.
- Resource exhaustion: bounded queue/workers, one source per job, carousel item,
  per-file, aggregate byte, response, retry, redirect, timeout, tmpfs, and PID
  limits. Multipart upload streams files rather than buffering them.
- Secrets: environment only; no values in logs, errors, repository, image, or
  health endpoint. `.env` is ignored and documented as mode 0600.
- Data minimization: no database, analytics, usernames, raw IDs, URLs, signed CDN
  links, source text, or media logs; private per-job directories are recursively
  removed.
- Container: UID/GID 10001, read-only root, dropped capabilities,
  `no-new-privileges`, no exposed host port, pinned bases and extractor versions.

## Abuse cases considered

Redirects, URL credentials, and lookalike domains are rejected. Profiles and
stories cannot trigger bulk collection. Private/login-gated Instagram failures
do not cause cookie use or bypass attempts. Oversized media fails before upload.
Albums preserve order and are split only at Telegram's 10-item boundary. Crafted
messages, captions, metadata, OCR, and model replies are escaped before Telegram
Rich Message delivery. Repeated requests cannot create unbounded goroutines,
queues, context scopes, history, files, or HTTP bodies.

## Residual risks

- Instagram can login-gate or rate-limit public posts and can change extractor or
  CDN behavior. Anonymous support is best-effort and requires reviewed
  `gallery-dl` updates.
- A new legitimate Instagram CDN suffix would be rejected until reviewed and
  added to the allowlist.
- Telegram may reject a file that fits byte limits because of dimensions, codec,
  duration, or transient upload behavior. The job reports a bounded failure and
  removes temporary files.
- YouTube can change extraction behavior or block the host IP; `yt-dlp` needs
  periodic reviewed updates.
- GLM conversation and recipe output is probabilistic. Bounded history can omit
  older context by design.
- Tesseract ships only English, Ukrainian, and Russian language data.
- Environment variables remain visible to privileged LXC/Docker administrators.
- Third parties receive content under their own policies.
- Another long poller using the same Telegram token would conflict.

## Questions anticipated

- Why no Instagram login? The requested boundary is public anonymous content.
  Cookies would add account security, retention, and access-control risks.
- Why `gallery-dl`? `yt-dlp` handles video but does not support image-only
  Instagram carousels. The exact pinned gallery extractor discovers ordered
  photo and video URLs without application-specific HTML scraping.
- Why not keep Telegram file IDs? Persistent cache state would conflict with this
  bot's no-history/no-media-retention policy.
- What survives a restart? Only the immutable image and external runtime
  configuration. Context, queued work, and downloads are intentionally lost.
