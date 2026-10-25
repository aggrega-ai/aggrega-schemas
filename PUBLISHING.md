# Publishing the schemas

The JSON Schemas in `json/` are the language-neutral contract for every Aggrega
entity. They are consumed three ways: by the compiler (as a Go module — see
`schemas.go` — embedded and version-pinned, never fetched), by editors and
third-party tooling (by HTTPS URL), and by the hub UI. This document describes the
**URL** consumption path.

This repository is the single source of truth. The served copies are published
**as-is** (the schemastore model — published from the repo, never forked). No
transformation, no fork.

## Identity vs. serving

These are two different URLs and must not be conflated:

- **Identity (`$id`): `https://aggrega.ai/schemas/<name>`.** This is the schema's
  JSON Schema `$id` — a stable identifier, exactly like any JSON Schema `$id`.
  `aggrega.ai` is **not a serving host**; it serves nothing. The `$id` is never a
  fetch endpoint. (Leave every `json/` `$id` exactly as released.)
- **Serving: `https://hub.aggrega.ai/schemas/<filename>`.** The hub serves the
  schema **files** statically (the hub Worker serves `/schemas/*` as
  `application/json`). This is the URL editors and tooling actually fetch, and the
  URL `catalog.json` points at. The served path is derived from the **filename**
  (`json/<filename>` → `/schemas/<filename>`), not from the `$id`.

The hub (`hub.aggrega.ai`) is the one serving surface for the ecosystem: the OCI
registry at `/v2/…` (catalog entities + plugins) and the static schema path at
`/schemas/…` (this document). Schemas are served statically, not as OCI artifacts:
editors fetch plain HTTPS (no cosign), and the compiler uses the embedded,
version-pinned `aggrega-schemas` Go module (never the hub copy), so an
OCI-artifact + signed schema would have no verifier.

## Publish step

Place each `json/<filename>` at `https://hub.aggrega.ai/schemas/<filename>` — a
flat, as-is static copy keyed by filename. No `$id` rewrite, no per-schema
transformation. `catalog.json` (this repo) is the editor-association map; publish
it alongside (or fold its entries into a served catalog) so editors auto-associate
an entity file with its schema.

## Editor association

`catalog.json` is a SchemaStore-style catalog mapping user-authored entity-file
patterns to the **served hub URL** of their schema. A YAML language server consumes
it (as a custom catalog, or its entries mirrored into `yaml.schemas` settings) to
associate a schema with an entity file automatically; per-file association via a
`# yaml-language-server: $schema=https://hub.aggrega.ai/schemas/<filename>` modeline
also works. The catalog covers user-authored kinds only — the internal/machine
schemas are intentionally excluded.

## Smoke

The smoke has two layers. `scripts/editor-association-smoke.sh` serves `json/`
locally by filename (standing in for the hub static path) and confirms a served URL
returns the schema bytes byte-identical to source. It then drives the association
engine via `scripts/lsp-association-smoke.mjs`, which runs the `yaml-language-server`
language service — the engine editors use — configured from `catalog.json`:

- a good sample entity is associated to its schema by `fileMatch` and validates clean;
- a bad sample is rejected with a diagnostic pointing at the offending field;
- a file representing a machine-schema kind (one excluded from `catalog.json`) is
  not associated — the same service flags the bad sample but returns nothing for the
  excluded one, so the empty result is genuine non-association.

The harness reads the committed `catalog.json` and points its schema URLs at the
local server in memory only; the committed `url`s (the hub static path) are not
modified. Serving the whole `json/` directory resolves any cross-document `$ref`
locally. When `node` is unavailable, the shell smoke falls back to validating the
good and bad samples through the project's own JSON Schema validator; in that mode
the served-URL claims hold but the association engine is not driven.

## Go-live (infrastructure)

Standing up `https://hub.aggrega.ai/schemas/` (the hub Worker serving `/schemas/*`
statically) is infrastructure — the same hosting effort as the public
`hub.aggrega.ai` OCI registry. Go-live is therefore an infrastructure action, not
a step performed in this
repository: this repo provides the artifacts (`json/`), the editor-association
catalog, and this publish path; standing up the hub serving (and the exact
`/schemas/<filename>` route plus any cross-schema `$ref` resolution) lands with that
hosting work.
