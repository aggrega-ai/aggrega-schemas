// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package schemas

import (
	"encoding/json"
	"strings"
	"testing"
)

// kindFromID derives the kind token from a canonical $id URI by stripping
// the served-path prefix and the file suffix, mirroring the file-stem rule
// SchemaByKind and Kinds resolve against.
func kindFromID(id string) string {
	name := strings.TrimPrefix(id, "https://aggrega.ai/schemas/")
	return strings.TrimSuffix(name, schemaFileSuffix)
}

// TestSchemaByKindMatchesExportedVars proves the FS-backed dynamic accessor
// returns exactly the same bytes as the compile-time-bound exported var for
// every schema. The two surfaces embed the same files independently; this
// parity check is the guard that they cannot silently diverge.
func TestSchemaByKindMatchesExportedVars(t *testing.T) {
	t.Parallel()
	for _, s := range allSchemas {
		s := s
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			kind := kindFromID(s.expectedID)
			got, ok := SchemaByKind(kind)
			if !ok {
				t.Fatalf("SchemaByKind(%q) reported not found; the FS accessor and the exported var disagree on availability", kind)
			}
			if got != s.source {
				t.Errorf("SchemaByKind(%q) bytes differ from the exported var %s", kind, s.name)
			}
		})
	}
}

// TestKindsCoversEveryExportedVar proves Kinds enumerates exactly the kind
// set the exported vars cover — no missing kind, no extra kind — and that the
// list is lexicographically ordered for deterministic output.
func TestKindsCoversEveryExportedVar(t *testing.T) {
	t.Parallel()
	kinds := Kinds()

	if len(kinds) != len(allSchemas) {
		t.Fatalf("Kinds() returned %d kinds; expected %d (one per exported var)", len(kinds), len(allSchemas))
	}

	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	for _, s := range allSchemas {
		if kind := kindFromID(s.expectedID); !set[kind] {
			t.Errorf("Kinds() omits %q (exported var %s)", kind, s.name)
		}
	}

	for i := 1; i < len(kinds); i++ {
		if kinds[i-1] >= kinds[i] {
			t.Errorf("Kinds() is not strictly lexicographically ordered at index %d: %q >= %q", i, kinds[i-1], kinds[i])
		}
	}
}

// TestSchemaByKindUnknownKind proves an unmatched kind is a clean miss, not a
// panic or a stray empty-string hit.
func TestSchemaByKindUnknownKind(t *testing.T) {
	t.Parallel()
	if got, ok := SchemaByKind("no-such-kind"); ok || got != "" {
		t.Errorf("SchemaByKind(unknown) = (%q, %v); want (\"\", false)", got, ok)
	}
}

// allSchemas enumerates every exported schema identifier paired
// with its canonical `$id` URI. The three table-driven tests below
// iterate this slice; adding a new schema requires one row here and
// one row in schemas.go.
//
// Every schema's `$id` uses the flat `https://aggrega.ai/schemas/<file-name>`
// form, matching its filename-derived served path.
//
// The machine schemas fall in two roles: wire formats and ABI contracts
// (ir, lockfile, plugins-trust, cache-metadata, the wasm-*), and
// compiler-internal render-context shapes (template-context,
// path-template-context). That distinction is convention, not encoded in
// the `$id`. What marks all of
// them non-published is their exclusion from catalog.json — none is
// hand-authored or editor-associated — so the `$id` need not carry the
// role, and identity matches the served location for every schema.
var allSchemas = []struct {
	name       string
	source     string
	expectedID string
}{
	{"AggregaConfigV1beta1", AggregaConfigV1beta1, "https://aggrega.ai/schemas/aggrega-config-v1beta1.schema.json"},
	{"AggregateV1beta1", AggregateV1beta1, "https://aggrega.ai/schemas/aggregate-v1beta1.schema.json"},
	{"ArtifactLibraryV1beta1", ArtifactLibraryV1beta1, "https://aggrega.ai/schemas/artifactlibrary-v1beta1.schema.json"},
	{"CacheMetadataV1beta1", CacheMetadataV1beta1, "https://aggrega.ai/schemas/cache-metadata-v1beta1.schema.json"},
	{"ComponentV1beta1", ComponentV1beta1, "https://aggrega.ai/schemas/component-v1beta1.schema.json"},
	{"EntityLockV1beta1", EntityLockV1beta1, "https://aggrega.ai/schemas/entity-lock-v1beta1.schema.json"},
	{"FeatureV1beta1", FeatureV1beta1, "https://aggrega.ai/schemas/feature-v1beta1.schema.json"},
	{"FeatureRecipeV1beta1", FeatureRecipeV1beta1, "https://aggrega.ai/schemas/featurerecipe-v1beta1.schema.json"},
	{"IRSchemaV1beta1", IRSchemaV1beta1, "https://aggrega.ai/schemas/ir-v1beta1.schema.json"},
	{"LockfileV1beta1", LockfileV1beta1, "https://aggrega.ai/schemas/lockfile-v1beta1.schema.json"},
	{"ManifestV1beta1", ManifestV1beta1, "https://aggrega.ai/schemas/manifest-v1beta1.schema.json"},
	{"MappingLibraryV1beta1", MappingLibraryV1beta1, "https://aggrega.ai/schemas/mappinglibrary-v1beta1.schema.json"},
	{"PathTemplateContextV1beta1", PathTemplateContextV1beta1, "https://aggrega.ai/schemas/path-template-context-v1beta1.schema.json"},
	{"PluginV1beta1", PluginV1beta1, "https://aggrega.ai/schemas/plugin-v1beta1.schema.json"},
	{"PluginsTrustV1beta1", PluginsTrustV1beta1, "https://aggrega.ai/schemas/plugins-trust-v1beta1.schema.json"},
	{"RecipeV1beta1", RecipeV1beta1, "https://aggrega.ai/schemas/recipe-v1beta1.schema.json"},
	{"ReleaseV1beta1", ReleaseV1beta1, "https://aggrega.ai/schemas/release-v1beta1.schema.json"},
	{"SchemaLibraryV1beta1", SchemaLibraryV1beta1, "https://aggrega.ai/schemas/schemalibrary-v1beta1.schema.json"},
	{"ServeWireV1beta1", ServeWireV1beta1, "https://aggrega.ai/schemas/serve-wire-v1beta1.schema.json"},
	{"TemplateContextV1beta1", TemplateContextV1beta1, "https://aggrega.ai/schemas/template-context-v1beta1.schema.json"},
	{"WasmCliV1beta1", WasmCliV1beta1, "https://aggrega.ai/schemas/wasm-cli-v1beta1.schema.json"},
	{"WasmDescribeV1beta1", WasmDescribeV1beta1, "https://aggrega.ai/schemas/wasm-describe-v1beta1.schema.json"},
	{"WasmDiagnosticV1beta1", WasmDiagnosticV1beta1, "https://aggrega.ai/schemas/wasm-diagnostic-v1beta1.schema.json"},
	{"WasmHostfuncErrorV1beta1", WasmHostfuncErrorV1beta1, "https://aggrega.ai/schemas/wasm-hostfunc-error-v1beta1.schema.json"},
	{"WasmLinterV1beta1", WasmLinterV1beta1, "https://aggrega.ai/schemas/wasm-linter-v1beta1.schema.json"},
	{"WasmAnnotationTransformV1beta1", WasmAnnotationTransformV1beta1, "https://aggrega.ai/schemas/wasm-annotation-transform-v1beta1.schema.json"},
	{"WasmMutatorV1beta1", WasmMutatorV1beta1, "https://aggrega.ai/schemas/wasm-mutator-v1beta1.schema.json"},
	{"WasmPolicyV1beta1", WasmPolicyV1beta1, "https://aggrega.ai/schemas/wasm-policy-v1beta1.schema.json"},
	{"WasmProviderV1beta1", WasmProviderV1beta1, "https://aggrega.ai/schemas/wasm-provider-v1beta1.schema.json"},
	{"WasmFrpStepV1beta1", WasmFrpStepV1beta1, "https://aggrega.ai/schemas/wasm-frp-step-v1beta1.schema.json"},
	{"WasmImportV1beta1", WasmImportV1beta1, "https://aggrega.ai/schemas/wasm-import-v1beta1.schema.json"},
	{"WasmParseV1beta1", WasmParseV1beta1, "https://aggrega.ai/schemas/wasm-parse-v1beta1.schema.json"},
	{"WorkloadV1beta1", WorkloadV1beta1, "https://aggrega.ai/schemas/workload-v1beta1.schema.json"},
	{"WorkloadLibraryV1beta1", WorkloadLibraryV1beta1, "https://aggrega.ai/schemas/workloadlibrary-v1beta1.schema.json"},
}

// TestEverySchemaIsNonEmpty asserts every exported identifier has
// len > 0. Catches the embed-bind-failed regression where a missing
// file would result in an empty string; the Go compiler treats a
// missing //go:embed source as a compile-time error so this is
// largely defensive, but the test pins the contract at the test
// boundary.
func TestEverySchemaIsNonEmpty(t *testing.T) {
	t.Parallel()
	for _, s := range allSchemas {
		s := s
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			if len(s.source) == 0 {
				t.Errorf("%s: schema source is empty; //go:embed binding failed", s.name)
			}
		})
	}
}

// TestEverySchemaParsesAsValidJSON asserts every embedded schema is
// well-formed JSON. Catches the malformed-JSON-published regression
// — a corrupted schema file in json/ would surface here
// before consumers hit a runtime json.Unmarshal failure.
func TestEverySchemaParsesAsValidJSON(t *testing.T) {
	t.Parallel()
	for _, s := range allSchemas {
		s := s
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			var doc json.RawMessage
			if err := json.Unmarshal([]byte(s.source), &doc); err != nil {
				t.Errorf("%s: source is not valid JSON: %v", s.name, err)
			}
		})
	}
}

// TestEverySchemaHasExpectedID asserts each schema's $id field
// matches the canonical https://aggrega.ai/schemas/<file-name> form.
// This is the specific test that would have caught the workload.json
// drift had it existed: a drift where one copy's $id doesn't match
// the file name surfaces immediately. Catches schema-file-misnaming
// and embed-binding-wrong-file regressions in one assertion.
func TestEverySchemaHasExpectedID(t *testing.T) {
	t.Parallel()
	for _, s := range allSchemas {
		s := s
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			var envelope struct {
				ID string `json:"$id"`
			}
			if err := json.Unmarshal([]byte(s.source), &envelope); err != nil {
				t.Fatalf("%s: failed to parse for $id check: %v", s.name, err)
			}
			if envelope.ID != s.expectedID {
				t.Errorf("%s: $id mismatch.\n  got:  %q\n  want: %q", s.name, envelope.ID, s.expectedID)
			}
		})
	}
}
