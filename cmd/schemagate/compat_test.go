// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"strings"
	"testing"
)

// baseSchema is a small schema in the shape the published schemas take: a
// closed object with a required property, an enum, a const, a pattern, a bound
// and a $defs entry.
const baseSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "description": "A thing.",
  "additionalProperties": false,
  "required": ["kind"],
  "properties": {
    "kind": {"const": "Thing", "description": "The kind."},
    "mode": {"type": "string", "enum": ["a", "b"]},
    "name": {"type": "string", "pattern": "^[a-z]+$"},
    "size": {"type": "integer", "minimum": 1, "maximum": 10},
    "spec": {"$ref": "#/$defs/Spec"}
  },
  "$defs": {
    "Spec": {"type": "object", "properties": {"x": {"type": "string"}}}
  }
}`

func mustDecode(t *testing.T, s string) any {
	t.Helper()
	v, err := decodeSchema([]byte(s))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

// TestCompareSchemasRefusesBreakingKinds plants one breaking change per case and
// requires the comparison to name its pointer and its kind.
func TestCompareSchemasRefusesBreakingKinds(t *testing.T) {
	cases := []struct {
		name, from, to, pointer, kind string
	}{
		{"property removed", `"mode": {"type": "string", "enum": ["a", "b"]},`, ``, "#/properties/mode", `property "mode" removed or renamed`},
		{"required added", `"required": ["kind"]`, `"required": ["kind", "name"]`, "#/required", `"name" added to required`},
		{"type narrowed", `"name": {"type": "string",`, `"name": {"type": "null",`, "#/properties/name/type", `type narrowed: "string" no longer accepted`},
		{"enum member removed", `"enum": ["a", "b"]`, `"enum": ["a"]`, "#/properties/mode/enum", `enum member "b" removed`},
		{"const changed", `"const": "Thing"`, `"const": "Other"`, "#/properties/kind/const", `const changed from "Thing" to "Other"`},
		{"pattern changed", `"^[a-z]+$"`, `"^[a-z]{2,}$"`, "#/properties/name/pattern", "pattern changed"},
		{"minimum tightened", `"minimum": 1`, `"minimum": 2`, "#/properties/size/minimum", "minimum tightened from 1 to 2"},
		{"maximum tightened", `"maximum": 10`, `"maximum": 9`, "#/properties/size/maximum", "maximum tightened from 10 to 9"},
		{"additionalProperties tightened", `"properties": {"x": {"type": "string"}}`, `"additionalProperties": false, "properties": {"x": {"type": "string"}}`, "#/$defs/Spec/additionalProperties", "additionalProperties tightened from true to false"},
		{"defs entry removed", `"Spec": {"type": "object", "properties": {"x": {"type": "string"}}}`, `"Other": {"type": "object"}`, "#/$defs/Spec", `$defs entry "Spec" removed`},
		{"unclassified ref change", `"$ref": "#/$defs/Spec"`, `"$ref": "#/$defs/Other"`, "#/properties/spec/$ref", "$ref changed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := strings.Replace(baseSchema, tc.from, tc.to, 1)
			if changed == baseSchema {
				t.Fatalf("the plant %q did not apply", tc.from)
			}
			found := compareSchemas("thing-v1beta1.schema.json", mustDecode(t, baseSchema), mustDecode(t, changed))
			for _, f := range found {
				if f.pointer == tc.pointer && strings.Contains(f.kind, tc.kind) {
					return
				}
			}
			t.Fatalf("want a refusal at %s naming %q; got %v", tc.pointer, tc.kind, found)
		})
	}
}

// TestCompareSchemasPassesAdditiveChanges requires the changes the rule allows
// within one API version to produce no finding.
func TestCompareSchemasPassesAdditiveChanges(t *testing.T) {
	cases := []struct{ name, from, to string }{
		{"description changed", `"description": "The kind."`, `"description": "The kind of the thing."`},
		{"optional property added", `"mode": {`, `"note": {"type": "string"}, "mode": {`},
		{"enum member added", `"enum": ["a", "b"]`, `"enum": ["a", "b", "c"]`},
		{"type widened", `"size": {"type": "integer",`, `"size": {"type": "number",`},
		{"required dropped", `"required": ["kind"]`, `"required": []`},
		{"pattern dropped", `, "pattern": "^[a-z]+$"`, ``},
		{"maximum loosened", `"maximum": 10`, `"maximum": 11`},
		{"defs entry added", `"Spec": {`, `"Extra": {"type": "string"}, "Spec": {`},
		{"title added", `"description": "A thing.",`, `"description": "A thing.", "title": "Thing",`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := strings.Replace(baseSchema, tc.from, tc.to, 1)
			if changed == baseSchema {
				t.Fatalf("the plant %q did not apply", tc.from)
			}
			if found := compareSchemas("thing-v1beta1.schema.json", mustDecode(t, baseSchema), mustDecode(t, changed)); len(found) != 0 {
				t.Fatalf("want no finding; got %v", found)
			}
		})
	}
}

// TestCompareSchemasBranchesComparedByPosition requires a branch of a oneOf to be
// compared with the same branch, and a change in the branch count to be refused
// as unclassified.
func TestCompareSchemasBranchesComparedByPosition(t *testing.T) {
	old := `{"oneOf": [{"type": "string", "description": "a"}, {"type": "integer"}]}`
	described := `{"oneOf": [{"type": "string", "description": "b"}, {"type": "integer"}]}`
	if found := compareSchemas("s", mustDecode(t, old), mustDecode(t, described)); len(found) != 0 {
		t.Fatalf("a description change inside a branch must pass; got %v", found)
	}
	added := `{"oneOf": [{"type": "string"}, {"type": "integer"}, {"type": "null"}]}`
	found := compareSchemas("s", mustDecode(t, old), mustDecode(t, added))
	if len(found) != 1 || !found[0].unclassified || found[0].pointer != "#/oneOf" {
		t.Fatalf("want one unclassified finding at #/oneOf; got %v", found)
	}
}

// TestApiVersionOf reads the API version from a schema file name.
func TestApiVersionOf(t *testing.T) {
	for name, want := range map[string]string{
		"json/component-v1beta1.schema.json":     "v1beta1",
		"json/wasm-frp-step-v1beta2.schema.json": "v1beta2",
		"json/ir-v1alpha1.schema.json":           "v1alpha1",
		"json/thing-v2.schema.json":              "v2",
	} {
		got, err := apiVersionOf(name)
		if err != nil || got != want {
			t.Errorf("apiVersionOf(%q) = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := apiVersionOf("json/unversioned.schema.json"); err == nil {
		t.Errorf("an unversioned file name must be refused")
	}
}

// TestNewestReleaseWith selects the newest release carrying the API version, and
// none when no release carries it.
func TestNewestReleaseWith(t *testing.T) {
	files := map[string][]string{
		"v0.14.0": {"json/a-v1beta2.schema.json"},
		"v0.13.1": {"json/a-v1beta1.schema.json"},
		"v0.13.0": {"json/a-v1beta1.schema.json"},
		"v0.12.0": {"json/a-v1alpha1.schema.json"},
	}
	tags := []string{"v0.14.0", "v0.13.1", "v0.13.0", "v0.12.0"}
	lookup := func(tag string) ([]string, error) { return files[tag], nil }
	for version, want := range map[string]string{"v1beta2": "v0.14.0", "v1beta1": "v0.13.1", "v1alpha1": "v0.12.0", "v1beta3": ""} {
		got, err := newestReleaseWith(tags, version, lookup)
		if err != nil || got != want {
			t.Errorf("newestReleaseWith(%s) = %q, %v; want %q", version, got, err, want)
		}
	}
}
