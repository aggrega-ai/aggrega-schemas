// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const registrySource = `package main

var kindSources = map[string]kindSource{
	"thing_gen.go": {"Thing", ""},
}

var enumSources = map[string]enumSource{
	"thing_enums_gen.rs": {},
}

var codecTargets = map[string]codecTarget{
	"thing_codec_gen.ts": {},
}
`

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestReadGeneratorRegistries reads every key of the three registries, and
// refuses each source shape that would leave a generated file unprotected: a
// missing registry, a key that is not a string literal, and a fourth map keyed
// by generated file names.
func TestReadGeneratorRegistries(t *testing.T) {
	names, err := parseGeneratorRegistries("main.go", []byte(registrySource))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"thing_gen.go", "thing_enums_gen.rs", "thing_codec_gen.ts"} {
		if !names[want] {
			t.Errorf("registry key %q not read; got %v", want, names)
		}
	}
	// A map keyed by something other than file names is not a registry.
	unrelated := registrySource + "\nvar importAliases = map[string]string{\"example.com/x\": \"x\"}\n"
	if _, err := parseGeneratorRegistries("main.go", []byte(unrelated)); err != nil {
		t.Fatalf("an unrelated map must not be refused; got %v", err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"registry missing", strings.Replace(registrySource, "var codecTargets = map[string]codecTarget{\n\t\"thing_codec_gen.ts\": {},\n}\n", "", 1), "no entries in codecTargets"},
		{"key not a string literal", strings.Replace(registrySource, `"thing_codec_gen.ts": {},`, `codecFile: {},`, 1), "not a string literal"},
		{"fourth registry", registrySource + "\nvar extraTargets = map[string]codecTarget{\n\t\"extra_gen.go\": {},\n}\n", "extraTargets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.src == registrySource {
				t.Fatal("the plant did not apply")
			}
			if _, err := parseGeneratorRegistries("main.go", []byte(tc.src)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want a refusal naming %q; got %v", tc.want, err)
			}
		})
	}
}

// TestProtection names a schema file and a registered codec as protected and
// leaves every other path alone.
func TestProtection(t *testing.T) {
	codecs := map[string]bool{"ir_gen.go": true}
	for p, want := range map[string]bool{
		"json/component-v1beta1.schema.json": true,
		"go/pkg/abi/ir_gen.go":               true,
		"pkg/types/ir_gen.go":                true,
		"json/nested/x.schema.json":          false,
		"docs/component.schema.json":         false,
		"go/pkg/abi/ir.go":                   false,
		"README.md":                          false,
	} {
		if got := protection(p, codecs) != ""; got != want {
			t.Errorf("protection(%q) = %v; want %v", p, got, want)
		}
	}
}

// gitRepo is a scratch repository driven by the real git binary.
type gitRepo struct {
	t   *testing.T
	dir string
}

func newGitRepo(t *testing.T) *gitRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	r := &gitRepo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	return r
}

func (r *gitRepo) git(args ...string) string {
	r.t.Helper()
	full := append([]string{"-C", r.dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid",
		"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (r *gitRepo) commit(msg string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

// TestRunEndToEnd drives both commands through run against a real repository:
// a repository with no release tag is not judged, an API version no release
// carries passes with no baseline, a planted required addition is refused
// against a release tag, an edit on main is refused, and a fast-forward to dev
// passes.
func TestRunEndToEnd(t *testing.T) {
	r := newGitRepo(t)
	writeFile(t, r.dir, "json/thing-v1alpha1.schema.json", baseSchema)
	r.commit("schema")

	var out, errOut bytes.Buffer
	if code := run([]string{"compat", "-repo", r.dir}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "no release tag") {
		t.Fatalf("no release tag at all: want exit 2 saying so; got %d\n%s%s", code, out.String(), errOut.String())
	}

	r.git("tag", "v0.0.1")
	r.git("mv", "json/thing-v1alpha1.schema.json", "json/thing-v1beta1.schema.json")
	r.commit("move to v1beta1")
	out.Reset()
	errOut.Reset()
	if code := run([]string{"compat", "-repo", r.dir}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "no baseline") {
		t.Fatalf("no release carries v1beta1: want exit 0 saying no baseline; got %d\n%s%s", code, out.String(), errOut.String())
	}

	r.git("tag", "v0.1.0")
	writeFile(t, r.dir, "json/thing-v1beta1.schema.json", strings.Replace(baseSchema, `"required": ["kind"]`, `"required": ["kind", "name"]`, 1))
	out.Reset()
	errOut.Reset()
	if code := run([]string{"compat", "-repo", r.dir}, &out, &errOut); code != 1 || !strings.Contains(out.String(), `#/required: "name" added to required`) {
		t.Fatalf("planted required addition: want exit 1 naming it; got %d\n%s%s", code, out.String(), errOut.String())
	}

	// Stage the edit on main: the freeze refuses it, naming the file.
	r.git("add", "-A")
	out.Reset()
	errOut.Reset()
	if code := run([]string{"freeze", "-repo", r.dir, "-staged"}, &out, &errOut); code != 1 || !strings.Contains(out.String(), "json/thing-v1beta1.schema.json (a schema file)") {
		t.Fatalf("staged edit on main: want exit 1 naming the file; got %d\n%s%s", code, out.String(), errOut.String())
	}
	r.git("reset", "-q", "--hard")

	// A description change on dev, then main fast-forwarded to it: the range
	// comparison passes; a commit made on main afterwards is refused.
	r.git("checkout", "-q", "-b", "dev")
	writeFile(t, r.dir, "json/thing-v1beta1.schema.json", strings.Replace(baseSchema, `"A thing."`, `"A described thing."`, 1))
	r.commit("describe")
	r.git("checkout", "-q", "main")
	r.git("merge", "-q", "--ff-only", "dev")
	out.Reset()
	errOut.Reset()
	if code := run([]string{"freeze", "-repo", r.dir, "-head", "main", "-dev", "dev"}, &out, &errOut); code != 0 {
		t.Fatalf("fast-forward to dev: want exit 0; got %d\n%s%s", code, out.String(), errOut.String())
	}
	writeFile(t, r.dir, "json/thing-v1beta1.schema.json", baseSchema)
	r.commit("edit on main")
	out.Reset()
	errOut.Reset()
	if code := run([]string{"freeze", "-repo", r.dir, "-head", "main", "-dev", "dev"}, &out, &errOut); code != 1 || !strings.Contains(out.String(), "json/thing-v1beta1.schema.json (a schema file)") {
		t.Fatalf("edit made on main: want exit 1 naming the file; got %d\n%s%s", code, out.String(), errOut.String())
	}
}

// TestGitIgnoresInheritedLocators requires a git command aimed at one repository
// to act on it even when the environment names another, as a git hook's does,
// and an index read to keep the inherited index file.
func TestGitIgnoresInheritedLocators(t *testing.T) {
	a, b := newGitRepo(t), newGitRepo(t)
	t.Setenv("GIT_DIR", filepath.Join(a.dir, ".git"))
	t.Setenv("GIT_INDEX_FILE", "staged-index")
	out, err := git(b.dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(b.dir, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("git -C %s acted on %s; want %s", b.dir, got, want)
	}
	for _, kv := range gitEnv(false) {
		if strings.HasPrefix(kv, "GIT_DIR=") || strings.HasPrefix(kv, "GIT_INDEX_FILE=") {
			t.Fatalf("gitEnv(false) kept %s", kv)
		}
	}
	kept := false
	for _, kv := range gitEnv(true) {
		if strings.HasPrefix(kv, "GIT_DIR=") {
			t.Fatalf("gitEnv(true) kept %s", kv)
		}
		if v, ok := strings.CutPrefix(kv, "GIT_INDEX_FILE="); ok {
			kept = filepath.IsAbs(v) && filepath.Base(v) == "staged-index"
		}
	}
	if !kept {
		t.Fatal("gitEnv(true) must keep GIT_INDEX_FILE, made absolute")
	}
}

// TestFreezeRegistryUnion requires a change that removes a codec's registry
// entry and edits that codec in the same change to be refused: a registry inside
// the judged repository is read at both ends of the change, for a commit and for
// a pushed head alike.
func TestFreezeRegistryUnion(t *testing.T) {
	r := newGitRepo(t)
	twoTargets := strings.Replace(registrySource, `"thing_codec_gen.ts": {},`, "\"thing_codec_gen.ts\": {},\n\t\"other_codec_gen.ts\": {},", 1)
	registry := writeFile(t, r.dir, "internal/tools/typegen/main.go", twoTargets)
	writeFile(t, r.dir, "src/thing_codec_gen.ts", "// generated\n")
	r.commit("generator and codec")
	r.git("branch", "dev")

	// The plant: drop the codec's registry entry and edit the codec, together.
	writeFile(t, r.dir, "internal/tools/typegen/main.go", strings.Replace(twoTargets, "\t\"thing_codec_gen.ts\": {},\n", "", 1))
	writeFile(t, r.dir, "src/thing_codec_gen.ts", "// edited on main\n")
	r.git("add", "-A")

	var out, errOut bytes.Buffer
	if code := run([]string{"freeze", "-repo", r.dir, "-registry", registry, "-staged"}, &out, &errOut); code != 1 || !strings.Contains(out.String(), "src/thing_codec_gen.ts (a codec generated from a schema)") {
		t.Fatalf("staged: want exit 1 naming the codec; got %d\n%s%s", code, out.String(), errOut.String())
	}

	r.git("commit", "-q", "-m", "unprotect and edit")
	out.Reset()
	errOut.Reset()
	if code := run([]string{"freeze", "-repo", r.dir, "-registry", registry, "-head", "main", "-dev", "dev"}, &out, &errOut); code != 1 || !strings.Contains(out.String(), "src/thing_codec_gen.ts (a codec generated from a schema)") {
		t.Fatalf("two heads: want exit 1 naming the codec; got %d\n%s%s", code, out.String(), errOut.String())
	}
}
