// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

type plainAudit struct {
	CreatedAt time.Time
	UpdatedBy string
}

// plainRow has no generated metadata: snake_case names, a tag, an embedded
// struct, pointer fields and a skipped field.
type plainRow struct {
	plainAudit
	ID       int64
	UserID   int64
	Title    string
	Display  string `momo:"column:display_name"`
	Note     *string
	Score    *int64
	Skipped  string `momo:"-"`
	internal string
}

var plainNames = []string{"updated_by", "id", "user_id", "title", "display_name", "note", "score", "created_at"}

func plainDB(rows [][]any, names []string) *momo.DB {
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows(rows)
	return momo.New(namedDriver{drv, names})
}

// Catches: a plain struct refused, scanned positionally instead of by name, a
// tag or snake_case name ignored, an embedded struct not flattened, or NULL
// into a pointer field not leaving it nil.
func TestQueryPlainStructScansByName(t *testing.T) {
	at := time.Date(2026, 11, 10, 1, 2, 3, 0, time.UTC)
	note := "n"
	db := plainDB([][]any{
		{"ann", int64(1), int64(9), "t1", "Ann", note, int64(5), at},
		{"bob", int64(2), int64(8), "t2", "Bob", nil, nil, at},
	}, plainNames)
	got, err := momo.Query[plainRow](t.Context(), db, "SELECT * FROM x")
	if err != nil || len(got) != 2 {
		t.Fatalf("Query = %+v, %v", got, err)
	}
	a, b := got[0], got[1]
	if a.ID != 1 || a.UserID != 9 || a.Title != "t1" || a.Display != "Ann" || a.UpdatedBy != "ann" || !a.CreatedAt.Equal(at) {
		t.Errorf("first row = %+v", a)
	}
	if a.Note == nil || *a.Note != "n" || a.Score == nil || *a.Score != 5 {
		t.Errorf("pointer fields with a value = %+v", a)
	}
	if b.Note != nil || b.Score != nil || b.Skipped != "" || b.internal != "" {
		t.Errorf("NULL pointer fields = %+v", b)
	}
}

// Catches: QueryRows not sharing the plain-struct path.
func TestQueryRowsPlainStruct(t *testing.T) {
	at := time.Now()
	db := plainDB([][]any{{"ann", int64(1), int64(9), "t1", "Ann", nil, nil, at}}, plainNames)
	n := 0
	for r, err := range momo.QueryRows[plainRow](t.Context(), db, "SELECT * FROM x") {
		if err != nil || r.Title != "t1" {
			t.Fatalf("row = %+v, %v", r, err)
		}
		n++
	}
	if n != 1 {
		t.Fatalf("rows = %d", n)
	}
}

// Catches: the documented rule (an unmatched column or field is an error that
// names it) silently dropping a column or leaving a field unset.
func TestQueryPlainStructUnmatchedIsNamedError(t *testing.T) {
	cases := map[string]struct {
		names []string
		want  string
	}{
		"extra column":  {append(append([]string{}, plainNames...), "surprise"), "surprise"},
		"missing field": {plainNames[:len(plainNames)-1], "created_at"},
		"duplicate":     {append(append([]string{}, plainNames...), "title"), "title"},
	}
	for name, c := range cases {
		_, err := momo.Query[plainRow](t.Context(), plainDB(nil, c.names), "SELECT 1")
		if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want ErrValidation naming %q", name, err, c.want)
		}
	}
}

// Catches: names compared case-sensitively against a database that folds them.
func TestQueryPlainStructCaseFoldsColumnNames(t *testing.T) {
	type r struct {
		ID    int64
		Title string
	}
	db := plainDB([][]any{{"x", int64(4)}}, []string{"TITLE", "Id"})
	got, err := momo.Query[r](t.Context(), db, "SELECT 1")
	if err != nil || len(got) != 1 || got[0].ID != 4 || got[0].Title != "x" {
		t.Fatalf("Query = %+v, %v", got, err)
	}
}

// Catches: a non-struct or field-less type reaching the driver, and two
// fields on one column silently shadowing each other.
func TestQueryPlainStructRejectsBadTypes(t *testing.T) {
	type dupe struct {
		A string `momo:"column:x"`
		B string `momo:"column:x"`
	}
	type none struct{ hidden int }
	ctx := t.Context()
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv)
	if _, err := momo.Query[int64](ctx, db, "SELECT 1"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("int64: err = %v", err)
	}
	if _, err := momo.Query[dupe](ctx, db, "SELECT 1"); !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), `"x"`) {
		t.Errorf("duplicate column: err = %v", err)
	}
	if _, err := momo.Query[none](ctx, db, "SELECT 1"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("no exported fields: err = %v", err)
	}
	if n := len(drv.Calls()); n != 0 {
		t.Errorf("a bad type must fail before the statement runs, got %d calls", n)
	}
}

type plainOuter struct {
	plainInner
	ID int64
}
type plainInner struct {
	ID    string
	Extra string
}

// Catches: an outer field not shadowing the same-named field of an embedded
// struct, as Go itself resolves it.
func TestQueryPlainStructOuterFieldShadowsEmbedded(t *testing.T) {
	db := plainDB([][]any{{int64(3), "e"}}, []string{"id", "extra"})
	got, err := momo.Query[plainOuter](t.Context(), db, "SELECT 1")
	if err != nil || len(got) != 1 || got[0].ID != 3 || got[0].Extra != "e" || got[0].plainInner.ID != "" {
		t.Fatalf("Query = %+v, %v", got, err)
	}
}

// Catches: a driver that cannot name columns breaking the plain-struct path
// (it falls back to field order).
func TestQueryPlainStructWithoutColumnNames(t *testing.T) {
	type r struct {
		ID    int64
		Title string
	}
	drv := fakedriver.New(dialect.Postgres)
	drv.SetRows([][]any{{int64(5), "x"}})
	got, err := momo.Query[r](t.Context(), momo.New(drv), "SELECT 1")
	if err != nil || len(got) != 1 || got[0].ID != 5 || got[0].Title != "x" {
		t.Fatalf("Query = %+v, %v", got, err)
	}
}
