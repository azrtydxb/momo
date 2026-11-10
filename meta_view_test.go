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
