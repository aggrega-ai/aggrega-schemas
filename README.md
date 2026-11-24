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
happens on `main`, and a fix to a released version on a `release/vX.Y` branch cut
from its tag; this check runs on every push to either and on every pull request
into `main`.

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
that matches no refusal is refused too when the schema does not declare that
property at that pointer, or when the release it is compared with has that
object without the property. It is kept when that release already declares the
property there, or lacks the object or the schema file, since a comparison with
an earlier release may still need it. A malformed file leaves nothing judged.
`-exceptions FILE` reads another file, and `-staged` reads both the schemas and
this file from the index.

The check needs the repository's full history and tags, and refuses to judge a
shallow clone. `go run ./cmd/schemagate compat -h` prints its flags. Built with
`go build ./cmd/schemagate`, the command exits 0 when the check passes, 1 when it
refuses, and 2 when it could not judge; `go run` reports any failure as 1.

## License

The Aggrega JSON Schemas are released under the Apache License, Version 2.0; the full text is in the
[`LICENSE`](LICENSE) file.
