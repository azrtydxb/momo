// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// bumpFlavor is one rendering family: MariaDB is its own case because it has no
// row alias and renders VALUES(col) where MySQL renders the alias.
type bumpFlavor struct {
	name string
	d    *dialect.Dialect
}

func bumpFlavors() []bumpFlavor {
	return []bumpFlavor{
		{"postgres", dialect.For(dialect.Postgres)},
		{"sqlite", dialect.For(dialect.SQLite)},
		{"mysql", dialect.MySQLFlavor(dialect.FlavorMySQL)},
		{"mariadb", dialect.MySQLFlavor(dialect.FlavorMariaDB)},
	}
}

func (f bumpFlavor) db(now time.Time) *momo.DB {
	return momo.New(fakedriver.NewDialect(f.d), clockAt(now))
}

func (f bumpFlavor) quote() string {
	if f.d.OnDuplicateKey {
		return "`"
	}
	return `"`
}

// marker is the text that opens the update list of the conflict clause.
func (f bumpFlavor) marker() string {
	if f.d.OnDuplicateKey {
		return "ON DUPLICATE KEY UPDATE "
	}
	return "DO UPDATE SET "
}

// last is the placeholder of the final bound argument.
func (f bumpFlavor) last(nargs int) string {
	if f.d.Name == dialect.Postgres {
		return "$" + strconv.Itoa(nargs)
	}
	return "?"
}

// insertColumns returns the INSERT text before its VALUES or SELECT part.
func insertColumns(sql string) string {
	cut := len(sql)
	for _, m := range []string{" VALUES", " SELECT"} {
		if i := strings.Index(sql, m); i >= 0 && i < cut {
			cut = i
		}
	}
	return sql[:cut]
}

// requireClockBump checks that the conflict branch ends with updated_at bound
// to the statement clock, and whether the INSERT column list carries it.
func requireClockBump(t *testing.T, label string, f bumpFlavor, c momo.Compiled, now time.Time, inInsert bool) {
	t.Helper()
	q := f.quote()
	_, tail, ok := strings.Cut(c.SQL, f.marker())
	want := q + "updated_at" + q + " = " + f.last(len(c.Args))
	if !ok || !strings.HasSuffix(tail, want) {
		t.Errorf("%s: update list = %q, want it to end with %s\nSQL: %s", label, tail, want, c.SQL)
		return
	}
	got, isTime := c.Args[len(c.Args)-1].(time.Time)
	if !isTime || !got.Equal(now) {
		t.Errorf("%s: bumped value = %#v, want the statement clock %v", label, c.Args[len(c.Args)-1], now)
	}
	if strings.Contains(tail, q+"created_at"+q) {
		t.Errorf("%s: created_at must never be overwritten: %s", label, c.SQL)
	}
	if has := strings.Contains(insertColumns(c.SQL), q+"updated_at"+q); has != inInsert {
		t.Errorf("%s: INSERT column list carries updated_at = %v, want %v\nSQL: %s", label, has, inInsert, c.SQL)
	}
}

// Catches: the conflict branch copying EXCLUDED.updated_at, which is the
// struct's own value, so a loaded row with an old UpdatedAt is written back
// unchanged and never bumped; also the bump missing from a source (Model,
// Values, ValuesRows, FromSelect) or a dialect (MariaDB included), or the
// INSERT column list losing updated_at.
func TestDoUpdateBumpsFromClockNotFromTheStruct(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	stale := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	doc := suite.Document{ID: 1, TenantID: 1, Title: "t", UpdatedAt: stale}
	rowA := []momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("a")}
	rowB := []momo.Assignment{suite.DocTenantID.Set(2), suite.DocTitle.Set("b")}
	src := momo.SelectExprs(momo.Value(int64(1)), momo.Value("t"))
	for _, f := range bumpFlavors() {
		db := f.db(now)
		for _, kind := range []string{"upsert", "insert"} {
			base := func() momo.InsertQuery[suite.Document] {
				if kind == "upsert" {
					return momo.Upsert[suite.Document](db)
				}
				return momo.Insert[suite.Document](db)
			}
			set := suite.DocTitle.SetExcluded()
			cases := []struct {
				name     string
				q        momo.InsertQuery[suite.Document]
				inInsert bool
			}{
				{"model", base().Model(&doc).OnConflict(suite.DocID).DoUpdate(set), true},
				{"values", base().Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("t")).OnConflict(suite.DocID).DoUpdate(set), true},
				{"rows", base().ValuesRows(rowA, rowB).OnConflict(suite.DocID).DoUpdate(set), true},
				{"fromselect", base().FromSelect(src, suite.DocTenantID, suite.DocTitle).OnConflict(suite.DocID).DoUpdate(set), false},
			}
			for _, c := range cases {
				got, err := c.q.SQL()
				if err != nil {
					t.Errorf("%s %s %s: %v", f.name, kind, c.name, err)
					continue
				}
				requireClockBump(t, f.name+" "+kind+" "+c.name, f, got, now, c.inInsert)
			}
		}
	}
}

// Catches: the default Upsert (no DoUpdate at all) keeping the EXCLUDED copy of
// a stale struct timestamp, or ending its SET list without the clock bump.
func TestUpsertDefaultSetBumpsFromClock(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	stale := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	doc := suite.Document{ID: 1, TenantID: 1, Title: "t", UpdatedAt: stale}
	for _, f := range bumpFlavors() {
		c, err := momo.Upsert[suite.Document](f.db(now)).Model(&doc).SQL()
		if err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}
		requireClockBump(t, f.name, f, c, now, true)
	}
}

// Catches: an Upsert whose default update set is empty failing (or silently
// not bumping) instead of rendering a touch-only SET updated_at = <clock>.
func TestUpsertWithEmptyDefaultSetTouchesUpdatedAt(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	doc := suite.Document{ID: 1}
	for _, f := range bumpFlavors() {
		c, err := momo.Upsert[suite.Document](f.db(now)).Model(&doc).
			Omit(suite.DocTenantID, suite.DocTitle, suite.DocBody, suite.DocSecret).SQL()
		if err != nil {
			t.Errorf("%s: %v", f.name, err)
			continue
		}
		requireClockBump(t, f.name, f, c, now, true)
		_, tail, _ := strings.Cut(c.SQL, f.marker())
		q := f.quote()
		if want := q + "updated_at" + q + " = " + f.last(len(c.Args)); tail != want {
			t.Errorf("%s: update list = %q, want only %q", f.name, tail, want)
		}
	}
}

// Catches: Omit(updated_at) silently skipping the bump on the conflict branch;
// Omit affects only the insert branch.
func TestUpsertOmitUpdatedAtStillBumpsConflictBranch(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	doc := suite.Document{ID: 1, TenantID: 1, Title: "t"}
	for _, f := range bumpFlavors() {
		db := f.db(now)
		for kind, q := range map[string]momo.InsertQuery[suite.Document]{
			"upsert": momo.Upsert[suite.Document](db).Model(&doc).Omit(suite.DocUpdatedAt).OnConflict(suite.DocID).DoUpdate(suite.DocTitle.SetExcluded()),
			"insert": momo.Insert[suite.Document](db).Model(&doc).Omit(suite.DocUpdatedAt).OnConflict(suite.DocID).DoUpdate(suite.DocTitle.SetExcluded()),
		} {
			c, err := q.SQL()
			if err != nil {
				t.Errorf("%s %s: %v", f.name, kind, err)
				continue
			}
			requireClockBump(t, f.name+" "+kind, f, c, now, false)
		}
	}
}

// Catches: a plain Insert with DoUpdate() and no assignment silently becoming
// valid, which would hide a missing conflict action behind the updated_at bump.
func TestPlainInsertEmptyDoUpdateStaysAnError(t *testing.T) {
	doc := suite.Document{ID: 1, TenantID: 1, Title: "t"}
	for _, f := range bumpFlavors() {
		_, err := momo.Insert[suite.Document](f.db(time.Now())).Model(&doc).OnConflict(suite.DocID).DoUpdate().SQL()
		if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "at least one assignment") {
			t.Errorf("%s: err = %v, want ErrValidation (needs at least one assignment)", f.name, err)
		}
	}
}

// Catches: a chunked bulk upsert or insert binding the bump in only some
// chunks, or reading a different clock per chunk, or the chunk size probe not
// counting the extra fixed parameter (a chunk over the bind limit).
func TestChunkedDoUpdateBumpsEveryChunk(t *testing.T) {
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	const limit = 50
	ms := make([]suite.Document, 25)
	for i := range ms {
		ms[i] = suite.Document{ID: int64(i + 1), TenantID: 1, Title: "b", UpdatedAt: time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)}
	}
	builds := map[string]func(db *momo.DB) momo.InsertQuery[suite.Document]{
		"upsert": func(db *momo.DB) momo.InsertQuery[suite.Document] {
			return momo.Upsert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms)
		},
		"insert": func(db *momo.DB) momo.InsertQuery[suite.Document] {
			return momo.Insert[suite.Document](db).Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt).Models(ms).
				OnConflict(suite.DocID).DoUpdate(suite.DocTitle.SetExcluded())
		},
	}
	for name, build := range builds {
		drv := newSmallLimit(limit)
		if _, err := build(momo.New(drv, clockAt(now))).Exec(t.Context()); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		n := 0
		for _, c := range drv.Calls() {
			if !strings.HasPrefix(c.SQL, "INSERT") {
				continue
			}
			n++
			want := `"updated_at" = $` + strconv.Itoa(len(c.Args))
			if !strings.HasSuffix(c.SQL, want) {
				t.Errorf("%s chunk %d: SQL does not end with %s: %s", name, n, want, c.SQL)
			}
			if got, ok := c.Args[len(c.Args)-1].(time.Time); !ok || !got.Equal(now) {
				t.Errorf("%s chunk %d: bumped value = %#v, want %v", name, n, c.Args[len(c.Args)-1], now)
			}
			if len(c.Args) > limit {
				t.Errorf("%s chunk %d binds %d, over the limit %d", name, n, len(c.Args), limit)
			}
		}
		if n < 3 {
			t.Errorf("%s: %d INSERT statements, want the rows split into at least 3", name, n)
		}
	}
}

// touchRO has an updated_at column that UPDATE cannot write.
type touchRO struct {
	ID int64
	At time.Time
}

var touchROMeta = momo.Meta[touchRO]{
	Table: "touch_ro",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "at", Flags: momo.FlagReadOnly | momo.FlagUpdatedAt},
	},
	Ptr:      func(m *touchRO, i int) any { return [...]any{&m.ID, &m.At}[i] },
	Value:    func(m *touchRO, i int) any { return [...]any{m.ID, m.At}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: 1,
	PK: []int{0},
}

func (*touchRO) MomoMeta() *momo.Meta[touchRO] { return &touchROMeta }

// Catches: an empty Update on a model whose updated_at cannot be written
// failing with the bare "no columns to update", leaving the user to guess why
// the touch is refused.
func TestEmptyUpdateExplainsAnUnassignableUpdatedAt(t *testing.T) {
	id := momo.NewColumn[int64]("touch_ro", "id", momo.FlagPK)
	db := momo.New(fakedriver.New(dialect.Postgres))
	_, err := momo.Update[touchRO](db).Where(id.Eq(1)).SQL()
	if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "cannot be written by UPDATE") {
		t.Fatalf("err = %v, want ErrValidation that says updated_at cannot be written by UPDATE", err)
	}
}

// touchPtr has a database-default pointer and an updated_at column.
type touchPtr struct {
	ID     int64
	Active *bool
	At     time.Time
}

var touchPtrMeta = momo.Meta[touchPtr]{
	Table: "touch_ptr",
	Columns: []momo.ColumnMeta{
		{Name: "id", Flags: momo.FlagPK},
		{Name: "active", Flags: momo.FlagHasDefault | momo.FlagDBDefault},
		{Name: "at", Flags: momo.FlagUpdatedAt},
	},
	Ptr:      func(m *touchPtr, i int) any { return [...]any{&m.ID, &m.Active, &m.At}[i] },
	Value:    func(m *touchPtr, i int) any { return [...]any{m.ID, m.Active, m.At}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: 2,
	PK: []int{0},
}

func (*touchPtr) MomoMeta() *momo.Meta[touchPtr] { return &touchPtrMeta }

// Catches: Update.Model on a model with updated_at whose only candidate
// columns are nil dbdefault pointers failing again instead of being a touch
// (a documented behaviour change), or writing NULL into the pointer column.
func TestModelUpdateWithAllNilDBDefaultPointersIsATouch(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	m := touchPtr{ID: 1}
	c, err := momo.Update[touchPtr](db).Model(&m).SQL()
	if err != nil {
		t.Fatalf("err = %v, want a touch", err)
	}
	head, _, _ := strings.Cut(c.SQL, " WHERE ")
	if head != `UPDATE "touch_ptr" SET "at" = $1` {
		t.Fatalf("SQL head = %s, want a touch of the updated_at column only", head)
	}
}
