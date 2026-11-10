// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

type scopeRow struct{ Title string }

var scopeRowMeta = Meta[scopeRow]{
	Columns:    []ColumnMeta{{Name: "title"}},
	Ptr:        func(m *scopeRow, _ int) any { return &m.Title },
	Value:      func(m *scopeRow, _ int) any { return m.Title },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
}

func (*scopeRow) MomoMeta() *Meta[scopeRow] { return &scopeRowMeta }

// A mutation builder renders inside a target-table scope; a correlated
// subquery in its WHERE may reference that target.
func TestMutationScopeAllowsCorrelatedSubquery(t *testing.T) {
	inner := Select[scopeRow](nil).
		From(TableRef{Name: "documents", Alias: "i"}).
		Columns(NewColumn[string]("i", "title", 0)).
		Where(NewColumn[int64]("i", "tenant_id", 0).EqExpr(NewColumn[int64]("t", "tenant_id", 0)))
	render := func(target string) error {
		_, err := compile(dialect.For(dialect.Postgres), func(b *Builder) {
			b.withScope([]string{target}, func() { Exists(inner).AppendSQL(b) })
		})
		return err
	}
	if err := render("t"); err != nil {
		t.Fatalf("target scope t: %v", err)
	}
	if err := render("other"); !errors.Is(err, ErrValidation) {
		t.Fatalf("wrong target scope = %v, want ErrValidation", err)
	}
}
