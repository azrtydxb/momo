// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// AssertUsesIndex fails the test unless the query plan of c uses the index
// named index. q is a *momo.DB or *momo.Tx on PostgreSQL or SQLite; any other
// dialect fails the test.
//
// It runs momo.Explain, a plain EXPLAIN and never EXPLAIN ANALYZE, so c is
// planned and not executed. On PostgreSQL the index counts as used when its
// name is a whole token in an Index Scan, Index Only Scan or Bitmap Index Scan
// node; on SQLite, when a plan line says USING INDEX or USING COVERING INDEX
// with that name. An index whose name merely starts with the asserted
// name does not match. A failure prints the plan and says so when it contains a sequential
// scan. The database may echo literal values into the plan, so run the
// assertion on test data that is not sensitive.
func AssertUsesIndex(tb testing.TB, q momo.Querier, c momo.Compiled, index string) {
	tb.Helper()
	d := q.Dialect().Name
	if d != dialect.Postgres && d != dialect.SQLite {
		tb.Fatalf("momotest.AssertUsesIndex: unsupported dialect %q (PostgreSQL and SQLite only)", d)
		return
	}
	lines, err := momo.Explain(context.Background(), q, c)
	if err != nil {
		tb.Fatalf("momotest.AssertUsesIndex: explain: %v", err)
		return
	}
	found, seq := planUsesIndex(d, lines, index)
	if found {
		return
	}
	plan := strings.Join(lines, "\n")
	if seq {
		tb.Errorf("momotest.AssertUsesIndex: the plan contains a sequential scan and does not use index %q\nplan:\n%s", index, plan)
		return
	}
	tb.Errorf("momotest.AssertUsesIndex: the plan does not use index %q\nplan:\n%s", index, plan)
}

// planUsesIndex scans the plan lines. found reports that index appears as a
// whole token in an index access node; seq that some node reads a table
// sequentially.
func planUsesIndex(d dialect.Name, lines []string, index string) (found, seq bool) {
	name := regexp.QuoteMeta(index)
	var use *regexp.Regexp
	if d == dialect.Postgres {
		use = regexp.MustCompile(`Index (?:Only )?Scan(?: Backward)? using ` + name + `\b|Bitmap Index Scan on ` + name + `\b`)
	} else {
		use = regexp.MustCompile(`USING (?:COVERING )?INDEX ` + name + `\b`)
	}
	for _, l := range lines {
		if use.MatchString(l) {
			found = true
		}
		if d == dialect.Postgres {
			if strings.Contains(l, "Seq Scan") {
				seq = true
			}
		} else if strings.Contains(l, "SCAN ") && !strings.Contains(l, " USING ") {
			seq = true
		}
	}
	return found, seq
}
