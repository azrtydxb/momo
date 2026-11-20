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

func mrgSource() momo.Subquery { return momo.SubquerySQL("SELECT id, k, val, n FROM mrg_src") }

func mrgOn() momo.Predicate { return mrgKey.EqExpr(momo.TypedSQL[string]("src.k")) }

func mrgInsertClause(m momo.MergeQuery[mrgRow]) momo.MergeQuery[mrgRow] {
	return m.WhenNotMatchedInsert([]momo.ColumnRef{mrgID, mrgKey, mrgVal},
		momo.TypedSQL[int64]("src.id"), momo.TypedSQL[string]("src.k"), momo.TypedSQL[string]("src.val"))
}

// mrgPair has a composite primary key.
type mrgPair struct {
	A, B int64
	V    string
}

var mrgPairMeta = momo.Meta[mrgPair]{
	Table: "mrg_pairs",
	Columns: []momo.ColumnMeta{
		{Name: "a", Flags: momo.FlagPK},
		{Name: "b", Flags: momo.FlagPK},
		{Name: "v"},
	},
	Ptr:      func(m *mrgPair, i int) any { return [...]any{&m.A, &m.B, &m.V}[i] },
	Value:    func(m *mrgPair, i int) any { return [...]any{m.A, m.B, m.V}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0, 1},
}

func (*mrgPair) MomoMeta() *momo.Meta[mrgPair] { return &mrgPairMeta }

var (
	mrgPairA = momo.NewColumn[int64]("mrg_pairs", "a", momo.FlagPK)
	mrgPairB = momo.NewColumn[int64]("mrg_pairs", "b", momo.FlagPK)
	mrgPairV = momo.NewColumn[string]("mrg_pairs", "v", 0)
)

// mrgPairMerge updates v and inserts the unmatched rows, on both key columns.
func mrgPairMerge(q momo.Querier) momo.MergeQuery[mrgPair] {
	src := momo.SubquerySQL("SELECT a, b, v FROM mrg_pairs_src")
	return momo.Merge[mrgPair](q, src).
		On(mrgPairA.EqExpr(momo.TypedSQL[int64]("src.a")), mrgPairB.EqExpr(momo.TypedSQL[int64]("src.b"))).
		WhenMatchedUpdate(mrgPairV.SetExpr(momo.TypedSQL[string]("src.v"))).
		WhenNotMatchedInsert([]momo.ColumnRef{mrgPairA, mrgPairB, mrgPairV},
			momo.TypedSQL[int64]("src.a"), momo.TypedSQL[int64]("src.b"), momo.TypedSQL[string]("src.v"))
}

// mrgUpsert updates val from the source and inserts the unmatched rows.
func mrgUpsert(q momo.Querier) momo.MergeQuery[mrgRow] {
	return mrgInsertClause(momo.Merge[mrgRow](q, mrgSource()).On(mrgOn()).
		WhenMatchedUpdate(mrgVal.SetExpr(momo.TypedSQL[string]("src.val"))))
}
