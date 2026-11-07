// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

var leaseTable = momo.TableRef{Name: "jobs_lock"}

// leaseGuarded is the issue's statement over Lease: insert the row only when
// no live row exists, in one statement. The guard selects the constant 1
// (a bound parameter would have no inferable type inside EXISTS).
func leaseGuarded(q momo.Querier, name string) momo.InsertQuery[Lease] {
	row := momo.SelectExprs(momo.Value(name), momo.Value("h"), momo.Value(leaseNow), momo.Value(true)).
		Where(momo.NotExists(momo.SelectExprs(momo.TypedSQL[int]("1")).From(leaseTable).Where(leaseLive.Eq(true))))
	return momo.Insert[Lease](q).FromSelect(row, leaseName, leaseHolder, leaseUntil, leaseLive)
}

// Catches: the select rendered inside parentheses, the target column list
// dropped, placeholders numbered per part instead of across the statement
// (the select, then ON CONFLICT), and a missing FROM-less SELECT.
func TestFromSelectGoldenSQL(t *testing.T) {
	const head = `INSERT INTO "jobs_lock" ("name", "holder", "until", "live") `
	cases := []struct {
		name  string
		d     dialect.Name
		build func(q momo.Querier) momo.InsertQuery[Lease]
		want  string
		nargs int
	}{
		{"not exists do nothing, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseGuarded(q, "a").OnConflict(leaseName).DoNothing()
		}, head + `SELECT $1, $2, $3, $4 WHERE NOT EXISTS (SELECT 1 FROM "jobs_lock" WHERE "jobs_lock"."live" = $5) ON CONFLICT ("name") DO NOTHING`, 5},
		{"not exists do nothing, sqlite", dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseGuarded(q, "a").OnConflict(leaseName).DoNothing()
		}, head + `SELECT ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM "jobs_lock" WHERE "jobs_lock"."live" = ?) ON CONFLICT ("name") DO NOTHING`, 5},
		{"conflict params after select params, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseGuarded(q, "a").OnConflict(leaseName).DoUpdate(leaseHolder.SetExcluded()).Where(leaseHolder.Ne("x"))
		}, head + `SELECT $1, $2, $3, $4 WHERE NOT EXISTS (SELECT 1 FROM "jobs_lock" WHERE "jobs_lock"."live" = $5) ON CONFLICT ("name") DO UPDATE SET "holder" = EXCLUDED."holder" WHERE "jobs_lock"."holder" <> $6`, 6},
		{"conflict params after select params, sqlite", dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseGuarded(q, "a").OnConflict(leaseName).DoUpdate(leaseHolder.SetExcluded()).Where(leaseHolder.Ne("x"))
		}, head + `SELECT ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM "jobs_lock" WHERE "jobs_lock"."live" = ?) ON CONFLICT ("name") DO UPDATE SET "holder" = EXCLUDED."holder" WHERE "jobs_lock"."holder" <> ?`, 6},
		{"returning, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseGuarded(q, "a").OnConflict(leaseName).DoNothing().Returning(leaseName, leaseHolder)
		}, head + `SELECT $1, $2, $3, $4 WHERE NOT EXISTS (SELECT 1 FROM "jobs_lock" WHERE "jobs_lock"."live" = $5) ON CONFLICT ("name") DO NOTHING RETURNING "name", "holder"`, 5},
		{"plain select from model query, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			src := momo.Select[Lease](q).Where(leaseLive.Eq(false))
			return momo.Insert[Lease](q).FromSelect(src, leaseName, leaseHolder, leaseUntil, leaseLive)
		}, head + `SELECT "jobs_lock"."name", "jobs_lock"."holder", "jobs_lock"."until", "jobs_lock"."live" FROM "jobs_lock" WHERE "jobs_lock"."live" = $1`, 1},
		{"mysql from-less with where uses dual", dialect.MySQL, func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(
				momo.SelectExprs(momo.Value("a"), momo.Value("h")).Where(momo.NotExists(momo.SelectExprs(momo.TypedSQL[int]("1")).From(leaseTable).Where(leaseLive.Eq(true)))), leaseName, leaseHolder)
		}, "INSERT INTO `jobs_lock` (`name`, `holder`) SELECT ?, ? FROM DUAL WHERE NOT EXISTS (SELECT 1 FROM `jobs_lock` WHERE `jobs_lock`.`live` = ?)", 3},
		{"sqlite from select without where adds where true", dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(
				momo.SelectExprs(momo.Value("a"), momo.Value("h")).From(leaseTable), leaseName, leaseHolder).
				OnConflict(leaseName).DoNothing()
		}, `INSERT INTO "jobs_lock" ("name", "holder") SELECT ?, ? FROM "jobs_lock" WHERE true ON CONFLICT ("name") DO NOTHING`, 2},
		{"pg from select without where has no where true", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(
				momo.SelectExprs(momo.Value("a"), momo.Value("h")).From(leaseTable), leaseName, leaseHolder).
				OnConflict(leaseName).DoNothing()
		}, `INSERT INTO "jobs_lock" ("name", "holder") SELECT $1, $2 FROM "jobs_lock" ON CONFLICT ("name") DO NOTHING`, 2},
		{"sqlite select[T] source without where adds where true", dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(momo.Select[Lease](q), leaseName, leaseHolder, leaseUntil, leaseLive).
				OnConflict(leaseName).DoNothing()
		}, head + `SELECT "jobs_lock"."name", "jobs_lock"."holder", "jobs_lock"."until", "jobs_lock"."live" FROM "jobs_lock" WHERE true ON CONFLICT ("name") DO NOTHING`, 0},
		{"sqlite select[T] source with where is unchanged", dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(momo.Select[Lease](q).Where(leaseLive.Eq(false)), leaseName, leaseHolder, leaseUntil, leaseLive).
				OnConflict(leaseName).DoNothing()
		}, head + `SELECT "jobs_lock"."name", "jobs_lock"."holder", "jobs_lock"."until", "jobs_lock"."live" FROM "jobs_lock" WHERE "jobs_lock"."live" = ? ON CONFLICT ("name") DO NOTHING`, 1},
		{"pg select[T] source without where is unchanged", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(momo.Select[Lease](q), leaseName, leaseHolder, leaseUntil, leaseLive).
				OnConflict(leaseName).DoNothing()
		}, head + `SELECT "jobs_lock"."name", "jobs_lock"."holder", "jobs_lock"."until", "jobs_lock"."live" FROM "jobs_lock" ON CONFLICT ("name") DO NOTHING`, 0},
		{"soft-delete table ref filters the guard and the source", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			sd := momo.TableRef{Name: "jobs_lock", SoftDelete: "deleted_at", SoftDeleteKind: momo.SoftDeleteTimestamp}
			row := momo.SelectExprs(momo.Value("a"), momo.Value("h")).
				Where(momo.NotExists(momo.SelectExprs(momo.TypedSQL[int]("1")).From(sd).Where(leaseLive.Eq(true))))
			return momo.Insert[Lease](q).FromSelect(row, leaseName, leaseHolder).OnConflict(leaseName).DoNothing()
		}, `INSERT INTO "jobs_lock" ("name", "holder") SELECT $1, $2 WHERE NOT EXISTS (SELECT 1 FROM "jobs_lock" WHERE ("jobs_lock"."live" = $3) AND ("jobs_lock"."deleted_at" IS NULL)) ON CONFLICT ("name") DO NOTHING`, 3},
		{"upsert default set covers listed columns only, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Upsert[Lease](q).FromSelect(momo.SelectExprs(momo.Value("a"), momo.Value("h")), leaseName, leaseHolder)
		}, `INSERT INTO "jobs_lock" ("name", "holder") SELECT $1, $2 ON CONFLICT ("name") DO UPDATE SET "holder" = EXCLUDED."holder"`, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := compileFor(t, c.d, c.build)
			if err != nil {
				t.Fatal(err)
			}
			if got.SQL != c.want {
				t.Fatalf("SQL\n got: %s\nwant: %s", got.SQL, c.want)
			}
			if len(got.Args) != c.nargs {
				t.Fatalf("args = %d %v, want %d", len(got.Args), got.Args, c.nargs)
			}
		})
	}
}

// Catches: argument order drifting from placeholder order (select values
// first, the guard next, then the ON CONFLICT part).
func TestFromSelectArgOrder(t *testing.T) {
	got, err := compileFor(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
		return leaseGuarded(q, "a").OnConflict(leaseName).DoUpdate(leaseHolder.SetExcluded()).Where(leaseHolder.Ne("x"))
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []any{"a", "h", leaseNow, true, true, "x"}
	if len(got.Args) != len(want) {
		t.Fatalf("args = %v, want %v", got.Args, want)
	}
	for i := range want {
		if got.Args[i] != want[i] {
			t.Fatalf("arg %d = %v, want %v (all: %v)", i, got.Args[i], want[i], got.Args)
		}
	}
}

// Catches: a width mismatch between the target columns and the select list
// reaching the database, and the other refused shapes being accepted.
func TestFromSelectValidation(t *testing.T) {
	cases := []struct {
		name  string
		build func(q momo.Querier) momo.InsertQuery[Lease]
	}{
		{"fewer select columns", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(momo.SelectExprs(momo.Value("a")), leaseName, leaseHolder)
		}},
		{"more select columns", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(momo.SelectExprs(momo.Value("a"), momo.Value("b")), leaseName)
		}},
		{"model query width mismatch", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(momo.Select[Lease](q), leaseName)
		}},
		{"no target columns", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(momo.SelectExprs(momo.Value("a")))
		}},
		{"duplicate column", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(momo.SelectExprs(momo.Value("a"), momo.Value("b")), leaseName, leaseName)
		}},
		{"unmapped column", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(momo.SelectExprs(momo.Value("a")), momo.NewColumn[string]("jobs_lock", "nope", 0))
		}},
		{"combined with Values", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).Values(leaseName.Set("a")).FromSelect(momo.SelectExprs(momo.Value("a")), leaseName)
		}},
		{"combined with Model", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).Model(&Lease{Name: "a"}).FromSelect(momo.SelectExprs(momo.Value("a")), leaseName)
		}},
		{"nil select", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(nil, leaseName)
		}},
		{"empty select list", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Insert[Lease](q).FromSelect(momo.SelectExprs(), leaseName)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := compileFor(t, dialect.Postgres, c.build)
			if !errors.Is(err, momo.ErrValidation) {
				t.Fatalf("err = %v, want ErrValidation", err)
			}
		})
	}
}
