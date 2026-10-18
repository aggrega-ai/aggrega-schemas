// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package schemas

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"unicode"
)

// This guard catches one specific way a schema `description` goes wrong: the
// same identifier spelled two ways inside one description — once fused, as the
// camelCase field name the author types, and once as separate lowercase words.
// A reader meets `injectMapping` and "inject mapping" in one sentence and
// cannot tell whether they are one thing or two.
//
// Why the camelCase split is the whole point: a detector comparing raw tokens
// sees `injectMapping` and `inject` as unrelated strings and reports the corpus
// clean while the defect sits in it. Splitting the identifier into its words
// before comparing is what makes the two spellings comparable at all, and
// TestFusedSpellingDetectorFiresOnKnownStutters below pins that behaviour to
// real strings so the guard cannot quietly regress into the blind version.
//
// What this guard deliberately does NOT cover, so nobody reads its silence as
// a broader clean bill: a description that names a compilation stage
// redundantly without any camelCase spelling — "assertions evaluate at
// assertion evaluation" — is invisible here. That defect is redundancy rather
// than a spelling mismatch, and redundancy is not mechanically decidable: a
// stage named after the thing it acts on necessarily sits near that thing, so
// proximity alone convicts correct prose. Those are judged by authors and
// reviewers.

// camelBoundary reports whether a word boundary falls between runes a and b of
// an identifier: lower-to-upper (injectMapping) or the tail of an initialism
// run before a new word (DNSPolicy -> DNS Policy).
func camelBoundary(prev, cur, next rune) bool {
	if unicode.IsLower(prev) && unicode.IsUpper(cur) {
		return true
	}
	if unicode.IsUpper(prev) && unicode.IsUpper(cur) && unicode.IsLower(next) {
		return true
	}
	return false
}

// splitIdentifier breaks one identifier into its constituent words on
// camelCase boundaries. A word with no internal case boundary comes back as a
// single element.
//
// An underscore is treated as prose spacing by the caller rather than as an
// identifier's internal boundary, and the asymmetry with camelCase is
// deliberate. A snake_case name IS its words with a separator between them, so
// counting it as "fused" makes it collide with its own plain-English rendering
// — `parse_cache` against "parse cache" — which is ordinary, correct prose and
// says nothing at all. camelCase carries no such collision, which is why it is
// the only boundary this guard reads.
func splitIdentifier(s string) []string {
	r := []rune(s)
	if len(r) < 2 {
		return []string{s}
	}
	var out []string
	start := 0
	for i := 1; i < len(r); i++ {
		next := rune(0)
		if i+1 < len(r) {
			next = r[i+1]
		}
		if camelBoundary(r[i-1], r[i], next) {
			out = append(out, string(r[start:i]))
			start = i
		}
	}
	return append(out, string(r[start:]))
}

// stemWord strips the common inflectional endings so that "mapping"/"mappings"
// and "port"/"ports" compare equal. Deliberately simple: it only has to make two
// spellings of ONE identifier match, not to be a general stemmer.
func stemWord(w string) string {
	w = strings.ToLower(w)
	for _, suf := range []string{"ings", "ing", "es", "s"} {
		if strings.HasSuffix(w, suf) && len(w)-len(suf) >= 3 {
			return w[:len(w)-len(suf)]
		}
	}
	return w
}

// token is one word of prose. group is the index of the source word it was
// split out of, or -1 when the source word carried no case boundary; that is
// what lets the scan tell a fused spelling from a spaced one.
type token struct {
	stem    string
	surface string
	group   int
}

// tokenize splits prose into words, then splits each word on camelCase.
func tokenize(text string) []token {
	var out []token
	word := 0
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		parts := splitIdentifier(field)
		group := -1
		if len(parts) > 1 {
			group = word
		}
		for _, p := range parts {
			out = append(out, token{stem: stemWord(p), surface: p, group: group})
		}
		word++
	}
	return out
}

// fusedHit is one identifier found spelled both ways in a single description.
type fusedHit struct {
	fused  string // the camelCase spelling, e.g. "injectMapping"
	spaced string // the separate-words spelling, e.g. "inject mapping"
}

// fusedSpellings finds every identifier in text that appears BOTH fused and as
// separate words. Returns hits in deterministic order.
func fusedSpellings(text string) []fusedHit {
	tk := tokenize(text)

	// Collect each multi-word fused identifier by its stem sequence.
	fused := map[string]string{}
	var order []string
	for i := 0; i < len(tk); {
		if tk[i].group < 0 {
			i++
			continue
		}
		j := i
		var stems, surfaces []string
		for j < len(tk) && tk[j].group == tk[i].group {
			stems = append(stems, tk[j].stem)
			surfaces = append(surfaces, tk[j].surface)
			j++
		}
		if len(stems) >= 2 {
			key := strings.Join(stems, " ")
			if _, seen := fused[key]; !seen {
				order = append(order, key)
			}
			fused[key] = strings.Join(surfaces, "")
		}
		i = j
	}

	var hits []fusedHit
	for _, key := range order {
		want := strings.Split(key, " ")
		for i := 0; i+len(want) <= len(tk); i++ {
			window := tk[i : i+len(want)]
			spacedOccurrence := true
			match := true
			var surfaces []string
			for k, tok := range window {
				if tok.group >= 0 {
					spacedOccurrence = false // this occurrence is itself fused
					break
				}
				if tok.stem != want[k] {
					match = false
					break
				}
				surfaces = append(surfaces, tok.surface)
			}
			if spacedOccurrence && match {
				hits = append(hits, fusedHit{fused: fused[key], spaced: strings.Join(surfaces, " ")})
				break
			}
		}
	}
	return hits
}

// collectDescriptions walks a decoded schema and returns every `description`
// string keyed by its JSON pointer.
func collectDescriptions(node any, path string, sink *[]struct{ Pointer, Text string }) {
	switch n := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := path + "/" + k
			if k == "description" {
				if s, ok := n[k].(string); ok {
					*sink = append(*sink, struct{ Pointer, Text string }{child, s})
					continue
				}
			}
			collectDescriptions(n[k], child, sink)
		}
	case []any:
		for i, v := range n {
			collectDescriptions(v, fmt.Sprintf("%s/%d", path, i), sink)
		}
	}
}

// ruledBenign lists every place a fused identifier legitimately shares a
// description with its plain-English rendering. Each key is
// "<schema>|<json-pointer>|<fused spelling>", so a hit anywhere else — or with
// a different identifier — still fails.
//
// The pattern in nearly all of them is a field name introduced beside the
// concept it configures: "Named container ports" beside `containerPort`, "PVC
// access modes" beside `accessModes`. That is how a description teaches what a
// key is for, and it must not be "corrected".
var ruledBenign = map[string]string{
	// A kind's name written as two English words in a list of entity kinds.
	"AggregaConfigV1alpha1|/$defs/SigningConfig/properties/kinds/description|FeatureRecipe": "entity kind named in prose beside the kind token",
	"LockfileV1alpha1|/properties/entities/description|FeatureRecipe":                       "entity kind named in prose beside the kind token",
	"WasmFrpStepV1alpha1|/description|FeatureRecipe":                                        "entity kind named in prose beside the kind token",
	"WasmLinterV1alpha1|/$defs/LinterInput/description|FeatureRecipe":                       "entity kind named in prose beside the kind token",

	// The spec description lists the areas a FeatureRecipe spec covers as
	// parallel English noun phrases — binding, mapping, pipeline, and inject
	// mapping — and names the field itself in the following clause. Renaming
	// the fourth to the field spelling would leave three English nouns beside
	// one identifier and break the parallel the sentence is built on.
	"FeatureRecipeV1alpha1|/properties/spec/description|injectMapping": "parallel English noun phrases naming the spec's content areas",

	// A field name introduced beside the concept it configures.
	"IRSchemaV1alpha1|/$defs/IRNode/properties/sync_wave/description|syncWave":                                    "field name beside the concept it configures",
	"PluginV1alpha1|/properties/spec/properties/provider/description|pluginType":                                  "field name beside the concept it configures",
	"PluginV1alpha1|/properties/spec/properties/pipelineStep/description|pluginType":                              "field name beside the concept it configures",
	"PluginV1alpha1|/properties/spec/properties/parser/description|pluginType":                                    "field name beside the concept it configures",
	"PluginV1alpha1|/properties/spec/properties/importer/description|pluginType":                                  "field name beside the concept it configures",
	"TemplateContextV1alpha1|/description|valuesMapping":                                                          "field name beside the concept it configures",
	"TemplateContextV1alpha1|/properties/source/description|valuesMapping":                                        "field name beside the concept it configures",
	"WasmProviderV1alpha1|/$defs/IRNodeProjection/properties/output_labels/description|outputLabels":              "field name beside the concept it configures",
	"WorkloadV1alpha1|/properties/imagePullSecrets/description|imagePullSecrets":                                  "field name beside the concept it configures",
	"WorkloadV1alpha1|/properties/runtimeClassName/description|RuntimeClass":                                      "field name beside the concept it configures",
	"WorkloadV1alpha1|/properties/dnsPolicy/description|dnsPolicy":                                                "field name beside the concept it configures",
	"WorkloadV1alpha1|/$defs/container/properties/imagePullPolicy/description|imagePullPolicy":                    "field name beside the concept it configures",
	"WorkloadV1alpha1|/$defs/container/properties/envFrom/description|envFrom":                                    "field name beside the concept it configures",
	"WorkloadV1alpha1|/$defs/container/properties/ports/description|containerPort":                                "field name beside the concept it configures",
	"WorkloadV1alpha1|/$defs/volume/properties/persistent/properties/accessModes/description|accessModes":         "field name beside the concept it configures",
	"WorkloadV1alpha1|/$defs/volume/properties/persistentTemplate/properties/accessModes/description|accessModes": "field name beside the concept it configures",

	// A field name beside the English noun for what it claims:
	// `contract.inputKinds` is the key an author types, and "an input kind" names
	// the thing that key claims support for. Prose describing a key's subject in
	// English is not a second spelling of the key.
	"PluginV1alpha1|/properties/spec/properties/provider/properties/wellKnownAnnotations/description|inputKinds":                        "field name beside the English noun for the kinds it claims",
	"PluginV1alpha1|/properties/spec/properties/provider/properties/wellKnownAnnotations/items/properties/scope/description|inputKinds": "field name beside the English noun for the kinds it claims",

	// An enum value quoted, then read back as English.
	"WorkloadV1alpha1|/$defs/container/properties/imagePullPolicy/description|IfNotPresent": "enum value quoted, then read back as English",

	// Two identifiers on two surfaces the reader types, not one identifier
	// written twice: `$dynamicRef` is the JSON Schema keyword, dynamic_refs is
	// the lint rule this key suppresses.
	"WasmLinterV1alpha1|/$defs/Policies/properties/allow_dynamic_refs/description|dynamicRef": "schema keyword beside the lint rule name that governs it",

	// The contrast IS the sentence. Each of these descriptions teaches that
	// segment-wise matching over a dotted path covers `db.password` and does not
	// reach a sibling leaf named `dbPassword` — so the two spellings name two
	// different leaves, and the whole point is that they are not the same one.
	// Rewriting either to match the other destroys the example: the sentence would
	// then say a declaration at `db` does not reach `db.password`, which is the
	// opposite of the rule. The detector reads the dotted path as the spaced
	// spelling of the fused one, which is correct in general and is exactly why
	// this deliberate near-miss needs a ruling rather than a rewrite.
	"IRSchemaV1alpha1|/$defs/IRNode/properties/sensitive_payload_leaves/description|dbPassword":            "a dotted path contrasted with a differently-named sibling leaf",
	"IRSchemaV1alpha1|/$defs/IRFeatureEntry/properties/sensitive_payload_leaves/description|dbPassword":    "a dotted path contrasted with a differently-named sibling leaf",
	"IRSchemaV1alpha1|/$defs/IRBoundaryFeature/properties/sensitive_payload_leaves/description|dbPassword": "a dotted path contrasted with a differently-named sibling leaf",
}

// TestNoIdentifierSpelledTwoWaysInOneDescription fails when a schema
// description spells one identifier both fused and as separate words without a
// ruling recorded above. A new hit is not automatically a defect — it is a
// sentence somebody must read. Fix it, or add it to ruledBenign with the reason
// it reads correctly.
func TestNoIdentifierSpelledTwoWaysInOneDescription(t *testing.T) {
	t.Parallel()
	inspected := 0
	for _, s := range allSchemas {
		s := s
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			var doc any
			if err := json.Unmarshal([]byte(s.source), &doc); err != nil {
				t.Fatalf("%s: schema does not parse: %v", s.name, err)
			}
			var found []struct{ Pointer, Text string }
			collectDescriptions(doc, "", &found)
			for _, d := range found {
				for _, hit := range fusedSpellings(d.Text) {
					key := s.name + "|" + d.Pointer + "|" + hit.fused
					if _, ok := ruledBenign[key]; ok {
						continue
					}
					t.Errorf("%s %s spells one identifier two ways: %q and %q.\n"+
						"Either rewrite the description to use a single spelling, or record the ruling as\n"+
						"\t%q: \"<why it reads correctly>\",\nin ruledBenign.",
						s.name, d.Pointer, hit.fused, hit.spaced, key)
				}
			}
		})
		inspected++
	}
	if inspected == 0 {
		t.Fatal("inspected no schemas; the guard is vacuous")
	}
}

// TestFusedSpellingDetectorFiresOnKnownStutters is the non-vacuity needle. Each
// string below is a real description that carried the defect. A token-identical
// detector — one that compares words without splitting camelCase first — reports
// every one of them clean, which is exactly how the defect survived review. If
// this test ever goes quiet, the guard above has stopped detecting anything and
// its silence means nothing.
func TestFusedSpellingDetectorFiresOnKnownStutters(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, text string }{
		{
			"featurerecipe executionScope",
			"Boundary-scope FeatureRecipes receive a different TemplateContext shape. " +
				"The injectMapping of inject mapping continues to fan per-consumer regardless of executionScope.",
		},
		{
			"featurerecipe eval-capability constraint",
			"injectMapping (a delivery transform applied at inject mapping, not a rendering surface) remains allowed.",
		},
		{
			"template-context inject pillar",
			"Mutated in-place by the injectMapping deep-merges of inject mapping.",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hits := fusedSpellings(tc.text)
			if len(hits) == 0 {
				t.Fatalf("detector stayed silent on a known two-spelling description %q; the guard is vacuous", tc.text)
			}
			for _, h := range hits {
				if h.fused == "injectMapping" {
					return
				}
			}
			t.Fatalf("detector fired but did not name injectMapping; got %+v", hits)
		})
	}
}

// TestFusedSpellingDetectorIgnoresSingleSpelling proves the guard is not
// trivially true — prose using one spelling consistently must not fire, or
// every description would be a hit and the signal would be worthless.
func TestFusedSpellingDetectorIgnoresSingleSpelling(t *testing.T) {
	t.Parallel()
	for _, clean := range []string{
		"The injectMapping continues to fan per-consumer regardless of executionScope.",
		"Mutated in-place by the injectMapping deep-merges.",
		"injectMapping (a delivery transform, not a rendering surface) remains allowed.",
		"Callers resolve inject keys against these values at inject hydration.",
	} {
		if hits := fusedSpellings(clean); len(hits) != 0 {
			t.Errorf("detector fired on single-spelling prose %q: %+v", clean, hits)
		}
	}
}
