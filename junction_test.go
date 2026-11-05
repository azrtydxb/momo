// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// A raw predicate containing a lower-precedence operator must stay grouped
// when it is combined with other predicates.
func TestJunctionParenthesisesRawParts(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	cases := []struct {
		name string
		p    momo.Predicate
		want string
	}{
		{"and", momo.And(momo.SQL("a=1 OR b=2"), DocTenantID.Eq(1)), `((a=1 OR b=2) AND "documents"."tenant_id" = $1)`},
		{"or", momo.Or(momo.SQL("a=1 AND b=2"), DocTenantID.Eq(1)), `((a=1 AND b=2) OR "documents"."tenant_id" = $1)`},
		{"single", momo.And(momo.SQL("a=1 OR b=2")), `(a=1 OR b=2) AND ("documents"."deleted_at" IS NULL)`},
	}
	for _, c := range cases {
		got, err := momo.Select[unitDocument](pg).Where(c.p).SQL()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.Contains(got.SQL, "WHERE "+c.want) {
			t.Errorf("%s: got %s, want WHERE %s", c.name, got.SQL, c.want)
		}
	}
}
