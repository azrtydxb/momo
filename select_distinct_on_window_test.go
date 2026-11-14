// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
)

// Catches: GROUP BY and HAVING moved outside the derived table (they must run
// before the dedupe), the soft-delete filter lost, the derived table breaking
// as a FromSubquery source, the arguments of an ORDER BY expression bound in
// the wrong order (it renders in the hidden column, the window, then WHERE and
// LIMIT), and a nullable ORDER BY item left on the server's NULL default.
func TestDistinctOnWindowGoldenInteractions(t *testing.T) {
	const innerKT = `SELECT "momo_d"."momo_c0" AS "tenant_id", "momo_d"."momo_c1" AS "t" FROM (SELECT "documents"."tenant_id" AS "momo_c0", "documents"."title" AS "momo_c1", "documents"."tenant_id" AS "momo_o0", "documents"."id" AS "momo_o1", ROW_NUMBER() OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC, "documents"."id" DESC) AS "momo_rn" FROM "documents") AS "momo_d" WHERE "momo_d"."momo_rn" = 1 ORDER BY "momo_d"."momo_o0" ASC, "momo_d"."momo_o1" DESC`
	byArg := momo.Order{Expr: momo.TypedSQL[int64]("? + 1", int64(3)), Desc: true}
	cases := []struct {
		name  string
		build func(db *momo.DB) (momo.Compiled, error)
		want  string
		my    string // MySQL differs from the quoted SQLite text when set
		args  []any
	}{
		{"group by and having", func(db *momo.DB) (momo.Compiled, error) {
			return momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, momo.CountAll()).
				GroupBy(DocTenantID).Having(momo.SQL("COUNT(*) > 1")).
				DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc()).SQL()
		}, `SELECT "momo_d"."momo_c0" AS "tenant_id", "momo_d"."momo_c1" FROM (SELECT "documents"."tenant_id" AS "momo_c0", COUNT(*) AS "momo_c1", "documents"."tenant_id" AS "momo_o0", ROW_NUMBER() OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC) AS "momo_rn" FROM "documents" GROUP BY "documents"."tenant_id" HAVING COUNT(*) > 1) AS "momo_d" WHERE "momo_d"."momo_rn" = 1 ORDER BY "momo_d"."momo_o0" ASC`, "", nil},
		{"soft delete model", func(db *momo.DB) (momo.Compiled, error) {
			return momo.Select[unitDocument](db).DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc(), DocID.Desc()).SQL()
		}, `SELECT "momo_d"."momo_c0" AS "id", "momo_d"."momo_c1" AS "tenant_id", "momo_d"."momo_c2" AS "title", "momo_d"."momo_c3" AS "published", "momo_d"."momo_c4" AS "deleted_at" FROM (SELECT "documents"."id" AS "momo_c0", "documents"."tenant_id" AS "momo_c1", "documents"."title" AS "momo_c2", "documents"."published" AS "momo_c3", "documents"."deleted_at" AS "momo_c4", "documents"."tenant_id" AS "momo_o0", "documents"."id" AS "momo_o1", ROW_NUMBER() OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC, "documents"."id" DESC) AS "momo_rn" FROM "documents" WHERE "documents"."deleted_at" IS NULL) AS "momo_d" WHERE "momo_d"."momo_rn" = 1 ORDER BY "momo_d"."momo_o0" ASC, "momo_d"."momo_o1" DESC`, "", nil},
		{"from subquery", func(db *momo.DB) (momo.Compiled, error) {
			inner := momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, momo.As[string](DocTitle, "t")).
				DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc(), DocID.Desc())
			return momo.Select[anKT](db).From(momo.FromSubquery(inner, "s")).
				Columns(momo.ColumnOf[int64]("s", "tenant_id"), momo.ColumnOf[string]("s", "t")).SQL()
		}, `SELECT "s"."tenant_id", "s"."t" FROM (` + innerKT + `) AS "s"`, "", nil},
		{"order by expression with arguments", func(db *momo.DB) (momo.Compiled, error) {
			return momo.Select[anKT](db).From(anDocs).Columns(DocTenantID, DocTitle).
				Where(anVersion.Gt(5)).DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc(), byArg).Limit(1).SQL()
		}, `SELECT "momo_d"."momo_c0" AS "tenant_id", "momo_d"."momo_c1" AS "title" FROM (SELECT "documents"."tenant_id" AS "momo_c0", "documents"."title" AS "momo_c1", "documents"."tenant_id" AS "momo_o0", ? + 1 AS "momo_o1", ROW_NUMBER() OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC, ? + 1 DESC) AS "momo_rn" FROM "documents" WHERE "documents"."version" > ?) AS "momo_d" WHERE "momo_d"."momo_rn" = 1 ORDER BY "momo_d"."momo_o0" ASC, "momo_d"."momo_o1" DESC LIMIT ?`, "",
			[]any{int64(3), int64(3), int64(5), int64(1)}},
		{"nullable order item gets the PostgreSQL placement", func(db *momo.DB) (momo.Compiled, error) {
			return momo.Select[anKNT](db).From(anDocs).Columns(DocTenantID, suite.DocBody).
				DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc(), suite.DocBody.Desc()).SQL()
		}, `SELECT "momo_d"."momo_c0" AS "tenant_id", "momo_d"."momo_c1" AS "body" FROM (SELECT "documents"."tenant_id" AS "momo_c0", "documents"."body" AS "momo_c1", "documents"."tenant_id" AS "momo_o0", "documents"."body" AS "momo_o1", ROW_NUMBER() OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC, "documents"."body" DESC NULLS FIRST) AS "momo_rn" FROM "documents") AS "momo_d" WHERE "momo_d"."momo_rn" = 1 ORDER BY "momo_d"."momo_o0" ASC, "momo_d"."momo_o1" DESC NULLS FIRST`,
			`SELECT "momo_d"."momo_c0" AS "tenant_id", "momo_d"."momo_c1" AS "body" FROM (SELECT "documents"."tenant_id" AS "momo_c0", "documents"."body" AS "momo_c1", "documents"."tenant_id" AS "momo_o0", "documents"."body" AS "momo_o1", ROW_NUMBER() OVER (PARTITION BY "documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC, ("documents"."body" IS NULL) DESC, "documents"."body" DESC) AS "momo_rn" FROM "documents") AS "momo_d" WHERE "momo_d"."momo_rn" = 1 ORDER BY "momo_d"."momo_o0" ASC, ("momo_d"."momo_o1" IS NULL) DESC, "momo_d"."momo_o1" DESC`, nil},
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		for _, c := range cases {
			got, err := c.build(typedDB(d))
			if err != nil {
				t.Errorf("%s %s: %v", d, c.name, err)
				continue
			}
			want := quoteFor(d, c.want)
			if d == dialect.MySQL && c.my != "" {
				want = quoteFor(d, c.my)
			}
			if got.SQL != want {
				t.Errorf("%s %s:\n got %s\nwant %s", d, c.name, got.SQL, want)
			}
			if len(got.Args) != len(c.args) || (len(c.args) > 0 && !reflect.DeepEqual(got.Args, c.args)) {
				t.Errorf("%s %s: args = %#v, want %#v", d, c.name, got.Args, c.args)
			}
		}
	}
}

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
		{"empty alias", func(db *momo.DB) error {
			_, err := momo.Select[anKT](db).From(anDocs).Columns(momo.As[int64](DocTenantID, ""), DocTitle).
				DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc()).SQL()
			return err
		}, momo.ErrValidation},
		{"alias with a space", func(db *momo.DB) error {
			_, err := momo.Select[anKT](db).From(anDocs).Columns(momo.As[int64](DocTenantID, "a b"), DocTitle).
				DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc()).SQL()
			return err
		}, momo.ErrValidation},
		{"alias of 64 bytes", func(db *momo.DB) error {
			_, err := momo.Select[anKT](db).From(anDocs).Columns(momo.As[int64](DocTenantID, strings.Repeat("a", 64)), DocTitle).
				DistinctOn(DocTenantID).OrderBy(DocTenantID.Asc()).SQL()
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
