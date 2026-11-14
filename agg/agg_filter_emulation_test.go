// SPDX-License-Identifier: Apache-2.0

package agg_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/suite"
)

func compileAgg(d dialect.Name, e momo.Expr) (momo.Compiled, error) {
	return momo.Select[exprtest.Tally](exprtest.DB(d)).From(momo.TableRef{Name: "documents"}).Columns(e).SQL()
}

// Catches: agg.Call.Filter on MySQL rendering a FILTER clause the server
// rejects, filtering nothing (the predicate dropped), or putting the bound
// arguments of the predicate and of the aggregate argument out of order.
func TestCallFilterEmulatedOnMySQL(t *testing.T) {
	title := suite.DocTitle
	where := title.Eq("x")
	cases := []struct {
		name string
		e    momo.Expr
		want string
		args []any
	}{
		{"generic max",
			agg.Func[int64]("max", suite.DocTenantID).Filter(where),
			"SELECT max(CASE WHEN `documents`.`title` = ? THEN `documents`.`tenant_id` END) FROM `documents`",
			[]any{"x"}},
		{"argument that binds a value",
			agg.Func[int64]("sum", momo.TypedSQL[int64]("tenant_id + ?", int64(5))).Filter(where),
			"SELECT sum(CASE WHEN `documents`.`title` = ? THEN tenant_id + ? END) FROM `documents`",
			[]any{"x", int64(5)}},
		{"count distinct",
			agg.Func[int64]("count", title).Distinct().Filter(where),
			"SELECT count(DISTINCT CASE WHEN `documents`.`title` = ? THEN `documents`.`title` END) FROM `documents`",
			[]any{"x"}},
		{"group_concat distinct ordered",
			agg.String(title, ",").Distinct().OrderBy(title.Asc()).Filter(where),
			"SELECT GROUP_CONCAT(DISTINCT CASE WHEN `documents`.`title` = ? THEN `documents`.`title` END ORDER BY `documents`.`title` ASC SEPARATOR ',') FROM `documents`",
			[]any{"x"}},
		{"upper-case function name",
			agg.Func[float64]("AVG", suite.DocTenantID).Filter(where),
			"SELECT AVG(CASE WHEN `documents`.`title` = ? THEN `documents`.`tenant_id` END) FROM `documents`",
			[]any{"x"}},
	}
	for _, c := range cases {
		got, err := compileAgg(dialect.MySQL, c.e)
		if err != nil || got.SQL != c.want {
			t.Errorf("%s:\n got %q (%v)\nwant %q", c.name, got.SQL, err, c.want)
		}
		if !slices.Equal(got.Args, c.args) {
			t.Errorf("%s: args = %v, want %v", c.name, got.Args, c.args)
		}
	}
	// PostgreSQL keeps the native clause.
	got, err := compileAgg(dialect.Postgres, agg.Func[int64]("max", suite.DocTenantID).Filter(where))
	if err != nil || got.SQL != `SELECT max("documents"."tenant_id") FILTER (WHERE "documents"."title" = $1) FROM "documents"` {
		t.Errorf("postgres: %q, %v", got.SQL, err)
	}
}

// Catches: the CASE WHEN rewrite applied to an aggregate that keeps NULL
// inputs (JSON_ARRAYAGG, array_agg, an unknown function), which would add a
// null element or change the result for every filtered-out row instead of
// refusing.
func TestCallFilterRefusedWhereRewriteChangesResult(t *testing.T) {
	where := suite.DocTitle.Eq("x")
	cases := map[string]momo.Expr{
		"json array":       agg.JSONB[int64](suite.DocID).Filter(where),
		"array":            agg.Array[string](suite.DocTitle).Filter(where),
		"unknown function": agg.Func[int64]("json_arrayagg", suite.DocID).Filter(where),
		"no argument":      agg.Func[int64]("count").Filter(where),
	}
	for name, e := range cases {
		_, err := compileAgg(dialect.MySQL, e)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
			continue
		}
		if !strings.Contains(err.Error(), "mysql") {
			t.Errorf("%s: error %q does not name the dialect", name, err)
		}
	}
	_, err := compileAgg(dialect.MySQL, agg.JSONB[int64](suite.DocID).Filter(where))
	if err == nil || !strings.Contains(err.Error(), "JSON_ARRAYAGG keeps NULLs") {
		t.Errorf("json array error = %v, want the reason (JSON_ARRAYAGG keeps NULLs)", err)
	}
}
