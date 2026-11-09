// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package schemas

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// Two vocabularies name a failure on the plugin wire, and a key's own name says
// which one a value is drawn from.
//
// A host-function reply envelope carries `hostfunc_error_code`, whose value is a
// member of the closed catalogue in wasm-hostfunc-error-v1beta1.schema.json: a
// fixed list of ways a host function can fail.
//
// A socket output travelling the other way carries `condition_code` beside
// `condition_short_code`, the two identifiers of a halting condition the guest
// raised. Those come from the open set of conditions the compiler registers, which
// the catalogue does not contain.
//
// The bare name `error_code` is therefore not a spelling this distribution admits
// anywhere: it names failure in general, and neither of the two vocabularies is
// the general case. Whoever adds the next reply envelope or the next socket output
// reads the key beside theirs, and a generic name there is the one that gets
// copied — so the constraint is a gate rather than a convention. `error_short_code`
// is retired for the same reason: it is the condition pair's other half, and the
// pair moves together or the two identifiers stop naming one register.
//
// The guard walks the whole distribution rather than the two schemas that carry
// these keys today, because the failure it exists to prevent is a NEW envelope
// somewhere else reaching for the generic name.

// retiredFailureKeys maps a retired property name to the register a value of that
// shape belongs on, so the failure says where to put it rather than only what not
// to write.
var retiredFailureKeys = map[string]string{
	"error_code": "a host-function reply envelope declares hostfunc_error_code and references " +
		"wasm-hostfunc-error-v1beta1.schema.json#/properties/code; a socket output reporting a " +
		"halting condition the guest raised declares condition_code",
	"error_short_code": "the guest-reported condition pair is condition_code and condition_short_code",
}

// liveFailureKeys are the two registers' own key names. The walk asserts it found
// them, because a walk that reached neither has not visited the documents this
// guard is about and its silence about the retired names would mean nothing.
var liveFailureKeys = []string{"hostfunc_error_code", "condition_code", "condition_short_code"}

// propertyKeyHit is one declared property name and the JSON pointer it sits at.
type propertyKeyHit struct {
	Pointer string
	Key     string
}

// collectPropertyKeys walks a decoded schema and returns every key declared
// inside a `properties` object, at any depth.
//
// It reads the KEYS of a properties object rather than searching the document
// text, and that is the whole reason it can be trusted here: `error_code` is a
// substring of `hostfunc_error_code`, so a text search reports the new name as a
// surviving occurrence of the old one, and a description that mentions a key by
// name is not a declaration of it. Comparing decoded keys for equality has
// neither failure.
func collectPropertyKeys(node any, path string, sink *[]propertyKeyHit) {
	switch n := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := path + "/" + k
			if k == "properties" {
				if props, ok := n[k].(map[string]any); ok {
					names := make([]string, 0, len(props))
					for name := range props {
						names = append(names, name)
					}
					sort.Strings(names)
					for _, name := range names {
						*sink = append(*sink, propertyKeyHit{Pointer: child + "/" + name, Key: name})
						collectPropertyKeys(props[name], child+"/"+name, sink)
					}
					continue
				}
			}
			collectPropertyKeys(n[k], child, sink)
		}
	case []any:
		for i, v := range n {
			collectPropertyKeys(v, path+"/"+itoa(i), sink)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// TestNoSchemaDeclaresARetiredFailureKey fails when any schema in the
// distribution declares a property under one of the retired generic names.
func TestNoSchemaDeclaresARetiredFailureKey(t *testing.T) {
	t.Parallel()

	inspected := 0
	keysSeen := 0
	live := map[string]int{}

	for _, s := range allSchemas {
		var doc any
		if err := json.Unmarshal([]byte(s.source), &doc); err != nil {
			t.Fatalf("%s: schema does not parse: %v", s.name, err)
		}
		var hits []propertyKeyHit
		collectPropertyKeys(doc, "", &hits)
		inspected++
		keysSeen += len(hits)
		for _, h := range hits {
			if remedy, retired := retiredFailureKeys[h.Key]; retired {
				t.Errorf("%s %s declares the retired key %q.\nA failure key names the vocabulary its value "+
					"is drawn from: %s.", s.name, h.Pointer, h.Key, remedy)
			}
			for _, k := range liveFailureKeys {
				if h.Key == k {
					live[k]++
				}
			}
		}
	}

	if inspected == 0 || keysSeen == 0 {
		t.Fatalf("inspected %d schema(s) and %d declared property key(s); a zero here is a statement about "+
			"the walk rather than about the corpus", inspected, keysSeen)
	}
	for _, k := range liveFailureKeys {
		if live[k] == 0 {
			t.Errorf("the walk found no declaration of %q anywhere in the distribution, so it is not reaching "+
				"the documents this guard is about and its silence on the retired names proves nothing", k)
		}
	}
}

// TestRetiredFailureKeyDetectorFires is the needle. The detector is an equality
// test over decoded property keys, and the way that goes blind is a walk that
// never descends far enough to see the key at all — so the needle is planted at a
// nested `$defs` position rather than at the document root.
//
// The two negative controls are the substring trap this guard exists on the right
// side of: `error_code` IS a substring of `hostfunc_error_code`, so an
// implementation that searched text instead of comparing keys would report both
// live names as surviving occurrences of the retired one.
func TestRetiredFailureKeyDetectorFires(t *testing.T) {
	t.Parallel()

	const planted = `{
	  "$defs": {
	    "SomeLaterReply": {
	      "type": "object",
	      "properties": {
	        "error": {"type": "string"},
	        "error_code": {"type": "string"},
	        "error_short_code": {"type": "string"}
	      }
	    }
	  }
	}`
	const clean = `{
	  "$defs": {
	    "SomeLaterReply": {
	      "type": "object",
	      "properties": {
	        "error": {"type": "string"},
	        "hostfunc_error_code": {"type": "string"},
	        "condition_code": {"type": "string"},
	        "condition_short_code": {"type": "string"}
	      }
	    }
	  }
	}`

	retiredIn := func(source string) []string {
		var doc any
		if err := json.Unmarshal([]byte(source), &doc); err != nil {
			t.Fatalf("fixture does not parse: %v", err)
		}
		var hits []propertyKeyHit
		collectPropertyKeys(doc, "", &hits)
		if len(hits) == 0 {
			t.Fatal("the walk found no declared property keys in the fixture, so neither outcome below means anything")
		}
		var found []string
		for _, h := range hits {
			if _, retired := retiredFailureKeys[h.Key]; retired {
				found = append(found, h.Key)
			}
		}
		sort.Strings(found)
		return found
	}

	got := retiredIn(planted)
	if want := "error_code,error_short_code"; strings.Join(got, ",") != want {
		t.Errorf("the detector stayed quiet on a planted nested declaration: found %v, want %s", got, want)
	}

	if got := retiredIn(clean); len(got) != 0 {
		t.Errorf("the detector convicted the live key names %v; error_code is a SUBSTRING of "+
			"hostfunc_error_code, so this is what a text search would report and what key equality must not", got)
	}
}
