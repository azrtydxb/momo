// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"github.com/azrtydxb/momo"
)

// The identity models below are hand-written so CreateGraph can be tested with
// database-generated keys: the child foreign key can only be right if the
// parent's identity was read back with RETURNING before the children insert.

type gParent struct {
	momo.Model `momo:"table:g_parents"`
	ID         int64
	Name       string
	Kids       []gKid
}

type gKid struct {
	momo.Model `momo:"table:g_kids"`
	ID         int64
	ParentID   int64
	Name       string
	Parent     *gParent
}

var gParentMeta = &momo.Meta[gParent]{
	Table:   "g_parents",
	Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "name"}},
	Ptr: func(m *gParent, i int) any {
		return []any{&m.ID, &m.Name}[i]
	},
	Value: func(m *gParent, i int) any {
		return []any{m.ID, m.Name}[i]
	},
	Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1, PK: []int{0},
}

var gKidMeta = &momo.Meta[gKid]{
	Table:   "g_kids",
	Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "parent_id"}, {Name: "name"}},
	Ptr: func(m *gKid, i int) any {
		return []any{&m.ID, &m.ParentID, &m.Name}[i]
	},
	Value: func(m *gKid, i int) any {
		return []any{m.ID, m.ParentID, m.Name}[i]
	},
	Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1, PK: []int{0},
}

func (*gParent) MomoMeta() *momo.Meta[gParent] { return gParentMeta }
func (*gKid) MomoMeta() *momo.Meta[gKid]       { return gKidMeta }
