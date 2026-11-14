// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// dBT is a nullable body and a title.
type dBT struct {
	B *string
	T string
}

var dBTMeta = anMeta([]string{"b", "t"}, func(m *dBT) []any { return []any{&m.B, &m.T} })

func (*dBT) MomoMeta() *momo.Meta[dBT] { return dBTMeta }

// seedDistinctOn inserts seven documents (ids 1 to 7 in this order):
//
//	tenant 1: a (body NULL, v10), b (NULL, v20), c (x, v30)
//	tenant 2: d (x, v5), e (y, v7)
//	tenant 3: f (y, v7), g (NULL, v20)
//
// Groups tie on the leading key (several rows per key), on the first
// tie-breaker (e and f, b and g share a version) and have a NULL key.
func seedDistinctOn(t *testing.T, db *momo.DB) {
	t.Helper()
	suite.CreateDocuments(t, db, suite.CapsOf(db))
	rows := []struct {
		tenant  int64
		title   string
		body    any
		version int64
	}{
		{1, "a", nil, 10}, {1, "b", nil, 20}, {1, "c", "x", 30},
		{2, "d", "x", 5}, {2, "e", "y", 7},
		{3, "f", "y", 7}, {3, "g", nil, 20},
	}
	for _, r := range rows {
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title, body, version) VALUES (?, ?, ?, ?)", r.tenant, r.title, r.body, r.version); err != nil {
			t.Fatal(err)
		}
	}
}

func strp(s string) *string { return &s }

// runDistinctOnWindow asserts the rows PostgreSQL's DISTINCT ON returns for
// the seed data: the expected values below are what a native DISTINCT ON
// produces, and every dialect must equal them.
func runDistinctOnWindow(t *testing.T, db *momo.DB) {
	t.Helper()
	seedDistinctOn(t, db)
	ctx := context.Background()
	tenantOrder := []momo.Orderable{suite.DocTenantID.Asc(), suite.DocVersion.Desc(), suite.DocID.Asc()}
	latest := func() momo.SelectQuery[anKT] {
		return momo.Select[anKT](db).From(anDocs).Columns(suite.DocTenantID, suite.DocTitle).
			DistinctOn(suite.DocTenantID).OrderBy(tenantOrder...)
	}

	t.Run("hidden order columns", func(t *testing.T) {
		// version and id are in the ORDER BY but not in the select list.
		rows, err := latest().All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if want := []anKT{{1, "c"}, {2, "e"}, {3, "g"}}; !reflect.DeepEqual(rows, want) {
			t.Fatalf("rows = %+v, want %+v", rows, want)
		}
	})

	t.Run("limit offset where", func(t *testing.T) {
		rows, err := latest().Limit(1).Offset(1).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if want := []anKT{{2, "e"}}; !reflect.DeepEqual(rows, want) {
			t.Fatalf("limit/offset rows = %+v, want %+v", rows, want)
		}
		// The WHERE runs before the dedupe: without c, b is tenant 1's newest.
		rows, err = latest().Where(suite.DocTitle.Ne("c")).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if want := []anKT{{1, "b"}, {2, "e"}, {3, "g"}}; !reflect.DeepEqual(rows, want) {
			t.Fatalf("filtered rows = %+v, want %+v", rows, want)
		}
	})

	t.Run("count and exists", func(t *testing.T) {
		n, err := latest().Count(ctx)
		if err != nil || n != 3 {
			t.Fatalf("Count = %d, %v, want 3", n, err)
		}
		ok, err := latest().Exists(ctx)
		if err != nil || !ok {
			t.Fatalf("Exists = %v, %v, want true", ok, err)
		}
	})

	t.Run("descending key", func(t *testing.T) {
		rows, err := momo.Select[anKT](db).From(anDocs).Columns(suite.DocTenantID, suite.DocTitle).
			DistinctOn(suite.DocTenantID).
			OrderBy(suite.DocTenantID.Desc(), suite.DocVersion.Asc(), suite.DocID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if want := []anKT{{3, "f"}, {2, "d"}, {1, "a"}}; !reflect.DeepEqual(rows, want) {
			t.Fatalf("rows = %+v, want %+v", rows, want)
		}
	})

	t.Run("null key is one group", func(t *testing.T) {
		rows, err := momo.Select[dBT](db).From(anDocs).Columns(suite.DocBody, suite.DocTitle).
			DistinctOn(suite.DocBody).
			OrderBy(suite.DocBody.Asc().NullsLast(), suite.DocVersion.Desc(), suite.DocID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// NULL: a10 b20 g20, so b wins the version tie on the lower id; x: c
		// over d; y: e over f on the lower id.
		want := []dBT{{strp("x"), "c"}, {strp("y"), "e"}, {nil, "b"}}
		if !reflect.DeepEqual(rows, want) {
			t.Fatalf("rows = %+v, want %+v", rows, want)
		}
	})

	t.Run("two keys with a null", func(t *testing.T) {
		rows, err := momo.Select[anKT](db).From(anDocs).Columns(suite.DocTenantID, suite.DocTitle).
			DistinctOn(suite.DocTenantID, suite.DocBody).
			OrderBy(suite.DocTenantID.Asc(), suite.DocBody.Asc().NullsLast(), suite.DocVersion.Desc(), suite.DocID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []anKT{{1, "c"}, {1, "b"}, {2, "d"}, {2, "e"}, {3, "f"}, {3, "g"}}
		if !reflect.DeepEqual(rows, want) {
			t.Fatalf("rows = %+v, want %+v", rows, want)
		}
	})

	t.Run("join", func(t *testing.T) {
		p := momo.TableRef{Name: "documents", Alias: "p"}
		c := momo.TableRef{Name: "documents", Alias: "c"}
		pTenant := momo.ColumnOf[int64]("p", "tenant_id")
		pID := momo.ColumnOf[int64]("p", "id")
		cID := momo.ColumnOf[int64]("c", "id")
		cTitle := momo.ColumnOf[string]("c", "title")
		cVersion := momo.ColumnOf[int64]("c", "version")
		rows, err := momo.Select[anKT](db).From(p).Join(c, pID.EqExpr(cID)).
			Columns(pTenant, cTitle).DistinctOn(pTenant).
			OrderBy(pTenant.Asc(), cVersion.Desc(), pID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if want := []anKT{{1, "c"}, {2, "e"}, {3, "g"}}; !reflect.DeepEqual(rows, want) {
			t.Fatalf("rows = %+v, want %+v", rows, want)
		}
	})

	t.Run("model and keyset", func(t *testing.T) {
		// The cursor is the row c of tenant 1: (1, 30, 3).
		docs, err := momo.Select[suite.Document](db).
			DistinctOn(suite.DocTenantID).OrderBy(tenantOrder...).
			After(int64(1), int64(30), int64(3)).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, d := range docs {
			got = append(got, d.Title)
		}
		if want := []string{"b", "e", "g"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("titles = %v, want %v", got, want)
		}
	})

	t.Run("result columns", func(t *testing.T) {
		// The ROW_NUMBER helper and the hidden ORDER BY columns stay inside the
		// derived table: a raw read of the statement sees only the select list.
		c, err := latest().SQL()
		if err != nil {
			t.Fatal(err)
		}
		rows, err := momo.MapRows(ctx, db, c.SQL, c.Args...)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 3 {
			t.Fatalf("rows = %v, want 3", rows)
		}
		for _, r := range rows {
			var names []string
			for k := range r {
				names = append(names, k)
			}
			sort.Strings(names)
			if want := []string{"tenant_id", "title"}; !reflect.DeepEqual(names, want) {
				t.Fatalf("columns = %v, want %v", names, want)
			}
		}
	})
}

func TestDistinctOnWindow(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		runDistinctOnWindow(t, db)
	})
}

func TestDistinctOnWindowMySQL(t *testing.T) {
	testopen.EachMySQL(t, func(t *testing.T, db *momo.DB) {
		runDistinctOnWindow(t, db)
	})
}
