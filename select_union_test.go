// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Catches: a union member with its own ORDER BY or LIMIT rendered bare, so the
// clause binds to the whole union (or is a syntax error), instead of being
// grouped per dialect: parentheses on PostgreSQL and MySQL, a derived table on
// SQLite, which rejects parenthesised compound members.
func TestUnionMemberOrderLimitGrouping(t *testing.T) {
	docs := momo.TableRef{Name: "documents"}
	type member = momo.SelectQuery[momo.Row2[int64, string]]
	byTenant := func(db *momo.DB, id int64) member {
		return momo.Select[momo.Row2[int64, string]](db).From(docs).Columns(DocTenantID, DocTitle).Where(DocTenantID.Eq(id))
	}

	cases := []struct {
		name    string
		d       dialect.Name
		members func(db *momo.DB) []momo.Subquery
		want    string
	}{
		{"postgres limited members", dialect.Postgres, func(db *momo.DB) []momo.Subquery {
			return []momo.Subquery{
				byTenant(db, 1).OrderBy(DocTitle.Asc()).Limit(2),
				byTenant(db, 2).OrderBy(DocTitle.Desc()).Limit(3).Offset(1),
			}
		}, `(SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = $1 ORDER BY "documents"."title" ASC LIMIT $2)` +
			` UNION ALL ` +
			`(SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = $3 ORDER BY "documents"."title" DESC LIMIT $4 OFFSET $5)`},
		{"postgres plain members stay bare", dialect.Postgres, func(db *momo.DB) []momo.Subquery {
			return []momo.Subquery{byTenant(db, 1), byTenant(db, 2)}
		}, `SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = $1` +
			` UNION ALL ` +
			`SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = $2`},
		{"postgres mixed members", dialect.Postgres, func(db *momo.DB) []momo.Subquery {
			return []momo.Subquery{byTenant(db, 1), byTenant(db, 2).Limit(1)}
		}, `SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = $1` +
			` UNION ALL ` +
			`(SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = $2 LIMIT $3)`},
		{"mysql locked member", dialect.MySQL, func(db *momo.DB) []momo.Subquery {
			return []momo.Subquery{byTenant(db, 1).ForUpdate(), byTenant(db, 2)}
		}, "(SELECT `documents`.`tenant_id`, `documents`.`title` FROM `documents` WHERE `documents`.`tenant_id` = ? FOR UPDATE)" +
			" UNION ALL " +
			"SELECT `documents`.`tenant_id`, `documents`.`title` FROM `documents` WHERE `documents`.`tenant_id` = ?"},
		{"mysql limited members", dialect.MySQL, func(db *momo.DB) []momo.Subquery {
			return []momo.Subquery{
				byTenant(db, 1).OrderBy(DocTitle.Asc()).Limit(2),
				byTenant(db, 2).OrderBy(DocTitle.Asc()).Limit(2),
			}
		}, "(SELECT `documents`.`tenant_id`, `documents`.`title` FROM `documents` WHERE `documents`.`tenant_id` = ? ORDER BY `documents`.`title` ASC LIMIT ?)" +
			" UNION ALL " +
			"(SELECT `documents`.`tenant_id`, `documents`.`title` FROM `documents` WHERE `documents`.`tenant_id` = ? ORDER BY `documents`.`title` ASC LIMIT ?)"},
		{"mysql plain members stay bare", dialect.MySQL, func(db *momo.DB) []momo.Subquery {
			return []momo.Subquery{byTenant(db, 1), byTenant(db, 2)}
		}, "SELECT `documents`.`tenant_id`, `documents`.`title` FROM `documents` WHERE `documents`.`tenant_id` = ?" +
			" UNION ALL " +
			"SELECT `documents`.`tenant_id`, `documents`.`title` FROM `documents` WHERE `documents`.`tenant_id` = ?"},
		{"sqlite limited members use a derived table", dialect.SQLite, func(db *momo.DB) []momo.Subquery {
			return []momo.Subquery{
				byTenant(db, 1).OrderBy(DocTitle.Asc()).Limit(2),
				byTenant(db, 2).OrderBy(DocTitle.Asc()).Limit(2),
			}
		}, `SELECT * FROM (SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = ? ORDER BY "documents"."title" ASC LIMIT ?)` +
			` UNION ALL ` +
			`SELECT * FROM (SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = ? ORDER BY "documents"."title" ASC LIMIT ?)`},
		{"sqlite plain members stay bare", dialect.SQLite, func(db *momo.DB) []momo.Subquery {
			return []momo.Subquery{byTenant(db, 1), byTenant(db, 2)}
		}, `SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = ?` +
			` UNION ALL ` +
			`SELECT "documents"."tenant_id", "documents"."title" FROM "documents" WHERE "documents"."tenant_id" = ?`},
	}
	for _, c := range cases {
		db := momo.New(fakedriver.New(c.d))
		got, err := momo.SelectUnion[momo.Row2[int64, string]](db, true, c.members(db)...).SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.SQL != c.want {
			t.Errorf("%s (top level):\n got %s\nwant %s", c.name, got.SQL, c.want)
		}
		// As a derived table the same members render inside one pair of
		// parentheses and the same grouping.
		src, err := momo.Select[momo.Row2[int64, string]](db).
			From(momo.FromSubquery(momo.Union(true, c.members(db)...), "u")).
			Columns(momo.ColumnOf[int64]("u", "tenant_id"), momo.ColumnOf[string]("u", "title")).SQL()
		if err != nil {
			t.Errorf("%s: derived: %v", c.name, err)
			continue
		}
		if !strings.Contains(src.SQL, "("+c.want+")") {
			t.Errorf("%s (derived):\n got %s\nwant it to contain (%s)", c.name, src.SQL, c.want)
		}
	}
}

// Catches: the outer ORDER BY, LIMIT and OFFSET of a top-level union lost,
// misplaced inside the last member, or bound out of order.
func TestSelectUnionOuterClauses(t *testing.T) {
	docs := momo.TableRef{Name: "documents"}
	title := momo.ColumnOf[string]("", "title")
	for _, c := range []struct {
		d    dialect.Name
		want string
	}{
		{dialect.Postgres, `(SELECT "documents"."title" FROM "documents" ORDER BY "documents"."title" ASC LIMIT $1) UNION (SELECT "documents"."title" FROM "documents" ORDER BY "documents"."title" ASC LIMIT $2) ORDER BY "title" DESC LIMIT $3 OFFSET $4`},
		{dialect.MySQL, "(SELECT `documents`.`title` FROM `documents` ORDER BY `documents`.`title` ASC LIMIT ?) UNION (SELECT `documents`.`title` FROM `documents` ORDER BY `documents`.`title` ASC LIMIT ?) ORDER BY `title` DESC LIMIT ? OFFSET ?"},
		{dialect.SQLite, `SELECT * FROM (SELECT "documents"."title" FROM "documents" ORDER BY "documents"."title" ASC LIMIT ?) UNION SELECT * FROM (SELECT "documents"."title" FROM "documents" ORDER BY "documents"."title" ASC LIMIT ?) ORDER BY "title" DESC LIMIT ? OFFSET ?`},
	} {
		db := momo.New(fakedriver.New(c.d))
		m := momo.Select[momo.Row1[string]](db).From(docs).Columns(DocTitle).OrderBy(DocTitle.Asc()).Limit(2)
		got, err := momo.SelectUnion[momo.Row1[string]](db, false, m, m).
			OrderBy(title.Desc()).Limit(3).Offset(1).SQL()
		if err != nil {
			t.Fatalf("%s: %v", c.d, err)
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.d, got.SQL, c.want)
		}
		if len(got.Args) != 4 || got.Args[0] != int64(2) || got.Args[2] != int64(3) || got.Args[3] != int64(1) {
			t.Errorf("%s: args = %v, want [2 2 3 1]", c.d, got.Args)
		}
	}
}

// Catches: a row lock inside a member being silently dropped on SQLite instead
// of reported, and invalid unions that build.
func TestSelectUnionErrors(t *testing.T) {
	docs := momo.TableRef{Name: "documents"}
	sq := momo.New(fakedriver.New(dialect.SQLite))
	m := momo.Select[momo.Row1[string]](sq).From(docs).Columns(DocTitle)
	if _, err := momo.SelectUnion[momo.Row1[string]](sq, true, m.ForUpdate(), m).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("locked member on SQLite = %v, want ErrUnsupported", err)
	}
	if _, err := momo.SelectUnion[momo.Row1[string]](sq, true).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("no members = %v, want ErrValidation", err)
	}
	if _, err := momo.SelectUnion[momo.Row1[string]](sq, true, m, m).Limit(-1).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("negative Limit = %v, want ErrValidation", err)
	}
	if _, err := momo.SelectUnion[momo.Row1[string]](nil, true, m, m).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("nil Querier = %v, want ErrValidation", err)
	}
}
