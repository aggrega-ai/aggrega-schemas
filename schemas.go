// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Package schemas exposes the canonical JSON Schema bytes for every
// Aggrega entity kind via package-local //go:embed. Consumers import
// the package and read the string-valued exported identifier for the
// schema they need; conversion to []byte at the call site is the
// consumer's responsibility (one allocation per validator-build).
//
// Naming convention: identifiers are PascalCase and carry a
// V1beta1 suffix matching the file name's `-v1beta1.schema.json`
// suffix. A single carve-out applies to the two-letter `ir-...`
// schema, which gets an explicit `Schema` separator word
// (IRSchemaV1beta1) to disambiguate the two-letter-initialism parse
// ambiguity (`IRV1beta1` reads as either `IR_V1beta1` or
// `IRV_1beta1`). The package namespace `schemas.` already provides
// "this is a schema" context for every other identifier.
//
// The string-vs-[]byte choice matches the pre-existing
// WorkloadJSONSchema string convention at
// aggrega-cli/pkg/schemas/jsonschema/embed.go: strings are
// immutable, so consumers cannot mutate the embedded source by
// accident.
//
// Each schema declares its own //go:embed directive rather than
// sharing a single embed.FS: this makes accessors zero-error (no
// (bytes, error) return), makes a missing schema file a
// compile-time error rather than a runtime error, and lets
// consumers reference the var directly. Adding a new schema is one
// extra line of Go.
//
// A companion embed.FS (schemaFiles) mirrors the same json/ tree for
// callers that need to resolve a schema dynamically by kind token
// rather than by a compile-time identifier — see SchemaByKind and
// Kinds. The FS is the single source for the kind→schema mapping: the
// kind token is the file stem (the file name minus the
// "-v1beta1.schema.json" suffix), so a new schema file joins the
// resolvable set automatically with no lookup table to maintain.
package schemas

import (
	"embed"
	"strings"
)

//go:embed json/aggrega-config-v1beta1.schema.json
var AggregaConfigV1beta1 string

//go:embed json/aggregate-v1beta1.schema.json
var AggregateV1beta1 string

//go:embed json/artifactlibrary-v1beta1.schema.json
var ArtifactLibraryV1beta1 string

//go:embed json/cache-metadata-v1beta1.schema.json
var CacheMetadataV1beta1 string

//go:embed json/component-v1beta1.schema.json
var ComponentV1beta1 string

//go:embed json/entity-lock-v1beta1.schema.json
var EntityLockV1beta1 string

//go:embed json/feature-v1beta1.schema.json
var FeatureV1beta1 string

//go:embed json/featurerecipe-v1beta1.schema.json
var FeatureRecipeV1beta1 string

//go:embed json/ir-v1beta1.schema.json
var IRSchemaV1beta1 string

//go:embed json/lockfile-v1beta1.schema.json
var LockfileV1beta1 string

//go:embed json/manifest-v1beta1.schema.json
var ManifestV1beta1 string

//go:embed json/mappinglibrary-v1beta1.schema.json
var MappingLibraryV1beta1 string

//go:embed json/path-template-context-v1beta1.schema.json
var PathTemplateContextV1beta1 string

//go:embed json/plugin-v1beta1.schema.json
var PluginV1beta1 string

//go:embed json/plugins-trust-v1beta1.schema.json
var PluginsTrustV1beta1 string

//go:embed json/recipe-v1beta1.schema.json
var RecipeV1beta1 string

//go:embed json/release-v1beta1.schema.json
var ReleaseV1beta1 string

//go:embed json/schemalibrary-v1beta1.schema.json
var SchemaLibraryV1beta1 string

//go:embed json/serve-wire-v1beta1.schema.json
var ServeWireV1beta1 string

//go:embed json/template-context-v1beta1.schema.json
var TemplateContextV1beta1 string

//go:embed json/wasm-cli-v1beta1.schema.json
var WasmCliV1beta1 string

//go:embed json/wasm-describe-v1beta1.schema.json
var WasmDescribeV1beta1 string

//go:embed json/wasm-diagnostic-v1beta1.schema.json
var WasmDiagnosticV1beta1 string

//go:embed json/wasm-hostfunc-error-v1beta1.schema.json
var WasmHostfuncErrorV1beta1 string

//go:embed json/wasm-linter-v1beta1.schema.json
var WasmLinterV1beta1 string

//go:embed json/wasm-annotation-transform-v1beta1.schema.json
var WasmAnnotationTransformV1beta1 string

//go:embed json/wasm-mutator-v1beta1.schema.json
var WasmMutatorV1beta1 string

//go:embed json/wasm-policy-v1beta1.schema.json
var WasmPolicyV1beta1 string

//go:embed json/wasm-provider-v1beta1.schema.json
var WasmProviderV1beta1 string

//go:embed json/wasm-frp-step-v1beta1.schema.json
var WasmFrpStepV1beta1 string

//go:embed json/wasm-import-v1beta1.schema.json
var WasmImportV1beta1 string

//go:embed json/wasm-parse-v1beta1.schema.json
var WasmParseV1beta1 string

//go:embed json/workload-v1beta1.schema.json
var WorkloadV1beta1 string

//go:embed json/workloadlibrary-v1beta1.schema.json
var WorkloadLibraryV1beta1 string

//go:embed json/*.schema.json
var schemaFiles embed.FS

// schemaFileSuffix is the common tail every schema file carries. Trimming
// it from a file name yields the kind token; appending it to a kind token
// yields the file name. The version segment is part of the suffix because
// the served identity of every schema is version-pinned at v1beta1.
const schemaFileSuffix = "-v1beta1.schema.json"

// SchemaByKind returns the canonical JSON Schema for the given kind token
// and true, or the empty string and false when no schema matches. The kind
// token is the lowercase-hyphenated file stem — "component", "aggregate",
// "ir", "template-context", "wasm-provider", and so on. The returned string
// is byte-identical to the distributed schema file. Callers that already
// know the kind at compile time should reference the exported string var
// directly; SchemaByKind serves the dynamic-lookup path where the kind is
// data.
func SchemaByKind(kind string) (string, bool) {
	data, err := schemaFiles.ReadFile("json/" + kind + schemaFileSuffix)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// Kinds returns every resolvable kind token, sorted lexicographically. The
// list is derived from the embedded json/ tree, so it always reflects the
// schemas actually shipped in the binary. embed.FS.ReadDir yields entries
// sorted by file name; because every file shares schemaFileSuffix, that
// order is identical to the kind-token order, so the result is deterministic
// by construction without an explicit sort.
func Kinds() []string {
	entries, err := schemaFiles.ReadDir("json")
	if err != nil {
		return nil
	}
	kinds := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, schemaFileSuffix) {
			kinds = append(kinds, strings.TrimSuffix(name, schemaFileSuffix))
		}
	}
	return kinds
}
