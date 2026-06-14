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

`scripts/editor-association-smoke.sh` serves `json/` locally by filename (standing
in for the hub static path), confirms a served URL returns the schema bytes, and
validates a known-good and a known-bad sample entity against the served schema. See
the script header for the validator it selects.

## Go-live (infrastructure)

Standing up `https://hub.aggrega.ai/schemas/` (the hub Worker serving `/schemas/*`
statically) is infrastructure — the same hosting effort as the public
`hub.aggrega.ai` OCI registry, and shared with the `hub-registry-hosting-and-signing`
work. Go-live is therefore an infrastructure action, not a step performed in this
repository: this repo provides the artifacts (`json/`), the editor-association
catalog, and this publish path; standing up the hub serving (and the exact
`/schemas/<filename>` route plus any cross-schema `$ref` resolution) lands with that
hosting work.
