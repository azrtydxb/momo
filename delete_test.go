// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// delNote is a hand-written model without a soft-delete column.
type delNote struct {
	ID    int64
	Title string
}

var (
	delNoteID    = momo.NewColumn[int64]("del_notes", "id", momo.FlagPK)
	delNoteTitle = momo.NewColumn[string]("del_notes", "title", 0)
)

var delNoteMeta = momo.Meta[delNote]{
	Table:      "del_notes",
	Columns:    []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "title"}},
	Ptr:        func(m *delNote, i int) any { return [...]any{&m.ID, &m.Title}[i] },
	Value:      func(m *delNote, i int) any { return [...]any{m.ID, m.Title}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0},
}

func (*delNote) MomoMeta() *momo.Meta[delNote] { return &delNoteMeta }

// delPair has a composite primary key.
type delPair struct {
	A, B int64
}

var delPairMeta = momo.Meta[delPair]{
	Table:      "del_pairs",
	Columns:    []momo.ColumnMeta{{Name: "a", Flags: momo.FlagPK}, {Name: "b", Flags: momo.FlagPK}},
	Ptr:        func(m *delPair, i int) any { return [...]any{&m.A, &m.B}[i] },
	Value:      func(m *delPair, i int) any { return [...]any{m.A, m.B}[i] },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
	PK:         []int{0, 1},
}

func (*delPair) MomoMeta() *momo.Meta[delPair] { return &delPairMeta }

// delKey projects the first column of del_pairs for subqueries.
type delKey struct{ A int64 }

var delKeyMeta = momo.Meta[delKey]{
	Table:      "del_pairs",
	Columns:    []momo.ColumnMeta{{Name: "a"}},
	Ptr:        func(m *delKey, _ int) any { return &m.A },
	Value:      func(m *delKey, _ int) any { return m.A },
	Identity:   -1,
	Version:    -1,
	SoftDelete: -1,
	CreatedAt:  -1,
	UpdatedAt:  -1,
}

func (*delKey) MomoMeta() *momo.Meta[delKey] { return &delKeyMeta }

func TestDeleteRendering(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	inner := momo.NewColumn[int64]("p", "a", 0)
	correlated := momo.Exists(momo.Select[delKey](pg).
		From(momo.TableRef{Name: "del_pairs", Alias: "p"}).
		Columns(inner).
		Where(inner.EqExpr(delNoteID)))

	cases := []struct {
		name string
		q    momo.DeleteQuery[delNote]
		want string
	}{
		{"where", momo.Delete[delNote](pg).Where(delNoteTitle.Eq("x")),
			`DELETE FROM "del_notes" WHERE "del_notes"."title" = $1`},
		{"or with a real branch", momo.Delete[delNote](pg).Where(momo.Or(momo.Or(), delNoteID.Eq(1))),
			`DELETE FROM "del_notes" WHERE (1 = 0 OR "del_notes"."id" = $1)`},
		{"correlated exists", momo.Delete[delNote](pg).Where(correlated),
			`DELETE FROM "del_notes" WHERE EXISTS (SELECT "p"."a" FROM "del_pairs" AS "p" WHERE "p"."a" = "del_notes"."id")`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := tc.q.SQL()
			if err != nil {
				t.Fatal(err)
			}
			if c.SQL != tc.want {
				t.Fatalf("SQL = %s\nwant  %s", c.SQL, tc.want)
			}
		})
	}
}

func TestDeleteGuardEmptyJunctions(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	for name, p := range map[string]momo.Predicate{
		"And":    momo.And(),
		"Or":     momo.Or(),
		"nested": momo.And(momo.Or(), momo.And(momo.Or(), momo.And())),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := momo.Delete[delNote](pg).Where(p).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
				t.Fatalf("err = %v, want ErrUnfilteredMutation", err)
			}
			if _, err := momo.Delete[delNote](pg).Where(p).AllRows().SQL(); err != nil {
				t.Fatalf("AllRows: %v", err)
			}
		})
	}
}

func TestDeleteCompositePrimaryKey(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Delete[delPair](pg).Model(&delPair{A: 1, B: 2}).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `DELETE FROM "del_pairs" WHERE ("del_pairs"."a" = $1) AND ("del_pairs"."b" = $2)`
	if c.SQL != want || len(c.Args) != 2 || c.Args[0] != int64(1) || c.Args[1] != int64(2) {
		t.Fatalf("got %s %v\nwant %s", c.SQL, c.Args, want)
	}
}

func TestDeleteRefusesSoftDeleteModel(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	_, err := momo.Delete[suite.Document](pg).Where(suite.DocID.Eq(1)).SQL()
	if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "SoftDelete or ForceDelete") {
		t.Fatalf("err = %v, want ErrValidation naming SoftDelete or ForceDelete", err)
	}
	if _, err := momo.Delete[suite.Document](pg).AllRows().SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("AllRows err = %v, want ErrValidation", err)
	}
}

func TestDeleteReturningRenders(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Delete[delNote](pg).Where(delNoteID.Eq(1)).Returning(delNoteTitle, delNoteID).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `DELETE FROM "del_notes" WHERE "del_notes"."id" = $1 RETURNING "title", "id"`; c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
	c, err = momo.Delete[delNote](pg).Where(delNoteID.Eq(1)).ReturningAll().SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `DELETE FROM "del_notes" WHERE "del_notes"."id" = $1 RETURNING "id", "title"`; c.SQL != want {
		t.Fatalf("SQL = %s\nwant  %s", c.SQL, want)
	}
}

func TestDeleteReturningUnsupportedOnMySQL(t *testing.T) {
	my := momo.New(fakedriver.New(dialect.MySQL))
	_, err := momo.Delete[delNote](my).Where(delNoteID.Eq(1)).Returning(delNoteID).SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}
