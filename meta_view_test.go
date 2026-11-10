// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// viewItem is a model backed by a database view (Meta.View is set).
type viewItem struct {
	ID   int64
	Name string
}

var viewItemMeta = &momo.Meta[viewItem]{
	Table: "item_view",
	View:  true,
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "name"},
	},
	Ptr: func(m *viewItem, i int) any {
		return [...]any{&m.ID, &m.Name}[i]
	},
	Value: func(m *viewItem, i int) any {
		return [...]any{m.ID, m.Name}[i]
	},
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*viewItem) MomoMeta() *momo.Meta[viewItem] { return viewItemMeta }

// TestViewModelsNotWritable checks the runtime guard for models that map a
// view: every write builder refuses the model with ErrValidation naming it,
// and a read is unaffected.
func TestViewModelsNotWritable(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	m := viewItem{ID: 1, Name: "n"}
	id := momo.NewColumn[int64]("item_view", "id", momo.FlagPK)
	name := momo.NewColumn[string]("item_view", "name", 0)
	src := momo.RawSubquery("SELECT 1 AS id, 'x' AS name")

	refused := func(op string, err error) {
		t.Helper()
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s on a view model: err = %v, want ErrValidation", op, err)
			return
		}
		if !strings.Contains(err.Error(), "viewItem") || !strings.Contains(err.Error(), "view model") {
			t.Errorf("%s: error does not name the model: %v", op, err)
		}
	}

	_, err := momo.Insert[viewItem](pg).Model(&m).SQL()
	refused("Insert.Model", err)
	_, err = momo.Insert[viewItem](pg).Models([]viewItem{m, m}).SQL()
	refused("Insert.Models", err)
	_, err = momo.Insert[viewItem](pg).Values(name.Set("x")).SQL()
	refused("Insert.Values", err)
	_, err = momo.Upsert[viewItem](pg).Model(&m).SQL()
	refused("Upsert", err)
	_, err = momo.Update[viewItem](pg).Set(name.Set("x")).Where(id.Eq(1)).SQL()
	refused("Update.Set", err)
	_, err = momo.Update[viewItem](pg).Model(&m).SQL()
	refused("Update.Model", err)
	_, err = momo.SoftDelete[viewItem](pg).Where(id.Eq(1)).SQL()
	refused("SoftDelete", err)
	_, err = momo.BulkUpdate[viewItem](pg, id, []momo.ColumnRef{name}, [][]any{{int64(1), "x"}}).SQL()
	refused("BulkUpdate", err)
	_, err = momo.Delete[viewItem](pg).Where(id.Eq(1)).SQL()
	refused("Delete", err)
	_, err = momo.Merge[viewItem](pg, src).
		On(id.EqExpr(momo.TypedSQL[int64]("src.id"))).
		WhenMatchedUpdate(name.Set("y")).SQL()
	refused("Merge", err)

	if _, err := momo.Select[viewItem](pg).SQL(); err != nil {
		t.Errorf("a view model must still be readable: %v", err)
	}
}

// viewKid is a view-backed child, so a graph can reach a view as a child.
type viewKid struct {
	ID       int64
	ParentID int64
}

var viewKidMeta = &momo.Meta[viewKid]{
	Table: "kid_view",
	View:  true,
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "parent_id"},
	},
	Ptr:      func(m *viewKid, i int) any { return [...]any{&m.ID, &m.ParentID}[i] },
	Value:    func(m *viewKid, i int) any { return [...]any{m.ID, m.ParentID}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*viewKid) MomoMeta() *momo.Meta[viewKid] { return viewKidMeta }

// tablePar is a table model that owns viewKid children.
type tablePar struct {
	ID   int64
	Kids []viewKid
}

var tableParMeta = &momo.Meta[tablePar]{
	Table:    "pars",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}},
	Ptr:      func(m *tablePar, _ int) any { return &m.ID },
	Value:    func(m *tablePar, _ int) any { return m.ID },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*tablePar) MomoMeta() *momo.Meta[tablePar] { return tableParMeta }

var parKids = momo.Relation[tablePar, viewKid]{
	Name: "Kids", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "pars"}, Child: momo.TableRef{Name: "kid_view"},
	ParentMeta: tableParMeta, ChildMeta: viewKidMeta,
	ParentCols: []string{"id"}, ChildCols: []string{"parent_id"}, ChildPK: []string{"id"},
	ParentKey:   func(p *tablePar) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:    func(c *viewKid) ([]any, bool) { return []any{c.ParentID}, true },
	SetChildKey: func(c *viewKid, k []any) { c.ParentID = k[0].(int64) },
	GetMany:     func(p *tablePar) []viewKid { return p.Kids },
	SetMany:     func(p *tablePar, cs []viewKid) { p.Kids = cs },
}

// TestViewModelsNotWritableThroughOtherInsertPaths covers the insert paths
// that do not start at Insert: CreateGraph (view as parent and as child),
// OnConflict, Copy, Exec of Models and Upsert, and SoftDelete.
func TestViewModelsNotWritableThroughOtherInsertPaths(t *testing.T) {
	ctx := context.Background()
	m := viewItem{ID: 1, Name: "n"}
	id := momo.NewColumn[int64]("item_view", "id", momo.FlagPK)

	db := momo.New(fakedriver.New(dialect.Postgres))
	if err := momo.CreateGraph(ctx, db, &m); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("CreateGraph with a view parent: err = %v, want ErrValidation", err)
	}

	d := fakedriver.New(dialect.Postgres)
	d.SetRows([][]any{{int64(5)}})
	par := &tablePar{ID: 5, Kids: []viewKid{{}, {}}}
	err := momo.CreateGraph(ctx, momo.New(d), par, parKids.Include())
	if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "view model") {
		t.Errorf("CreateGraph with view children: err = %v, want ErrValidation naming the view model", err)
	}

	pg := momo.New(fakedriver.New(dialect.Postgres))
	if _, err := momo.Insert[viewItem](pg).Model(&m).OnConflict(id).DoNothing().SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("OnConflict: err = %v, want ErrValidation", err)
	}
	if _, err := momo.Insert[viewItem](pg).Copy(ctx, []viewItem{m}); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Copy: err = %v, want ErrValidation", err)
	}
	if _, err := momo.Insert[viewItem](pg).Models([]viewItem{m, m}).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Models.Exec: err = %v, want ErrValidation", err)
	}
	if _, err := momo.Upsert[viewItem](pg).Model(&m).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("Upsert.Exec: err = %v, want ErrValidation", err)
	}
	if _, err := momo.SoftDelete[viewItem](pg).Where(id.Eq(1)).Exec(ctx); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("SoftDelete.Exec: err = %v, want ErrValidation", err)
	}
}
