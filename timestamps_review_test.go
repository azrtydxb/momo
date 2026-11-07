// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// unixSeconds is a named int64: not one of the supported timestamp types.
type unixSeconds int64

type namedStampRow struct {
	ID int64
	At unixSeconds
}

var namedStampMeta = &momo.Meta[namedStampRow]{
	Table: "named_stamps",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "created_at", Flags: momo.FlagCreatedAt},
	},
	Ptr:      func(m *namedStampRow, i int) any { return []any{&m.ID, &m.At}[i] },
	Value:    func(m *namedStampRow, i int) any { return []any{m.ID, m.At}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: 1, UpdatedAt: -1,
	PK: []int{0},
}

func (*namedStampRow) MomoMeta() *momo.Meta[namedStampRow] { return namedStampMeta }

type nullStampRow struct {
	ID int64
	V  string
	At sql.NullTime
}

var nullStampV = momo.NewColumn[string]("null_stamps", "v", 0)

var nullStampMeta = &momo.Meta[nullStampRow]{
	Table: "null_stamps",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "v"},
		{Name: "updated_at", Flags: momo.FlagUpdatedAt},
	},
	Ptr:      func(m *nullStampRow, i int) any { return []any{&m.ID, &m.V, &m.At}[i] },
	Value:    func(m *nullStampRow, i int) any { return []any{m.ID, m.V, m.At}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: 2,
	PK: []int{0},
}

func (*nullStampRow) MomoMeta() *momo.Meta[nullStampRow] { return nullStampMeta }

func TestAutoTimestampUnsupportedFieldTypeFails(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	cases := map[string]func() error{
		"named int64 insert Model": func() error {
			_, err := momo.Insert[namedStampRow](db).Model(&namedStampRow{ID: 1}).SQL()
			return err
		},
		"named int64 insert Values": func() error {
			_, err := momo.Insert[namedStampRow](db).Values().SQL()
			return err
		},
		"sql.NullTime update": func() error {
			_, err := momo.Update[nullStampRow](db).Set(nullStampV.Set("x")).Where(nullStampV.Eq("y")).SQL()
			return err
		},
	}
	for name, run := range cases {
		err := run()
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
			continue
		}
		if !strings.Contains(err.Error(), "unsupported field type") {
			t.Errorf("%s: error %q does not name the unsupported field type", name, err)
		}
	}
	if err := cases["named int64 insert Model"](); err == nil || !strings.Contains(err.Error(), "created_at") || !strings.Contains(err.Error(), "unixSeconds") {
		t.Errorf("error must name column and type, got %v", err)
	}
}
