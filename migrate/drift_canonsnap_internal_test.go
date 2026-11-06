// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// failingCanonicalizer is a whole-schema Canonicalizer that always fails, like
// a role without CREATE.
type failingCanonicalizer struct{}

func (failingCanonicalizer) Dialect() *dialect.Dialect { return nil }
func (failingCanonicalizer) Inspect(context.Context, momo.PinnedConn, InspectOptions) (*schema.Schema, error) {
	return nil, errors.New("unused")
}
func (failingCanonicalizer) Render([]Change) ([]Statement, error) { return nil, errors.New("unused") }
func (failingCanonicalizer) HistoryDDL() []string                 { return nil }
func (failingCanonicalizer) Setup(context.Context, momo.PinnedConn, SessionOptions) (func() error, error) {
	return nil, errors.New("unused")
}
func (failingCanonicalizer) Lock(context.Context, momo.PinnedConn, string) (func() error, func(context.Context) (string, error), error) {
	return nil, nil, errors.New("unused")
}

func (failingCanonicalizer) Canonicalize(context.Context, momo.PinnedConn, *schema.Schema) error {
	return errors.New("no privilege")
}

// When a dialect only has the whole-schema Canonicalizer and it fails, the
// skipped list must name only the tables that carry text (the ones whose
// comparison is affected), sorted.
func TestCanonicalSnapshotFallbackListsOnlyTablesWithText(t *testing.T) {
	e := &Engine{opts: options{dialect: failingCanonicalizer{}}}
	s := &schema.Schema{Tables: []schema.Table{
		{Name: "z_check", Checks: []schema.Check{{Name: "c", Expr: "true"}}},
		{Name: "plain", Columns: []schema.Column{{Name: "id", Type: "int"}}},
		{Name: "a_default", Columns: []schema.Column{{Name: "id", Type: "int", Default: "0"}}},
		{Name: "m_idx", Indexes: []schema.Index{{Name: "i", Where: "id > 0"}}},
		{Name: "n_gen", Columns: []schema.Column{{Name: "g", Type: "int", Generated: "id + 1"}}},
		{Name: "plain_idx", Indexes: []schema.Index{{Name: "j", Parts: []schema.IndexPart{{Column: "id"}}}}},
	}}
	got, skipped, err := e.canonicalSnapshot(context.Background(), nil, s)
	if err != nil || got != s {
		t.Fatalf("a failing canonicalizer falls back to the declared snapshot: %v %v", got == s, err)
	}
	var names []string
	for _, r := range skipped {
		names = append(names, r.Name)
	}
	if want := []string{"a_default", "m_idx", "n_gen", "z_check"}; !slices.Equal(names, want) {
		t.Fatalf("skipped %v, want %v", names, want)
	}
}
