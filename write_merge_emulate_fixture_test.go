// SPDX-License-Identifier: Apache-2.0

package momo_test

import "github.com/azrtydxb/momo"

// mrgRow is the target of the MERGE emulation tests: a primary key, a nullable
// key to merge on, a unique value, a counter and a version.
type mrgRow struct {
	ID      int64
	Key     *string
	Val     string
	N       int64
	Version int64
}

var mrgMeta = momo.Meta[mrgRow]{
	Table: "mrg_rows",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "k", Flags: momo.FlagNullable},
		{Name: "val"},
		{Name: "n"},
		{Name: "version", Flags: momo.FlagVersion},
	},
	Ptr: func(m *mrgRow, i int) any {
		return [...]any{&m.ID, &m.Key, &m.Val, &m.N, &m.Version}[i]
	},
	Value: func(m *mrgRow, i int) any {
		return [...]any{m.ID, m.Key, m.Val, m.N, m.Version}[i]
	},
	Identity: -1, Version: 4, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*mrgRow) MomoMeta() *momo.Meta[mrgRow] { return &mrgMeta }

var (
	mrgID  = momo.NewColumn[int64]("mrg_rows", "id", momo.FlagPK)
	mrgKey = momo.NewColumn[string]("mrg_rows", "k", momo.FlagNullable)
	mrgVal = momo.NewColumn[string]("mrg_rows", "val", 0)
	mrgN   = momo.NewColumn[int64]("mrg_rows", "n", 0)
)

func mrgSource() momo.Subquery { return momo.RawSubquery("SELECT id, k, val, n FROM mrg_src") }

func mrgOn() momo.Predicate { return mrgKey.EqExpr(momo.TypedSQL[string]("src.k")) }

func mrgInsertClause(m momo.MergeQuery[mrgRow]) momo.MergeQuery[mrgRow] {
	return m.WhenNotMatchedInsert([]momo.ColumnRef{mrgID, mrgKey, mrgVal},
		momo.TypedSQL[int64]("src.id"), momo.TypedSQL[string]("src.k"), momo.TypedSQL[string]("src.val"))
}

// mrgUpsert updates val from the source and inserts the unmatched rows.
func mrgUpsert(q momo.Querier) momo.MergeQuery[mrgRow] {
	return mrgInsertClause(momo.Merge[mrgRow](q, mrgSource()).On(mrgOn()).
		WhenMatchedUpdate(mrgVal.SetExpr(momo.TypedSQL[string]("src.val"))))
}
