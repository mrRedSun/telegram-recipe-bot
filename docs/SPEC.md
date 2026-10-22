# Product and engineering specification

## Problem

A permitted Telegram user needs three privacy-minimal behaviors from one bot:
turn one public YouTube video into a structured recipe, hold a bounded general
conversation through the existing GLM provider, and return media from one public
Instagram post or Reel without credentials or persistent storage.

## Goals and acceptance criteria

1. Preserve exact YouTube URL validation and the existing recipe workflow,
   evidence extraction, structured output, confidence policy, progress UX, and
   `/recipe` command semantics.
2. Allow only configured Telegram user IDs. There is deliberately no public or
   wildcard mode.
3. Add a general GLM conversational assistant. Direct-message text and captions
   are conversational by default. Groups and supergroups require `/ask`, an
   exact bot mention, or a reply to the bot.
4. Supply GLM with at most the latest 20 relevant user and assistant messages,
   scoped independently by Telegram chat ID and forum topic ID. Bound each
   message and the number of resident scopes. Keep context in memory only; reset
   it with `/reset` and lose it on restart.
5. Recognize exact public Instagram `/p/`, `/reel/`, and `/reels/` URLs after the
   interaction policy is satisfied. Resolve media anonymously, preserve carousel
   order, and return every item as Telegram-native photos, videos, or ordered
   media groups. Never treat an Instagram URL after `/recipe` as a media request.
6. Do not support private/login-gated content, stories, profiles, highlights,
   cookies, Instagram credentials, access-control bypasses, or persistent media.
7. Bound Instagram discovery to 20 items, each hosted Bot API photo to 10 MiB,
   each video to 50 MiB, aggregate job data to the configured download limit,
   extractor output to 1 MiB, redirects to five, network retries to two, and the
   complete job to the existing deadline. Split more than 10 items into ordered
   Telegram batches.
8. Use Z.AI's OpenAI-compatible Chat Completions endpoint for both recipes and
   conversation. Recipe mode remains structured JSON; conversation mode returns
   bounded plain text.
9. Retain no application data. Job files live only in a private tmpfs directory
   and are removed after success, failure, cancellation, or timeout.
10. Ship a non-root, read-only Docker service with no capabilities, no published
    port, health checking, graceful shutdown, tests, CI, vulnerability scanning,
    exact extractor versions, and immutable GHCR commit tags.

## Non-goals

- Downloading private, authenticated, story, profile, highlight, DRM-protected,
  live, or playlist media.
- Storing Instagram cookies, Telegram `file_id` values, chat history, analytics,
  or downloaded content.
- Re-encoding oversized Instagram media to evade Telegram limits.
- Producing a guaranteed copy of a creator's recipe when evidence is missing.
- Making Telegram, Instagram, YouTube, or Z.AI private; all are third-party
  processors.

## User interaction

- `/start`, `/help`: concise usage for all three workflows.
- `/privacy`: memory, cleanup, logging, and third-party disclosure.
- `/status`: liveness confirmation.
- `/reset`: clear the current DM/group/forum-topic context.
- `/recipe URL`: group-safe YouTube recipe request, including
  `/recipe@BotName URL`.
- `/ask TEXT`: explicit conversational or Instagram request.
- Direct-message ordinary text/caption: conversational request unless it contains
  a supported Instagram URL or is one valid YouTube URL.
- Group/supergroup ordinary messages: ignored unless they are an existing
  YouTube recipe URL delivered by Telegram, `/ask`, an exact mention, or a reply
  to the bot.
- Unauthorized messages: ignored so the bot does not disclose its behavior.

BotFather privacy mode may remain enabled because commands, mentions, and replies
are delivered. Sender authorization always uses `message.from.id`, never the
chat ID. Replies are sent into the originating `message_thread_id` when present.

## Conversation memory

A scope key is `(chat_id, message_thread_id)`. Non-forum chats use topic ID zero.
The in-memory store retains only `user` and `assistant` roles, trims text and
captions to 4,000 runes, keeps the newest 20 messages, and limits resident scopes
to 256. Arrival-order checks and per-scope locking keep simultaneous requests ordered
without blocking unrelated chats. Active scopes are pinned during requests; unique
generations prevent evicted entries from reviving old work. Reset generations plus per-scope cancellation remove queued prompt text, cancel
in-flight model requests, suppress their replies, and prevent old work from restoring
cleared history. No context field is emitted to logs or metrics.

## Instagram media contract

URL normalization accepts only exact `instagram.com` and `www.instagram.com`
hosts with one shortcode under `/p/`, `/reel/`, or `/reels/`. Query strings and fragments are removed during canonicalization; credentials,
profiles, and lookalike hosts are rejected before an extractor runs. Canonicalization converts `/reels/` to `/reel/`.

The exact-version-pinned `gallery-dl` process runs with configuration ignored,
short timeouts, two retries, a 21-item discovery guard, direct MP4 video selection, and URL-only output.
Stderr is discarded and stdout is bounded. Only HTTPS media URLs under
`cdninstagram.com` or `fbcdn.net` are accepted, including redirects. Downloads
are sequential, use generated numeric filenames, permit JPEG, PNG, and MP4 only,
and enforce per-file plus aggregate limits while streaming to mode-0600 files.
No extractor output, signed CDN URL, original URL, shortcode, username, caption,
or filename reaches logs.

One item uses `sendPhoto` or `sendVideo`. Two to ten items use
`sendMediaGroup`. Longer carousels are split into ordered batches of ten while
preserving source order. Multipart uploads stream from disk and do not buffer
whole files in memory.

## Recipe contract

The existing model contract remains unchanged: `title`, `summary`, `yield`,
structured times, ingredients, equipment, steps, assumptions, warnings, and
confidence. Telegram output continues to use Bot API 10.3 Rich Messages with
application-generated escaped markup.

## Capacity and failure model

Default capacity is two active jobs plus eight queued jobs shared across all
workflows. Each job has a five-minute deadline and uses a private directory under
the shared 512 MiB tmpfs. Queue overflow fails fast. Restart loses queued jobs
and conversation context by design.

Progress and diagnostic logs use only a process-local job sequence. They expose
workflow kind, stage, elapsed time, bounded counts/sizes, GLM finish reason, and
token counts. They never log Telegram identities, messages, captions, URLs,
conversation context, media URLs, recipe evidence, model responses, reasoning,
or credentials.

## Model compatibility decision

Z.AI documents `glm-5.3` as text-only and `glm-5.3-flash` as multimodal. Recipe
mode uses Flash frames when configured. Conversation mode sends text history to
either model. Both use the existing runtime key, endpoint, reasoning effort, and
request authentication pattern.
