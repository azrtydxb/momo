// SPDX-License-Identifier: Apache-2.0

package example_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testmodels/example"
	"github.com/azrtydxb/momo/internal/testmodels/example/document"
	"github.com/azrtydxb/momo/vecop"
	"github.com/azrtydxb/momo/vector"
)

func vecDB() *momo.DB { return momo.New(pgxdriver.New(nil)) }

// TestGeneratedVectorColumnMethodParity calls every method a plain generated
// Column offers (bar the range comparisons) on a generated vector column, so
// switching a field to VectorColumn cannot break user code. It is a compile
// test first; the value checks guard the delegation.
func TestGeneratedVectorColumnMethodParity(t *testing.T) {
	c := document.Embedding
	v := vector.Vector(make([]float32, 1536))
	ex := c.AsExpr()
	_ = []momo.Predicate{
		c.Eq(v), c.Ne(v), c.EqExpr(ex), c.NeExpr(ex),
		c.IsNull(), c.IsNotNull(), c.IsDistinctFrom(v), c.IsNotDistinctFrom(v),
		c.In(v), c.NotIn(v),
	}
	_ = []momo.Order{c.Asc(), c.Desc(), c.Desc().NullsLast()}
	_ = []momo.Assignment{c.Set(v), c.SetExpr(ex), c.SetDefault(), c.SetExcluded()}
	if c.Table() != "documents" || c.ColumnName() != "embedding" || c.Flags() != 0 || c.Dim() != 1536 {
		t.Fatalf("accessors: %q %q %d %d", c.Table(), c.ColumnName(), c.Flags(), c.Dim())
	}
	if a := c.As("d"); a.Table() != "d" || a.Dim() != 1536 {
		t.Fatalf("As: %q dim %d", a.Table(), a.Dim())
	}
	var _ momo.ColumnRef = c
	var _ momo.Orderable = c
	var _ momo.Expression[vector.Vector] = c
}

func TestVectorColumnIsNotNullFilterSQL(t *testing.T) {
	comp, err := momo.Select[example.Document, *example.Document](vecDB()).
		Where(document.Embedding.IsNotNull()).
		OrderBy(document.Embedding.Desc()).
		SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(comp.SQL, `"documents"."embedding" IS NOT NULL`) || !strings.Contains(comp.SQL, `ORDER BY "documents"."embedding" DESC`) {
		t.Fatalf("SQL = %s", comp.SQL)
	}
}

func TestVectorColumnSetNullWritesNull(t *testing.T) {
	col := momo.NewNullVectorColumn[vector.Vector]("documents", "embedding", 1536, 0)
	if col.Flags()&momo.FlagNullable == 0 {
		t.Fatal("NullVectorColumn lacks FlagNullable")
	}
	comp, err := momo.Update[example.Document](vecDB()).
		Set(col.SetNull()).
		Where(document.ID.Eq(1)).
		SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(comp.SQL, `"embedding" = NULL`) {
		t.Fatalf("SQL = %s", comp.SQL)
	}
}

func TestVectorColumnSetExcludedInUpsert(t *testing.T) {
	doc := example.Document{TenantID: 1, Title: "t", Embedding: make(vector.Vector, 1536)}
	comp, err := momo.Upsert[example.Document](vecDB()).Model(&doc).
		OnConflict(document.Title).
		DoUpdate(document.Embedding.SetExcluded()).
		SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(comp.SQL, `"embedding" = EXCLUDED."embedding"`) {
		t.Fatalf("SQL = %s", comp.SQL)
	}
}

// TestVectorColumnNullableKeyset proves the keyset nullable check, which
// reads Flags through the ordering expression, still sees a nullable vector.
func TestVectorColumnNullableKeyset(t *testing.T) {
	plain := momo.NewVectorColumn[vector.Vector]("documents", "embedding", 1536, momo.FlagNullable)
	if plain.Flags()&momo.FlagNullable == 0 {
		t.Fatal("Flags() lost FlagNullable")
	}
	_, err := momo.Select[example.Document, *example.Document](vecDB()).
		OrderBy(plain.Asc()).
		After(vector.Vector(make([]float32, 1536)), int64(1)).
		SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("keyset over a nullable vector key: err = %v, want ErrUnsupported", err)
	}
}

// TestVectorPermissionColumnsSearchable proves the read-only, create-only and
// immutable vector variants are accepted by the distance builders, and keep
// their write restrictions.
func TestVectorPermissionColumnsSearchable(t *testing.T) {
	v := vector.Vector(make([]float32, 3))
	ro := momo.NewReadOnlyVectorColumn[vector.Vector]("t", "ro", 3, momo.FlagReadOnly)
	co := momo.NewCreateOnlyVectorColumn[vector.Vector]("t", "co", 3, momo.FlagCreateOnly)
	im := momo.NewImmutableVectorColumn[vector.Vector]("t", "im", 3, momo.FlagImmutable)
	for name, d := range map[string]vecop.Expr{
		"read_only":   vecop.Cosine(ro, v),
		"create_only": vecop.L2(co.As("a"), v),
		"immutable":   vecop.InnerProduct(im, v),
	} {
		b := momo.NewBuilder(vecDB().Dialect())
		d.AppendSQL(b)
		if b.Err() != nil || !strings.Contains(b.String(), `"`) {
			t.Errorf("%s: %q err %v", name, b.String(), b.Err())
		}
	}
	// Create-only and immutable writes are insert-only values; read-only has
	// no Set at all (a compile-time guarantee, so nothing to call here).
	var _ momo.InsertValue = co.Set(v)
	var _ momo.InsertValue = im.Set(v)
	var _ momo.ColumnRef = ro
}
