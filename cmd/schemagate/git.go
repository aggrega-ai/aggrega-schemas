// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// repositoryLocators are the environment variables through which git names the
// repository, work tree and index it acts on. A git hook runs with them exported
// for the repository being committed to, so a git command aimed at any other
// repository must not inherit them: it would act on the committing repository
// instead.
var repositoryLocators = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_PREFIX",
	"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE",
}

// git runs one git command in repo and returns its standard output. The
// repository is found from repo alone, never from an inherited locator. A
// failure carries git's own standard error, so the reason reaches the caller
// intact.
func git(repo string, args ...string) ([]byte, error) {
	return runGit(repo, false, args...)
}

// gitIndex is git for a command that reads the committing repository's index.
// It keeps an inherited GIT_INDEX_FILE, because a commit in progress may stage
// into an index other than the repository's own file and a pre-commit hook
// names it there; every other locator is dropped as for git.
func gitIndex(repo string, args ...string) ([]byte, error) {
	return runGit(repo, true, args...)
}

func runGit(repo string, keepIndex bool, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = gitEnv(keepIndex)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// gitEnv is this process's environment without the repository locators; with
// keepIndex an inherited GIT_INDEX_FILE stays, made absolute against this
// process's working directory, where git resolved it.
func gitEnv(keepIndex bool) []string {
	var env []string
	for _, kv := range os.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		if !containsString(repositoryLocators, name) {
			env = append(env, kv)
			continue
		}
		if name == "GIT_INDEX_FILE" && keepIndex && value != "" {
			if abs, err := filepath.Abs(value); err == nil {
				value = abs
			}
			env = append(env, name+"="+value)
		}
	}
	return env
}

// gitLines runs one git command whose output is a NUL-separated list (its -z
// form) and returns the entries.
func gitLines(repo string, args ...string) ([]string, error) {
	out, err := git(repo, args...)
	if err != nil {
		return nil, err
	}
	return splitNUL(out), nil
}

// gitIndexLines is gitLines for a command that reads the index.
func gitIndexLines(repo string, args ...string) ([]string, error) {
	out, err := gitIndex(repo, args...)
	if err != nil {
		return nil, err
	}
	return splitNUL(out), nil
}

func splitNUL(out []byte) []string {
	var lines []string
	for _, l := range strings.Split(string(out), "\x00") {
		if l = strings.TrimRight(l, "\n"); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// requireFullHistory refuses a shallow clone: its missing tags and history would
// make the comparison silently find no baseline, or no common commit, and pass.
func requireFullHistory(repo string) error {
	out, err := git(repo, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) == "true" {
		return fmt.Errorf("%s is a shallow clone, so its tags and history are incomplete; fetch the full history (for example `git fetch --unshallow --tags`) and run again", repo)
	}
	return nil
}
