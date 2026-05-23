// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

// Package schemas exposes the canonical JSON Schema bytes for every
// Aggrega entity kind via package-local //go:embed. Consumers import
// the package and read the string-valued exported identifier for the
// schema they need; conversion to []byte at the call site is the
// consumer's responsibility (one allocation per validator-build).
//
// Naming convention: identifiers are PascalCase and carry a
// V1alpha1 suffix matching the file name's `-v1alpha1.schema.json`
// suffix. A single carve-out applies to the two-letter `ir-...`
// schema, which gets an explicit `Schema` separator word
// (IRSchemaV1alpha1) to disambiguate the two-letter-initialism parse
// ambiguity (`IRV1alpha1` reads as either `IR_V1alpha1` or
// `IRV_1alpha1`). The package namespace `schemas.` already provides
// "this is a schema" context for the other 17 identifiers.
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
package schemas

import _ "embed"

//go:embed json-schemas/aggrega-config-v1alpha1.schema.json
var AggregaConfigV1alpha1 string

//go:embed json-schemas/aggregate-v1alpha1.schema.json
var AggregateV1alpha1 string

//go:embed json-schemas/cache-metadata-v1alpha1.schema.json
var CacheMetadataV1alpha1 string

//go:embed json-schemas/component-v1alpha1.schema.json
var ComponentV1alpha1 string

//go:embed json-schemas/feature-v1alpha1.schema.json
var FeatureV1alpha1 string

//go:embed json-schemas/featurerecipe-v1alpha1.schema.json
var FeatureRecipeV1alpha1 string

//go:embed json-schemas/ir-v1alpha1.schema.json
var IRSchemaV1alpha1 string

//go:embed json-schemas/lockfile-v1alpha1.schema.json
var LockfileV1alpha1 string

//go:embed json-schemas/manifest-v1alpha1.schema.json
var ManifestV1alpha1 string

//go:embed json-schemas/mappinglibrary-v1alpha1.schema.json
var MappingLibraryV1alpha1 string

//go:embed json-schemas/path-template-context-v1alpha1.schema.json
var PathTemplateContextV1alpha1 string

//go:embed json-schemas/provider-v1alpha1.schema.json
var ProviderV1alpha1 string

//go:embed json-schemas/recipe-v1alpha1.schema.json
var RecipeV1alpha1 string

//go:embed json-schemas/release-v1alpha1.schema.json
var ReleaseV1alpha1 string

//go:embed json-schemas/schemalibrary-v1alpha1.schema.json
var SchemaLibraryV1alpha1 string

//go:embed json-schemas/template-context-v1alpha1.schema.json
var TemplateContextV1alpha1 string

//go:embed json-schemas/wasm-linter-v1alpha1.schema.json
var WasmLinterV1alpha1 string

//go:embed json-schemas/wasm-mutator-v1alpha1.schema.json
var WasmMutatorV1alpha1 string

//go:embed json-schemas/wasm-policy-validator-v1alpha1.schema.json
var WasmPolicyValidatorV1alpha1 string

//go:embed json-schemas/workload-v1alpha1.schema.json
var WorkloadV1alpha1 string
