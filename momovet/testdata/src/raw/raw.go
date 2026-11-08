// SPDX-License-Identifier: Apache-2.0

package raw

import (
	"context"
	"fmt"

	"github.com/azrtydxb/momo"
)

const table = "documents"

const selectAll = "SELECT * FROM " + table

var dynamic = "SELECT 1"

func constants(ctx context.Context, db *momo.DB, user string) {
	momo.Exec(ctx, db, "UPDATE t SET x = 1")
	momo.Exec(ctx, db, "UPDATE t SET x = ?", user)
	momo.Exec(ctx, db, "UPDATE "+table+" SET x = 1")
	momo.Exec(ctx, db, selectAll)
	momo.Scalars[int](ctx, db, "SELECT id FROM t WHERE name = ?", user)
	momo.SQL("x = ?", user)
	momo.TypedSQL[int]("count(*)")
	momo.RawSubquery("SELECT 1")
}

func concatenated(ctx context.Context, db *momo.DB, user string) {
	momo.Exec(ctx, db, "UPDATE t SET x = '"+user+"'") // want "non-constant SQL string passed to momo.Exec"

	momo.Exec(ctx, db, dynamic) // want "non-constant SQL string passed to momo.Exec"

	momo.Exec(ctx, db, fmt.Sprintf("DELETE FROM %s", user)) // want "non-constant SQL string passed to momo.Exec"

	momo.Scalars[int](ctx, db, "SELECT "+user) // want "non-constant SQL string passed to momo.Scalars"

	momo.SQL("SELECT " + user) // want "non-constant SQL string passed to momo.SQL"

	momo.TypedSQL[int]("count(" + user + ")") // want "non-constant SQL string passed to momo.TypedSQL"

	momo.RawSubquery(user) // want "non-constant SQL string passed to momo.RawSubquery"
}

// Exec here is this package's own function, not momo.Exec; it is never checked.
func Exec(ctx context.Context, db *momo.DB, sql string) {}

func otherCalls(ctx context.Context, db *momo.DB, user string) {
	Exec(ctx, db, user)
}
