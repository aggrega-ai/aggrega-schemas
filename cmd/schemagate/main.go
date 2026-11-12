// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio s.r.l.

// Command schemagate holds the published schemas to their compatibility rule
// on the two release branches.
//
// Within one API version a schema changes only additively: a description may
// change and an optional property may be added. Removing or renaming a
// property, making one required, and narrowing what a value may be each need a
// new API version.
//
//	schemagate compat [-repo DIR] [-baseline REV] [-staged]
//
// compares every schema in json/ with the same schema at the newest release tag
// carrying that API version, and refuses a removed or renamed property, a
// property added to required, a narrowed type, a removed enum member, an added
// or changed const or pattern, a tightened minimum, maximum or
// additionalProperties, a removed $defs entry, and any other change it does not
// classify as additive. It passes a changed description and an added optional
// property — including one added where an object accepted any extra property
// before, so a document that already used that name with a different kind of
// value can stop validating. A schema whose API version no release carries yet
// is compared with nothing and passes, saying so; a repository with no release
// tag at all is not judged. It is the check for the development branch.
//
//	schemagate freeze [-repo DIR] [-registry FILE]... (-staged | -head REV -dev REV) [-list]
//
// refuses any change to a schema file or to a codec generated from one. The
// codecs are named by the codec generator's registries, read from its source
// file at run time. A source inside the judged repository is read at both ends
// of the judged change, so the change cannot remove its own protection. With
// -head and -dev it judges what -head changes since the newest commit it shares
// with -dev, so a fast-forward to the development branch passes and an edit
// made on the release branch does not. It is the check for the release branch.
//
// Exit status: 0 passed, 1 refused, 2 nothing was judged (a usage error, an
// unreadable repository, a shallow clone, a repository with no release tag, or
// registries that cannot be read).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

const usage = `usage:
  schemagate compat [-repo DIR] [-baseline REV] [-staged]
  schemagate freeze [-repo DIR] [-registry FILE]... (-staged | -head REV -dev REV) [-list]

compat  compares the schemas with the last release carrying their API version and
        refuses a change it does not accept within one API version.
freeze  refuses any change to a schema file or to a codec generated from one.

Run "schemagate compat -h" or "schemagate freeze -h" for each command's flags.
Exit status: 0 passed, 1 refused, 2 nothing was judged.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	var (
		refused bool
		err     error
	)
	switch args[0] {
	case "compat":
		fs := flag.NewFlagSet("schemagate compat", flag.ContinueOnError)
		fs.SetOutput(stderr)
		var opts compatOptions
		fs.StringVar(&opts.repo, "repo", ".", "the schemas repository")
		fs.StringVar(&opts.baseline, "baseline", "", "compare with this revision instead of the newest release carrying each API version")
		fs.BoolVar(&opts.staged, "staged", false, "check the schemas in the index, and only when it changes one")
		if err := fs.Parse(args[1:]); err != nil {
			return parseStatus(err)
		}
		if fs.NArg() != 0 {
			fmt.Fprintf(stderr, "schemagate compat: unexpected argument %q\n", fs.Arg(0))
			return 2
		}
		refused, err = runCompat(opts, stdout)
	case "freeze":
		fs := flag.NewFlagSet("schemagate freeze", flag.ContinueOnError)
		fs.SetOutput(stderr)
		var opts freezeOptions
		fs.StringVar(&opts.repo, "repo", ".", "the repository whose change is judged")
		fs.Var(&opts.registries, "registry", "the codec generator's source file, whose registries name the generated codecs; repeat it to read several, and without it only schema files are protected")
		fs.BoolVar(&opts.staged, "staged", false, "judge the change the index stages")
		fs.StringVar(&opts.head, "head", "", "judge the change this revision carries ...")
		fs.StringVar(&opts.dev, "dev", "", "... since the newest commit it shares with this revision")
		fs.BoolVar(&opts.list, "list", false, "print the protected files the repository tracks, and judge nothing")
		if err := fs.Parse(args[1:]); err != nil {
			return parseStatus(err)
		}
		if fs.NArg() != 0 {
			fmt.Fprintf(stderr, "schemagate freeze: unexpected argument %q\n", fs.Arg(0))
			return 2
		}
		ranged := opts.head != "" || opts.dev != ""
		switch {
		case opts.list && (opts.staged || ranged):
			fmt.Fprintln(stderr, "schemagate freeze: -list judges nothing, so it takes neither -staged nor -head and -dev")
			return 2
		case !opts.list && opts.staged == ranged:
			fmt.Fprintln(stderr, "schemagate freeze: give either -staged, or -head and -dev")
			return 2
		case ranged && (opts.head == "" || opts.dev == ""):
			fmt.Fprintln(stderr, "schemagate freeze: -head and -dev go together")
			return 2
		}
		refused, err = runFreeze(opts, stdout)
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "schemagate: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "schemagate %s: nothing was judged: %v\n", args[0], err)
		return 2
	}
	if refused {
		return 1
	}
	return 0
}

// parseStatus is the exit status for a flag-parsing error: asking for the flags
// with -h is not a failure.
func parseStatus(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 2
}
