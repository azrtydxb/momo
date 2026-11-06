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

func TestUpdateEmptyJunctionIsNotAFilter(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	for name, p := range map[string]momo.Predicate{
		"And":        momo.And(),
		"Or":         momo.Or(),
		"And(And())": momo.And(momo.And()),
		"Or(And())":  momo.Or(momo.And(), momo.Or()),
	} {
		if _, err := momo.Update[unitDocument](pg).Set(DocTitle.Set("x")).Where(p).SQL(); !errors.Is(err, momo.ErrUnfilteredMutation) {
			t.Errorf("Where(%s) err = %v, want ErrUnfilteredMutation", name, err)
		}
	}
	if _, err := momo.Update[unitDocument](pg).Set(DocTitle.Set("x")).Where(momo.And(DocID.Eq(1))).SQL(); err != nil {
		t.Errorf("Where(And(real)) err = %v", err)
	}
	if _, err := momo.Update[unitDocument](pg).Set(DocTitle.Set("x")).Where(momo.And(), DocID.Eq(1)).SQL(); err != nil {
		t.Errorf("Where(And(), real) err = %v", err)
	}
	if _, err := momo.Update[unitDocument](pg).Set(DocTitle.Set("x")).Where(momo.And()).AllRows().SQL(); err != nil {
		t.Errorf("AllRows with empty And err = %v", err)
	}
}

type updRoles struct {
	momo.Model
	ID      int64
	Ver     uint8
	Deleted *int64
	Created int64
	Updated int64
	Val     string
}

var updRolesMeta = momo.Meta[updRoles]{
	Table: "roles",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "ver", Flags: momo.FlagVersion},
		{Name: "deleted_at", Flags: momo.FlagSoftDelete | momo.FlagNullable},
		{Name: "created_at", Flags: momo.FlagCreatedAt},
		{Name: "updated_at", Flags: momo.FlagUpdatedAt},
		{Name: "val"},
	},
	Ptr: func(m *updRoles, i int) any {
		return []any{&m.ID, &m.Ver, &m.Deleted, &m.Created, &m.Updated, &m.Val}[i]
	},
	Value: func(m *updRoles, i int) any {
		return []any{m.ID, m.Ver, m.Deleted, m.Created, m.Updated, m.Val}[i]
	},
	Identity: -1, Version: 1, SoftDelete: 2, CreatedAt: 3, UpdatedAt: 4,
	PK: []int{0},
}

func (*updRoles) MomoMeta() *momo.Meta[updRoles] { return &updRolesMeta }

func TestUpdateSetRefusesRoleColumns(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	for _, name := range []string{"id", "ver", "deleted_at", "created_at", "updated_at"} {
		col := momo.NewColumn[int64]("roles", name, 0)
		if _, err := momo.Update[updRoles](pg).Set(col.Set(1)).AllRows().SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("Set(%s) err = %v, want ErrValidation", name, err)
		}
	}
	val := momo.NewColumn[string]("roles", "val", 0)
	if _, err := momo.Update[updRoles](pg).Set(val.Set("ok")).AllRows().SQL(); err != nil {
		t.Errorf("Set(val) err = %v", err)
	}
}

type updBadVer struct {
	momo.Model
	ID  int64
	Ver string
}

var updBadVerMeta = momo.Meta[updBadVer]{
	Table:    "badver",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "ver", Flags: momo.FlagVersion}},
	Ptr:      func(m *updBadVer, i int) any { return []any{&m.ID, &m.Ver}[i] },
	Value:    func(m *updBadVer, i int) any { return []any{m.ID, m.Ver}[i] },
	Identity: -1, Version: 1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*updBadVer) MomoMeta() *momo.Meta[updBadVer] { return &updBadVerMeta }

func TestUpdateVersionKinds(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	bad := updBadVer{ID: 1, Ver: "1"}
	if _, err := momo.Update[updBadVer](pg).Model(&bad).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("string version err = %v, want ErrValidation", err)
	}
	if _, err := momo.Update[updRoles](pg).Model(nil).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Model(nil) err = %v, want ErrValidation", err)
	}
	ok := updRoles{ID: 1, Ver: 3}
	c, err := momo.Update[updRoles](pg).Model(&ok).SQL()
	if err != nil || !strings.Contains(c.SQL, `"ver" = "roles"."ver" + 1`) {
		t.Fatalf("uint8 version = %q %v", c.SQL, err)
	}
}
