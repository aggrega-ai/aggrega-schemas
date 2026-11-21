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

## Changing a schema

Within one API version a schema changes only additively: a description may
change and an optional property may be added where the object accepted no extra
property under its name, or where this repository's `compat-exceptions.json` says
why no document can have used it. Removing or renaming a property, making one
required, narrowing a type or an enum, or changing what a value means needs a new
API version.

`cmd/schemagate` holds the schemas to that rule, and runs from a clone of this
repository alone:

```sh
go run ./cmd/schemagate compat
```

compares every schema in `json/` with the same schema at the newest release tag
carrying its API version. It refuses a removed or renamed property, a property
added to `required`, a narrowed type, a removed enum member, an added or changed
`const` or `pattern`, a tightened `minimum`, `maximum` or `additionalProperties`,
a removed `$defs` entry, and any other change it does not classify as additive,
naming the schema file, the JSON pointer and the kind of change. It passes a
changed description and an optional property added where the object accepted no
extra property under that name. It refuses an optional property added where the
object accepted one before — its `additionalProperties` absent, `true` or a
schema, or the name matching one of its `patternProperties` — because a document
written before may already carry that name with a value the new property refuses.
A schema whose API version no release carries yet is compared with nothing and
passes, saying so; a clone with no release tag at all is not judged. Development
happens on `dev`, where this check runs on every push and pull request.

`compat-exceptions.json`, at the root of this repository, admits such an addition
when no document written before can have used the name. It is absent while
nothing needs admitting:

```json
{
  "exceptions": [
    {
      "schema": "component-v1beta1.schema.json",
      "pointer": "#/properties/spec/properties/labels",
      "property": "example",
      "reason": "Why no document written before can have used this name with another kind of value."
    }
  ]
}
```

Each entry names the schema file in `json/` by its file name, the JSON pointer of
the object and the property as the refusal prints them, and a reason. An entry
that matches no refusal is refused too, unless the schema and the release it is
compared with both declare that property at that pointer, as they do once a
release carries the admission, which a comparison with an earlier release still
needs. So the file keeps no admission nothing needs; a malformed file leaves
nothing judged. `-exceptions FILE` reads another
file, and `-staged` reads both the schemas and this file from the index.

```sh
go run ./cmd/schemagate freeze -head main -dev origin/dev
```

refuses any change `main` carries to a schema file since the newest commit it
shares with `dev`. `main` holds what was last released and moves only by a
release, which fast-forwards it to `dev`; this check runs on every push to it.
Both commands need the repository's full history and tags, and refuse to judge a
shallow clone. `go run ./cmd/schemagate -h` prints their flags. Built with
`go build ./cmd/schemagate`, the command exits 0 when the check passes, 1 when it
refuses, and 2 when it could not judge; `go run` reports any failure as 1.

## License

The Aggrega JSON Schemas are released under the Apache License, Version 2.0; the full text is in the
[`LICENSE`](LICENSE) file.
