// SPDX-License-Identifier: Apache-2.0

package agg_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
)

// Catches: Distinct missing from the rendered call, misplaced relative to
// ORDER BY and FILTER, or rendered on a dialect that cannot express it.
func TestAggregateDistinctGolden(t *testing.T) {
	const from = ` FROM "documents"`
	title := suite.DocTitle
	cases := []aggCase{
		{"array distinct", func() momo.Expr { return agg.Array[string](title).Distinct() }, map[dialect.Name]string{
			dialect.Postgres: `SELECT array_agg(DISTINCT "documents"."title")` + from,
		}},
		{"array distinct order filter", func() momo.Expr {
			return agg.Array[string](title).Distinct().OrderBy(title.Asc()).Filter(suite.DocTenantID.Gt(1))
		}, map[dialect.Name]string{
			dialect.Postgres: `SELECT array_agg(DISTINCT "documents"."title" ORDER BY "documents"."title" ASC) FILTER (WHERE "documents"."tenant_id" > $1)` + from,
		}},
		{"string distinct", func() momo.Expr { return agg.String(title, ",").Distinct().OrderBy(title.Asc()) }, map[dialect.Name]string{
			dialect.Postgres: `SELECT string_agg(DISTINCT "documents"."title", ',' ORDER BY "documents"."title" ASC)` + from,
			dialect.MySQL:    "SELECT GROUP_CONCAT(DISTINCT `documents`.`title` ORDER BY `documents`.`title` ASC SEPARATOR ',') FROM `documents`",
		}},
		{"count distinct", func() momo.Expr { return agg.Func[int64]("count", title).Distinct() }, map[dialect.Name]string{
			dialect.Postgres: `SELECT count(DISTINCT "documents"."title")` + from,
			dialect.SQLite:   `SELECT count(DISTINCT "documents"."title")` + from,
			dialect.MySQL:    "SELECT count(DISTINCT `documents`.`title`) FROM `documents`",
		}},
		{"jsonb distinct", func() momo.Expr { return agg.JSONB[string](title).Distinct() }, map[dialect.Name]string{
			dialect.Postgres: `SELECT jsonb_agg(DISTINCT "documents"."title")` + from,
			dialect.SQLite:   `SELECT json_group_array(DISTINCT "documents"."title")` + from,
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

// Catches: Distinct accepted on an ordered-set aggregate, where it has no
// meaning.
func TestAggregateDistinctOrderedSetRejected(t *testing.T) {
	e := agg.PercentileCont(0.5).Distinct().WithinGroup(suite.DocTitle.Asc())
	if _, err := aggSQL(dialect.Postgres, e); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v; want ErrValidation", err)
	}
}
