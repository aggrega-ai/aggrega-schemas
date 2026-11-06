# aggrega-schemas

This repository holds the JSON Schemas that define [Aggrega](https://aggrega.ai)'s
entities, plugin payloads and workspace files, one file per kind in `json/`.

It is also the Go module that carries them into the compiler: `schemas.go`
embeds every file in `json/`, so the `aggrega` binary validates against the
schemas it was built with.
[`aggrega get-schema`](https://aggrega.ai/docs/language-reference/cli/aggrega-get-schema/)
prints the schema for a kind from the binary you have installed, and the
[Schema reference](https://aggrega.ai/docs/language-reference/schema/) has one
page per schema in this repository, generated from it.

`catalog.json` maps the file names of Aggrega YAML documents to the address each
schema is served from, so a YAML language server in an editor can associate a
document with its schema; [PUBLISHING.md](PUBLISHING.md) describes how the
schemas are served.

## License

The Aggrega JSON Schemas are released under the Apache License, Version 2.0; the full text is in the
[`LICENSE`](LICENSE) file.
