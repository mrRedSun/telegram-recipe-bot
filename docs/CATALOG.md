# Local recipe catalogue

The optional catalogue retrieves similar recipes for the video reconstruction job.
It does not train or replace GLM. With `RECIPE_CATALOG_PATH` unset, the existing
video-only workflow makes one inference call and requires no database.

With a catalogue configured, a bounded GLM query extracts an English dish name
and distinctive observed ingredients from creator text/OCR and one sampled frame.
The query is used only in memory. SQLite FTS retrieves up to three matching
reference recipes. The main inference receives those recipes alongside the video
evidence. Explicit video details take priority; unrelated or conflicting recipes
are discarded. Reference-only ingredients must not be introduced. Missing amounts
require an explicit amount and unit in the reference text. Recipe-wide times and
yield are never filled from references. A failed query/search or no suitable match
preserves ordinary video-only inference.

Each estimated ingredient/step carries reference IDs. Application code rejects
fabricated IDs, binds titles/URLs/licences from the catalogue rather than model
output, forces estimated fields to low confidence, and caps overall confidence
at medium when estimates are used. Cards mark `reference estimate [1]` and link
the used source recipes. Unused search candidates are not shown as citations.

## Initial data

The initial source is Food.com - Recipes and Reviews, published on Kaggle by
`irkaal`, snapshot 2020-12-28:
https://www.kaggle.com/datasets/irkaal/foodcom-recipes-and-reviews

The publisher lists CC0: Public Domain. Retain this source notice, snapshot,
source checksum, and individual Food.com recipe links with the derived database.
Only the `recipes.csv` member is imported: no reviews, author profiles, photos,
nutrition claims, or user conversations. The catalogue is English; the query
planner translates source evidence into English for retrieval.

Upstream quantity vectors often differ in length from ingredient vectors and
omit units. **All of those quantity vectors are discarded**, including aligned
ones; they cannot establish trustworthy amounts. Ingredient names and cooking
instructions remain available. Explicit amounts appearing in instruction text
can support labelled estimates. The 2020 snapshot can have stale links or errors;
matching establishes similarity, not verification of a creator's recipe.

RecipeNLG was considered but not imported: its terms restrict use to
non-commercial research and education. Do not treat its public download or a
mirror's licence label as unrestricted permission.

## Import on the development machine

Python 3.11+ with standard-library sqlite3 and FTS5 is required; no Python packages
or database server are needed. The Go binaries remain statically built. The small
Python search helper is embedded in the Go binary; the corpus is separate data
and must never enter Git, a build context, or an image layer.

After obtaining the source CSV and recording its checksum:

```sh
python3 tools/catalog/build.py --input /path/recipes.csv --output /path/catalog-v1.sqlite
python3 tools/catalog/benchmark.py /path/catalog-v1.sqlite
```

`--limit 10000` builds a sample. `--format jsonl` accepts normalized records with
`title`, `ingredients` (string list), `steps` (string list), and HTTPS `url`;
set `--dataset` and `--license` explicitly for another source. Only import sources
whose terms support the intended use. Do not import private Telegram data.

The importer streams rows, parses R string vectors without eval, rejects invalid
or excessive records, deduplicates normalized recipe content, builds the FTS
index, runs SQLite integrity checks, and records input SHA-256 and schema metadata.
It exclusively creates its output and refuses to overwrite anything. An incomplete
file is retained for diagnosis and fails runtime validation. Successful files are
mode 0444 and immutable by operating procedure. Do not rebuild a mounted file;
produce a new version, preserve the old file, and switch the mount.

## Run with a separate read-only database

Provision an immutable file outside the image. For the optional Compose override:

```sh
export RECIPE_CATALOG_FILE=/absolute/path/catalog-v1.sqlite
# IMAGE_TAG and normal runtime credentials still come from the standard deployment.
docker compose -f compose.yaml -f compose.catalog.yaml config --quiet
docker compose -f compose.yaml -f compose.catalog.yaml up -d
```

The override mounts only that file at `/data/recipes.sqlite`, read-only, refuses
missing host paths, and sets `RECIPE_CATALOG_PATH`. The non-root app user must be
able to read the file. Existing tmpfs video storage and dropped capabilities
remain unchanged. No service, port, GPU, or vector database is introduced.

Startup validates the configured file and schema and fails clearly when the
configured catalogue is missing/invalid. Each search gets an 8 MiB SQLite cache,
a 2.5 second query budget, a 3 second process deadline, bounded stdin/stdout, and
at most 24 KiB of recipe text. The query planner has a 20 second deadline and
512 output tokens with thinking disabled; normal reconstruction retains its
existing thinking/output budget and job timeout. This adds one small model call
when enabled. Corpus updates are explicit operator actions, not runtime downloads.

## Verification and rollback

Run `make fmt vet test race catalog-test` and `govulncheck` before release. Tests
cover relevance/abstention, read-only concurrent queries, malformed catalogues,
parser safety, deduplication, fallback, forged citations, escaped source links,
and provenance/confidence. CI also checks runtime Python SQLite FTS5 support.
`benchmark.py` uses developer-authored sample dish queries and reports retrieval
latency and candidate availability; it is not an accuracy claim about recipes.

`recipeprobe VIDEO_URL` with the catalogue configured exercises the full live path
without Telegram delivery. Require `recipe_validated=true`, cleanup confirmation,
and examine candidate/reference/estimate counts. Log only counts, fixed reasons,
and timings; never log user search phrases, matched recipe text, or reasoning.

Back up exact Compose/environment settings under the deployment `.backups` root
before enabling. Roll back by restoring the previous Compose pin/settings and
recreating only recipebot. Retain previous catalogue versions and images.
Disabling the mount/path restores video-only inference without modifying data.
