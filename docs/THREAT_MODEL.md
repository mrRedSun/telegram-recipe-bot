# Threat model and self-review

## Assets and trust boundaries

Secrets are the Telegram bot token and Z.AI API key. Private user submissions
cross Telegram, YouTube, this container, and Z.AI. GitHub/GHCR holds code and
images, never runtime secrets. The LXC and Docker daemon are administrator-owned
trust boundaries.

## Controls

- Authentication: numeric Telegram sender allowlist, required at startup.
- SSRF/input injection: exact YouTube hostname and video-ID parsing; no arbitrary
  URLs reach `yt-dlp`; commands use argument arrays, never a shell.
- Prompt injection: all source text, including uploader comments, is explicitly
  untrusted evidence; the fixed
  system prompt restricts output; JSON is parsed and semantically validated.
- Resource exhaustion: one video, no playlists/live media, duration/size limits,
  bounded queue/workers, timeouts, response limits, tmpfs, PID limit.
- Secrets: environment only; no values in logs, errors, repository, image, or
  health endpoint. `.env` is ignored and documented as mode 0600.
- Data minimization: no database, analytics, usernames, raw IDs, URLs, or media
  logs; private per-job directory is recursively removed.
- Container: UID/GID 10001, read-only root, dropped capabilities,
  `no-new-privileges`, no exposed host port, pinned bases and dependencies.

## Abuse cases considered

Redirects and lookalike domains are rejected before download. Crafted titles,
descriptions, uploader comments, subtitles, OCR, and frame text cannot alter application configuration or invoke
tools. Large, long, live, and playlist inputs are rejected. Repeated requests
cannot create unbounded goroutines or queues. Telegram HTML is escaped. GLM and
Telegram response bodies are bounded before logging.

## Residual risks

- YouTube can change extraction behavior or block the host IP; `yt-dlp` needs
  periodic reviewed updates.
- Captions and sparse frames can omit ingredients or timing. Even multimodal
  output is probabilistic and must not be treated as authoritative.
- The bounded top-comment sample can miss a buried uploader reply. Comment
  extraction can also be unavailable or rate-limited; it fails open to the
  remaining evidence. Viewer comments are excluded.
- Tesseract ships only English, Ukrainian, and Russian language data.
- Environment variables remain visible to privileged LXC/Docker administrators.
- Third parties receive URLs/content according to their own policies.
- No global Telegram webhook exclusivity check exists; another long poller using
  the same token would conflict.

## Questions anticipated

- Why not send the YouTube URL directly to GLM? Fetching is unreliable and gives
  the model an uncontrolled network dependency. Local bounded extraction makes
  evidence and cleanup auditable.
- Why not use plain GLM-5.3 for images? Z.AI documents it as text-only. The bot
  uses OCR/captions with it and uses 5.3 Flash for actual visual input.
- Why no database/cache? Privacy and operational simplicity outweigh repeated
  inference cost for the expected personal workload.
- Can the recipe be exact? Only when the source evidence is exact. Unobserved
  values are deliberately marked uncertain.
- What survives a restart? Nothing except the immutable image and external
  service configuration; queued work is intentionally disposable.
