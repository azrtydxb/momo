// SPDX-License-Identifier: Apache-2.0

package fts

import (
	"fmt"
	"slices"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/exprkit"
	"github.com/azrtydxb/momo/schema"
	"github.com/azrtydxb/momo/types"
)

type targetKind uint8

const (
	kindDeclared targetKind = iota // columns of a model's full-text declaration
	kindVector                     // a hand-managed tsvector column (PostgreSQL)
)

// Target is what Search, Score and Snippet search. Build one with Over or
// OverVector. A target that cannot be built (for example a model without a
// full-text declaration) fails the statement that uses it with ErrValidation.
type Target struct {
	kind targetKind
	// declared targets
	table     string // qualifier of the source columns
	tbl       *schema.Table
	cols      []momo.Column[string]
	ft        *schema.FullText
	indexName string
	// vector targets
	vec    momo.Column[types.TSVector]
	config string

	err error
}

// validationError is an ErrValidation build error.
func validationError(format string, args ...any) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "build", Err: fmt.Errorf(format, args...)}
}

// Over is the target of the full-text declaration of model T. cols must be
// exactly the declared columns, in the declared order; the generated
// descriptors make a typo a compile error. On PostgreSQL the search runs over
// the declared generated tsvector column (or the declared expression), on
// SQLite through the FTS5 table of the model.
func Over[T any, PT momo.Modeler[T]](cols ...momo.Column[string]) Target {
	t := Target{kind: kindDeclared, cols: cols}
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.ColumnName()
	}
	list := strings.Join(names, ", ")
	if len(cols) == 0 {
		t.err = validationError("fts.Over needs at least one column")
		return t
	}
	t.table = cols[0].Table()
	ts := exprkit.TableSchema[T, PT]()
	if ts == nil {
		t.err = validationError("fts.Over(%s): the model has no table definition", list)
		return t
	}
	ix, ok := ts.FullTextIndex()
	if !ok {
		t.err = validationError("fts.Over(%s): table %s declares no full-text search", list, ts.Name)
		return t
	}
	if err := ix.FullText.Validate(); err != nil {
		t.err = validationError("fts.Over(%s): index %s: %v", list, ix.Name, err)
		return t
	}
	if !slices.Equal(names, ix.FullText.Columns) {
		t.err = validationError("fts.Over(%s): the columns are not the declared full-text columns (%s) of table %s",
			list, strings.Join(ix.FullText.Columns, ", "), ts.Name)
		return t
	}
	t.tbl, t.ft, t.indexName = ts, ix.FullText, ix.Name
	return t
}

// OverVector is the target of a tsvector column the application maintains
// itself (a trigger, or a generated column it declared by hand), searched with
// the text search configuration config. PostgreSQL only; other dialects fail
// the build with ErrUnsupported. The configuration is bound as a parameter, as
// in the other fts builders.
func OverVector(col momo.Column[types.TSVector], config string) Target {
	return Target{kind: kindVector, vec: col, config: config}
}

// check records the target's own build error and reports whether it is usable.
func (t Target) check(b *momo.Builder) bool {
	switch {
	case t.err != nil:
		b.SetErr(t.err)
	case t.kind == kindDeclared && t.ft == nil:
		b.SetErr(validationError("full-text target was not built with fts.Over or fts.OverVector"))
	default:
		return true
	}
	return false
}

// exprs are the expressions whose sensitivity masks the bound arguments.
func (t Target) exprs() []momo.Expr {
	if t.kind == kindVector {
		return []momo.Expr{t.vec}
	}
	es := make([]momo.Expr, len(t.cols))
	for i, c := range t.cols {
		es[i] = c
	}
	return es
}

// declaredConfig is the PostgreSQL configuration literal of a declared target.
func (t Target) declaredConfig(q Query) (string, error) {
	if t.ft.Config == "" {
		return "", validationError("a PostgreSQL full-text declaration on table %s needs Config", t.tbl.Name)
	}
	if q.config != "" && q.config != t.ft.Config {
		return "", validationError("search config %q differs from the index config %q", q.config, t.ft.Config)
	}
	return t.ft.Config, nil
}
