// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/vector"
)

// Lease is a lock row taken with an expiry: name is the key, holder owns it
// until the until instant. Hand-written as the generator would emit it.
type Lease struct {
	Name   string
	Holder string
	Until  time.Time
	Live   bool
}

var (
	leaseName   = momo.NewColumn[string]("jobs_lock", "name", momo.FlagPK)
	leaseHolder = momo.NewColumn[string]("jobs_lock", "holder", 0)
	leaseUntil  = momo.NewColumn[time.Time]("jobs_lock", "until", 0)
	leaseLive   = momo.NewColumn[bool]("jobs_lock", "live", 0)
)

var leaseMeta = &momo.Meta[Lease]{
	Table: "jobs_lock",
	Columns: []momo.ColumnMeta{
		{Name: "name", Flags: leaseName.Flags()},
		{Name: "holder", Flags: leaseHolder.Flags()},
		{Name: "until", Flags: leaseUntil.Flags()},
		{Name: "live", Flags: leaseLive.Flags()},
	},
	Ptr: func(m *Lease, i int) any {
		return [...]any{&m.Name, &m.Holder, &m.Until, &m.Live}[i]
	},
	Value: func(m *Lease, i int) any {
		return [...]any{m.Name, m.Holder, m.Until, m.Live}[i]
	},
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*Lease) MomoMeta() *momo.Meta[Lease] { return leaseMeta }

var leaseNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func leaseInsert(q momo.Querier) momo.InsertQuery[Lease] {
	return momo.Insert[Lease](q).Values(leaseName.Set("a"), leaseHolder.Set("h"), leaseUntil.Set(leaseNow))
}

func leaseTake(q momo.Querier) momo.ConflictQuery[Lease] {
	return leaseInsert(q).OnConflict(leaseName)
}

func compileFor(t *testing.T, name dialect.Name, b func(q momo.Querier) momo.InsertQuery[Lease]) (momo.Compiled, error) {
	t.Helper()
	return b(momo.New(fakedriver.New(name))).SQL()
}

// Catches: either WHERE dropped from the render, the table qualification
// dropped from the DO UPDATE WHERE, the wrong clause order, and multiple
// Where calls not ANDed.
func TestConflictWhereGoldenSQL(t *testing.T) {
	const ins = `INSERT INTO "jobs_lock" ("name", "holder", "until") VALUES (`
	takeSet := func(q momo.Querier) momo.InsertQuery[Lease] {
		return leaseTake(q).DoUpdate(leaseHolder.SetExcluded(), leaseUntil.SetExcluded())
	}
	cases := []struct {
		name  string
		d     dialect.Name
		build func(q momo.Querier) momo.InsertQuery[Lease]
		want  string
		nargs int
	}{
		{"action where, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return takeSet(q).Where(momo.Or(leaseUntil.Le(leaseNow), leaseHolder.Eq("h")))
		}, ins + `$1, $2, $3) ON CONFLICT ("name") DO UPDATE SET "holder" = EXCLUDED."holder", "until" = EXCLUDED."until" WHERE ("jobs_lock"."until" <= $4 OR "jobs_lock"."holder" = $5)`, 5},
		{"action where, sqlite", dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
			return takeSet(q).Where(momo.Or(leaseUntil.Le(leaseNow), leaseHolder.Eq("h")))
		}, ins + `?, ?, ?) ON CONFLICT ("name") DO UPDATE SET "holder" = EXCLUDED."holder", "until" = EXCLUDED."until" WHERE ("jobs_lock"."until" <= ? OR "jobs_lock"."holder" = ?)`, 5},
		{"target where do update, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseTake(q).Where(leaseLive.Eq(true)).DoUpdate(leaseHolder.SetExcluded())
		}, ins + `$1, $2, $3) ON CONFLICT ("name") WHERE "live" = $4 DO UPDATE SET "holder" = EXCLUDED."holder"`, 4},
		{"target where do nothing, sqlite", dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseTake(q).Where(leaseLive.Eq(true)).DoNothing()
		}, ins + `?, ?, ?) ON CONFLICT ("name") WHERE "live" = ? DO NOTHING`, 4},
		{"both wheres, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseTake(q).Where(leaseLive.Eq(true)).DoUpdate(leaseHolder.SetExcluded()).Where(leaseUntil.LtExpr(leaseUntil.Excluded()))
		}, ins + `$1, $2, $3) ON CONFLICT ("name") WHERE "live" = $4 DO UPDATE SET "holder" = EXCLUDED."holder" WHERE "jobs_lock"."until" < EXCLUDED."until"`, 4},
		{"multiple action wheres AND, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return takeSet(q).Where(leaseHolder.Eq("h")).Where(leaseLive.Eq(false), leaseUntil.Gt(leaseNow))
		}, ins + `$1, $2, $3) ON CONFLICT ("name") DO UPDATE SET "holder" = EXCLUDED."holder", "until" = EXCLUDED."until" WHERE ("jobs_lock"."holder" = $4 AND "jobs_lock"."live" = $5 AND "jobs_lock"."until" > $6)`, 6},
		{"multiple target wheres AND, sqlite", dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseTake(q).Where(leaseLive.Eq(true)).Where(leaseHolder.Ne("x")).DoNothing()
		}, ins + `?, ?, ?) ON CONFLICT ("name") WHERE ("live" = ? AND "holder" <> ?) DO NOTHING`, 5},
		{"where then returning, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return takeSet(q).Where(leaseHolder.Eq("h")).Returning(leaseHolder)
		}, ins + `$1, $2, $3) ON CONFLICT ("name") DO UPDATE SET "holder" = EXCLUDED."holder", "until" = EXCLUDED."until" WHERE "jobs_lock"."holder" = $4 RETURNING "holder"`, 4},
		{"upsert default set with where, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Upsert[Lease](q).Model(&Lease{Name: "a", Holder: "h", Until: leaseNow}).Where(leaseUntil.LeExpr(leaseUntil.Excluded()))
		}, `INSERT INTO "jobs_lock" ("name", "holder", "until", "live") VALUES ($1, $2, $3, $4) ON CONFLICT ("name") DO UPDATE SET "holder" = EXCLUDED."holder", "until" = EXCLUDED."until", "live" = EXCLUDED."live" WHERE "jobs_lock"."until" <= EXCLUDED."until"`, 4},
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

// Catches: the table qualification dropped from the DO UPDATE WHERE (a bare
// column is ambiguous against EXCLUDED), and a qualifier leaking into the
// conflict-target predicate, where PostgreSQL index predicates are bare.
func TestConflictWhereQualification(t *testing.T) {
	got, err := compileFor(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
		return leaseTake(q).Where(leaseLive.IsNotNull()).DoUpdate(leaseHolder.SetExcluded()).Where(leaseHolder.IsNotNull())
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "jobs_lock" ("name", "holder", "until") VALUES ($1, $2, $3) ON CONFLICT ("name") WHERE "live" IS NOT NULL DO UPDATE SET "holder" = EXCLUDED."holder" WHERE "jobs_lock"."holder" IS NOT NULL`
	if got.SQL != want {
		t.Fatalf("SQL\n got: %s\nwant: %s", got.SQL, want)
	}
}

// Catches: a Where that is silently ignored or accepted when it has no
// DO UPDATE to attach to, or a target Where without a target.
func TestConflictWhereValidation(t *testing.T) {
	cases := []struct {
		name  string
		build func(q momo.Querier) momo.InsertQuery[Lease]
	}{
		{"target where without target columns", func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseInsert(q).OnConflict().Where(leaseLive.Eq(true)).DoNothing()
		}},
		{"action where after DoNothing", func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseTake(q).DoNothing().Where(leaseHolder.Eq("h"))
		}},
		{"action where without any conflict clause", func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseInsert(q).Where(leaseHolder.Eq("h"))
		}},
		{"action where before the conflict action is chosen", func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseInsert(q).Where(leaseHolder.Eq("h")).OnConflict(leaseName).DoNothing()
		}},
		{"upsert with DoNothing", func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Upsert[Lease](q).Model(&Lease{Name: "a"}).OnConflict(leaseName).DoNothing().Where(leaseHolder.Eq("h"))
		}},
	}
	for _, c := range cases {
		for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
			t.Run(c.name+"/"+string(d), func(t *testing.T) {
				_, err := compileFor(t, d, c.build)
				if !errors.Is(err, momo.ErrValidation) {
					t.Fatalf("err = %v, want ErrValidation", err)
				}
			})
		}
	}
}

// Catches: MySQL rendering or silently dropping a conditional upsert.
func TestConflictWhereMySQLUnsupported(t *testing.T) {
	cases := map[string]func(q momo.Querier) momo.InsertQuery[Lease]{
		"target where": func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseTake(q).Where(leaseLive.Eq(true)).DoNothing()
		},
		"action where": func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseTake(q).DoUpdate(leaseHolder.SetExcluded()).Where(leaseHolder.Eq("h"))
		},
		"upsert action where": func(q momo.Querier) momo.InsertQuery[Lease] {
			return momo.Upsert[Lease](q).Model(&Lease{Name: "a"}).Where(leaseHolder.Eq("h"))
		},
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := compileFor(t, dialect.MySQL, b)
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("err = %v, want ErrUnsupported", err)
			}
		})
	}
}

// Catches: Excluded rendering the wrong qualifier or an unquoted name, for
// both the plain and the vector column.
func TestExcludedRendersEXCLUDEDColumn(t *testing.T) {
	render := func(e momo.Expr) string {
		b := momo.NewBuilder(dialect.For(dialect.Postgres))
		e.AppendSQL(b)
		return b.String()
	}
	if got, want := render(leaseHolder.Excluded()), `EXCLUDED."holder"`; got != want {
		t.Fatalf("Column.Excluded = %s, want %s", got, want)
	}
	emb := momo.NewVectorColumn[vector.Vector]("items", "embedding", 3, 0)
	if got, want := render(emb.Excluded()), `EXCLUDED."embedding"`; got != want {
		t.Fatalf("VectorColumn.Excluded = %s, want %s", got, want)
	}
	nemb := momo.NewNullVectorColumn[vector.Vector]("items", "embedding", 3, 0)
	if got, want := render(nemb.Excluded()), `EXCLUDED."embedding"`; got != want {
		t.Fatalf("NullVectorColumn.Excluded = %s, want %s", got, want)
	}
}
