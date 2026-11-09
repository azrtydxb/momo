// SPDX-License-Identifier: Apache-2.0

package agg_test

import (
	"errors"
	"math"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/sqlfn"
)

var dialects = []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL}

type aggCase struct {
	name  string
	build func() momo.Expr
	want  map[dialect.Name]string // a missing dialect must fail with ErrUnsupported
}

type item struct {
	Label string `json:"label"`
}

func aggSQL(d dialect.Name, e momo.Expr) (string, error) {
	c, err := momo.Select[exprtest.Tally](exprtest.DB(d)).From(momo.TableRef{Name: "documents"}).Columns(e).SQL()
	return c.SQL, err
}

// Catches: aggregate ORDER BY, WITHIN GROUP, separators and JSON helpers
// rendering wrongly or silently on a dialect that cannot express them.
func TestAggregateGolden(t *testing.T) {
	const from = ` FROM "documents"`
	title := suite.DocTitle
	cases := []aggCase{
		{"string_agg order", func() momo.Expr { return agg.String(title, ", ").OrderBy(title.Desc()) }, map[dialect.Name]string{
			dialect.Postgres: `SELECT string_agg("documents"."title", ', ' ORDER BY "documents"."title" DESC)` + from,
			dialect.SQLite:   `SELECT group_concat("documents"."title", ', ' ORDER BY "documents"."title" DESC)` + from,
			dialect.MySQL:    "SELECT GROUP_CONCAT(`documents`.`title` ORDER BY `documents`.`title` DESC SEPARATOR ', ')" + " FROM `documents`",
		}},
		{"string_agg plain with quote", func() momo.Expr { return agg.String(title, "it's") }, map[dialect.Name]string{
			dialect.Postgres: `SELECT string_agg("documents"."title", 'it''s')` + from,
			dialect.SQLite:   `SELECT group_concat("documents"."title", 'it''s')` + from,
			dialect.MySQL:    "SELECT GROUP_CONCAT(`documents`.`title` SEPARATOR 'it''s') FROM `documents`",
		}},
		{"filter after order by", func() momo.Expr {
			return agg.String(title, ",").Filter(suite.DocTenantID.Gt(1)).OrderBy(title.Asc())
		}, map[dialect.Name]string{
			dialect.Postgres: `SELECT string_agg("documents"."title", ',' ORDER BY "documents"."title" ASC) FILTER (WHERE "documents"."tenant_id" > $1)` + from,
			dialect.SQLite:   `SELECT group_concat("documents"."title", ',' ORDER BY "documents"."title" ASC) FILTER (WHERE "documents"."tenant_id" > ?)` + from,
		}},
		{"array_agg", func() momo.Expr {
			return agg.Array[string](title).OrderBy(suite.DocTenantID.Asc(), title.Desc().NullsLast())
		}, map[dialect.Name]string{
			dialect.Postgres: `SELECT array_agg("documents"."title" ORDER BY "documents"."tenant_id" ASC, "documents"."title" DESC NULLS LAST)` + from,
		}},
		{"percentile_cont", func() momo.Expr { return agg.PercentileCont(0.5).WithinGroup(suite.DocTenantID.Asc()) }, map[dialect.Name]string{
			dialect.Postgres: `SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY "documents"."tenant_id" ASC)` + from,
		}},
		{"percentile_disc", func() momo.Expr { return agg.PercentileDisc[int64](0.9).WithinGroup(suite.DocTenantID.Desc()) }, map[dialect.Name]string{
			dialect.Postgres: `SELECT percentile_disc(0.9) WITHIN GROUP (ORDER BY "documents"."tenant_id" DESC)` + from,
		}},
		{"jsonb_agg order", func() momo.Expr { return agg.JSONB[string](title).OrderBy(title.Asc()) }, map[dialect.Name]string{
			dialect.Postgres: `SELECT jsonb_agg("documents"."title" ORDER BY "documents"."title" ASC)` + from,
			dialect.SQLite:   `SELECT json_group_array("documents"."title" ORDER BY "documents"."title" ASC)` + from,
		}},
		{"jsonb_agg of object", func() momo.Expr {
			return agg.JSONB[item](agg.BuildObject[item](agg.Field("label", title), agg.Field("o'k", suite.DocTenantID)))
		}, map[dialect.Name]string{
			dialect.Postgres: `SELECT jsonb_agg(jsonb_build_object('label', "documents"."title", 'o''k', "documents"."tenant_id"))` + from,
			dialect.SQLite:   `SELECT json_group_array(json_object('label', "documents"."title", 'o''k', "documents"."tenant_id"))` + from,
			dialect.MySQL:    "SELECT JSON_ARRAYAGG(JSON_OBJECT('label', `documents`.`title`, 'o''k', `documents`.`tenant_id`)) FROM `documents`",
		}},
		{"coalesce empty array", func() momo.Expr {
			return sqlfn.CoalesceExpr(agg.JSONB[string](title).OrderBy(title.Asc()), agg.EmptyJSONArray[string]())
		}, map[dialect.Name]string{
			dialect.Postgres: `SELECT coalesce(jsonb_agg("documents"."title" ORDER BY "documents"."title" ASC), '[]'::jsonb)` + from,
			dialect.SQLite:   `SELECT coalesce(json_group_array("documents"."title" ORDER BY "documents"."title" ASC), '[]')` + from,
		}},
		{"to_jsonb", func() momo.Expr { return agg.ToJSONB[item](title) }, map[dialect.Name]string{
			dialect.Postgres: `SELECT to_jsonb("documents"."title")` + from,
			dialect.SQLite:   `SELECT json_quote("documents"."title")` + from,
		}},
		{"generic aggregate", func() momo.Expr { return agg.Func[int64]("max", suite.DocTenantID).OrderBy(suite.DocTenantID.Asc()) }, map[dialect.Name]string{
			dialect.Postgres: `SELECT max("documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC)` + from,
			dialect.SQLite:   `SELECT max("documents"."tenant_id" ORDER BY "documents"."tenant_id" ASC)` + from,
		}},
	}
	for _, c := range cases {
		for _, d := range dialects {
			got, err := aggSQL(d, c.build())
			want, ok := c.want[d]
			if !ok {
				if !errors.Is(err, momo.ErrUnsupported) {
					t.Errorf("%s on %s: got %q, %v; want ErrUnsupported", c.name, d, got, err)
				}
				continue
			}
			if err != nil || got != want {
				t.Errorf("%s on %s:\n got %s (%v)\nwant %s", c.name, d, got, err, want)
			}
		}
	}
}

// Catches: bad input reaching the SQL text, or an ordered-set aggregate
// built without its WITHIN GROUP.
func TestAggregateValidation(t *testing.T) {
	title := suite.DocTitle
	bad := map[string]momo.Expr{
		"separator backslash":  agg.String(title, `a\`),
		"json key backslash":   agg.BuildObject[item](agg.Field(`a\`, title)),
		"function name":        agg.Func[int64]("max; drop table x", title),
		"no within group":      agg.PercentileCont(0.5),
		"fraction range":       agg.PercentileCont(1.5).WithinGroup(title.Asc()),
		"order and within":     agg.PercentileCont(0.5).WithinGroup(title.Asc()).OrderBy(title.Asc()),
		"within on string_agg": agg.String(title, ",").WithinGroup(title.Asc()),
		"within on generic":    agg.Func[int64]("max", suite.DocTenantID).WithinGroup(suite.DocTenantID.Asc()),
		"nil filter":           agg.String(title, ",").Filter(nil),
		"nil json value":       agg.BuildObject[item](agg.Field("a", nil)),
		"separator NUL":        agg.String(title, "a\x00b"),
		"json key NUL":         agg.BuildObject[item](agg.Field("a\x00", title)),
		"fraction NaN":         agg.PercentileCont(math.NaN()).WithinGroup(title.Asc()),
		"fraction negative":    agg.PercentileDisc[int64](-0.1).WithinGroup(suite.DocTenantID.Asc()),
	}
	for name, e := range bad {
		if _, err := aggSQL(dialect.Postgres, e); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v; want ErrValidation", name, err)
		}
	}
	// MySQL has no aggregate ORDER BY outside GROUP_CONCAT.
	if _, err := aggSQL(dialect.MySQL, agg.Func[int64]("max", suite.DocTenantID).OrderBy(suite.DocTenantID.Asc())); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("mysql generic ORDER BY: err = %v; want ErrUnsupported", err)
	}
}

// Catches: OrderBy mutating the receiver, so one base aggregate reused in two
// queries leaks the ordering of the first.
func TestAggregateImmutable(t *testing.T) {
	base := agg.String(suite.DocTitle, ",")
	_ = base.OrderBy(suite.DocTitle.Asc())
	got, err := aggSQL(dialect.Postgres, base)
	if err != nil || got != `SELECT string_agg("documents"."title", ',') FROM "documents"` {
		t.Errorf("base after OrderBy = %q, %v", got, err)
	}
}
