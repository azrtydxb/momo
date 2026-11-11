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

// Catches: Distinct with an ORDER BY outside the argument list, or with Over,
// or with no or several arguments, reaching the server instead of failing the
// build with ErrValidation.
func TestAggregateDistinctValidation(t *testing.T) {
	title, tenant := suite.DocTitle, suite.DocTenantID
	bad := map[string]momo.Expr{
		"order by another column":   agg.Array[string](title).Distinct().OrderBy(tenant.Asc()),
		"order by one of two":       agg.Array[string](title).Distinct().OrderBy(title.Asc(), tenant.Asc()),
		"string order by other":     agg.String(title, ",").Distinct().OrderBy(tenant.Asc()),
		"over":                      agg.Func[int64]("count", title).Distinct().Over(momo.Over().PartitionBy(tenant)),
		"over with filter":          agg.Func[int64]("count", title).Distinct().Filter(tenant.Gt(1)).Over(momo.Over().PartitionBy(tenant)),
		"count without argument":    agg.Func[int64]("count").Distinct(),
		"count with two arguments":  agg.Func[int64]("count", title, tenant).Distinct(),
		"distinct then order moved": agg.Array[string](title).OrderBy(tenant.Asc()).Distinct(),
	}
	for name, e := range bad {
		if _, err := aggSQL(dialect.Postgres, e); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("%s: err = %v; want ErrValidation", name, err)
		}
	}
}

// Catches: the ORDER BY rule applied on dialects that do not have it, and a
// matching ORDER BY rejected.
func TestAggregateDistinctOrderRuleScope(t *testing.T) {
	title, tenant := suite.DocTitle, suite.DocTenantID
	got, err := aggSQL(dialect.MySQL, agg.String(title, ",").Distinct().OrderBy(tenant.Asc()))
	want := "SELECT GROUP_CONCAT(DISTINCT `documents`.`title` ORDER BY `documents`.`tenant_id` ASC SEPARATOR ',') FROM `documents`"
	if err != nil || got != want {
		t.Errorf("mysql: got %s (%v), want %s", got, err, want)
	}
	got, err = aggSQL(dialect.Postgres, agg.Array[string](title).Distinct().OrderBy(title.Desc()))
	want = `SELECT array_agg(DISTINCT "documents"."title" ORDER BY "documents"."title" DESC NULLS LAST) FROM "documents"`
	if err != nil || got != want {
		t.Errorf("postgres matching order: got %s (%v), want %s", got, err, want)
	}
}
