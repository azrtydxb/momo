// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

// dvThing is a model whose only column is an identity key, so a Values insert
// with no assignments writes no column at all.
type dvThing struct{ ID int64 }

var dvThingMeta = momo.Meta[dvThing]{
	Table:    "dv_things",
	Columns:  []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}},
	Ptr:      func(m *dvThing, _ int) any { return &m.ID },
	Value:    func(m *dvThing, _ int) any { return m.ID },
	Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*dvThing) MomoMeta() *momo.Meta[dvThing] { return &dvThingMeta }

func stateDB(d dialect.Name) *momo.DB { return momo.New(fakedriver.New(d)) }

func TestInsertSetDefaultSingleRow(t *testing.T) {
	q := func(d dialect.Name) momo.InsertQuery[suite.StatusDocument] {
		return momo.Insert[suite.StatusDocument](stateDB(d)).
			Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("a"), suite.DocStatus.SetDefault())
	}
	c, err := q(dialect.Postgres).SQL()
	want := `INSERT INTO "documents" ("tenant_id", "title", "status") VALUES ($1, $2, DEFAULT)`
	if err != nil || c.SQL != want || len(c.Args) != 2 {
		t.Fatalf("PG = %q %v, want %q", c.SQL, err, want)
	}
	c, err = q(dialect.SQLite).SQL()
	want = `INSERT INTO "documents" ("tenant_id", "title") VALUES (?, ?)`
	if err != nil || c.SQL != want || len(c.Args) != 2 {
		t.Fatalf("SQLite = %q %v, want %q", c.SQL, err, want)
	}
	c, err = q(dialect.MySQL).SQL()
	want = "INSERT INTO `documents` (`tenant_id`, `title`, `status`) VALUES (?, ?, DEFAULT)"
	if err != nil || c.SQL != want {
		t.Fatalf("MySQL = %q %v, want %q", c.SQL, err, want)
	}
}

func TestInsertValuesRowsRendering(t *testing.T) {
	uniform := func(d dialect.Name) (momo.Compiled, error) {
		return momo.Insert[suite.StatusDocument](stateDB(d)).ValuesRows(
			[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("m1"), suite.DocStatus.SetDefault()},
			[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("m2"), suite.DocStatus.SetDefault()},
		).SQL()
	}
	c, err := uniform(dialect.Postgres)
	want := `INSERT INTO "documents" ("tenant_id", "title", "status") VALUES ($1, $2, DEFAULT), ($3, $4, DEFAULT)`
	if err != nil || c.SQL != want || len(c.Args) != 4 {
		t.Fatalf("PG uniform = %q %v, want %q", c.SQL, err, want)
	}
	c, err = uniform(dialect.SQLite)
	want = `INSERT INTO "documents" ("tenant_id", "title") VALUES (?, ?), (?, ?)`
	if err != nil || c.SQL != want {
		t.Fatalf("SQLite uniform = %q %v, want %q", c.SQL, err, want)
	}

	mixed := func(d dialect.Name) (momo.Compiled, error) {
		return momo.Insert[suite.StatusDocument](stateDB(d)).ValuesRows(
			[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("n1"), suite.DocStatus.SetDefault()},
			[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("n2"), suite.DocStatus.Set("y")},
		).SQL()
	}
	c, err = mixed(dialect.Postgres)
	want = `INSERT INTO "documents" ("tenant_id", "title", "status") VALUES ($1, $2, DEFAULT), ($3, $4, $5)`
	if err != nil || c.SQL != want {
		t.Fatalf("PG mixed = %q %v, want %q", c.SQL, err, want)
	}
	if _, err = mixed(dialect.SQLite); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("SQLite mixed err = %v, want ErrUnsupported", err)
	}

	// A column omitted in one row and given in another: DEFAULT on PG, unsupported on SQLite.
	partial := func(d dialect.Name) (momo.Compiled, error) {
		return momo.Insert[suite.StatusDocument](stateDB(d)).ValuesRows(
			[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("p1")},
			[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("p2"), suite.DocStatus.Set("y")},
		).SQL()
	}
	c, err = partial(dialect.Postgres)
	want = `INSERT INTO "documents" ("tenant_id", "title", "status") VALUES ($1, $2, DEFAULT), ($3, $4, $5)`
	if err != nil || c.SQL != want {
		t.Fatalf("PG partial = %q %v, want %q", c.SQL, err, want)
	}
	if _, err = partial(dialect.SQLite); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("SQLite partial err = %v, want ErrUnsupported", err)
	}
}

func TestInsertValuesRowsShareOneClockRead(t *testing.T) {
	c, err := momo.Insert[suite.Document](stateDB(dialect.Postgres)).ValuesRows(
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("a")},
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("b")},
	).SQL()
	want := `INSERT INTO "documents" ("tenant_id", "title", "created_at", "updated_at") VALUES ($1, $2, $3, $4), ($5, $6, $7, $8)`
	if err != nil || c.SQL != want || len(c.Args) != 8 {
		t.Fatalf("SQL = %q %v, want %q", c.SQL, err, want)
	}
	if c.Args[2] != c.Args[6] || c.Args[3] != c.Args[7] {
		t.Fatalf("automatic timestamps differ across rows of one statement: %v", c.Args)
	}
}

func TestInsertValuesRowsValidation(t *testing.T) {
	db := stateDB(dialect.Postgres)
	cases := map[string]momo.InsertQuery[suite.StatusDocument]{
		"no rows":           momo.Insert[suite.StatusDocument](db).ValuesRows(),
		"duplicate in row":  momo.Insert[suite.StatusDocument](db).ValuesRows([]momo.Assignment{suite.DocTitle.Set("a"), suite.DocTitle.Set("b")}),
		"identity in row":   momo.Insert[suite.StatusDocument](db).ValuesRows([]momo.Assignment{suite.DocID.Set(1)}),
		"values and rows":   momo.Insert[suite.StatusDocument](db).Values(suite.DocTitle.Set("a")).ValuesRows([]momo.Assignment{suite.DocTitle.Set("b")}),
		"model and rows":    momo.Insert[suite.StatusDocument](db).Model(&suite.StatusDocument{}).ValuesRows([]momo.Assignment{suite.DocTitle.Set("b")}),
		"omit with values":  momo.Insert[suite.StatusDocument](db).Values(suite.DocTitle.Set("a")).Omit(suite.DocBody),
		"omit with rows":    momo.Insert[suite.StatusDocument](db).ValuesRows([]momo.Assignment{suite.DocTitle.Set("a")}).Omit(suite.DocBody),
		"excluded in a row": momo.Insert[suite.StatusDocument](db).ValuesRows([]momo.Assignment{suite.DocTitle.SetExcluded()}),
	}
	for name, q := range cases {
		if _, err := q.SQL(); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v, want ErrValidation", name, err)
		}
	}
}

func TestInsertValuesRowsParamLimit(t *testing.T) {
	// NULL literals bind no parameter, so they do not count toward the limit;
	// bound values do.
	d := dialect.SQLite
	limit := stateDB(d).Dialect().MaxParams
	rows := func(n int) [][]momo.Assignment {
		out := make([][]momo.Assignment, n)
		for i := range out {
			out[i] = []momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("t"), suite.DocStatus.SetNull()}
		}
		return out
	}
	// Each row binds tenant_id and title: 2 parameters.
	fits := limit / 2
	if _, err := momo.Insert[suite.StatusDocument](stateDB(d)).ValuesRows(rows(fits)...).SQL(); err != nil {
		t.Fatalf("rows at the limit: %v", err)
	}
	if _, err := momo.Insert[suite.StatusDocument](stateDB(d)).ValuesRows(rows(fits + 1)...).SQL(); !errors.Is(err, momo.ErrLimitExceeded) {
		t.Fatalf("rows over the limit: err = %v, want ErrLimitExceeded", err)
	}
}

func TestInsertDefaultValuesWhenNoColumns(t *testing.T) {
	for d, want := range map[dialect.Name]string{
		dialect.Postgres: `INSERT INTO "dv_things" DEFAULT VALUES`,
		dialect.SQLite:   `INSERT INTO "dv_things" DEFAULT VALUES`,
		dialect.MySQL:    "INSERT INTO `dv_things` () VALUES ()",
	} {
		c, err := momo.Insert[dvThing](stateDB(d)).Values().SQL()
		if err != nil || c.SQL != want || len(c.Args) != 0 {
			t.Errorf("%s: %q %v, want %q", d, c.SQL, err, want)
		}
	}
	c, err := momo.Insert[dvThing](stateDB(dialect.Postgres)).Values().ReturningAll().SQL()
	if err != nil || !strings.HasSuffix(c.SQL, `DEFAULT VALUES RETURNING "id"`) {
		t.Errorf("returning: %q %v", c.SQL, err)
	}
	// SQLite omits a DEFAULT column; with nothing else written that is DEFAULT VALUES.
	c, err = momo.Insert[suite.StatusDocument](stateDB(dialect.SQLite)).Values(suite.DocStatus.SetDefault()).SQL()
	if err != nil || c.SQL != `INSERT INTO "documents" DEFAULT VALUES` {
		t.Errorf("sqlite all-default: %q %v", c.SQL, err)
	}
	// Two rows of nothing cannot be one DEFAULT VALUES statement.
	if _, err := momo.Insert[dvThing](stateDB(dialect.Postgres)).ValuesRows(nil, nil).SQL(); err == nil {
		t.Error("two empty rows: want an error")
	}
}

// TestUpsertValuesRowsDefaultSetUnion pins that the default DO UPDATE list of an
// upsert over ValuesRows is the union of the columns written in any row: status
// is DEFAULT in the first row and a value in the second, so it must be updated.
func TestUpsertValuesRowsDefaultSetUnion(t *testing.T) {
	c, err := momo.Upsert[suite.StatusDocument](stateDB(dialect.Postgres)).
		ValuesRows(
			[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("u1"), suite.DocStatus.SetDefault()},
			[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("u2"), suite.DocStatus.Set("z")},
		).
		OnConflict(suite.DocTitle).DoUpdate().
		SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `"status" = EXCLUDED."status"`) {
		t.Fatalf("upsert over ValuesRows does not update status: %s", c.SQL)
	}
}
