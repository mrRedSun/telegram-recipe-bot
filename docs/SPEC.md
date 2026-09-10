# Product and engineering specification

## Problem

A permitted Telegram user sends one public YouTube video or Short URL. The bot
collects bounded evidence from that video and returns a usable recipe while
clearly distinguishing what the source showed from what the model inferred.

## Goals and acceptance criteria

1. Accept canonical YouTube, `youtu.be`, `/watch`, and `/shorts/` URLs; reject
   every other host, playlists, malformed IDs, URL credentials, and non-HTTP(S)
   schemes before invoking a downloader.
2. Allow only configured Telegram user IDs. There is deliberately no public or
   wildcard mode.
3. Download one public, non-live video of at most 240 seconds and 100 MiB by
   default. Extract metadata, available English/Ukrainian/Russian subtitles,
   up to five uploader-authored comments from a bounded top-comment sample,
   OCR, and at most 12 resized frames.
4. Use Z.AI's OpenAI-compatible Chat Completions endpoint. `glm-5.3-flash`
   receives the textual evidence and frames; text-only `glm-5.3` receives the
   textual evidence only.
5. Require structured recipe output with per-ingredient and overall confidence,
   assumptions, and food-safety warnings. Never silently invent exact values.
6. Acknowledge immediately, enforce a bounded queue and worker count, impose a
   five-minute job deadline, and replace the acknowledgement with the result or
   a useful bounded error.
7. Retain no application data. Job files live only in a private tmpfs directory
   and are removed after success, failure, cancellation, or timeout.
8. Ship a non-root, read-only Docker service with no capabilities, no published
   port, a health check, graceful shutdown, tests, CI, vulnerability scanning,
   and immutable GHCR commit tags.

## Non-goals

- Downloading private, age-restricted, DRM-protected, live, or playlist media.
- Producing a guaranteed copy of the creator's recipe when evidence is missing.
- Nutrition or medical advice, calorie estimation, long-term history, social
  features, or bypassing YouTube controls.
- Making Z.AI or Telegram private: both are third-party processors.

## User interaction

- `/start`, `/help`: concise usage.
- `/privacy`: retention and third-party processing disclosure.
- `/status`: liveness confirmation.
- A single YouTube URL: acknowledgement, then recipe or failure message.
- Unauthorized messages: ignored so the bot does not disclose its behavior.

## Recipe contract

The model returns JSON containing `title`, `summary`, `yield`, structured
`times` (`prep`, `cook`, and `total`), `ingredients[]`, `equipment[]`,
structured `steps[]`, `assumptions[]`, `warnings[]`, and overall `confidence`.
Each ingredient separates `amount`, `unit`, `item`, and `preparation`; each step
separates its `instruction`, optional `duration`, optional `temperature`, and
confidence. Confidence is exactly `high`, `medium`, or `low` at ingredient,
step, and recipe level. The application validates the contract and entry counts
before rendering it.

Telegram output uses supported HTML message entities: bold and underlined
headings, italic metadata, visible safety blockquotes, and an expandable
uncertainty blockquote. Telegram has no native table entity, so overview and
ingredient tables use escaped, fixed-width `<pre>` blocks of about 38
monospaced characters per row for practical mobile viewing. Model output never
supplies markup, and
all dynamic content is HTML-escaped. The complete message stays below
Telegram's 4,096-character limit.

## Accuracy policy

Evidence priority is explicit source description and uploader-authored comments
(pinned first), explicit captions/onscreen text, observable action, then culinary
inference. General viewer comments are never model evidence. Unsupported exact values become "not
shown", "as needed", or an honest range. Low-confidence conclusions are marked
and explained. Raw protein, allergens, cross-contamination, and doneness risks
are surfaced when relevant.

Comment collection is best effort and independent of the media download. It
sorts by YouTube's top ordering, inspects at most 50 comments (40 parent comments
and 10 replies), selects only entries explicitly marked by YouTube as authored
by the uploader, deduplicates them, places pinned entries first, and limits the
result to five comments and 6,000 characters. Disabled comments, rate limits,
the 45-second comment-stage deadline, or comment extraction failures do not fail
the recipe job. A deeply buried
uploader comment can therefore be missed by design.

## Capacity and failure model

Default capacity is two active jobs plus eight queued jobs. Each job may consume
up to 100 MiB download space and 12 JPEG frames, inside a 512 MiB tmpfs shared
by the container. Queue overflow fails fast. Network calls, downloader retries,
media duration, output size, HTTP bodies, Telegram messages, and job runtime are
bounded. Restart loses queued jobs by design; users can resubmit.

## Model compatibility decision

Z.AI documents `glm-5.3` as text-only and `glm-5.3-flash` as multimodal. The
default is therefore `glm-5.3-flash`, the only member of the requested 5.3
family that can inspect frames. Plain `glm-5.3` remains supported for users who
prefer it, but its accuracy depends heavily on captions and OCR.
