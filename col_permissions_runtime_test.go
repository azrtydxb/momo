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

// permItem has one column of each write access: a read-only column, a
// create-only one, an immutable one and a plain writable one.
type permItem struct {
	ID     int64
	Code   string
	Owner  string
	Serial string
	Name   string
}

var permItemMeta = &momo.Meta[permItem]{
	Table: "items",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity},
		{Name: "code", Flags: momo.FlagReadOnly},
		{Name: "owner", Flags: momo.FlagCreateOnly},
		{Name: "serial", Flags: momo.FlagImmutable},
		{Name: "name"},
	},
	Ptr: func(m *permItem, i int) any {
		return [...]any{&m.ID, &m.Code, &m.Owner, &m.Serial, &m.Name}[i]
	},
	Value: func(m *permItem, i int) any {
		return [...]any{m.ID, m.Code, m.Owner, m.Serial, m.Name}[i]
	},
	Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*permItem) MomoMeta() *momo.Meta[permItem] { return permItemMeta }

// TestFieldPermissionsRuntime checks the S-71 rule on the paths the compiler
// cannot check: the whole-model builders skip the columns a statement may not
// write, and an assignment built from a restricted column descriptor is
// refused when the statement is built.
func TestFieldPermissionsRuntime(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	m := permItem{ID: 1, Code: "c", Owner: "o", Serial: "s", Name: "n"}

	upd, err := momo.Update[permItem](pg).Model(&m).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`"code"`, `"owner"`, `"serial"`} {
		if strings.Contains(upd.SQL, banned) {
			t.Errorf("Update.Model writes %s: %s", banned, upd.SQL)
		}
	}
	if !strings.Contains(upd.SQL, `"name"`) {
		t.Errorf("Update.Model omits the writable column: %s", upd.SQL)
	}

	ins, err := momo.Insert[permItem](pg).Model(&m).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ins.SQL, `"code"`) {
		t.Errorf("Insert.Model writes a read-only column: %s", ins.SQL)
	}
	for _, want := range []string{`"owner"`, `"serial"`, `"name"`} {
		if !strings.Contains(ins.SQL, want) {
			t.Errorf("Insert.Model omits %s: %s", want, ins.SQL)
		}
	}

	// A restricted column cannot be assigned in an UPDATE, even through a
	// descriptor that does not carry the restriction (the metadata decides).
	for _, name := range []string{"code", "owner", "serial"} {
		col := momo.NewColumn[string]("items", name, 0)
		if _, err := momo.Update[permItem](pg).Set(col.Set("v")).AllRows().SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s column in Update.Set: err = %v, want ErrValidation", name, err)
		}
	}
}
