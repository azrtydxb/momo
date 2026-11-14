// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: DISTINCT ON refused on SQLite, MySQL and MariaDB instead of
// emulated, the ROW_NUMBER helper column reaching the select list, an ORDER BY
// expression missing from the outer query, and PostgreSQL's native rendering
// changing.
func TestDistinctOnWindowGolden(t *testing.T) {
	const (
		basic = `SELECT "momo_d"."momo_c0" AS "tenant_id", "momo_d"."momo_c1" AS "title" FROM (SELECT "documents"."tenant_id" AS "momo_c0", "documents"."title" AS "momo_c1", "documents"."tenant_id" AS "momo_o0", "documents"."id" AS "momo_o1", ROW_NUMBER() OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC, "documents"."id" DESC) AS "momo_rn" FROM "documents") AS "momo_d" WHERE "momo_d"."momo_rn" = 1 ORDER BY "momo_d"."momo_o0" ASC, "momo_d"."momo_o1" DESC`
		paged = `SELECT "momo_d"."momo_c0" AS "k", "momo_d"."momo_c1" AS "title" FROM (SELECT "documents"."tenant_id" AS "momo_c0", "documents"."title" AS "momo_c1", "documents"."tenant_id" AS "momo_o0", "documents"."version" AS "momo_o1", ROW_NUMBER() OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC NULLS LAST, "documents"."version" DESC) AS "momo_rn" FROM "documents" WHERE "documents"."version" > ?) AS "momo_d" WHERE "momo_d"."momo_rn" = 1 ORDER BY "momo_d"."momo_o0" ASC NULLS LAST, "momo_d"."momo_o1" DESC LIMIT ? OFFSET ?`
		// MySQL and MariaDB have no NULLS LAST: the IS NULL sort key stands in.
		pagedMySQL = `SELECT "momo_d"."momo_c0" AS "k", "momo_d"."momo_c1" AS "title" FROM (SELECT "documents"."tenant_id" AS "momo_c0", "documents"."title" AS "momo_c1", "documents"."tenant_id" AS "momo_o0", "documents"."version" AS "momo_o1", ROW_NUMBER() OVER (PARTITION BY "documents"."tenant_id" ORDER BY ("documents"."tenant_id" IS NULL) ASC, "documents"."tenant_id" ASC, "documents"."version" DESC) AS "momo_rn" FROM "documents" WHERE "documents"."version" > ?) AS "momo_d" WHERE "momo_d"."momo_rn" = 1 ORDER BY ("momo_d"."momo_o0" IS NULL) ASC, "momo_d"."momo_o0" ASC, "momo_d"."momo_o1" DESC LIMIT ? OFFSET ?`
		// An ORDER BY on a select-list alias orders the window by the aliased
		// expression, which MySQL cannot see by its alias.
		aliasOrder = `SELECT "momo_d"."momo_c0" AS "tenant_id", "momo_d"."momo_c1" AS "v" FROM (SELECT "documents"."tenant_id" AS "momo_c0", "documents"."version" AS "momo_c1", "documents"."tenant_id" AS "momo_o0", "documents"."version" AS "momo_o1", ROW_NUMBER() OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC, "documents"."version" DESC) AS "momo_rn" FROM "documents") AS "momo_d" WHERE "momo_d"."momo_rn" = 1 ORDER BY "momo_d"."momo_o0" ASC, "momo_d"."momo_o1" DESC`
		native     = `SELECT DISTINCT ON ("documents"."tenant_id") "documents"."tenant_id", "documents"."title" FROM "documents" ORDER BY "documents"."tenant_id" ASC, "documents"."id" DESC`
	)
	for _, d := range typedDialects {
		db := typedDB(d)
		wantBasic := quoteFor(d, basic)
		if d == dialect.Postgres {
			wantBasic = native
		}
		got, err := momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, DocTitle).
			DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc(), DocID.Desc()).SQL()
		if err != nil {
			t.Fatalf("%s basic: %v", d, err)
		}
		if got.SQL != wantBasic {
			t.Errorf("%s basic:\n got %s\nwant %s", d, got.SQL, wantBasic)
		}
		if d == dialect.Postgres {
			continue
		}

		wantPaged := paged
		if d == dialect.MySQL {
			wantPaged = quoteFor(d, pagedMySQL)
		}
		got, err = momo.Select[anKT](db).From(anDocs).Columns(momo.As[int64](DocTenantID, "k"), DocTitle).
			Where(anVersion.Gt(5)).DistinctOn(DocTenantID).
			OrderBy(DocTenantID.Asc().NullsLast(), anVersion.Desc()).Limit(2).Offset(1).SQL()
		if err != nil {
			t.Fatalf("%s paged: %v", d, err)
		}
		if got.SQL != wantPaged {
			t.Errorf("%s paged:\n got %s\nwant %s", d, got.SQL, wantPaged)
		}
		if want := []any{int64(5), int64(2), int64(1)}; !reflect.DeepEqual(got.Args, want) {
			t.Errorf("%s paged args = %#v, want %#v", d, got.Args, want)
		}

		v := momo.As[int64](anVersion, "v")
		got, err = momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, v).
			DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc(), v.Desc()).SQL()
		if err != nil {
			t.Fatalf("%s alias order: %v", d, err)
		}
		if want := quoteFor(d, aliasOrder); got.SQL != want {
			t.Errorf("%s alias order:\n got %s\nwant %s", d, got.SQL, want)
		}
	}
}

// Catches: the ORDER BY prefix rule checked on PostgreSQL only, and a
// combination that cannot be emulated soundly (a row lock) building anyway.
func TestDistinctOnWindowErrors(t *testing.T) {
	missing := momo.As[int64](anVersion, "v")
	cases := []struct {
		name string
		run  func(db *momo.DB) error
		want error
	}{
		{"without order by", func(db *momo.DB) error {
			_, err := momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, DocTitle).DistinctOn(DocTenantID).SQL()
			return err
		}, momo.ErrValidation},
		{"other leading order", func(db *momo.DB) error {
			_, err := momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, DocTitle).
				DistinctOn(DocTenantID).OrderBy(DocID.Asc(), DocTenantID.Asc()).SQL()
			return err
		}, momo.ErrValidation},
		{"too short order", func(db *momo.DB) error {
			_, err := momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, DocTitle).
				DistinctOn(DocTenantID, DocTitle).OrderBy(DocTenantID.Asc()).SQL()
			return err
		}, momo.ErrValidation},
		{"alias missing from the select list", func(db *momo.DB) error {
			_, err := momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, DocTitle).
				DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc(), missing.Desc()).SQL()
			return err
		}, momo.ErrValidation},
		{"for update", func(db *momo.DB) error {
			_, err := momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, DocTitle).
				DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc()).ForUpdate().SQL()
			return err
		}, momo.ErrUnsupported},
		{"for share", func(db *momo.DB) error {
			_, err := momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, DocTitle).
				DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc()).ForShare().SQL()
			return err
		}, momo.ErrUnsupported},
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		for _, c := range cases {
			if err := c.run(typedDB(d)); !errors.Is(err, c.want) {
				t.Errorf("%s %s: err = %v, want %v", d, c.name, err, c.want)
			}
		}
	}
}
