// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"

	"github.com/azrtydxb/momo/dialect"
)

// errExplainRollback ends the transaction ExplainAnalyze opens for a write, so
// the statement it just ran is rolled back.
var errExplainRollback = errors.New("momo: explain rollback")

// Explain returns the database's plan for the compiled statement c without
// executing it: EXPLAIN on PostgreSQL, EXPLAIN FORMAT=JSON on MySQL and
// MariaDB (one text column) and EXPLAIN QUERY PLAN on SQLite. Any statement can
// be explained, writes included; c.Args are bound, never interpolated. Hooks
// see the statement with Op "explain". Momo never calls Explain on its own.
//
// SelectQuery.Explain is the shorthand for a select; Explain takes the
// Compiled value of any query's SQL method.
func Explain(ctx context.Context, q Querier, c Compiled) ([]string, error) {
	prefix := "EXPLAIN "
	switch q.Dialect().Name {
	case dialect.SQLite:
		prefix = "EXPLAIN QUERY PLAN "
	case dialect.MySQL:
		prefix = "EXPLAIN FORMAT=JSON "
	}
	return planLines(ctx, q, prefix+c.SQL, c)
}

// ExplainAnalyze runs c under EXPLAIN (ANALYZE, FORMAT TEXT) and returns the
// plan with actual timings. ANALYZE executes the statement, so every call runs
// inside a transaction (or, on a *Tx, a savepoint) that is always rolled back:
// a write leaves no row behind, and a SELECT that calls a writing function does
// not keep its writes. A rollback cannot undo effects outside the transaction:
// sequence values taken by nextval, NOTIFY sent by a function, work done
// through dblink or a foreign data wrapper, and anything else the statement does
// outside the database. PostgreSQL only: other dialects return ErrUnsupported
// and issue nothing. Momo never calls ExplainAnalyze on its own.
func ExplainAnalyze(ctx context.Context, q Querier, c Compiled) ([]string, error) {
	if q.Dialect().Name != dialect.Postgres {
		return nil, &Error{Kind: ErrUnsupported, Op: "explain analyze"}
	}
	sql := "EXPLAIN (ANALYZE, FORMAT TEXT) " + c.SQL
	var lines []string
	run := func(tx *Tx) error {
		var err error
		if lines, err = planLines(ctx, tx, sql, c); err != nil {
			return err
		}
		return errExplainRollback
	}
	var err error
	switch v := q.(type) {
	case *DB:
		err = v.Tx(ctx, TxOptions{}, run)
	case *Tx:
		err = v.Tx(ctx, run)
	default:
		return nil, &Error{Kind: ErrUnsupported, Op: "explain analyze"}
	}
	if errors.Is(err, errExplainRollback) {
		err = nil
	}
	return lines, err
}

// planLines runs the explain statement sql with the arguments (and the
// sensitive-argument mask) of c and returns one string per plan row.
func planLines(ctx context.Context, q Querier, sql string, c Compiled) ([]string, error) {
	c.SQL = sql
	r, err := q.conn().query(ctx, "explain", "", "", c)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	sqlite := q.Dialect().Name == dialect.SQLite
	var lines []string
	for r.Next() {
		var line any
		if sqlite {
			// EXPLAIN QUERY PLAN returns (id, parent, notused, detail).
			var id, parent, unused any
			err = r.Scan(&id, &parent, &unused, &line)
		} else {
			err = r.Scan(&line)
		}
		if err != nil {
			return nil, err
		}
		switch v := line.(type) {
		case string:
			lines = append(lines, v)
		case []byte:
			lines = append(lines, string(v))
		default:
			lines = append(lines, fmt.Sprint(v))
		}
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return lines, r.Close()
}
