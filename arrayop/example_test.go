// SPDX-License-Identifier: Apache-2.0

package arrayop_test

import (
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/arrayop"
	"github.com/azrtydxb/momo/dialect"
)

// The predicates bind the values as one array parameter, however many there
// are.
func Example() {
	tags := momo.NewColumn[[]string]("docs", "tags", 0)

	q, err := momo.Select[tally](dbFor(dialect.Postgres)).
		From(momo.TableRef{Name: "docs"}).
		Columns(momo.CountAll()).
		Where(arrayop.Overlaps(tags, "go", "sql", "db")).
		SQL()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(q.SQL)
	fmt.Println(q.Args)
	// Output:
	// SELECT COUNT(*) FROM "docs" WHERE "docs"."tags" && $1
	// [[go sql db]]
}
