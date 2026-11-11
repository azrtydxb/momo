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
	momo.MapRows(ctx, db, "SELECT * FROM t WHERE name = ?", user)
	momo.Query[int](ctx, db, "SELECT id FROM t WHERE name = ?", user)
	momo.QueryRows[int](ctx, db, "SELECT id FROM t WHERE name = ?", user)
	momo.RawQuery(db, "SELECT id FROM t WHERE name = ?", user)
	momo.SQL("x = ?", user)
	momo.TypedSQL[int]("count(*)")
	momo.RawSubquery("SELECT 1")
}

func concatenated(ctx context.Context, db *momo.DB, user string) {
	momo.Exec(ctx, db, "UPDATE t SET x = '"+user+"'") // want "non-constant SQL string passed to momo.Exec"

	momo.Exec(ctx, db, dynamic) // want "non-constant SQL string passed to momo.Exec"

	momo.Exec(ctx, db, fmt.Sprintf("DELETE FROM %s", user)) // want "non-constant SQL string passed to momo.Exec"

	momo.Scalars[int](ctx, db, "SELECT "+user) // want "non-constant SQL string passed to momo.Scalars"

	momo.MapRows(ctx, db, "SELECT * FROM "+user) // want "non-constant SQL string passed to momo.MapRows"

	momo.Query[int](ctx, db, "SELECT "+user) // want "non-constant SQL string passed to momo.Query"

	momo.QueryRows[int](ctx, db, "SELECT "+user) // want "non-constant SQL string passed to momo.QueryRows"

	momo.RawQuery(db, "SELECT "+user) // want "non-constant SQL string passed to momo.RawQuery"

	momo.SQL("SELECT " + user) // want "non-constant SQL string passed to momo.SQL"

	momo.TypedSQL[int]("count(" + user + ")") // want "non-constant SQL string passed to momo.TypedSQL"

	momo.RawSubquery(user) // want "non-constant SQL string passed to momo.RawSubquery"
}

func marked(ctx context.Context, db *momo.DB, user string) {
	momo.Exec(ctx, db, "UPDATE "+user+" SET x = 1") //momovet:ok user is an allowlisted identifier

	//momovet:ok identifier checked against the schema
	momo.Query[int](ctx, db, "SELECT id FROM "+user)

	momo.RawSubquery(user) // want "non-constant SQL string passed to momo.RawSubquery"
}

// A trailing directive covers its own line only: the call on the next line is
// still reported.
func trailingDoesNotLeak(ctx context.Context, db *momo.DB, user string) {
	momo.Exec(ctx, db, "UPDATE "+user+" SET a = 1") //momovet:ok identifier is allowlisted
	momo.Exec(ctx, db, "UPDATE "+user+" SET b = 1") // want "non-constant SQL string passed to momo.Exec"
}

// Only the exact //momovet:ok form is a directive.
func lookalikes(ctx context.Context, db *momo.DB, user string) {
	momo.Exec(ctx, db, "A"+user) //momovet:okay not a directive // want "non-constant SQL string passed to momo.Exec"

	//momovet:ok_x not a directive
	momo.Exec(ctx, db, "B"+user) // want "non-constant SQL string passed to momo.Exec"

	// momovet:ok a space after the slashes is not accepted
	momo.Exec(ctx, db, "C"+user) // want "non-constant SQL string passed to momo.Exec"

	//momovet:OK upper case is not accepted
	momo.Exec(ctx, db, "D"+user) // want "non-constant SQL string passed to momo.Exec"

	/*momovet:ok a block comment is not accepted*/
	momo.Exec(ctx, db, "E"+user) // want "non-constant SQL string passed to momo.Exec"
}

// A directive on the first line of a multi-line call, or alone on the line
// above it, covers the call.
func multiLine(ctx context.Context, db *momo.DB, user string) {
	momo.Exec(ctx, db, //momovet:ok first line of the call
		"UPDATE "+user+" SET x = 1")

	//momovet:ok alone on the line above
	momo.Exec(ctx, db,
		"UPDATE "+user+" SET x = 1")
}

func badDirectives(ctx context.Context, db *momo.DB, user string) {
	// want +1 "needs a reason"
	//momovet:ok
	momo.Exec(ctx, db, "UPDATE "+user+" SET x = 1")

	// want +1 "unused //momovet:ok directive"
	//momovet:ok this call is constant
	momo.Exec(ctx, db, "UPDATE t SET x = 1")
}

// Exec here is this package's own function, not momo.Exec; it is never checked.
func Exec(ctx context.Context, db *momo.DB, sql string) {}

func otherCalls(ctx context.Context, db *momo.DB, user string) {
	Exec(ctx, db, user)
}
