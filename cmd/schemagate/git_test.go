// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
