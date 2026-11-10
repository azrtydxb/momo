// SPDX-License-Identifier: Apache-2.0

package obs_test

import (
	"context"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/obs"
)

// Example records the statements a load runs. Register the Recorder with
// momo.WithHook; it also receives the relation load events.
func Example() {
	rec := &obs.Recorder{}
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec))

	if err := momo.Load(context.Background(), db, parents(3), kids); err != nil {
		fmt.Println(err)
	}
	fmt.Println(len(rec.Events()), "statement,", len(rec.Relations()), "relation load")
	fmt.Println(rec.Relations()[0].Parent, rec.Relations()[0].Name, rec.Relations()[0].Parents)
	// Output:
	// 1 statement, 1 relation load
	// obs_parents Kids 3
}

// ExampleNPlusOne loads a relation once per parent inside a scope. The
// detector reports the relation once, when the count first exceeds the
// threshold. One batched Load would be a single load and report nothing.
func ExampleNPlusOne() {
	det := obs.NewNPlusOne(2, func(f obs.Finding) {
		fmt.Printf("%s loaded %d times, %d parent each\n", f.Relation, f.Count, f.Parents)
	})
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(det))

	ctx := obs.WithScope(context.Background())
	for _, p := range parents(5) {
		if err := momo.Load(ctx, db, []parent{p}, kids); err != nil {
			fmt.Println(err)
		}
	}
	// Output:
	// obs_parents.Kids loaded 3 times, 1 parent each
}
