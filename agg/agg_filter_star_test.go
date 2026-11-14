// SPDX-License-Identifier: Apache-2.0

package agg_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/agg"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
)

// Catches: a generic aggregate over a raw * with Filter rendering
// count(CASE WHEN p THEN * END) on MySQL, a syntax error the server reports,
// instead of refusing with ErrUnsupported and pointing to momo.CountAll.
func TestCallFilterOverRawStarRefusedOnMySQL(t *testing.T) {
	where := suite.DocTitle.Eq("x")
	_, err := compileAgg(dialect.MySQL, agg.Func[int64]("count", momo.SQL("*")).Filter(where))
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if !strings.Contains(err.Error(), "CountAll") {
		t.Errorf("error %q does not point to momo.CountAll", err)
	}
	// PostgreSQL keeps the native clause.
	got, err := compileAgg(dialect.Postgres, agg.Func[int64]("count", momo.SQL("*")).Filter(where))
	if err != nil || got.SQL != `SELECT count(*) FILTER (WHERE "documents"."title" = $1) FROM "documents"` {
		t.Errorf("postgres: %q, %v", got.SQL, err)
	}
}
