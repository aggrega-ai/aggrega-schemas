// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// TestCompareSchemasPropertyAddedToOpenObject adds one optional property, "y",
// to the object at #/properties/spec, whose old shape alone differs per case.
// Where the old object accepted an extra property under that name, a document
// written before may already carry it with another kind of value, so the
// addition is refused, naming the object and the property; where it did not,
// the addition passes.
func TestCompareSchemasPropertyAddedToOpenObject(t *testing.T) {
	const wrapper = `{"type": "object", "additionalProperties": false, "properties": {"spec": %s}}`
	cases := []struct {
		name, old, updated string
		refused            bool
	}{
		{"additionalProperties absent",
			`{"type": "object", "properties": {"x": {"type": "string"}}}`,
			`{"type": "object", "properties": {"x": {"type": "string"}, "y": {"type": "integer"}}}`, true},
		{"additionalProperties true",
			`{"type": "object", "additionalProperties": true, "properties": {"x": {"type": "string"}}}`,
			`{"type": "object", "additionalProperties": true, "properties": {"x": {"type": "string"}, "y": {"type": "integer"}}}`, true},
		{"additionalProperties a schema",
			`{"type": "object", "additionalProperties": {"type": "string"}, "properties": {"x": {"type": "string"}}}`,
			`{"type": "object", "additionalProperties": {"type": "string"}, "properties": {"x": {"type": "string"}, "y": {"type": "integer"}}}`, true},
		{"no properties before",
			`{"type": "object"}`,
			`{"type": "object", "properties": {"y": {"type": "integer"}}}`, true},
		{"name matches an old patternProperties key",
			`{"type": "object", "additionalProperties": false, "patternProperties": {"^[a-z]$": {"type": "string"}}, "properties": {"x": {"type": "string"}}}`,
			`{"type": "object", "additionalProperties": false, "patternProperties": {"^[a-z]$": {"type": "string"}}, "properties": {"x": {"type": "string"}, "y": {"type": "integer"}}}`, true},
		{"additionalProperties false",
			`{"type": "object", "additionalProperties": false, "properties": {"x": {"type": "string"}}}`,
			`{"type": "object", "additionalProperties": false, "properties": {"x": {"type": "string"}, "y": {"type": "integer"}}}`, false},
		{"name matches no old patternProperties key",
			`{"type": "object", "additionalProperties": false, "patternProperties": {"^x-": {"type": "string"}}, "properties": {"x": {"type": "string"}}}`,
			`{"type": "object", "additionalProperties": false, "patternProperties": {"^x-": {"type": "string"}}, "properties": {"x": {"type": "string"}, "y": {"type": "integer"}}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := mustDecode(t, fmt.Sprintf(wrapper, tc.old))
			updated := mustDecode(t, fmt.Sprintf(wrapper, tc.updated))
			found := compareSchemas("json/thing-v1beta1.schema.json", old, updated)
			if !tc.refused {
				if len(found) != 0 {
					t.Fatalf("want no finding; got %v", found)
				}
				return
			}
			want := `json/thing-v1beta1.schema.json #/properties/spec: property "y" added where any extra property was accepted before`
			if len(found) != 1 || found[0].String() != want || found[0].added != "y" || found[0].unclassified {
				t.Fatalf("want exactly the finding %q; got %v", want, found)
			}
		})
	}
}

// admitY is an exceptions file admitting the property "y" that openObjectRepo
// adds to the open object #/$defs/Spec.
const admitY = `{"exceptions": [{"schema": "thing-v1beta1.schema.json", "pointer": "#/$defs/Spec", "property": "y", "reason": "No document can carry a Spec yet."}]}`

// addedY is the refusal openObjectRepo's addition draws without an admission.
const addedY = `json/thing-v1beta1.schema.json #/$defs/Spec: property "y" added where any extra property was accepted before`

// openObjectRepo is a released repository whose working tree adds the optional
// property "y" to #/$defs/Spec of thing-v1beta1.schema.json, an object that
// accepted any extra property at the release.
func openObjectRepo(t *testing.T) *gitRepo {
	t.Helper()
	r := newGitRepo(t)
	writeFile(t, r.dir, "json/thing-v1beta1.schema.json", baseSchema)
	r.commit("schema")
	r.git("tag", "v0.1.0")
	changed := strings.Replace(baseSchema, `"properties": {"x": {"type": "string"}}`, `"properties": {"x": {"type": "string"}, "y": {"type": "integer"}}`, 1)
	if changed == baseSchema {
		t.Fatal("the plant did not apply")
	}
	writeFile(t, r.dir, "json/thing-v1beta1.schema.json", changed)
	return r
}

// TestExceptionsFile drives compat through run over a repository adding a
// property to an open object: with no exceptions file it is refused; an entry
// naming it admits it; an entry matching no finding is refused as stale; a
// malformed file, or a named file that does not exist, leaves nothing judged;
// -exceptions reads another file; and under -staged the file is read from the
// index, a change to it alone being enough to judge.
func TestExceptionsFile(t *testing.T) {
	compat := func(t *testing.T, args ...string) (int, string) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := run(append([]string{"compat"}, args...), &out, &errOut)
		return code, out.String() + errOut.String()
	}

	t.Run("no exceptions file", func(t *testing.T) {
		r := openObjectRepo(t)
		if code, out := compat(t, "-repo", r.dir); code != 1 || !strings.Contains(out, addedY) {
			t.Fatalf("want exit 1 naming %q; got %d\n%s", addedY, code, out)
		}
	})
	t.Run("excepted entry passes", func(t *testing.T) {
		r := openObjectRepo(t)
		writeFile(t, r.dir, "compat-exceptions.json", admitY)
		if code, out := compat(t, "-repo", r.dir); code != 0 || !strings.Contains(out, "No document can carry a Spec yet.") {
			t.Fatalf("want exit 0 naming the admission and its reason; got %d\n%s", code, out)
		}
	})
	t.Run("entry matching no finding refused as stale", func(t *testing.T) {
		r := openObjectRepo(t)
		stale := strings.Replace(admitY, `]}`, `, {"schema": "thing-v1beta1.schema.json", "pointer": "#/$defs/Spec", "property": "z", "reason": "Never added."}]}`, 1)
		writeFile(t, r.dir, "compat-exceptions.json", stale)
		code, out := compat(t, "-repo", r.dir)
		if code != 1 || !strings.Contains(out, "refused: 1 change ") || !strings.Contains(out, `json/thing-v1beta1.schema.json #/$defs/Spec: compat-exceptions.json excepts property "z"`) {
			t.Fatalf("want exit 1 refusing only the stale entry for \"z\"; got %d\n%s", code, out)
		}
	})
	t.Run("malformed file not judged", func(t *testing.T) {
		r := openObjectRepo(t)
		writeFile(t, r.dir, "compat-exceptions.json", `{"exceptions": [`)
		if code, out := compat(t, "-repo", r.dir); code != 2 || !strings.Contains(out, "nothing was judged") || !strings.Contains(out, "compat-exceptions.json") {
			t.Fatalf("want exit 2 naming the file; got %d\n%s", code, out)
		}
	})
	t.Run("-exceptions reads another file", func(t *testing.T) {
		r := openObjectRepo(t)
		other := writeFile(t, t.TempDir(), "admit.json", admitY)
		if code, out := compat(t, "-repo", r.dir, "-exceptions", other); code != 0 {
			t.Fatalf("want exit 0; got %d\n%s", code, out)
		}
	})
	t.Run("-exceptions names a missing file", func(t *testing.T) {
		r := openObjectRepo(t)
		missing := filepath.Join(t.TempDir(), "absent.json")
		if code, out := compat(t, "-repo", r.dir, "-exceptions", missing); code != 2 || !strings.Contains(out, "absent.json") {
			t.Fatalf("want exit 2 naming the file; got %d\n%s", code, out)
		}
	})
	t.Run("staged file read from the index", func(t *testing.T) {
		// The index stages only an exceptions file holding a stale entry; the
		// working tree no longer has it, so reading the working tree would pass.
		r := newGitRepo(t)
		writeFile(t, r.dir, "json/thing-v1beta1.schema.json", baseSchema)
		r.commit("schema")
		r.git("tag", "v0.1.0")
		writeFile(t, r.dir, "compat-exceptions.json", admitY)
		r.git("add", "compat-exceptions.json")
		if err := os.Remove(filepath.Join(r.dir, "compat-exceptions.json")); err != nil {
			t.Fatal(err)
		}
		code, out := compat(t, "-repo", r.dir, "-staged")
		if code != 1 || !strings.Contains(out, `#/$defs/Spec: compat-exceptions.json in the index excepts property "y"`) {
			t.Fatalf("want exit 1 naming the staged stale entry; got %d\n%s", code, out)
		}
	})
}

// TestExceptionsLoad reads a well-formed exceptions file and refuses each shape
// that would admit something other than what it seems to, one fault per case,
// without panicking.
func TestExceptionsLoad(t *testing.T) {
	got, err := loadExceptions(writeFile(t, t.TempDir(), "compat-exceptions.json", admitY))
	if err != nil {
		t.Fatal(err)
	}
	want := exception{Schema: "thing-v1beta1.schema.json", Pointer: "#/$defs/Spec", Property: "y", Reason: "No document can carry a Spec yet."}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("want [%+v]; got %+v", want, got)
	}
	if got, err := parseExceptions("compat-exceptions.json", []byte(`{"exceptions": []}`)); err != nil || len(got) != 0 {
		t.Fatalf("an empty list admits nothing and is well formed; got %v, %v", got, err)
	}
	if _, err := loadExceptions(filepath.Join(t.TempDir(), "absent.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing file: want an error wrapping fs.ErrNotExist; got %v", err)
	}
	entry := `{"schema": "thing-v1beta1.schema.json", "pointer": "#/$defs/Spec", "property": "y", "reason": "r"}`
	base := `{"exceptions": [` + entry + `]}`
	plant := func(from, to string) string {
		t.Helper()
		s := strings.Replace(base, from, to, 1)
		if s == base {
			t.Fatalf("the plant %q did not apply", from)
		}
		return s
	}
	for _, tc := range []struct{ name, src, want string }{
		{"not JSON", `{"exceptions": [`, "unexpected EOF"},
		{"no exceptions list", `{}`, "has no exceptions list"},
		{"unknown key", plant(`"reason": "r"}`, `"reason": "r", "note": "x"}`), `unknown field "note"`},
		{"no schema", plant(`"schema": "thing-v1beta1.schema.json"`, `"schema": ""`), "entry 1 has no schema"},
		{"no pointer", plant(`"pointer": "#/$defs/Spec"`, `"pointer": ""`), "entry 1 has no pointer"},
		{"no property", plant(`"property": "y"`, `"property": ""`), "entry 1 has no property"},
		{"blank reason", plant(`"reason": "r"`, `"reason": "  "`), "entry 1 has no reason"},
		{"schema with its directory", plant(`"schema": "thing-v1beta1.schema.json"`, `"schema": "json/thing-v1beta1.schema.json"`), "entry 1 names its schema as json/thing-v1beta1.schema.json"},
		{"pointer not a fragment", plant(`"pointer": "#/$defs/Spec"`, `"pointer": "/$defs/Spec"`), `entry 1 has the pointer "/$defs/Spec"`},
		{"repeated entry", plant(`]}`, `, `+entry+`]}`), "entry 2 repeats entry 1"},
		{"trailing data", plant(`]}`, `]} {}`), "after the exceptions object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseExceptions("compat-exceptions.json", []byte(tc.src))
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.HasPrefix(err.Error(), "compat-exceptions.json: ") {
				t.Fatalf("want an error from compat-exceptions.json naming %q; got %v", tc.want, err)
			}
		})
	}
}
