// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

type updGen struct {
	momo.Model
	ID  int64
	Gen string
	Val string
}

var updGenMeta = momo.Meta[updGen]{
	Table: "gens",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity},
		{Name: "gen", Flags: momo.FlagReadOnly},
		{Name: "val"},
	},
	Ptr: func(m *updGen, i int) any { return []any{&m.ID, &m.Gen, &m.Val}[i] },
	Value: func(m *updGen, i int) any {
		return []any{m.ID, m.Gen, m.Val}[i]
	},
	Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*updGen) MomoMeta() *momo.Meta[updGen] { return &updGenMeta }

func TestUpdateBuilderRendering(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	sq := momo.New(fakedriver.New(dialect.SQLite))

	if _, err := momo.Update[unitDocument](pg).Set(DocTitle.Set("x")).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
		t.Fatalf("no WHERE err = %v", err)
	}
	c, err := momo.Update[unitDocument](pg).Set(DocTitle.Set("x")).AllRows().SQL()
	if err != nil || c.SQL != `UPDATE "documents" SET "title" = $1` || len(c.Args) != 1 {
		t.Fatalf("AllRows = %+v %v", c, err)
	}
	c, err = momo.Update[unitDocument](pg).Set(DocTitle.Set("x"), DocPublished.Set(false)).Where(DocID.Eq(7)).SQL()
	want := `UPDATE "documents" SET "title" = $1, "published" = $2 WHERE "documents"."id" = $3`
	if err != nil || c.SQL != want {
		t.Fatalf("Where = %q %v, want %q", c.SQL, err, want)
	}
	if _, err := momo.Update[unitDocument](pg).Where(DocID.Eq(1)).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("empty SET err = %v", err)
	}
	if _, err := momo.Update[unitDocument](sq).Set(DocTitle.SetDefault()).AllRows().SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("SetDefault on SQLite err = %v", err)
	}
	if c, err := momo.Update[unitDocument](pg).Set(DocTitle.SetDefault()).AllRows().SQL(); err != nil || !strings.HasSuffix(c.SQL, `SET "title" = DEFAULT`) {
		t.Fatalf("SetDefault on PG = %q %v", c.SQL, err)
	}
	if _, err := momo.Update[unitDocument](pg).Set(DocTitle.SetExcluded()).AllRows().SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("SetExcluded in UPDATE err = %v", err)
	}
	// The value is bound, never concatenated.
	c, err = momo.Update[unitDocument](pg).Set(DocTitle.Set(`x"; DROP TABLE documents; --`)).AllRows().SQL()
	if err != nil || strings.Contains(c.SQL, "DROP") || c.Args[0] != `x"; DROP TABLE documents; --` {
		t.Fatalf("value not bound: %+v %v", c, err)
	}
}

func TestUpdateReadOnlyColumnRefused(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	genCol := momo.NewColumn[string]("gens", "gen", momo.FlagReadOnly)
	idCol := momo.NewColumn[int64]("gens", "id", momo.FlagPK|momo.FlagIdentity)
	if _, err := momo.Update[updGen](pg).Set(genCol.Set("z")).AllRows().SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("read-only SET err = %v", err)
	}
	if _, err := momo.Update[updGen](pg).Set(idCol.Set(5)).AllRows().SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("identity SET err = %v", err)
	}
	// Model writes skip the identity and read-only columns and filter by PK.
	m := updGen{ID: 3, Gen: "g", Val: "v"}
	c, err := momo.Update[updGen](pg).Model(&m).SQL()
	want := `UPDATE "gens" SET "val" = $1 WHERE "gens"."id" = $2`
	if err != nil || c.SQL != want {
		t.Fatalf("Model = %q %v, want %q", c.SQL, err, want)
	}
}

// A correlated subquery in WHERE may reference the update target; a column of
// a table that is not in the statement is refused.
func TestUpdateRendersInTargetScope(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	inner := momo.Select[unitDocument](pg).From(momo.TableRef{Name: "documents", Alias: "d2"}).
		Where(DocTenantID.As("d2").EqExpr(DocTenantID), DocTitle.As("d2").Eq("peer"))
	c, err := momo.Update[unitDocument](pg).Set(DocTitle.Set("x")).Where(momo.Exists(inner)).SQL()
	if err != nil {
		t.Fatalf("correlated WHERE: %v", err)
	}
	if !strings.Contains(c.SQL, `"d2"."tenant_id" = "documents"."tenant_id"`) {
		t.Fatalf("correlation not rendered: %s", c.SQL)
	}
	other := momo.NewColumn[string]("elsewhere", "title", 0)
	if _, err := momo.Update[unitDocument](pg).Set(DocTitle.Set("x")).Where(other.Eq("a")).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("foreign table column err = %v", err)
	}
}
