// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestIDGenOmitWritesBack(t *testing.T) {
	db := idDB()
	m := idRow{ID: "ignored", Name: "a"}
	idCol := momo.NewColumn[string]("id_rows", "id", momo.FlagPK)
	if _, err := momo.Insert[idRow](db).Omit(idCol).Model(&m).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := db.Driver().(*fakedriver.Driver).Calls()
	if m.ID == "ignored" || !reCUID.MatchString(m.ID) || calls[0].Args[0] != m.ID {
		t.Fatalf("omitted id: model %q, bound %v", m.ID, calls[0].Args)
	}
}

func TestIDGenRowsWritesBack(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"stored", "a"}})
	db := momo.New(drv)
	m := idKindRow{Name: "a"}
	rows, err := momo.Insert[idKindRow](db).Model(&m).ReturningAll().Rows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if !reUUID.MatchString(string(m.ID)) {
		t.Fatalf("named id not written back through Rows: %q", m.ID)
	}
}

func TestIDGenPointerIDsAreDistinct(t *testing.T) {
	rows := make([]idPtrRow, 3)
	if _, err := momo.Insert[idPtrRow](idDB()).Models(rows).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rows[0].ID == rows[1].ID || rows[1].ID == rows[2].ID {
		t.Fatal("elements share one id pointer")
	}
}

func TestIDGenNilAndPanickingGeneratorsFail(t *testing.T) {
	for name, fn := range map[string]func() string{
		"nil":   nil,
		"panic": func() string { panic("boom") },
	} {
		db := idDB(momo.WithIDGenerator("cuid", fn))
		m := idRow{Name: "a"}
		_, err := momo.Insert[idRow](db).Model(&m).Exec(context.Background())
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("%s: err = %v, want ErrValidation", name, err)
		}
		nameCol := momo.NewColumn[string]("id_rows", "name", 0)
		_, err = momo.Insert[idRow](db).Values(nameCol.Set("a")).SQL()
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("%s via Values: err = %v, want ErrValidation", name, err)
		}
	}
}

func TestIDGenValuesFillsID(t *testing.T) {
	nameCol := momo.NewColumn[string]("id_rows", "name", 0)
	c, err := momo.Insert[idRow](idDB()).Values(nameCol.Set("a")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	// The assigned column comes first, the generated default after it.
	if len(c.Args) != 2 || c.Args[0] != "a" || !reCUID.MatchString(c.Args[1].(string)) {
		t.Fatalf("args = %v", c.Args)
	}
}
