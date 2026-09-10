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

The model returns JSON containing `title`, `summary`, `servings`, `time`,
`ingredients[]`, `steps[]`, `assumptions[]`, `warnings[]`, and overall
`confidence`. Each ingredient contains `quantity`, `item`, optional `notes`, and
`confidence`. Confidence is exactly `high`, `medium`, or `low`. The application
validates this contract before sending anything to Telegram.

## Accuracy policy

Evidence priority is explicit captions/onscreen text, observable action, source
description, then culinary inference. Unsupported exact values become "not
shown", "as needed", or an honest range. Low-confidence conclusions are marked
and explained. Raw protein, allergens, cross-contamination, and doneness risks
are surfaced when relevant.

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

