// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// unnamedSource is a custom TableSource that exposes no Qualifier, so no
// predicate can be shown to reference it.
type unnamedSource struct{}

func (unnamedSource) appendFrom(b *Builder) { b.WriteString("unnamed") }

// Catches: a source without a Qualifier being skipped by the several-source
// guard, so a cross-joined custom source slips through. It must count as
// unconstrained; a single source is still not checked.
func TestUnnamedSourceIsUnconstrained(t *testing.T) {
	d := dialect.For(dialect.Postgres)
	ps := []Predicate{ColumnOf[int64]("a", "id").Eq(1)}
	if got := unconstrainedSource(d, ps, []TableSource{CTE("a"), unnamedSource{}}); got == "" {
		t.Errorf("two sources, one unnamed: unconstrainedSource = %q, want a non-empty name", got)
	}
	if got := unconstrainedSource(d, ps, []TableSource{unnamedSource{}}); got != "" {
		t.Errorf("one unnamed source: unconstrainedSource = %q, want empty (single sources are not checked)", got)
	}
	if got := unconstrainedSource(d, ps, []TableSource{CTE("a"), CTE("a")}); got != "" {
		t.Errorf("two constrained sources: unconstrainedSource = %q, want empty", got)
	}
}
