# Product and engineering specification

## Contract

Allowlisted users send public YouTube videos/Shorts or Instagram Reels and receive
structured recipes. Both platforms use one recipe job, source evidence engine,
GLM inference/validation, progress UX, and Rich Message renderer. General GLM
conversation remains independently available. Standalone media delivery is removed.

## Routing

`/recipe URL` accepts either supported source. Bare valid video links are recipes
for both platforms. Relevant Instagram links in allowed DM text/captions and
explicit group requests are also recipes. Ordinary DM text is conversational;
groups require `/ask`, an exact bot mention, or a reply to the bot for conversation.
Commands for another bot and unauthorized senders are ignored. Forum replies stay
in the originating topic. `/reset` clears/cancels assistant work in that scope.

URL policy accepts exact YouTube/youtu.be hosts and direct Instagram `/reel/`,
`/reels/`, or single-video `/p/` links. Sharing queries are removed during
canonicalization. Profiles, stories, arbitrary hosts, URL credentials, playlists,
private/authenticated access, and access-control bypasses are excluded.

## One recipe pipeline

1. Normalize the source and obtain bounded anonymous yt-dlp metadata with config
   and cache disabled. Flatten exactly one video from Instagram playlist-shaped
   metadata; reject missing/multiple videos.
2. Preserve the selected video's metadata as a mode-0600 tmpfs snapshot. Download
   from that snapshot with yt-dlp, source size limits, timeouts, and retries.
3. Collect creator caption/description and available subtitles. YouTube creator
   comments remain optional and bounded; missing Instagram comments/subtitles do
   not discard the caption or video.
4. Verify actual duration with ffprobe, including missing source duration metadata.
   Reject zero/non-finite/excessive durations and oversized sources. Local media
   analysis allows only file/pipe protocols.
5. Extract up to 12 bounded-size frames and run OCR.
6. Run the existing GLM structured recipe inference with text and configured vision.
7. Validate and render the same escaped recipe card with the existing seven-stage UX.

## Limits and privacy

Defaults remain two active/eight queued jobs, 100 MiB source size, four-minute
video duration, five-minute job deadline, and private per-job directories in a
512 MiB tmpfs. Metadata stdout is capped at 2 MiB. Other command output and stderr
are capped at 64 KiB. Context cancellation terminates subprocesses. No source
media or recipe evidence persists after the job.

Logs correlate anonymous job IDs with source platform, stage, backend, outcome,
fixed reason, extractor exit/HTTP code, count/size, and elapsed time. Raw stderr,
metadata, source or CDN URLs, filenames, captions, identities, model response
content, and reasoning never reach logs.

Conversation context is independent of recipe jobs: in memory only, 20 messages
of up to 4,000 runes per chat/topic, at most 256 resident scopes, ordered requests,
pinned active entries, unique reset generations, and cancellation on reset.

## Verification

Unit/race tests cover source routing, identical pipeline stages, creator caption
preservation, one-video metadata flattening, missing/actual duration limits,
output bounds, anonymous diagnostics, recipe rendering for Instagram and YouTube,
and existing conversation controls. CI also performs vet, vulnerability and
secret scans, image build, and hardened runtime smoke checks.

`recipeprobe VIDEO_URL` exercises extraction, GLM inference, and recipe validation
for either platform without Telegram delivery. It prints only bounded diagnostics
and recipe counts/confidence, then confirms temporary-workspace cleanup.

### Optional local recipe catalogue

See [CATALOG.md](CATALOG.md) for data provenance, read-only mounting, configuration,
query/resource limits, fallback, attribution, and source-quality limits. Catalogue
text is untrusted reference material. It cannot override explicit video details,
introduce reference-only ingredients, or establish high-confidence fields. Only
used sources receive application-bound citations. No Telegram history or source
URLs are added to the catalogue, and runtime search phrases are never persisted.
