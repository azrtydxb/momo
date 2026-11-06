// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"math"
	"strings"
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
		}, ins + `?, ?, ?) ON CONFLICT ("name") WHERE "live" = 1 DO NOTHING`, 3},
		{"both wheres, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseTake(q).Where(leaseLive.Eq(true)).DoUpdate(leaseHolder.SetExcluded()).Where(leaseUntil.LtExpr(leaseUntil.Excluded()))
		}, ins + `$1, $2, $3) ON CONFLICT ("name") WHERE "live" = $4 DO UPDATE SET "holder" = EXCLUDED."holder" WHERE "jobs_lock"."until" < EXCLUDED."until"`, 4},
		{"multiple action wheres AND, pg", dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
			return takeSet(q).Where(leaseHolder.Eq("h")).Where(leaseLive.Eq(false), leaseUntil.Gt(leaseNow))
		}, ins + `$1, $2, $3) ON CONFLICT ("name") DO UPDATE SET "holder" = EXCLUDED."holder", "until" = EXCLUDED."until" WHERE ("jobs_lock"."holder" = $4 AND "jobs_lock"."live" = $5 AND "jobs_lock"."until" > $6)`, 6},
		{"multiple target wheres AND, sqlite", dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
			return leaseTake(q).Where(leaseLive.Eq(true)).Where(leaseHolder.Ne("x")).DoNothing()
		}, ins + `?, ?, ?) ON CONFLICT ("name") WHERE ("live" = 1 AND "holder" <> 'x') DO NOTHING`, 3},
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

var leasePrio = momo.NewColumn[int]("jobs_lock", "prio", 0)

// Catches: SQLite binding a target-predicate value (the index text never
// matches a placeholder), a bool rendered as true/false instead of 1/0, an
// unescaped quote in a string literal, and PostgreSQL losing its bind
// parameters to the inline rendering.
func TestConflictTargetSQLiteLiterals(t *testing.T) {
	cases := []struct {
		name  string
		d     dialect.Name
		where momo.Predicate
		want  string
		nargs int
	}{
		{"bool true", dialect.SQLite, leaseLive.Eq(true), `WHERE "live" = 1`, 3},
		{"bool false", dialect.SQLite, leaseLive.Eq(false), `WHERE "live" = 0`, 3},
		{"integer", dialect.SQLite, leasePrio.Gt(-7), `WHERE "prio" > -7`, 3},
		{"string", dialect.SQLite, leaseHolder.Eq("a"), `WHERE "holder" = 'a'`, 3},
		{"string with quote", dialect.SQLite, leaseHolder.Eq("o'x'; DROP"), `WHERE "holder" = 'o''x''; DROP'`, 3},
		{"in and not", dialect.SQLite, momo.Not(leasePrio.In(1, 2)), `WHERE NOT ("prio" IN (1, 2))`, 3},
		{"between and is null", dialect.SQLite, momo.And(leasePrio.Between(1, 9), leaseHolder.IsNull()), `WHERE ("prio" BETWEEN 1 AND 9 AND "holder" IS NULL)`, 3},
		{"postgres still binds", dialect.Postgres, leaseLive.Eq(true), `WHERE "live" = $4`, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := compileFor(t, c.d, func(q momo.Querier) momo.InsertQuery[Lease] {
				return leaseTake(q).Where(c.where).DoNothing()
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got.SQL, c.want) {
				t.Fatalf("SQL %s\n does not contain %s", got.SQL, c.want)
			}
			if len(got.Args) != c.nargs {
				t.Fatalf("args = %d %v, want %d", len(got.Args), got.Args, c.nargs)
			}
		})
	}
}

// Catches: inline literals leaking into the DO UPDATE WHERE or the VALUES
// list on SQLite (values there must stay bound).
func TestConflictActionStaysBoundOnSQLite(t *testing.T) {
	got, err := compileFor(t, dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
		return leaseTake(q).Where(leaseLive.Eq(true)).DoUpdate(leaseHolder.SetExcluded()).Where(leaseHolder.Eq("h"))
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "jobs_lock" ("name", "holder", "until") VALUES (?, ?, ?) ON CONFLICT ("name") WHERE "live" = 1 DO UPDATE SET "holder" = EXCLUDED."holder" WHERE "jobs_lock"."holder" = ?`
	if got.SQL != want || len(got.Args) != 4 {
		t.Fatalf("SQL\n got: %s (%d args)\nwant: %s (4 args)", got.SQL, len(got.Args), want)
	}
}

// Catches: Excluded() accepted in the conflict-target predicate (the
// database rejects it with an opaque error), including when nested.
func TestConflictTargetExcludedIsValidation(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		for name, p := range map[string]momo.Predicate{
			"direct": leaseUntil.LtExpr(leaseUntil.Excluded()),
			"nested": momo.And(leaseLive.Eq(true), momo.Not(leaseUntil.LeExpr(leaseUntil.Excluded()))),
		} {
			t.Run(string(d)+"/"+name, func(t *testing.T) {
				_, err := compileFor(t, d, func(q momo.Querier) momo.InsertQuery[Lease] {
					return leaseTake(q).Where(p).DoNothing()
				})
				if !errors.Is(err, momo.ErrValidation) {
					t.Fatalf("err = %v, want ErrValidation", err)
				}
			})
		}
	}
	// The action predicate keeps accepting it.
	if _, err := compileFor(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
		return leaseTake(q).DoUpdate(leaseHolder.SetExcluded()).Where(leaseUntil.LtExpr(leaseUntil.Excluded()))
	}); err != nil {
		t.Fatalf("Excluded in the action predicate: %v", err)
	}
}

// Catches: nested junctions, Not, In, Between and IsNull losing parentheses
// or their table qualifier in the DO UPDATE WHERE.
func TestConflictActionWhereNestedGolden(t *testing.T) {
	got, err := compileFor(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
		return leaseTake(q).DoUpdate(leaseHolder.SetExcluded()).Where(
			momo.Or(
				momo.Not(momo.And(leaseHolder.In("a", "b"), leaseUntil.Between(leaseNow, leaseNow.Add(1)))),
				momo.And(leaseLive.IsNull(), leasePrio.NotIn(1))))
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "jobs_lock" ("name", "holder", "until") VALUES ($1, $2, $3) ON CONFLICT ("name") DO UPDATE SET "holder" = EXCLUDED."holder" WHERE (NOT (("jobs_lock"."holder" IN ($4, $5) AND "jobs_lock"."until" BETWEEN $6 AND $7)) OR ("jobs_lock"."live" IS NULL AND "jobs_lock"."prio" NOT IN ($8)))`
	if got.SQL != want || len(got.Args) != 8 {
		t.Fatalf("SQL\n got: %s (%d args)\nwant: %s (8 args)", got.SQL, len(got.Args), want)
	}
}

// Catches: a chunked Models upsert numbering the target and action predicate
// arguments from the wrong offset, or sizing chunks without reserving room
// for them (two predicate binds per chunk).
func TestConflictWhereChunkedArgNumbering(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithMaxParams(10))
	rows := []Lease{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	_, err := momo.Insert[Lease](db).Models(rows).
		OnConflict(leaseName).Where(leaseLive.Eq(true)).
		DoUpdate(leaseHolder.SetExcluded()).Where(leaseHolder.Ne("x")).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	const head = `INSERT INTO "jobs_lock" ("name", "holder", "until", "live") VALUES `
	const tail = ` DO UPDATE SET "holder" = EXCLUDED."holder" WHERE "jobs_lock"."holder" <> `
	want := []string{
		head + `($1, $2, $3, $4), ($5, $6, $7, $8) ON CONFLICT ("name") WHERE "live" = $9` + tail + `$10`,
		head + `($1, $2, $3, $4) ON CONFLICT ("name") WHERE "live" = $5` + tail + `$6`,
	}
	var got []string
	for _, c := range drv.Calls() {
		if strings.HasPrefix(c.SQL, "INSERT") {
			got = append(got, c.SQL)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("statements = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk %d\n got: %s\nwant: %s", i, got[i], want[i])
		}
	}
}

// Catches: ConflictQuery.Where / InsertQuery.Where appending into a shared
// backing array, so two queries derived from one base overwrite each other.
func TestConflictWhereDoesNotAlias(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	// Three predicates leave spare capacity in the backing array, which is
	// what makes an unclipped append alias; the action is chosen only after
	// both siblings exist, so neither has been cloned yet.
	baseTarget := leaseTake(db).Where(leaseLive.Eq(true)).Where(leaseLive.Ne(false)).Where(leaseUntil.IsNotNull())
	ta := baseTarget.Where(leaseHolder.Eq("A"))
	tb := baseTarget.Where(leaseHolder.Eq("B"))
	a, b := ta.DoNothing(), tb.DoNothing()
	baseAction := leaseTake(db).DoUpdate(leaseHolder.SetExcluded()).Where(leaseLive.Eq(true)).Where(leaseLive.Ne(false)).Where(leaseUntil.IsNotNull())
	c := baseAction.Where(leaseHolder.Eq("A"))
	d := baseAction.Where(leaseHolder.Eq("B"))
	for name, tc := range map[string]struct {
		q    momo.InsertQuery[Lease]
		want string
	}{"target a": {a, "A"}, "target b": {b, "B"}, "action a": {c, "A"}, "action b": {d, "B"}} {
		got, err := tc.q.SQL()
		if err != nil {
			t.Fatal(name, err)
		}
		if len(got.Args) < 1 || got.Args[len(got.Args)-1] != tc.want {
			t.Fatalf("%s: args = %v, want last %q", name, got.Args, tc.want)
		}
	}
}

// status is a named string whose bound form differs from its Go value.
type status string

func (s status) Value() (driver.Value, error) { return strings.ToUpper(string(s)), nil }

// badStatus is a Valuer whose bound form cannot be rendered inline.
type badStatus string

func (badStatus) Value() (driver.Value, error) { return time.Unix(0, 0).UTC(), nil }

var (
	leaseRatio  = momo.NewColumn[float64]("jobs_lock", "ratio", 0)
	leaseBlob   = momo.NewColumn[[]byte]("jobs_lock", "blob", 0)
	leasePtr    = momo.NewColumn[*string]("jobs_lock", "ptr", 0)
	leaseNull   = momo.NewColumn[sql.NullString]("jobs_lock", "nul", 0)
	leaseStatus = momo.NewColumn[status]("jobs_lock", "status", 0)
	leaseBad    = momo.NewColumn[badStatus]("jobs_lock", "bad", 0)
)

// Catches: a target-predicate value SQLite cannot match inline staying bound
// and reproducing the "does not match any PRIMARY KEY or UNIQUE constraint"
// failure, instead of failing the build with ErrValidation.
func TestConflictTargetSQLiteUnrenderableIsValidation(t *testing.T) {
	str := "x"
	cases := map[string]momo.Predicate{
		"time":            leaseUntil.Le(leaseNow),
		"nan":             leaseRatio.Gt(math.NaN()),
		"inf":             leaseRatio.Lt(math.Inf(1)),
		"bytes":           leaseBlob.Eq([]byte("a")),
		"pointer":         leasePtr.Eq(&str),
		"sql null (NULL)": leaseNull.Eq(sql.NullString{}),
		"string with NUL": leaseHolder.Eq("a\x00b"),
		"valuer to time":  leaseBad.Eq("b"),
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := compileFor(t, dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
				return leaseTake(q).Where(p).DoNothing()
			})
			if !errors.Is(err, momo.ErrValidation) {
				t.Fatalf("err = %v, want ErrValidation", err)
			}
			if err != nil && !strings.Contains(err.Error(), "ON CONFLICT target") {
				t.Fatalf("err = %v, want it to name the ON CONFLICT target", err)
			}
		})
		t.Run(name+"/postgres binds", func(t *testing.T) {
			if _, err := compileFor(t, dialect.Postgres, func(q momo.Querier) momo.InsertQuery[Lease] {
				return leaseTake(q).Where(p).DoNothing()
			}); err != nil {
				t.Fatalf("postgres: %v", err)
			}
		})
	}
}

// Catches: floats rendered inexactly or as integer literals, and a named
// type rendered from its Go value instead of the value it binds as.
func TestConflictTargetSQLiteFloatAndValuer(t *testing.T) {
	cases := []struct {
		name  string
		where momo.Predicate
		want  string
	}{
		{"float", leaseRatio.Gt(1.5), `WHERE "ratio" > 1.5`},
		{"whole float stays a real", leaseRatio.Gt(2), `WHERE "ratio" > 2.0`},
		{"exponent float", leaseRatio.Lt(1e21), `WHERE "ratio" < 1e+21`},
		{"valuer", leaseStatus.Eq("a"), `WHERE "status" = 'A'`},
		{"sql null (valid)", leaseNull.Eq(sql.NullString{String: "a", Valid: true}), `WHERE "nul" = 'a'`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := compileFor(t, dialect.SQLite, func(q momo.Querier) momo.InsertQuery[Lease] {
				return leaseTake(q).Where(c.where).DoNothing()
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got.SQL, c.want) || len(got.Args) != 3 {
				t.Fatalf("SQL %s (%d args), want %s and 3 args", got.SQL, len(got.Args), c.want)
			}
		})
	}
}
