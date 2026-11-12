// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// finding is one change a comparison refuses: the schema file, the JSON
// pointer of the node that changed, and the kind of change in words.
// unclassified marks a change to a keyword this check does not classify as
// additive or breaking; such a change is refused too, so that only the kinds
// known to be additive pass.
type finding struct {
	schema       string
	pointer      string
	kind         string
	unclassified bool
}

func (f finding) String() string {
	return fmt.Sprintf("%s %s: %s", f.schema, f.pointer, f.kind)
}

// annotationKeywords validate nothing: a document valid before a change to one
// of them is valid after it and means the same. Keywords spelled with the x-
// prefix are annotations as well (see isAnnotation).
var annotationKeywords = map[string]bool{
	"description": true,
	"title":       true,
	"$comment":    true,
	"examples":    true,
}

// schemaValued keywords hold one subschema each; when both sides carry one it
// is compared as a schema.
var schemaValued = map[string]bool{
	"items":            true,
	"not":              true,
	"if":               true,
	"then":             true,
	"else":             true,
	"contains":         true,
	"propertyNames":    true,
	"additionalItems":  true,
	"unevaluatedItems": true,
}

// schemaMapValued keywords hold a map from a name to a subschema.
var schemaMapValued = map[string]bool{
	"patternProperties": true,
	"dependentSchemas":  true,
}

// schemaListValued keywords hold a list of subschemas, compared position by
// position.
var schemaListValued = map[string]bool{
	"oneOf":       true,
	"anyOf":       true,
	"allOf":       true,
	"prefixItems": true,
}

func isAnnotation(keyword string) bool {
	return annotationKeywords[keyword] || strings.HasPrefix(keyword, "x-")
}

// decodeSchema parses one schema document, keeping every number as written so
// that two numbers compare exactly.
func decodeSchema(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// compareSchemas returns every change from old to updated that a document valid
// against old could fail, in a deterministic order. schema names the file in
// each finding.
func compareSchemas(schema string, old, updated any) []finding {
	c := &comparer{schema: schema}
	c.node("#", old, updated)
	return c.found
}

type comparer struct {
	schema string
	found  []finding
}

func (c *comparer) refuse(pointer, kind string) {
	c.found = append(c.found, finding{schema: c.schema, pointer: pointer, kind: kind})
}

func (c *comparer) unclassified(pointer, kind string) {
	c.found = append(c.found, finding{schema: c.schema, pointer: pointer, kind: kind, unclassified: true})
}

// node compares two schema nodes found at the same pointer.
func (c *comparer) node(pointer string, old, updated any) {
	if reflect.DeepEqual(old, updated) {
		return
	}
	oldMap, oldIsMap := old.(map[string]any)
	newMap, newIsMap := updated.(map[string]any)
	if !oldIsMap || !newIsMap {
		c.unclassified(pointer, fmt.Sprintf("schema changed from %s to %s", render(old), render(updated)))
		return
	}
	for _, kw := range unionKeys(oldMap, newMap) {
		ov, inOld := oldMap[kw]
		nv, inNew := newMap[kw]
		if inOld && inNew && reflect.DeepEqual(ov, nv) {
			continue
		}
		at := pointer + "/" + escapePointer(kw)
		switch {
		case isAnnotation(kw):
			// Validates nothing.
		case kw == "properties":
			c.properties(at, asMap(ov), asMap(nv))
		case kw == "$defs":
			c.defs(at, asMap(ov), asMap(nv))
		case kw == "required":
			c.required(at, ov, nv)
		case kw == "type":
			c.typeKeyword(at, ov, inOld, nv, inNew)
		case kw == "enum":
			c.enum(at, ov, inOld, nv, inNew)
		case kw == "const":
			c.constKeyword(at, ov, inOld, nv, inNew)
		case kw == "pattern":
			c.pattern(at, ov, inOld, nv, inNew)
		case kw == "minimum":
			c.bound(at, kw, ov, inOld, nv, inNew, +1)
		case kw == "maximum":
			c.bound(at, kw, ov, inOld, nv, inNew, -1)
		case kw == "additionalProperties":
			c.additionalProperties(at, ov, inOld, nv, inNew)
		case schemaValued[kw]:
			if inOld && inNew {
				c.node(at, ov, nv)
			} else {
				c.unclassified(at, presenceChange(kw, inOld))
			}
		case schemaMapValued[kw]:
			c.schemaMap(at, kw, ov, inOld, nv, inNew)
		case schemaListValued[kw]:
			c.schemaList(at, kw, ov, inOld, nv, inNew)
		default:
			if inOld && inNew {
				c.unclassified(at, fmt.Sprintf("%s changed from %s to %s", kw, render(ov), render(nv)))
			} else {
				c.unclassified(at, presenceChange(kw, inOld))
			}
		}
	}
}

func (c *comparer) properties(pointer string, old, updated map[string]any) {
	for _, name := range sortedKeys(old) {
		nv, ok := updated[name]
		at := pointer + "/" + escapePointer(name)
		if !ok {
			c.refuse(at, fmt.Sprintf("property %q removed or renamed", name))
			continue
		}
		c.node(at, old[name], nv)
	}
	// A property only in updated is an added property; when it is also added
	// to required, the required comparison refuses it.
}

func (c *comparer) defs(pointer string, old, updated map[string]any) {
	for _, name := range sortedKeys(old) {
		nv, ok := updated[name]
		at := pointer + "/" + escapePointer(name)
		if !ok {
			c.refuse(at, fmt.Sprintf("$defs entry %q removed", name))
			continue
		}
		c.node(at, old[name], nv)
	}
}

func (c *comparer) required(pointer string, old, updated any) {
	before := map[string]bool{}
	for _, v := range asList(old) {
		if s, ok := v.(string); ok {
			before[s] = true
		}
	}
	for _, v := range asList(updated) {
		s, ok := v.(string)
		if ok && !before[s] {
			c.refuse(pointer, fmt.Sprintf("%q added to required", s))
		}
	}
}

func (c *comparer) typeKeyword(pointer string, old any, inOld bool, updated any, inNew bool) {
	if !inNew {
		return // no type constraint accepts every type
	}
	if !inOld {
		c.refuse(pointer, fmt.Sprintf("type narrowed: any type before, %s now", render(updated)))
		return
	}
	accepted := map[string]bool{}
	for _, t := range typeList(updated) {
		accepted[t] = true
	}
	for _, t := range typeList(old) {
		if accepted[t] || (t == "integer" && accepted["number"]) {
			continue
		}
		c.refuse(pointer, fmt.Sprintf("type narrowed: %q no longer accepted", t))
	}
}

func (c *comparer) enum(pointer string, old any, inOld bool, updated any, inNew bool) {
	if !inNew {
		return // dropping the enum accepts every value
	}
	if !inOld {
		c.refuse(pointer, fmt.Sprintf("enum added: only %s accepted now", render(updated)))
		return
	}
	newMembers := asList(updated)
	for _, m := range asList(old) {
		if !containsValue(newMembers, m) {
			c.refuse(pointer, fmt.Sprintf("enum member %s removed", render(m)))
		}
	}
}

func (c *comparer) constKeyword(pointer string, old any, inOld bool, updated any, inNew bool) {
	switch {
	case !inNew:
		return // dropping the const accepts every value
	case !inOld:
		c.refuse(pointer, fmt.Sprintf("const added: only %s accepted now", render(updated)))
	default:
		c.refuse(pointer, fmt.Sprintf("const changed from %s to %s", render(old), render(updated)))
	}
}

func (c *comparer) pattern(pointer string, old any, inOld bool, updated any, inNew bool) {
	switch {
	case !inNew:
		return // dropping the pattern accepts every string
	case !inOld:
		c.refuse(pointer, fmt.Sprintf("pattern added: %s", render(updated)))
	default:
		// Whether one regular expression accepts everything another does is not
		// decided here, so any change to a pattern is treated as a tightening.
		c.refuse(pointer, fmt.Sprintf("pattern changed from %s to %s", render(old), render(updated)))
	}
}

// bound compares minimum (direction +1: a larger value tightens) or maximum
// (direction -1: a smaller value tightens).
func (c *comparer) bound(pointer, kw string, old any, inOld bool, updated any, inNew bool, direction int) {
	if !inNew {
		return
	}
	if !inOld {
		c.refuse(pointer, fmt.Sprintf("%s added: %s", kw, render(updated)))
		return
	}
	o, okOld := asFloat(old)
	n, okNew := asFloat(updated)
	if !okOld || !okNew {
		c.unclassified(pointer, fmt.Sprintf("%s changed from %s to %s", kw, render(old), render(updated)))
		return
	}
	if (direction > 0 && n > o) || (direction < 0 && n < o) {
		c.refuse(pointer, fmt.Sprintf("%s tightened from %s to %s", kw, render(old), render(updated)))
	}
}

func (c *comparer) additionalProperties(pointer string, old any, inOld bool, updated any, inNew bool) {
	// An absent additionalProperties accepts any extra property, as true does.
	if !inOld {
		old = true
	}
	if !inNew {
		updated = true
	}
	if reflect.DeepEqual(old, updated) {
		return
	}
	if b, ok := old.(bool); ok && !b {
		return // nothing extra was accepted before, so nothing can be lost
	}
	if b, ok := updated.(bool); ok && b {
		return // everything extra is accepted now
	}
	if b, ok := updated.(bool); ok && !b {
		c.refuse(pointer, fmt.Sprintf("additionalProperties tightened from %s to false", render(old)))
		return
	}
	if b, ok := old.(bool); ok && b {
		c.refuse(pointer, fmt.Sprintf("additionalProperties tightened from any value to %s", render(updated)))
		return
	}
	c.node(pointer, old, updated)
}

func (c *comparer) schemaMap(pointer, kw string, old any, inOld bool, updated any, inNew bool) {
	if !inOld || !inNew {
		c.unclassified(pointer, presenceChange(kw, inOld))
		return
	}
	om, nm := asMap(old), asMap(updated)
	for _, name := range unionKeys(om, nm) {
		ov, a := om[name]
		nv, b := nm[name]
		at := pointer + "/" + escapePointer(name)
		if a && b {
			c.node(at, ov, nv)
			continue
		}
		c.unclassified(at, presenceChange(kw+" entry "+strconv.Quote(name), a))
	}
}

func (c *comparer) schemaList(pointer, kw string, old any, inOld bool, updated any, inNew bool) {
	if !inOld || !inNew {
		c.unclassified(pointer, presenceChange(kw, inOld))
		return
	}
	ol, nl := asList(old), asList(updated)
	if len(ol) != len(nl) {
		c.unclassified(pointer, fmt.Sprintf("%s changed from %d to %d branches", kw, len(ol), len(nl)))
		return
	}
	for i := range ol {
		c.node(pointer+"/"+strconv.Itoa(i), ol[i], nl[i])
	}
}

func presenceChange(what string, wasPresent bool) string {
	if wasPresent {
		return what + " removed"
	}
	return what + " added"
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func asFloat(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

func typeList(v any) []string {
	if s, ok := v.(string); ok {
		return []string{s}
	}
	var out []string
	for _, e := range asList(v) {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func containsValue(list []any, v any) bool {
	for _, e := range list {
		if reflect.DeepEqual(e, v) {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func unionKeys(a, b map[string]any) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range []map[string]any{a, b} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

// escapePointer escapes one JSON pointer reference token.
func escapePointer(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

// render prints a JSON value compactly for a message.
func render(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
