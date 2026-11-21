// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// exceptionsFile is the file, at the root of the schemas repository, that
// admits a property added where an object accepted any extra property before.
// Its absence admits nothing.
const exceptionsFile = "compat-exceptions.json"

// exception admits one property added to an object that accepted any extra
// property under its name before: the schema file in json/ by its file name,
// the JSON pointer of the object, the property's name, and why no document
// written before can have used that name with another kind of value.
type exception struct {
	Schema   string `json:"schema"`
	Pointer  string `json:"pointer"`
	Property string `json:"property"`
	Reason   string `json:"reason"`
}

// exceptionsDocument is the shape of the exceptions file.
type exceptionsDocument struct {
	Exceptions *[]exception `json:"exceptions"`
}

// loadExceptions reads an exceptions file from disk. A file that cannot be read
// returns its error wrapped, so a caller can tell a missing file apart.
func loadExceptions(path string) ([]exception, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("the exceptions file: %w", err)
	}
	return parseExceptions(path, data)
}

// parseExceptions decodes an exceptions file and refuses every shape that
// could admit something other than what it appears to: an unknown key, a
// missing list, an entry missing a field or carrying a blank reason, a schema
// named with its directory, a pointer that is not a fragment, a repeated entry,
// and anything after the document. source names the file in each error.
func parseExceptions(source string, data []byte) ([]exception, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var doc exceptionsDocument
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %v", source, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: something follows after the exceptions object", source)
	}
	if doc.Exceptions == nil {
		return nil, fmt.Errorf(`%s: has no exceptions list; write {"exceptions": []} to admit nothing`, source)
	}
	entries := *doc.Exceptions
	for i, e := range entries {
		n := i + 1
		switch {
		case e.Schema == "":
			return nil, fmt.Errorf("%s: entry %d has no schema", source, n)
		case e.Pointer == "":
			return nil, fmt.Errorf("%s: entry %d has no pointer", source, n)
		case e.Property == "":
			return nil, fmt.Errorf("%s: entry %d has no property", source, n)
		case strings.TrimSpace(e.Reason) == "":
			return nil, fmt.Errorf("%s: entry %d has no reason", source, n)
		case strings.Contains(e.Schema, "/") || !isSchemaPath(schemaDir+"/"+e.Schema):
			return nil, fmt.Errorf("%s: entry %d names its schema as %s; name a schema file in %s/ by its file name alone, such as %s",
				source, n, e.Schema, schemaDir, path.Base(e.Schema))
		case e.Pointer != "#" && !strings.HasPrefix(e.Pointer, "#/"):
			return nil, fmt.Errorf("%s: entry %d has the pointer %q; a pointer is a fragment from the schema's root, such as #/properties/spec, as the refusal prints it",
				source, n, e.Pointer)
		}
		for j := 0; j < i; j++ {
			if entries[j].Schema == e.Schema && entries[j].Pointer == e.Pointer && entries[j].Property == e.Property {
				return nil, fmt.Errorf("%s: entry %d repeats entry %d", source, n, j+1)
			}
		}
	}
	return entries, nil
}

// compatExceptions reads the exceptions the check applies, and names where it
// read them for its messages: the file -exceptions names, or else the
// exceptions file at the root of the judged schemas, from the index under
// -staged and from the working tree otherwise. An exceptions file absent from
// the judged schemas admits nothing; a file -exceptions names must exist.
func compatExceptions(opts compatOptions) ([]exception, string, error) {
	if opts.exceptions != "" {
		exs, err := loadExceptions(opts.exceptions)
		return exs, opts.exceptions, err
	}
	if opts.staged {
		source := exceptionsFile + " in the index"
		staged, err := gitIndexLines(opts.repo, "ls-files", "-z", "--cached", "--", exceptionsFile)
		if err != nil || len(staged) == 0 {
			return nil, source, err
		}
		data, err := gitIndex(opts.repo, "show", ":"+exceptionsFile)
		if err != nil {
			return nil, source, err
		}
		exs, err := parseExceptions(source, data)
		return exs, source, err
	}
	exs, err := loadExceptions(filepath.Join(opts.repo, exceptionsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, exceptionsFile, nil
	}
	return exs, exceptionsFile, err
}

// admission is a refusal an exceptions entry admits, with the entry's reason.
type admission struct {
	finding finding
	reason  string
}

// comparedSchema is one schema file a comparison read at both ends: old at the
// baseline, updated in the judged tree.
type comparedSchema struct {
	old, updated any
}

// applyExceptions removes from found every property addition an entry names,
// returning what remains and what was admitted. An entry that names no finding
// is itself refused, so the file cannot keep an admission nothing needs, unless
// the property it names is declared at that pointer by both the judged schema
// and its baseline: that is an admission a published release already carries,
// which a comparison with an earlier release still needs. compared holds the
// schemas this comparison read, keyed by file name; a schema it did not read
// declares nothing. source names the exceptions file in a refusal.
func applyExceptions(found []finding, exs []exception, source string, compared map[string]comparedSchema) ([]finding, []admission) {
	used := make([]bool, len(exs))
	var kept []finding
	var admitted []admission
	for _, f := range found {
		matched := -1
		if f.added != "" {
			for i, e := range exs {
				if path.Base(f.schema) == e.Schema && f.pointer == e.Pointer && f.added == e.Property {
					matched = i
					break
				}
			}
		}
		if matched < 0 {
			kept = append(kept, f)
			continue
		}
		used[matched] = true
		admitted = append(admitted, admission{finding: f, reason: exs[matched].Reason})
	}
	for i, e := range exs {
		if used[i] {
			continue
		}
		if doc, ok := compared[e.Schema]; ok &&
			declaresProperty(doc.updated, e.Pointer, e.Property) && declaresProperty(doc.old, e.Pointer, e.Property) {
			continue
		}
		kept = append(kept, finding{
			schema:  schemaDir + "/" + e.Schema,
			pointer: e.Pointer,
			kind: fmt.Sprintf("%s excepts property %q, but no addition of it is refused here and the schema and its baseline do not both declare it at this pointer; remove the entry",
				source, e.Property),
		})
	}
	return kept, admitted
}

// declaresProperty reports whether the node at pointer, a fragment such as
// #/properties/spec, lists name under its properties. A pointer that leads
// nowhere declares nothing.
func declaresProperty(doc any, pointer, name string) bool {
	node := doc
	if pointer != "#" {
		for _, token := range strings.Split(strings.TrimPrefix(pointer, "#/"), "/") {
			token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
			switch n := node.(type) {
			case map[string]any:
				node = n[token]
			case []any:
				i, err := strconv.Atoi(token)
				if err != nil || i < 0 || i >= len(n) {
					return false
				}
				node = n[i]
			default:
				return false
			}
		}
	}
	_, ok := asMap(asMap(node)["properties"])[name]
	return ok
}
