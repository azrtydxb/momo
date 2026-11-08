// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/types"
)

// Catches: the select list and GROUP BY rendering a plain key differently,
// which PostgreSQL rejects, a non-plain key silently changing behavior, and a
// sensitive column inlining its key.
func TestJSONGroupByRendering(t *testing.T) {
	render := func(col momo.Expression[types.JSON[jsonPayload]], key string) string {
		e := momo.JSONText(col, key)
		got, err := momo.Select[kvRow](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "events"}).
			Columns(e, momo.CountAll()).GroupBy(e).SQL()
		if err != nil {
			t.Fatal(err)
		}
		return got.SQL
	}
	cases := []struct {
		name string
		col  momo.Expression[types.JSON[jsonPayload]]
		key  string
		want string
	}{
		{"plain key is identical text", jsonBefore, "via",
			`SELECT ("events"."before" ->> 'via'), COUNT(*) FROM "events" GROUP BY ("events"."before" ->> 'via')`},
		{"non-plain key binds per clause (PostgreSQL rejects this grouping)", jsonBefore, "a.b",
			`SELECT ("events"."before" ->> $1), COUNT(*) FROM "events" GROUP BY ("events"."before" ->> $2)`},
		{"sensitive column binds even a plain key", jsonAfter, "via",
			`SELECT ("events"."after" ->> $1), COUNT(*) FROM "events" GROUP BY ("events"."after" ->> $2)`},
	}
	for _, c := range cases {
		if got := render(c.col, c.key); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}
