// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// schemaDir is the directory, at the root of the schemas repository, that holds
// every published schema file.
const schemaDir = "json"

// schemaFileName splits a schema file name into its stem and the API version it
// declares: component-v1beta1.schema.json is the component schema at v1beta1.
var schemaFileName = regexp.MustCompile(`^(.+)-(v[0-9]+(?:(?:alpha|beta)[0-9]+)?)\.schema\.json$`)

// releaseTag matches a release tag of the schemas repository.
var releaseTag = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

// isSchemaPath reports whether a repository-relative path names a published
// schema file.
func isSchemaPath(p string) bool {
	return path.Dir(p) == schemaDir && strings.HasSuffix(p, ".schema.json")
}

// apiVersionOf returns the API version a schema file name declares.
func apiVersionOf(file string) (string, error) {
	m := schemaFileName.FindStringSubmatch(path.Base(file))
	if m == nil {
		return "", fmt.Errorf("%s: the file name does not end in -<api version>.schema.json, so its API version cannot be read", file)
	}
	return m[2], nil
}

// releaseTags returns the repository's release tags, newest first.
func releaseTags(repo string) ([]string, error) {
	out, err := git(repo, "tag", "--list", "v*")
	if err != nil {
		return nil, err
	}
	type tagged struct {
		name string
		v    [3]int
	}
	var tags []tagged
	for _, name := range strings.Fields(string(out)) {
		m := releaseTag.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		var t tagged
		t.name = name
		for i := 0; i < 3; i++ {
			t.v[i], _ = strconv.Atoi(m[i+1])
		}
		tags = append(tags, t)
	}
	sort.Slice(tags, func(i, j int) bool {
		a, b := tags[i].v, tags[j].v
		for k := 0; k < 3; k++ {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return tags[i].name < tags[j].name
	})
	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.name
	}
	return names, nil
}

// schemaFilesAt returns the schema file paths a revision carries.
func schemaFilesAt(repo, rev string) ([]string, error) {
	entries, err := gitLines(repo, "ls-tree", "-z", "--name-only", rev, schemaDir+"/")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if isSchemaPath(e) {
			files = append(files, e)
		}
	}
	sort.Strings(files)
	return files, nil
}

// versionsIn returns the API versions a set of schema files declares, sorted.
func versionsIn(files []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range files {
		if v, err := apiVersionOf(f); err == nil && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// compatOptions is what one compatibility check reads.
type compatOptions struct {
	repo       string // the schemas repository
	baseline   string // a revision to compare with; empty selects the newest release per API version
	staged     bool   // read the schemas and the exceptions file from the index rather than the working tree
	exceptions string // an exceptions file to read instead of the one at the root of the judged schemas
}

// candidateSchemas returns the schemas being checked, keyed by repository-relative
// path. With staged it returns nil and no error when the index changes neither a
// schema nor the exceptions file, which leaves nothing to check.
func candidateSchemas(opts compatOptions) (map[string][]byte, error) {
	out := map[string][]byte{}
	if opts.staged {
		changed, err := gitIndexLines(opts.repo, "diff", "--cached", "--name-only", "-z", "--no-renames", "--", schemaDir+"/", exceptionsFile)
		if err != nil {
			return nil, err
		}
		touched := false
		for _, p := range changed {
			if isSchemaPath(p) || p == exceptionsFile {
				touched = true
			}
		}
		if !touched {
			return nil, nil
		}
		files, err := gitIndexLines(opts.repo, "ls-files", "-z", "--cached", "--", schemaDir+"/")
		if err != nil {
			return nil, err
		}
		for _, p := range files {
			if !isSchemaPath(p) {
				continue
			}
			data, err := gitIndex(opts.repo, "show", ":"+p)
			if err != nil {
				return nil, err
			}
			out[p] = data
		}
		return out, nil
	}
	entries, err := os.ReadDir(filepath.Join(opts.repo, schemaDir))
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		p := schemaDir + "/" + e.Name()
		if e.IsDir() || !isSchemaPath(p) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(opts.repo, schemaDir, e.Name()))
		if err != nil {
			return nil, err
		}
		out[p] = data
	}
	return out, nil
}

// runCompat compares the candidate schemas with their baseline, printing what it
// compared and every refused change to w. It returns whether anything was
// refused; an error means nothing was judged.
func runCompat(opts compatOptions, w io.Writer) (bool, error) {
	if err := requireFullHistory(opts.repo); err != nil {
		return false, err
	}
	candidates, err := candidateSchemas(opts)
	if err != nil {
		return false, err
	}
	source := "the working tree"
	if opts.staged {
		source = "the index"
		if candidates == nil {
			fmt.Fprintf(w, "schemagate compat: the index changes no schema file and not %s; nothing to compare.\n", exceptionsFile)
			return false, nil
		}
	}
	exceptions, exceptionsSource, err := compatExceptions(opts)
	if err != nil {
		return false, err
	}
	byVersion := map[string][]string{}
	for p := range candidates {
		v, err := apiVersionOf(p)
		if err != nil {
			return false, err
		}
		byVersion[v] = append(byVersion[v], p)
	}
	versions := make([]string, 0, len(byVersion))
	for v := range byVersion {
		versions = append(versions, v)
		sort.Strings(byVersion[v])
	}
	sort.Strings(versions)

	tags, err := releaseTags(opts.repo)
	if err != nil {
		return false, err
	}
	if opts.baseline == "" && len(tags) == 0 {
		// A clone made without its tags looks exactly like a repository that was
		// never released; passing it would pass every change unjudged.
		return false, fmt.Errorf("%s has no release tag, so no baseline can be chosen; fetch the tags (git fetch --tags) or name one with -baseline", opts.repo)
	}
	filesAt := map[string][]string{}
	lookup := func(rev string) ([]string, error) {
		if f, ok := filesAt[rev]; ok {
			return f, nil
		}
		f, err := schemaFilesAt(opts.repo, rev)
		if err != nil {
			return nil, err
		}
		filesAt[rev] = f
		return f, nil
	}

	fmt.Fprintf(w, "schemagate compat: %d schema files in %s, in %s\n", len(candidates), source, opts.repo)
	var found []finding
	compared := 0
	read := map[string]comparedSchema{}
	for _, v := range versions {
		files := byVersion[v]
		base := opts.baseline
		if base == "" {
			base, err = newestReleaseWith(tags, v, lookup)
			if err != nil {
				return false, err
			}
		}
		if base == "" {
			fmt.Fprintf(w, "  %s: no baseline — no release tag carries %s schemas yet%s, so %d schemas are compared with nothing and pass.\n",
				v, v, newestReleaseNote(tags, lookup), len(files))
			continue
		}
		baseFiles, err := lookup(base)
		if err != nil {
			return false, err
		}
		inBase := map[string]bool{}
		for _, f := range baseFiles {
			inBase[f] = true
		}
		if !containsString(versionsIn(baseFiles), v) {
			fmt.Fprintf(w, "  %s: %s carries no %s schemas, so %d schemas declaring a different API version from it are compared with nothing and pass.\n",
				v, base, v, len(files))
			continue
		}
		fmt.Fprintf(w, "  %s: compared with %s\n", v, base)
		inCandidates := map[string]bool{}
		for _, p := range files {
			inCandidates[p] = true
			if !inBase[p] {
				fmt.Fprintf(w, "    %s is new since %s; nothing to compare.\n", p, base)
				continue
			}
			oldData, err := git(opts.repo, "show", base+":"+p)
			if err != nil {
				return false, err
			}
			oldDoc, err := decodeSchema(oldData)
			if err != nil {
				return false, fmt.Errorf("%s at %s: %v", p, base, err)
			}
			newDoc, err := decodeSchema(candidates[p])
			if err != nil {
				return false, fmt.Errorf("%s in %s: %v", p, source, err)
			}
			found = append(found, compareSchemas(p, oldDoc, newDoc)...)
			read[path.Base(p)] = comparedSchema{old: oldDoc, updated: newDoc}
			compared++
		}
		for _, f := range baseFiles {
			if fv, err := apiVersionOf(f); err == nil && fv == v && !inCandidates[f] {
				found = append(found, finding{schema: f, pointer: "#", kind: "schema file removed", unclassified: true})
			}
		}
	}

	found, admitted := applyExceptions(found, exceptions, exceptionsSource, read)
	if len(admitted) > 0 {
		fmt.Fprintf(w, "admitted by %s:\n", exceptionsSource)
		for _, a := range admitted {
			fmt.Fprintf(w, "  %s — %s\n", a.finding, a.reason)
		}
	}
	if len(found) == 0 {
		if compared == 0 {
			fmt.Fprintln(w, "passed: no schema has a released baseline to compare with.")
		} else {
			fmt.Fprintf(w, "passed: %d schemas compared; every change is one this check accepts within one API version.\n", compared)
		}
		return false, nil
	}
	fmt.Fprintf(w, "refused: %s this check does not accept within one API version:\n", countChanges(len(found)))
	for _, f := range found {
		line := "  " + f.String()
		if f.unclassified {
			line += " (a change this check does not classify as additive)"
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w, "Within one API version a schema may only change its descriptions and add optional properties")
	fmt.Fprintln(w, "where the object accepted no extra property under their name, or where "+exceptionsFile)
	fmt.Fprintln(w, "says why no document can have used it. Removing or renaming a property, making one required,")
	fmt.Fprintln(w, "or narrowing a type, an enum or a value needs a new API version.")
	return true, nil
}

// newestReleaseWith returns the newest release tag carrying at least one schema
// at the API version, or the empty string when none does.
func newestReleaseWith(tags []string, version string, lookup func(string) ([]string, error)) (string, error) {
	for _, t := range tags {
		files, err := lookup(t)
		if err != nil {
			return "", err
		}
		if containsString(versionsIn(files), version) {
			return t, nil
		}
	}
	return "", nil
}

// newestReleaseNote names the newest release and the API versions it carries,
// for a message explaining why no baseline exists. tags is never empty here:
// a repository with no release tag is refused before any baseline is chosen.
func newestReleaseNote(tags []string, lookup func(string) ([]string, error)) string {
	files, err := lookup(tags[0])
	if err != nil || len(files) == 0 {
		return fmt.Sprintf(" (the newest release is %s)", tags[0])
	}
	return fmt.Sprintf(" (the newest release, %s, carries %s)", tags[0], strings.Join(versionsIn(files), ", "))
}

func countChanges(n int) string {
	if n == 1 {
		return "1 change"
	}
	return fmt.Sprintf("%d changes", n)
}

func containsString(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}
