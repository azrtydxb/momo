// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// mrgNoPK has no primary key, so a matched update cannot be emulated.
type mrgNoPK struct{ K, V string }

var mrgNoPKMeta = momo.Meta[mrgNoPK]{
	Table:    "mrg_nopk",
	Columns:  []momo.ColumnMeta{{Name: "k"}, {Name: "v"}},
	Ptr:      func(m *mrgNoPK, i int) any { return [...]any{&m.K, &m.V}[i] },
	Value:    func(m *mrgNoPK, i int) any { return [...]any{m.K, m.V}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*mrgNoPK) MomoMeta() *momo.Meta[mrgNoPK] { return &mrgNoPKMeta }

func statementSQL(t *testing.T, q momo.MergeQuery[mrgRow]) []string {
	t.Helper()
	cs, err := q.Statements()
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(cs))
	for i, c := range cs {
		if len(c.Args) != 0 {
			t.Errorf("statement %d binds %v, want no arguments", i, c.Args)
		}
		out[i] = c.SQL
	}
	return out
}

func assertStatements(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d statements, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("statement %d:\n got %s\nwant %s", i, got[i], want[i])
		}
	}
}

// Catches: a wrong statement order (the INSERT must read the flag the snapshot
// holds, and the UPDATE must come after the cardinality check), an UPDATE that
// qualifies the SET target where SQLite refuses it, or a lost version bump.
func TestMergeEmulationGoldenSQLite(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.SQLite))
	const drop = `DROP TABLE IF EXISTS temp."momo_merge_src"`
	assertStatements(t, statementSQL(t, mrgUpsert(db)), []string{
		drop,
		`CREATE TEMPORARY TABLE "momo_merge_src" AS SELECT "src".*, CASE WHEN EXISTS (SELECT 1 FROM "mrg_rows" WHERE "mrg_rows"."k" = src.k) THEN 1 ELSE 0 END AS "momo_merge_matched" FROM (SELECT id, k, val, n FROM mrg_src) AS "src"`,
		`SELECT 1 FROM "mrg_rows", "momo_merge_src" AS "src" WHERE "mrg_rows"."k" = src.k GROUP BY "mrg_rows"."id" HAVING COUNT(*) > 1 LIMIT 1`,
		`SELECT COUNT(*) FROM "mrg_rows", "momo_merge_src" AS "src" WHERE "mrg_rows"."k" = src.k`,
		`UPDATE "mrg_rows" SET "val" = src.val, "version" = "mrg_rows"."version" + 1 FROM "momo_merge_src" AS "src" WHERE "mrg_rows"."k" = src.k`,
		`INSERT INTO "mrg_rows" ("id", "k", "val") SELECT src.id, src.k, src.val FROM "momo_merge_src" AS "src" WHERE "src"."momo_merge_matched" = 0`,
		drop,
	})
}

// Catches: the SQLite UPDATE ... FROM form on MySQL, which has the multi-table
// form, or an unqualified SET target there; and MariaDB rendering differently
// from MySQL.
func TestMergeEmulationGoldenMySQLAndMariaDB(t *testing.T) {
	const drop = "DROP TEMPORARY TABLE IF EXISTS `momo_merge_src`"
	want := []string{
		drop,
		"CREATE TEMPORARY TABLE `momo_merge_src` AS SELECT `src`.*, CASE WHEN EXISTS (SELECT 1 FROM `mrg_rows` WHERE `mrg_rows`.`k` = src.k) THEN 1 ELSE 0 END AS `momo_merge_matched` FROM (SELECT id, k, val, n FROM mrg_src) AS `src`",
		"SELECT 1 FROM `mrg_rows`, `momo_merge_src` AS `src` WHERE `mrg_rows`.`k` = src.k GROUP BY `mrg_rows`.`id` HAVING COUNT(*) > 1 LIMIT 1",
		"SELECT COUNT(*) FROM `mrg_rows`, `momo_merge_src` AS `src` WHERE `mrg_rows`.`k` = src.k",
		"UPDATE `mrg_rows`, `momo_merge_src` AS `src` SET `mrg_rows`.`val` = src.val, `mrg_rows`.`version` = `mrg_rows`.`version` + 1 WHERE `mrg_rows`.`k` = src.k",
		"INSERT INTO `mrg_rows` (`id`, `k`, `val`) SELECT src.id, src.k, src.val FROM `momo_merge_src` AS `src` WHERE `src`.`momo_merge_matched` = 0",
		drop,
	}
	for _, f := range []dialect.Flavor{dialect.FlavorMySQL, dialect.FlavorMariaDB} {
		assertStatements(t, statementSQL(t, mrgUpsert(flavorDB(f))), want)
	}
}

// Catches: a clause that is not asked for still running: no cardinality check,
// count or UPDATE without WhenMatchedUpdate, and no INSERT without
// WhenNotMatchedInsert.
func TestMergeEmulationRunsOnlyTheClausesGiven(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.SQLite))
	insOnly := statementSQL(t, mrgInsertClause(momo.Merge[mrgRow](db, mrgSource()).On(mrgOn())))
	if len(insOnly) != 4 || !strings.HasPrefix(insOnly[2], "INSERT INTO") {
		t.Errorf("insert only = %v, want drop, create, insert, drop", insOnly)
	}
	updOnly := statementSQL(t, momo.Merge[mrgRow](db, mrgSource()).On(mrgOn()).
		WhenMatchedUpdate(mrgN.SetExpr(momo.TypedSQL[int64]("src.n"))))
	if len(updOnly) != 6 || !strings.HasPrefix(updOnly[4], "UPDATE ") || slices.ContainsFunc(updOnly, func(s string) bool { return strings.HasPrefix(s, "INSERT") }) {
		t.Errorf("update only = %v, want drop, create, check, count, update, drop", updOnly)
	}
}

// Catches: RETURNING being emulated from a pre-update snapshot (wrong rows) or
// reaching the server, and a matched update on a keyless table, where a target
// row matched twice cannot be detected.
func TestMergeEmulationRefusals(t *testing.T) {
	for _, db := range []*momo.DB{momo.New(fakedriver.New(dialect.SQLite)), flavorDB(dialect.FlavorMySQL), flavorDB(dialect.FlavorMariaDB)} {
		_, err := mrgUpsert(db).Returning(mrgID).Rows(t.Context())
		if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "RETURNING") {
			t.Errorf("%s Rows with Returning = %v, want ErrUnsupported naming RETURNING", db.Dialect().Name, err)
		}
		_, err = mrgUpsert(db).Returning(mrgID).Statements()
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s Statements with Returning = %v, want ErrUnsupported", db.Dialect().Name, err)
		}
		_, err = mrgUpsert(db).Returning(mrgID).Exec(t.Context())
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s Exec with Returning = %v, want ErrUnsupported", db.Dialect().Name, err)
		}

		src := momo.RawSubquery("SELECT k, v FROM mrg_nopk_src")
		on := momo.NewColumn[string]("mrg_nopk", "k", 0).EqExpr(momo.TypedSQL[string]("src.k"))
		_, err = momo.Merge[mrgNoPK](db, src).On(on).
			WhenMatchedUpdate(momo.NewColumn[string]("mrg_nopk", "v", 0).SetExpr(momo.TypedSQL[string]("src.v"))).Statements()
		if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "primary key") {
			t.Errorf("%s matched update without a primary key = %v, want ErrUnsupported naming the primary key", db.Dialect().Name, err)
		}
		_, err = momo.Merge[mrgNoPK](db, src).On(on).
			WhenNotMatchedInsert([]momo.ColumnRef{momo.NewColumn[string]("mrg_nopk", "k", 0)}, momo.TypedSQL[string]("src.k")).Statements()
		if err != nil {
			t.Errorf("%s insert only without a primary key = %v, want it accepted", db.Dialect().Name, err)
		}
	}
}

// Catches: the structural checks of MERGE (no On, no clause, a column Update
// refuses) being skipped on the emulated dialects, and the emulation writing
// before it validated.
func TestMergeEmulationValidatesBeforeRunning(t *testing.T) {
	drv := fakedriver.New(dialect.SQLite)
	db := momo.New(drv)
	cases := map[string]momo.MergeQuery[mrgRow]{
		"no on":     momo.Merge[mrgRow](db, mrgSource()).WhenMatchedUpdate(mrgVal.Set("x")),
		"no clause": momo.Merge[mrgRow](db, mrgSource()).On(mrgOn()),
		"no source": momo.Merge[mrgRow](db, nil).On(mrgOn()).WhenMatchedUpdate(mrgVal.Set("x")),
		"pk update": momo.Merge[mrgRow](db, mrgSource()).On(mrgOn()).WhenMatchedUpdate(mrgID.Set(1)),
	}
	for name, q := range cases {
		if _, err := q.Exec(t.Context()); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: Exec = %v, want ErrValidation", name, err)
		}
	}
	if len(drv.Calls()) != 0 {
		t.Fatalf("calls = %v, want none", drv.Calls())
	}
}

// Catches: the emulation running outside one transaction (a partial result
// after a failure), the UPDATE running when no source row matched, and the
// snapshot table being left behind.
func TestMergeEmulationRunsInOneTransaction(t *testing.T) {
	drv := fakedriver.New(dialect.SQLite)
	if _, err := mrgUpsert(momo.New(drv)).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	prefixes := []string{"DROP TABLE IF EXISTS temp.", "CREATE TEMPORARY TABLE ", "SELECT 1 FROM ", "SELECT COUNT(*) FROM ", "INSERT INTO ", "DROP TABLE IF EXISTS temp."}
	calls := drv.Calls()
	if len(calls) != len(prefixes) {
		t.Fatalf("%d statements ran, want %d (the UPDATE is skipped when nothing matched): %+v", len(calls), len(prefixes), calls)
	}
	for i, p := range prefixes {
		if !strings.HasPrefix(calls[i].SQL, p) {
			t.Errorf("statement %d = %s, want prefix %q", i, calls[i].SQL, p)
		}
	}
	if drv.Commits() != 1 {
		t.Fatalf("commits = %d, want the whole sequence in one transaction", drv.Commits())
	}
}
