// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package schemas

import (
	"encoding/json"
	"testing"
)

// allSchemas enumerates every exported schema identifier paired
// with its canonical `$id` URI. The three table-driven tests below
// iterate this slice; adding a new schema requires one row here and
// one row in schemas.go.
//
// Every schema's `$id` uses the flat `https://aggrega.ai/schemas/<file-name>`
// form, matching its filename-derived served path.
//
// The machine schemas fall in two roles: wire formats and ABI contracts
// (ir, lockfile, cache-metadata, the wasm-*), and compiler-internal
// render-context shapes (template-context, path-template-context). That
// distinction is convention, not encoded in the `$id`. What marks all of
// them non-published is their exclusion from catalog.json — none is
// hand-authored or editor-associated — so the `$id` need not carry the
// role, and identity matches the served location for every schema.
var allSchemas = []struct {
	name       string
	source     string
	expectedID string
}{
	{"AggregaConfigV1alpha1", AggregaConfigV1alpha1, "https://aggrega.ai/schemas/aggrega-config-v1alpha1.schema.json"},
	{"AggregateV1alpha1", AggregateV1alpha1, "https://aggrega.ai/schemas/aggregate-v1alpha1.schema.json"},
	{"ComponentV1alpha1", ComponentV1alpha1, "https://aggrega.ai/schemas/component-v1alpha1.schema.json"},
	{"FeatureV1alpha1", FeatureV1alpha1, "https://aggrega.ai/schemas/feature-v1alpha1.schema.json"},
	{"FeatureRecipeV1alpha1", FeatureRecipeV1alpha1, "https://aggrega.ai/schemas/featurerecipe-v1alpha1.schema.json"},
	{"IRSchemaV1alpha1", IRSchemaV1alpha1, "https://aggrega.ai/schemas/ir-v1alpha1.schema.json"},
	{"LockfileV1alpha1", LockfileV1alpha1, "https://aggrega.ai/schemas/lockfile-v1alpha1.schema.json"},
	{"ManifestV1alpha1", ManifestV1alpha1, "https://aggrega.ai/schemas/manifest-v1alpha1.schema.json"},
	{"MappingLibraryV1alpha1", MappingLibraryV1alpha1, "https://aggrega.ai/schemas/mappinglibrary-v1alpha1.schema.json"},
	{"PathTemplateContextV1alpha1", PathTemplateContextV1alpha1, "https://aggrega.ai/schemas/path-template-context-v1alpha1.schema.json"},
	{"PluginV1alpha1", PluginV1alpha1, "https://aggrega.ai/schemas/plugin-v1alpha1.schema.json"},
	{"RecipeV1alpha1", RecipeV1alpha1, "https://aggrega.ai/schemas/recipe-v1alpha1.schema.json"},
	{"ReleaseV1alpha1", ReleaseV1alpha1, "https://aggrega.ai/schemas/release-v1alpha1.schema.json"},
	{"SchemaLibraryV1alpha1", SchemaLibraryV1alpha1, "https://aggrega.ai/schemas/schemalibrary-v1alpha1.schema.json"},
	{"TemplateContextV1alpha1", TemplateContextV1alpha1, "https://aggrega.ai/schemas/template-context-v1alpha1.schema.json"},
	{"WasmCliV1alpha1", WasmCliV1alpha1, "https://aggrega.ai/schemas/wasm-cli-v1alpha1.schema.json"},
	{"WasmLinterV1alpha1", WasmLinterV1alpha1, "https://aggrega.ai/schemas/wasm-linter-v1alpha1.schema.json"},
	{"WasmMutatorV1alpha1", WasmMutatorV1alpha1, "https://aggrega.ai/schemas/wasm-mutator-v1alpha1.schema.json"},
	{"WasmPolicyValidatorV1alpha1", WasmPolicyValidatorV1alpha1, "https://aggrega.ai/schemas/wasm-policy-validator-v1alpha1.schema.json"},
	{"WasmProviderV1alpha1", WasmProviderV1alpha1, "https://aggrega.ai/schemas/wasm-provider-v1alpha1.schema.json"},
	{"WasmFrpStepV1alpha1", WasmFrpStepV1alpha1, "https://aggrega.ai/schemas/wasm-frp-step-v1alpha1.schema.json"},
	{"WorkloadV1alpha1", WorkloadV1alpha1, "https://aggrega.ai/schemas/workload-v1alpha1.schema.json"},
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
