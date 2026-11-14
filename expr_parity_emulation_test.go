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

func parityDB(d dialect.Name) *momo.DB { return momo.New(fakedriver.New(d)) }

// Catches: NULLS FIRST or LAST on MySQL and MariaDB rendering the native
// clause, rendering the IS NULL key with the wrong direction (NULLs on the
// wrong end), or dropping the original key after the IS NULL key.
func TestNullsOrderingEmulationGolden(t *testing.T) {
	const prefix = "SELECT `documents`.`id`, `documents`.`tenant_id`, `documents`.`title`, `documents`.`published`, `documents`.`deleted_at` FROM `documents` WHERE `documents`.`deleted_at` IS NULL ORDER BY "
	cases := []struct {
		name  string
		order momo.Order
		want  string
	}{
		{"asc nulls last", DocTitle.Asc().NullsLast(), "(`documents`.`title` IS NULL) ASC, `documents`.`title` ASC"},
		{"asc nulls first", DocTitle.Asc().NullsFirst(), "(`documents`.`title` IS NULL) DESC, `documents`.`title` ASC"},
		{"desc nulls first", DocTitle.Desc().NullsFirst(), "(`documents`.`title` IS NULL) DESC, `documents`.`title` DESC"},
		{"desc nulls last", DocTitle.Desc().NullsLast(), "(`documents`.`title` IS NULL) ASC, `documents`.`title` DESC"},
		{"no nulls clause", DocTitle.Desc(), "`documents`.`title` DESC"},
	}
	for _, c := range cases {
		got, err := momo.Select[unitDocument](parityDB(dialect.MySQL)).OrderBy(c.order).SQL()
		if err != nil || got.SQL != prefix+c.want {
			t.Errorf("%s:\n got %q (%v)\nwant %q", c.name, got.SQL, err, prefix+c.want)
		}
	}
	// PostgreSQL and SQLite keep the native clause, one sort key.
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		got, err := momo.Select[unitDocument](parityDB(d)).OrderBy(DocTitle.Asc().NullsFirst()).SQL()
		if err != nil || !strings.HasSuffix(got.SQL, `ORDER BY "documents"."title" ASC NULLS FIRST`) {
			t.Errorf("%s: %q, %v", d, got.SQL, err)
		}
	}
}

// Catches: the duplicated NULLS expression binding its parameter once, or the
// arguments of a predicate sort key landing out of order against the WHERE
// arguments, which sends the wrong value to the wrong placeholder.
func TestNullsOrderingEmulationBindsArgumentsTwiceInOrder(t *testing.T) {
	got, err := momo.Select[unitDocument](parityDB(dialect.MySQL)).
		Where(DocTenantID.Eq(7)).
		OrderBy(momo.OrderByPredicate(DocTitle.Eq("x")).Desc().NullsFirst(), DocID.Asc()).
		SQL()
	if err != nil {
		t.Fatal(err)
	}
	const want = "ORDER BY ((`documents`.`title` = ?) IS NULL) DESC, (`documents`.`title` = ?) DESC, `documents`.`id` ASC"
	if !strings.HasSuffix(got.SQL, want) {
		t.Errorf("got %q, want suffix %q", got.SQL, want)
	}
	if !slices.Equal(got.Args, []any{int64(7), "x", "x"}) {
		t.Errorf("args = %v, want [7 x x]", got.Args)
	}
	if n := strings.Count(got.SQL, "?"); n != len(got.Args) {
		t.Errorf("%d placeholders for %d args", n, len(got.Args))
	}
}

// Catches: keyset pagination building a cursor over a nullable key when the
// NULLS term is emulated (the cursor skips or repeats the NULL rows), or the
// emulated term breaking the cursor over a key that cannot be NULL.
func TestKeysetWithEmulatedNullsOrdering(t *testing.T) {
	my := parityDB(dialect.MySQL)
	_, err := momo.Select[unitDocument](my).OrderBy(DocDeletedAt.Asc().NullsLast()).After(nil, int64(1)).SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("After over a nullable key with NULLS LAST = %v, want ErrUnsupported", err)
	}
	got, err := momo.Select[unitDocument](my).OrderBy(DocTitle.Asc().NullsFirst()).After("m", int64(5)).SQL()
	if err != nil {
		t.Fatalf("After over a non-nullable key with NULLS FIRST: %v", err)
	}
	const order = "ORDER BY (`documents`.`title` IS NULL) DESC, `documents`.`title` ASC, `documents`.`id` ASC"
	if !strings.HasSuffix(got.SQL, order) {
		t.Errorf("got %q, want suffix %q", got.SQL, order)
	}
	// The cursor arguments bind once each, in the WHERE clause, ahead of ORDER BY.
	if !slices.Equal(got.Args, []any{"m", "m", int64(5)}) {
		t.Errorf("args = %v, want [m m 5]", got.Args)
	}
}

// Catches: the emulated NULLS term reaching the outer ORDER BY of a union,
// where an expression is not allowed on every dialect; it must refuse with
// ErrUnsupported instead of sending SQL the server may reject.
func TestUnionOrderByRefusesEmulatedNulls(t *testing.T) {
	my := parityDB(dialect.MySQL)
	docs := momo.TableRef{Name: "documents"}
	m := momo.Select[momo.Row1[string]](my).From(docs).Columns(DocTitle)
	title := momo.ColumnOf[string]("", "title")
	_, err := momo.SelectUnion[momo.Row1[string]](my, true, m, m).OrderBy(title.Asc().NullsLast()).SQL()
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("union ORDER BY with NULLS LAST on MySQL = %v, want ErrUnsupported", err)
	}
	pg := parityDB(dialect.Postgres)
	mp := momo.Select[momo.Row1[string]](pg).From(docs).Columns(DocTitle)
	if _, err := momo.SelectUnion[momo.Row1[string]](pg, true, mp, mp).OrderBy(title.Asc().NullsLast()).SQL(); err != nil {
		t.Errorf("union ORDER BY with NULLS LAST on PostgreSQL: %v", err)
	}
}

// selectOne renders SELECT e FROM documents; the result type is never scanned.
func selectOne(d dialect.Name, e momo.Expr) (momo.Compiled, error) {
	return momo.Select[momo.Row1[int64]](parityDB(d)).From(momo.TableRef{Name: "documents"}).Columns(e).SQL()
}

// Catches: the aggregate FILTER on MySQL rendering a FILTER clause the server
// rejects, filtering the wrong argument, or turning COUNT(*) into a count of
// every row (COUNT(*) with a CASE instead of COUNT(CASE ... THEN 1 END)).
func TestAggregateFilterEmulationGolden(t *testing.T) {
	pub := DocPublished.Eq(true)
	cases := []struct {
		name  string
		e     momo.Expr
		pg    string
		mysql string
	}{
		{"count star", momo.CountAll().Filter(pub),
			`SELECT COUNT(*) FILTER (WHERE "documents"."published" = $1) FROM "documents"`,
			"SELECT COUNT(CASE WHEN `documents`.`published` = ? THEN 1 END) FROM `documents`"},
		{"count of", momo.CountOf(DocTitle).Filter(pub),
			`SELECT COUNT("documents"."title") FILTER (WHERE "documents"."published" = $1) FROM "documents"`,
			"SELECT COUNT(CASE WHEN `documents`.`published` = ? THEN `documents`.`title` END) FROM `documents`"},
		{"count distinct", momo.CountDistinct(DocTitle).Filter(pub),
			`SELECT COUNT(DISTINCT "documents"."title") FILTER (WHERE "documents"."published" = $1) FROM "documents"`,
			"SELECT COUNT(DISTINCT CASE WHEN `documents`.`published` = ? THEN `documents`.`title` END) FROM `documents`"},
		{"sum", momo.Sum(DocTenantID).Filter(pub),
			`SELECT SUM("documents"."tenant_id") FILTER (WHERE "documents"."published" = $1) FROM "documents"`,
			"SELECT SUM(CASE WHEN `documents`.`published` = ? THEN `documents`.`tenant_id` END) FROM `documents`"},
		{"min", momo.Min(DocTitle).Filter(pub),
			`SELECT MIN("documents"."title") FILTER (WHERE "documents"."published" = $1) FROM "documents"`,
			"SELECT MIN(CASE WHEN `documents`.`published` = ? THEN `documents`.`title` END) FROM `documents`"},
		{"window", momo.Sum(DocTenantID).Filter(pub).Over(momo.Over().PartitionBy(DocTitle)),
			`SELECT SUM("documents"."tenant_id") FILTER (WHERE "documents"."published" = $1) OVER (PARTITION BY "documents"."title") FROM "documents"`,
			"SELECT SUM(CASE WHEN `documents`.`published` = ? THEN `documents`.`tenant_id` END) OVER (PARTITION BY `documents`.`title`) FROM `documents`"},
	}
	for _, c := range cases {
		for d, want := range map[dialect.Name]string{dialect.Postgres: c.pg, dialect.MySQL: c.mysql} {
			got, err := selectOne(d, c.e)
			if err != nil || got.SQL != want {
				t.Errorf("%s on %s:\n got %q (%v)\nwant %q", c.name, d, got.SQL, err, want)
			}
			if !slices.Equal(got.Args, []any{true}) {
				t.Errorf("%s on %s: args = %v, want [true]", c.name, d, got.Args)
			}
		}
	}
}

// Catches: an aggregate FILTER on MySQL that is silently dropped (the nil
// predicate case) instead of failing with ErrValidation.
func TestAggregateFilterNilPredicateOnMySQL(t *testing.T) {
	_, err := selectOne(dialect.MySQL, momo.CountAll().Filter(nil))
	if !errors.Is(err, momo.ErrValidation) {
		t.Errorf("nil FILTER predicate on MySQL = %v, want ErrValidation", err)
	}
}
