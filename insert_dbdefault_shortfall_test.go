// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Catches: insert-if-absent (one Model, DoNothing, row already there) turning
// into an error because no RETURNING row came back.
func TestDBDefaultSingleModelDoNothingSkippedIsNotAnError(t *testing.T) {
	db, _ := dbdSetup(dialect.Postgres, nil)
	m := dbdRow{Title: "a", At: dbdStamp}
	res, err := momo.Insert[dbdRow](db).Model(&m).OnConflict(dbdTitle).DoNothing().Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsAffected != 0 || m.ID != "" {
		t.Fatalf("res %+v, model %+v: a skipped row reports 0 rows and leaves the model alone", res, m)
	}
}

// Catches: a trigger or rule that drops a row turning a valid insert into an
// error that blames a conflict that does not exist.
func TestDBDefaultShortfallWithoutConflictIsTolerated(t *testing.T) {
	db, _ := dbdSetup(dialect.Postgres, [][]any{{"g1"}})
	ms := []dbdRow{{Title: "a", At: dbdStamp}, {Title: "b", At: dbdStamp}}
	res, err := momo.Insert[dbdRow](db).Models(ms).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsAffected != 1 || ms[0].ID != "g1" || ms[1].ID != "" {
		t.Fatalf("res %+v, models %+v: the returned rows are written back by position", res, ms)
	}
}

type dbdPinned struct{ *fakedriver.Driver }

func (dbdPinned) Release() error { return nil }

// Catches: an error for a row that a pinned handle already committed (it
// cannot roll back).
func TestDBDefaultDoNothingShortfallOnPinnedHandleIsPartial(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{"g1"}})
	db := momo.New(drv).OnConn(dbdPinned{drv})
	ms := []dbdRow{{Title: "a", At: dbdStamp}, {Title: "b", At: dbdStamp}}
	if _, err := momo.Insert[dbdRow](db).Models(ms).OnConflict(dbdTitle).DoNothing().Exec(context.Background()); err != nil {
		t.Fatalf("pinned handle: %v", err)
	}
	if ms[0].ID != "g1" {
		t.Fatalf("partial write-back missing: %+v", ms)
	}
}

// Catches: the rollback reaching the caller's transaction instead of a
// savepoint inside it.
func TestDBDefaultDoNothingShortfallRollsBackToSavepointInTx(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, [][]any{{"g1"}})
	ms := []dbdRow{{Title: "a", At: dbdStamp}, {Title: "b", At: dbdStamp}}
	var insertErr error
	err := db.Tx(context.Background(), momo.TxOptions{}, func(tx *momo.Tx) error {
		_, insertErr = momo.Insert[dbdRow](tx).Models(ms).OnConflict(dbdTitle).DoNothing().Exec(context.Background())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(insertErr, momo.ErrUnsupported) || ms[0].ID != "" {
		t.Fatalf("err %v, models %+v", insertErr, ms)
	}
	rolledBack := false
	for _, c := range drv.Calls() {
		rolledBack = rolledBack || strings.HasPrefix(c.SQL, "ROLLBACK TO SAVEPOINT")
	}
	if !rolledBack || drv.Rollbacks() != 0 || drv.Commits() != 1 {
		t.Fatalf("calls %v, rollbacks %d, commits %d: want a savepoint rollback inside a committed caller transaction", drv.Calls(), drv.Rollbacks(), drv.Commits())
	}
}

// Catches: placeholder numbering or the WITH clause breaking when a dbdefault
// column leaves the column list.
func TestDBDefaultWithClausePlaceholders(t *testing.T) {
	db, _ := dbdSetup(dialect.Postgres, nil)
	inner := momo.Select[dbdRow](db).Where(dbdTitle.Eq("x"))
	m := dbdRow{Title: "a", At: dbdStamp}
	c, err := momo.Insert[dbdRow](db).With("t", inner).Model(&m).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `INSERT INTO "dbd_rows" ("title", "at") VALUES ($2, $3)`
	if !strings.HasPrefix(c.SQL, `WITH "t" AS (`) || !strings.Contains(c.SQL, want) ||
		strings.Contains(c.SQL, "$4") || len(c.Args) != 3 || c.Args[0] != "x" || c.Args[1] != "a" {
		t.Fatalf("SQL %s args %v", c.SQL, c.Args)
	}
}

// dbdGen combines an id generator with a dbdefault column, to reach the
// reconcile path.
type dbdGen struct {
	ID   string
	Name string
	Slug string
}

var dbdGenMeta = momo.Meta[dbdGen]{
	Table: "dbd_gen",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "name", Flags: momo.FlagUnique},
		{Name: "slug", Flags: momo.FlagHasDefault | momo.FlagDBDefault},
	},
	Ptr:      func(m *dbdGen, i int) any { return [...]any{&m.ID, &m.Name, &m.Slug}[i] },
	Value:    func(m *dbdGen, i int) any { return [...]any{m.ID, m.Name, m.Slug}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK:     []int{0},
	IDGens: []momo.IDGen{{Column: 0, Name: "cuid"}},
}

func (*dbdGen) MomoMeta() *momo.Meta[dbdGen] { return &dbdGenMeta }

// Catches: the id-generator reconcile path silently skipping a DoNothing-skipped
// row that has dbdefault cells, while the plain read-back path errors.
func TestDBDefaultReconcileShortfallMatchesReadBack(t *testing.T) {
	db, _ := dbdSetup(dialect.Postgres, [][]any{{"stored-id", "a", "slug1"}})
	name := momo.NewColumn[string]("dbd_gen", "name", momo.FlagUnique)
	ms := []dbdGen{{Name: "a"}, {Name: "b"}}
	_, err := momo.Insert[dbdGen](db).Models(ms).OnConflict(name).DoNothing().Exec(context.Background())
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "returned 1 rows for 2 models") {
		t.Fatalf("err = %v, want the same count-mismatch ErrUnsupported as the read-back path", err)
	}
}

// Catches: a zero dbdefault column as conflict target ending in "matched no
// model" after the write.
func TestDBDefaultConflictTargetIsRejected(t *testing.T) {
	db, drv := dbdSetup(dialect.Postgres, nil)
	slug := momo.NewColumn[string]("dbd_gen", "slug", 0)
	ms := []dbdGen{{Name: "a"}}
	_, err := momo.Insert[dbdGen](db).Models(ms).OnConflict(slug).DoNothing().Exec(context.Background())
	if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), `"slug"`) {
		t.Fatalf("err = %v, want ErrValidation naming the column", err)
	}
	for _, c := range drv.Calls() {
		if strings.HasPrefix(c.SQL, "INSERT") {
			t.Fatalf("a rejected insert must not run: %s", c.SQL)
		}
	}
}
