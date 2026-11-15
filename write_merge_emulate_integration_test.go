// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// The cases below hold the final table and the affected-row count that
// PostgreSQL's native MERGE produces. The same cases run on every dialect, so
// SQLite, MySQL and MariaDB are held to the PostgreSQL result.

// mrgSetup recreates the target mrg_rows and the staging table mrg_src, and
// seeds both. A target row is {id, k, val}; a source row is {id, k, val, n}.
// val is unique, so a bad insert fails the statement.
func mrgSetup(t *testing.T, db *momo.DB, d dialect.Name, target, src [][]any) {
	t.Helper()
	ctx := context.Background()
	text := "text"
	if d == dialect.MySQL {
		text = "varchar(64)"
	}
	drop := []string{"DROP TABLE IF EXISTS mrg_rows", "DROP TABLE IF EXISTS mrg_src"}
	for _, stmt := range append(slices.Clone(drop),
		"CREATE TABLE mrg_rows (id bigint primary key, k "+text+", val "+text+" not null unique, n bigint not null default 0, version bigint not null default 1)",
		"CREATE TABLE mrg_src (id bigint, k "+text+", val "+text+" not null, n bigint not null default 0)",
	) {
		if _, err := db.Driver().Exec(ctx, stmt, nil); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		for _, stmt := range drop {
			_, _ = db.Driver().Exec(context.Background(), stmt, nil)
		}
	})
	for _, r := range target {
		if _, err := momo.Exec(ctx, db, "INSERT INTO mrg_rows (id, k, val) VALUES (?, ?, ?)", r...); err != nil {
			t.Fatal(err)
		}
	}
	for _, r := range src {
		if _, err := momo.Exec(ctx, db, "INSERT INTO mrg_src (id, k, val, n) VALUES (?, ?, ?, ?)", r...); err != nil {
			t.Fatal(err)
		}
	}
}

// mrgState reads mrg_rows as "id|k|val|n|version" lines in id order.
func mrgState(t *testing.T, q momo.Querier) []string {
	t.Helper()
	rows, err := momo.Select[mrgRow](q).OrderBy(mrgID.Asc()).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		k := "NULL"
		if r.Key != nil {
			k = *r.Key
		}
		out[i] = fmt.Sprintf("%d|%s|%s|%d|%d", r.ID, k, r.Val, r.N, r.Version)
	}
	return out
}

var (
	mrgBaseTarget = [][]any{{int64(1), "a", "t1"}, {int64(2), "b", "t2"}, {int64(3), nil, "t3"}}
	mrgBase       = []string{"1|a|t1|0|1", "2|b|t2|0|1", "3|NULL|t3|0|1"}
)

func withBase(extra ...string) []string { return append(slices.Clone(mrgBase), extra...) }

func mrgMergeOf(q momo.Querier) momo.MergeQuery[mrgRow] {
	return momo.Merge[mrgRow](q, mrgSource()).On(mrgOn())
}

func mrgUpdateOnly(q momo.Querier) momo.MergeQuery[mrgRow] {
	return mrgMergeOf(q).WhenMatchedUpdate(mrgVal.SetExpr(momo.TypedSQL[string]("src.val")))
}

func mrgInsertOnly(q momo.Querier) momo.MergeQuery[mrgRow] { return mrgInsertClause(mrgMergeOf(q)) }

// Catches: any clause combination whose final table or affected-row count
// differs from PostgreSQL's native MERGE: the match decided after a write (the
// ON column updated, so the INSERT sees the new value), a NULL key matching, a
// duplicate source row mishandled, the count not being updated plus inserted,
// and a failure part-way leaving the update behind.
func TestMergeEmulationMatchesPostgresIntegration(t *testing.T) {
	cases := []struct {
		name     string
		target   [][]any
		src      [][]any
		build    func(q momo.Querier) momo.MergeQuery[mrgRow]
		want     []string
		affected int64
		// after is run after the seed, to give a target row another value.
		after string
		// err, when set, is the kind the statement must fail with and the table
		// must be left as it was.
		err error
	}{
		{
			name:   "update and insert",
			target: mrgBaseTarget,
			src:    [][]any{{int64(10), "a", "s_a", int64(5)}, {int64(11), "c", "s_c", int64(6)}, {int64(12), nil, "s_null", int64(7)}},
			build:  mrgUpsert,
			// Row 1 matched and updated (version 2). Row 11 is new. The NULL key
			// matches nothing, not even the NULL key of row 3, so row 12 is new.
			want:     []string{"1|a|s_a|0|2", "2|b|t2|0|1", "3|NULL|t3|0|1", "11|c|s_c|0|1", "12|NULL|s_null|0|1"},
			affected: 3,
		},
		{
			name:     "update only",
			target:   mrgBaseTarget,
			src:      [][]any{{int64(10), "a", "s_a", int64(5)}, {int64(11), "c", "s_c", int64(6)}, {int64(12), nil, "s_null", int64(7)}},
			build:    mrgUpdateOnly,
			want:     []string{"1|a|s_a|0|2", "2|b|t2|0|1", "3|NULL|t3|0|1"},
			affected: 1,
		},
		{
			name:     "insert only",
			target:   mrgBaseTarget,
			src:      [][]any{{int64(10), "a", "s_a", int64(5)}, {int64(11), "c", "s_c", int64(6)}, {int64(12), nil, "s_null", int64(7)}},
			build:    mrgInsertOnly,
			want:     withBase("11|c|s_c|0|1", "12|NULL|s_null|0|1"),
			affected: 2,
		},
		{
			name:   "NULL keys never match",
			target: [][]any{{int64(1), nil, "t1"}},
			src:    [][]any{{int64(10), nil, "s", int64(1)}},
			build:  mrgUpsert,
			want:   []string{"1|NULL|t1|0|1", "10|NULL|s|0|1"}, affected: 1,
		},
		{
			// The update rewrites the column ON reads. The match is decided
			// before it: source row 'a' matched, and source row 'zz' did not, so
			// it is inserted even though the update just made a 'zz' target row.
			name:   "update of the ON column does not change the match",
			target: mrgBaseTarget,
			src:    [][]any{{int64(10), "a", "s_a", int64(1)}, {int64(13), "zz", "s_zz", int64(2)}},
			build: func(q momo.Querier) momo.MergeQuery[mrgRow] {
				return mrgInsertClause(mrgMergeOf(q).WhenMatchedUpdate(mrgKey.Set("zz")))
			},
			want:     []string{"1|zz|t1|0|2", "2|b|t2|0|1", "3|NULL|t3|0|1", "13|zz|s_zz|0|1"},
			affected: 2,
		},
		{
			name:   "one source row updates every matching target row",
			target: [][]any{{int64(1), "a", "t1"}, {int64(4), "a", "t4"}, {int64(2), "b", "t2"}},
			src:    [][]any{{int64(10), "a", "s_a", int64(9)}},
			build: func(q momo.Querier) momo.MergeQuery[mrgRow] {
				return mrgMergeOf(q).WhenMatchedUpdate(mrgN.SetExpr(momo.TypedSQL[int64]("src.n")))
			},
			want:     []string{"1|a|t1|9|2", "2|b|t2|0|1", "4|a|t4|9|2"},
			affected: 2,
		},
		{
			name:   "two source rows for one target row fail the update",
			target: mrgBaseTarget,
			src:    [][]any{{int64(10), "a", "x", int64(1)}, {int64(11), "a", "y", int64(2)}},
			build:  mrgUpsert,
			want:   mrgBase,
			err:    momo.ErrTooManyRows,
		},
		{
			// Bound values in the source and in an extra, non-key On predicate:
			// row 2 has n = 5, so source 'b' does not match it and is inserted.
			name:   "bound arguments and an extra On predicate",
			target: mrgBaseTarget,
			src:    [][]any{{int64(10), "b", "s_b", int64(0)}, {int64(11), "a", "s_a", int64(0)}},
			after:  "UPDATE mrg_rows SET n = 5 WHERE id = 2",
			build: func(q momo.Querier) momo.MergeQuery[mrgRow] {
				src := momo.RawSubquery("SELECT id, k, val, n FROM mrg_src WHERE id >= ?", int64(0))
				return mrgInsertClause(momo.Merge[mrgRow](q, src).On(mrgOn(), mrgN.Eq(0)).
					WhenMatchedUpdate(mrgVal.SetExpr(momo.TypedSQL[string]("src.val"))))
			},
			want:     []string{"1|a|s_a|0|2", "2|b|t2|5|1", "3|NULL|t3|0|1", "10|b|s_b|0|1"},
			affected: 2,
		},
		{
			// val is assigned from the old k and k from the source: every
			// assignment reads the row as it was before the statement (MySQL
			// assigns left to right and would read the new k).
			name:   "an assignment reading another assigned column sees the old value",
			target: mrgBaseTarget,
			src:    [][]any{{int64(10), "a", "s_a", int64(0)}},
			build: func(q momo.Querier) momo.MergeQuery[mrgRow] {
				return mrgMergeOf(q).WhenMatchedUpdate(
					mrgKey.SetExpr(momo.TypedSQL[string]("src.val")),
					mrgVal.SetExpr(momo.TypedSQL[string]("mrg_rows.k")))
			},
			want:     []string{"1|s_a|a|0|2", "2|b|t2|0|1", "3|NULL|t3|0|1"},
			affected: 1,
		},
		{
			name:     "two source rows for one target row are fine without an update",
			target:   mrgBaseTarget,
			src:      [][]any{{int64(10), "a", "x", int64(1)}, {int64(11), "a", "y", int64(2)}},
			build:    mrgInsertOnly,
			want:     mrgBase,
			affected: 0,
		},
		{
			name:     "duplicate unmatched source rows are both inserted",
			target:   mrgBaseTarget,
			src:      [][]any{{int64(20), "new", "p", int64(0)}, {int64(21), "new", "q", int64(0)}},
			build:    mrgInsertOnly,
			want:     withBase("20|new|p|0|1", "21|new|q|0|1"),
			affected: 2,
		},
		{
			name:     "empty source",
			target:   mrgBaseTarget,
			build:    mrgUpsert,
			want:     mrgBase,
			affected: 0,
		},
		{
			// The update of row 1 succeeds and the insert of row 11 then fails on
			// the unique val 't2'. The whole statement is undone, update included.
			name:   "a failure after the update rolls the update back",
			target: mrgBaseTarget,
			src:    [][]any{{int64(10), "a", "s_a", int64(0)}, {int64(11), "c", "t2", int64(0)}},
			build:  mrgUpsert,
			want:   mrgBase,
			err:    momo.ErrUniqueViolation,
		},
	}
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				mrgSetup(t, db, d, c.target, c.src)
				if c.after != "" {
					if _, err := momo.Exec(context.Background(), db, c.after); err != nil {
						t.Fatal(err)
					}
				}
				res, err := c.build(db).Exec(context.Background())
				switch {
				case c.err == nil && err != nil:
					t.Fatalf("Merge failed: %v", err)
				case c.err != nil && err == nil:
					t.Fatalf("Merge succeeded, want a failure of kind %v", c.err)
				case c.err != nil && !errors.Is(err, c.err):
					t.Fatalf("Merge failed with %v, want kind %v", err, c.err)
				}
				if c.err == nil && res.RowsAffected != c.affected {
					t.Errorf("RowsAffected = %d, want %d", res.RowsAffected, c.affected)
				}
				if got := mrgState(t, db); !slices.Equal(got, c.want) {
					t.Errorf("table after the Merge:\n got %v\nwant %v", got, c.want)
				}
			})
		}
	})
}

// Catches: the emulation not being atomic inside the caller's transaction
// (a failed step leaving the update behind, or rolling back the caller's own
// rows), the snapshot table surviving the first Merge so that a second one in
// the same transaction fails, and a rollback of the caller not undoing a Merge
// that succeeded. PostgreSQL has no emulation and aborts the whole transaction
// on an error, so only the emulated dialects run this.
func TestMergeEmulationInsideATransactionIntegration(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d == dialect.Postgres {
			t.Skip("native MERGE: nothing is emulated")
		}
		ctx := context.Background()
		failing := [][]any{{int64(10), "a", "s_a", int64(0)}, {int64(11), "c", "t2", int64(0)}}
		mrgSetup(t, db, d, mrgBaseTarget, failing)

		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if _, err := momo.Exec(ctx, tx, "INSERT INTO mrg_rows (id, k, val) VALUES (99, 'z', 'zz')"); err != nil {
				return err
			}
			if _, err := mrgUpsert(tx).Exec(ctx); !errors.Is(err, momo.ErrUniqueViolation) {
				t.Errorf("Merge inside a Tx = %v, want ErrUniqueViolation", err)
			}
			// Only the Merge was undone: the caller's own row is still here.
			if got, want := mrgState(t, tx), withBase("99|z|zz|0|1"); !slices.Equal(got, want) {
				t.Errorf("inside the Tx after the failed Merge:\n got %v\nwant %v", got, want)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := mrgState(t, db), withBase("99|z|zz|0|1"); !slices.Equal(got, want) {
			t.Errorf("after commit:\n got %v\nwant %v", got, want)
		}

		// Two Merges in one Tx, then the caller rolls back.
		mrgSetup(t, db, d, mrgBaseTarget, [][]any{{int64(10), "a", "s_a", int64(0)}, {int64(11), "c", "s_c", int64(0)}})
		boom := errors.New("caller rolls back")
		err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			for i := range 2 {
				if _, err := mrgUpsert(tx).Exec(ctx); err != nil {
					t.Errorf("Merge %d inside a Tx: %v", i+1, err)
				}
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("Tx = %v, want the caller's error", err)
		}
		if got := mrgState(t, db); !slices.Equal(got, mrgBase) {
			t.Errorf("after the rollback:\n got %v\nwant %v", got, mrgBase)
		}

		// A target row that two source rows match fails inside a Tx, and only
		// the Merge is undone.
		mrgSetup(t, db, d, mrgBaseTarget, [][]any{{int64(10), "a", "x", int64(0)}, {int64(11), "a", "y", int64(0)}})
		err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if _, err := momo.Exec(ctx, tx, "INSERT INTO mrg_rows (id, k, val) VALUES (99, 'z', 'zz')"); err != nil {
				return err
			}
			if _, err := mrgUpsert(tx).Exec(ctx); !errors.Is(err, momo.ErrTooManyRows) {
				t.Errorf("Merge with two source rows for one target row inside a Tx = %v, want ErrTooManyRows", err)
			}
			if got, want := mrgState(t, tx), withBase("99|z|zz|0|1"); !slices.Equal(got, want) {
				t.Errorf("inside the Tx after the failed Merge:\n got %v\nwant %v", got, want)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

// Catches: the pair key of the emulation using only the first primary-key
// column (a composite key then misses a duplicate or updates the wrong rows),
// and the same final table and count as PostgreSQL for a two-column key.
func TestMergeEmulationCompositeKeyIntegration(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		ctx := context.Background()
		text := "text"
		if d == dialect.MySQL {
			text = "varchar(64)"
		}
		drop := []string{"DROP TABLE IF EXISTS mrg_pairs", "DROP TABLE IF EXISTS mrg_pairs_src"}
		for _, stmt := range append(slices.Clone(drop),
			"CREATE TABLE mrg_pairs (a bigint not null, b bigint not null, v "+text+" not null, primary key (a, b))",
			"CREATE TABLE mrg_pairs_src (a bigint not null, b bigint not null, v "+text+" not null)",
		) {
			if _, err := db.Driver().Exec(ctx, stmt, nil); err != nil {
				t.Fatalf("%s: %v", stmt, err)
			}
		}
		t.Cleanup(func() {
			for _, stmt := range drop {
				_, _ = db.Driver().Exec(context.Background(), stmt, nil)
			}
		})
		state := func() []string {
			rows, err := momo.Select[mrgPair](db).OrderBy(mrgPairA.Asc(), mrgPairB.Asc()).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			out := make([]string, len(rows))
			for i, r := range rows {
				out[i] = fmt.Sprintf("%d|%d|%s", r.A, r.B, r.V)
			}
			return out
		}
		exec := func(sql string, args ...any) {
			if _, err := momo.Exec(ctx, db, sql, args...); err != nil {
				t.Fatal(err)
			}
		}
		exec("INSERT INTO mrg_pairs (a, b, v) VALUES (1, 1, 'x'), (1, 2, 'y')")
		exec("INSERT INTO mrg_pairs_src (a, b, v) VALUES (1, 1, 'X'), (2, 1, 'z')")
		res, err := mrgPairMerge(db).Exec(ctx)
		if err != nil || res.RowsAffected != 2 {
			t.Fatalf("Merge = %+v, %v, want 2 rows", res, err)
		}
		if got, want := state(), []string{"1|1|X", "1|2|y", "2|1|z"}; !slices.Equal(got, want) {
			t.Errorf("table:\n got %v\nwant %v", got, want)
		}
		// Two source rows for the key (1, 1): rows that differ in one key column
		// only are not duplicates, the same pair twice is.
		exec("DELETE FROM mrg_pairs_src")
		exec("INSERT INTO mrg_pairs_src (a, b, v) VALUES (1, 1, 'p'), (1, 2, 'q'), (1, 1, 'r')")
		if _, err := mrgPairMerge(db).Exec(ctx); !errors.Is(err, momo.ErrTooManyRows) {
			t.Fatalf("Merge with the key (1, 1) twice = %v, want ErrTooManyRows", err)
		}
		if got, want := state(), []string{"1|1|X", "1|2|y", "2|1|z"}; !slices.Equal(got, want) {
			t.Errorf("table after the failed Merge:\n got %v\nwant %v", got, want)
		}
	})
}

// Catches: RETURNING on the emulated dialects being accepted (it would report
// rows from a snapshot, not from the statement) or leaving a write behind
// before the refusal.
func TestMergeEmulationRefusesReturningIntegration(t *testing.T) {
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if d == dialect.Postgres {
			t.Skip("PostgreSQL 17 has native MERGE ... RETURNING")
		}
		mrgSetup(t, db, d, mrgBaseTarget, [][]any{{int64(10), "a", "s_a", int64(0)}})
		if _, err := mrgUpsert(db).Returning(mrgID).Rows(context.Background()); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("Rows = %v, want ErrUnsupported", err)
		}
		if got := mrgState(t, db); !slices.Equal(got, mrgBase) {
			t.Errorf("a refused Merge changed the table: %v", got)
		}
	})
}
