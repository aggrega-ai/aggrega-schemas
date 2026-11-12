// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// generatorRegistries are the variables in the codec generator's source whose
// keys are the file names of everything it writes from a schema. The protected
// set is read from them at run time, so a target registered there is protected
// with no second list to edit.
var generatorRegistries = []string{"kindSources", "enumSources", "codecTargets"}

// generatedFileKey matches a map key that names a generated source file. A
// package-level map keyed that way which is not one of generatorRegistries is a
// registry this check would not read, so its presence refuses the run.
var generatedFileKey = regexp.MustCompile(`\.(go|rs|ts)$`)

// parseGeneratorRegistries parses one version of the codec generator's source
// and returns the file names its registries declare. It refuses a source in
// which a registry is missing or empty, in which a registry entry's key is not
// a string literal, or which declares another package-level map keyed by
// generated file names: each would leave a generated file unprotected and pass
// every change to it.
func parseGeneratorRegistries(name string, src []byte) (map[string]bool, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("reading the codec generator's registries: %v", err)
	}
	names := map[string]bool{}
	counts := map[string]int{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, ident := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.CompositeLit)
				if !ok {
					continue
				}
				if !containsString(generatorRegistries, ident.Name) {
					if _, isMap := lit.Type.(*ast.MapType); isMap && keysNameGeneratedFiles(lit) {
						return nil, fmt.Errorf("%s declares %s, a map keyed by generated file names that is not one of %s, so the files it names would go unprotected",
							name, ident.Name, strings.Join(generatorRegistries, ", "))
					}
					continue
				}
				for _, elt := range lit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						return nil, fmt.Errorf("%s: %s holds an entry that is not a key and a value, so its file name cannot be read", name, ident.Name)
					}
					key, ok := kv.Key.(*ast.BasicLit)
					if !ok || key.Kind != token.STRING {
						return nil, fmt.Errorf("%s: %s holds an entry whose key is not a string literal, so its file name cannot be read", name, ident.Name)
					}
					file, err := strconv.Unquote(key.Value)
					if err != nil {
						return nil, fmt.Errorf("%s: %s holds an unreadable key %s: %v", name, ident.Name, key.Value, err)
					}
					names[file] = true
					counts[ident.Name]++
				}
			}
		}
	}
	for _, r := range generatorRegistries {
		if counts[r] == 0 {
			return nil, fmt.Errorf("%s declares no entries in %s, so the protected codecs cannot be derived", name, r)
		}
	}
	return names, nil
}

// keysNameGeneratedFiles reports whether a map literal has a string-literal key
// naming a generated source file.
func keysNameGeneratedFiles(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.BasicLit)
		if !ok || key.Kind != token.STRING {
			continue
		}
		if s, err := strconv.Unquote(key.Value); err == nil && generatedFileKey.MatchString(s) {
			return true
		}
	}
	return false
}

// protection says why a path is protected, or returns the empty string when it
// is not.
func protection(p string, codecs map[string]bool) string {
	if isSchemaPath(p) {
		return "a schema file"
	}
	if codecs[path.Base(p)] {
		return "a codec generated from a schema"
	}
	return ""
}

// stringList is a flag that may be given more than once.
type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

// freezeOptions is what one freeze check reads.
type freezeOptions struct {
	repo       string     // the repository whose change is judged
	registries stringList // the codec generator's source files; none protects schema files only
	staged     bool       // judge the index against HEAD
	head       string     // judge this revision ...
	dev        string     // ... against the newest commit it shares with this branch
	list       bool       // print the protected tracked paths instead of judging a change
}

// registryRead is one version of the generator's source to read: a revision of
// a repository (rev, with "" meaning its index) or, with repo empty, a file.
type registryRead struct {
	label     string
	repo, rev string // read rev:rel from repo
	rel       string
	file      string // or read this file
}

// registryReads decides which versions of a generator source file the protected
// set is read from. A source inside the judged repository is read at both ends
// of the judged change — the comparison base and the head, or HEAD and the index
// for a commit — so a change cannot remove its own protection by editing the
// registry it is judged against. A source elsewhere is read as checked out and
// at its own HEAD.
func registryReads(opts freezeOptions, file, base string) ([]registryRead, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(abs)
	srcCommon, srcErr := gitCommonDir(dir)
	top, topErr := git(dir, "rev-parse", "--show-toplevel")
	if srcErr != nil || topErr != nil {
		return []registryRead{{label: file, file: abs}}, nil
	}
	root, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		resolved = abs
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)
	judgedCommon, err := gitCommonDir(opts.repo)
	if err != nil {
		return nil, err
	}
	if srcCommon == judgedCommon && !opts.list {
		if opts.staged {
			return []registryRead{
				{label: "HEAD:" + rel, repo: opts.repo, rev: "HEAD", rel: rel},
				{label: "the index's " + rel, repo: opts.repo, rev: "", rel: rel},
			}, nil
		}
		return []registryRead{
			{label: base + ":" + rel, repo: opts.repo, rev: base, rel: rel},
			{label: opts.head + ":" + rel, repo: opts.repo, rev: opts.head, rel: rel},
		}, nil
	}
	return []registryRead{
		{label: file, file: abs},
		{label: "HEAD:" + rel + " in " + root, repo: dir, rev: "HEAD", rel: rel},
	}, nil
}

// readRegistries returns the union of the file names every readable version of
// every generator source declares. A version that does not exist — the file
// absent at a revision, or not checked out — is skipped, but each source must
// yield at least one version, and every version read must parse.
func readRegistries(opts freezeOptions, base string) (map[string]bool, error) {
	codecs := map[string]bool{}
	for _, file := range opts.registries {
		reads, err := registryReads(opts, file, base)
		if err != nil {
			return nil, err
		}
		readAny := false
		for _, r := range reads {
			var src []byte
			if r.file != "" {
				if src, err = os.ReadFile(r.file); err != nil {
					continue
				}
			} else {
				// An empty rev names the index, read the way the commit being
				// judged stages it.
				read := git
				if r.rev == "" {
					read = gitIndex
				}
				spec := r.rev + ":" + r.rel
				if _, err := read(r.repo, "cat-file", "-e", spec); err != nil {
					continue
				}
				if src, err = read(r.repo, "show", spec); err != nil {
					return nil, err
				}
			}
			names, err := parseGeneratorRegistries(r.label, src)
			if err != nil {
				return nil, err
			}
			for n := range names {
				codecs[n] = true
			}
			readAny = true
		}
		if !readAny {
			return nil, fmt.Errorf("%s: no version of the codec generator's source could be read", file)
		}
	}
	return codecs, nil
}

// gitCommonDir names the repository a directory belongs to, the same for every
// worktree of it.
func gitCommonDir(dir string) (string, error) {
	out, err := git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return p, nil
}

// runFreeze judges a change to a branch that holds released schemas: any change
// to a schema file or to a codec generated from one is refused. It returns
// whether anything was refused; an error means nothing was judged.
func runFreeze(opts freezeOptions, w io.Writer) (bool, error) {
	if opts.list {
		codecs, err := readRegistries(opts, "")
		if err != nil {
			return false, err
		}
		tracked, err := gitLines(opts.repo, "ls-files", "-z")
		if err != nil {
			return false, err
		}
		var protected []string
		for _, p := range tracked {
			if protection(p, codecs) != "" {
				protected = append(protected, p)
			}
		}
		sort.Strings(protected)
		fmt.Fprintf(w, "schemagate freeze: %d protected files tracked in %s\n", len(protected), opts.repo)
		for _, p := range protected {
			fmt.Fprintf(w, "  %s (%s)\n", p, protection(p, codecs))
		}
		return false, nil
	}

	var changed []string
	var against, base string
	if opts.staged {
		var err error
		if changed, err = gitIndexLines(opts.repo, "diff", "--cached", "--name-only", "-z", "--no-renames"); err != nil {
			return false, err
		}
		against = "this commit"
	} else {
		if err := requireFullHistory(opts.repo); err != nil {
			return false, err
		}
		out, err := git(opts.repo, "merge-base", opts.head, opts.dev)
		if err != nil {
			return false, fmt.Errorf("%s and %s share no commit, so the change cannot be judged: %v", opts.head, opts.dev, err)
		}
		base = strings.TrimSpace(string(out))
		if changed, err = gitLines(opts.repo, "diff", "--name-only", "-z", "--no-renames", base, opts.head); err != nil {
			return false, err
		}
		short := base
		if len(short) > 12 {
			short = short[:12]
		}
		against = fmt.Sprintf("%s, compared with %s — the newest commit it shares with %s —", opts.head, short, opts.dev)
	}
	codecs, err := readRegistries(opts, base)
	if err != nil {
		return false, err
	}

	var refused []string
	for _, p := range changed {
		if why := protection(p, codecs); why != "" {
			refused = append(refused, fmt.Sprintf("  %s (%s)", p, why))
		}
	}
	if len(refused) == 0 {
		fmt.Fprintf(w, "schemagate freeze: passed: %s changes %d paths, none of them a schema file or a codec generated from one.\n", against, len(changed))
		return false, nil
	}
	fmt.Fprintf(w, "refused: %s changes what main holds as released:\n", against)
	for _, r := range refused {
		fmt.Fprintln(w, r)
	}
	fmt.Fprintln(w, "Schema changes land on dev and reach main only by a release, which fast-forwards main to dev.")
	fmt.Fprintln(w, "Make this change on dev.")
	return true, nil
}
