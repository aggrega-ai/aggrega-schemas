// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Cognitio S.r.l.

package schemas

import (
	"regexp"
	"testing"
)

// hostBearingSchemaRef matches the host-bearing SchemaLibrary cross-document
// $ref form (oci://<host>/<library>@<version>#/schemas/<entry>) that the
// compiler rejects (ErrHostInLibraryRef). A SchemaLibrary $ref addresses its
// target by bare-name identity (<Library>#/schemas/<entry>) with no registry
// host, so a schema description teaching the host-bearing form contradicts the
// compiler.
//
// Scope is the SchemaLibrary $ref surface (#/schemas/) only. The MappingLibrary
// $import surface (#/mappings/) still recognises a library import by the oci://
// prefix (the renderer's HasOCIPrefix), so its host-bearing form is the live
// discrimination form, not a rejected one — extending the guard to #/mappings/
// awaits the $import reconciliation that moves it to bare-name identity. The
// character class stops the match at spaces, double-quotes, and backticks so a
// truncated rejection mention (oci://<host>/…, no fragment) does not trip it.
var hostBearingSchemaRef = regexp.MustCompile("oci://[^ \"`]*#/schemas/")

// TestNoHostBearingSchemaRefInSchemas fails if any embedded schema description
// teaches the rejected host-bearing SchemaLibrary $ref form. This is the
// recurrence guard for the identity-reference doc model: the schema
// `description` is the documented element, and it must agree with the
// compiler's rejection of host-bearing $ref references.
func TestNoHostBearingSchemaRefInSchemas(t *testing.T) {
	t.Parallel()
	for _, s := range allSchemas {
		s := s
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			if got := hostBearingSchemaRef.FindString(s.source); got != "" {
				t.Errorf("%s teaches the rejected host-bearing SchemaLibrary $ref form %q; use the bare-name identity form <Library>#/schemas/<entry> (no host)", s.name, got)
			}
		})
	}
}

// TestHostBearingSchemaRefMatcherIsNonVacuous proves the matcher actually fires
// on the rejected form, so the guard above cannot pass vacuously on a broken
// pattern.
func TestHostBearingSchemaRefMatcherIsNonVacuous(t *testing.T) {
	t.Parallel()
	const planted = "{$ref: oci://hub.aggrega.ai/schemalibrary/core/DbSizing@1.0.0#/schemas/Storage}"
	if !hostBearingSchemaRef.MatchString(planted) {
		t.Fatalf("matcher failed to catch a planted host-bearing $ref %q; the guard is vacuous", planted)
	}
}
