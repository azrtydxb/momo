// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/azrtydxb/momo/dialect"
)

const (
	// mergeSnapshotTable is the temporary table that holds the source rows, each
	// numbered in mergeRowColumn.
	mergeSnapshotTable = "momo_merge_src"
	mergeRowColumn     = "momo_merge_row"
	// mergePairsTable is the temporary table of (target row, source row) pairs
	// that On joins, with the new value of every assigned column computed from
	// the target row as it was before any write.
	mergePairsTable = "momo_merge_pairs"
	mergePairRow    = "src_row"
	mergePairKey    = "k"
	mergePairValue  = "v"
)

// mergeStepKind says how an emulation step is run.
type mergeStepKind int

const (
	// mergeStepSetup runs and ignores its row count (DROP and CREATE).
	mergeStepSetup mergeStepKind = iota
	// mergeStepCardinality is a query that returns a row when a target row
	// matches more than one source row.
	mergeStepCardinality
	// mergeStepCount is a query that returns the number of pairs: the rows the
	// update changes.
	mergeStepCount
	mergeStepUpdate
	mergeStepInsert
	// mergeStepCleanup drops a temporary table; the cleanup steps come last.
	mergeStepCleanup
)

type mergeStep struct {
	kind mergeStepKind
	c    Compiled
}

// emulated reports whether the handle's dialect has no MERGE, so Exec runs the
// emulation.
func (m MergeQuery[T]) emulated() bool {
	if m.q == nil {
		return false
	}
	d := m.q.Dialect()
	return d != nil && !d.Merge
}

func (m MergeQuery[T]) unsupportedErr(format string, args ...any) error {
	return &Error{Kind: ErrUnsupported, Op: "merge", Table: m.meta.Table, Err: fmt.Errorf(format, args...)}
}

// Statements returns the statements Exec runs, in order. On a dialect with
// MERGE that is the one MERGE statement. On SQLite, MySQL and MariaDB it is
// the emulation: drop and create the temporary snapshot of the source and the
// temporary table of matched pairs; when there is a WhenMatchedUpdate clause,
// the check for a target row that more than one source row matches, the count
// of rows it changes and the UPDATE; when there is a WhenNotMatchedInsert
// clause, the INSERT of the unmatched rows; and the drops. The check and the
// count are queries, the others are executed. Nothing is run and no server is
// called. Interceptors run as for SQL.
func (m MergeQuery[T]) Statements() ([]Compiled, error) {
	if m.q == nil {
		return nil, noQuerier("merge", m.meta.Table)
	}
	if err := m.intercept(context.Background()); err != nil {
		return nil, err
	}
	if !m.emulated() {
		c, err := compile(m.q.Dialect(), m.render)
		if err != nil {
			return nil, err
		}
		return []Compiled{c}, nil
	}
	steps, err := m.emulation()
	if err != nil {
		return nil, err
	}
	out := make([]Compiled, len(steps))
	for i, s := range steps {
		out[i] = s.c
	}
	return out, nil
}

// emulation builds the statement sequence for a dialect without MERGE.
//
// Everything MERGE decides is decided once, before any write. The source is
// copied into a snapshot with a row number. The pairs table joins the target
// to the snapshot on On and holds, per pair, the key of the target row and the
// new value of every assigned column, computed from the target row as it is
// before any write (so an assignment that reads another assigned column sees
// the old value, as in PostgreSQL, whatever order MySQL assigns in). The UPDATE
// then assigns from the pairs table only, and the INSERT takes the snapshot rows
// that have no pair. Neither depends on what the UPDATE changed. On MySQL and
// MariaDB the pairs are read with FOR UPDATE when there is an update, so the
// matched target rows are locked and read at their latest committed version.
func (m MergeQuery[T]) emulation() ([]mergeStep, error) {
	mp, err := m.plan()
	if err != nil {
		return nil, err
	}
	d := m.q.Dialect()
	if d.Name != dialect.SQLite && d.Name != dialect.MySQL {
		return nil, m.unsupportedErr("MERGE cannot be emulated on %s", d.Name)
	}
	if m.returning.active() {
		return nil, m.unsupportedErr("MERGE ... RETURNING cannot be emulated exactly on %s: run the MERGE without Returning and read the rows afterwards", d.Name)
	}
	if m.hasMatched && len(m.meta.PK) == 0 {
		return nil, m.unsupportedErr("WhenMatchedUpdate on %s needs a primary key on %s, to detect a target row that more than one source row matches", d.Name, m.meta.Table)
	}
	for _, a := range mp.sets {
		if a.kind == setDefault {
			return nil, m.unsupportedErr("WhenMatchedUpdate with SET %s = DEFAULT cannot be emulated on %s", a.col, d.Name)
		}
	}
	mysql := d.Name == dialect.MySQL
	target := TableRef{Name: m.meta.Table, Schema: m.meta.Schema}
	var steps []mergeStep
	var buildErr error
	add := func(kind mergeStepKind, fn func(b *Builder)) {
		c, err := compile(d, func(b *Builder) {
			b.withScope([]string{target.Qualifier(), "src", "n"}, func() { fn(b) })
		})
		if err != nil && buildErr == nil {
			buildErr = err
		}
		steps = append(steps, mergeStep{kind: kind, c: c})
	}
	drop := func(table string) func(b *Builder) {
		return func(b *Builder) {
			if mysql {
				b.WriteString("DROP TEMPORARY TABLE IF EXISTS ")
			} else {
				b.WriteString("DROP TABLE IF EXISTS temp.")
			}
			b.Ident(table)
		}
	}
	from := func(b *Builder, table, alias string) {
		b.Ident(table)
		b.WriteString(" AS ")
		b.Ident(alias)
	}
	pk := func(i int) string { return mergePairKey + strconv.Itoa(i) }
	val := func(i int) string { return mergePairValue + strconv.Itoa(i) }

	add(mergeStepSetup, drop(mergeSnapshotTable))
	add(mergeStepSetup, drop(mergePairsTable))
	add(mergeStepSetup, func(b *Builder) {
		b.WriteString("CREATE TEMPORARY TABLE ")
		b.Ident(mergeSnapshotTable)
		b.WriteString(" AS SELECT ")
		b.Ident("src")
		b.WriteString(".*, ROW_NUMBER() OVER () AS ")
		b.Ident(mergeRowColumn)
		b.WriteString(" FROM ")
		m.source.AppendSQL(b)
		b.WriteString(" AS ")
		b.Ident("src")
	})
	add(mergeStepSetup, func(b *Builder) {
		b.WriteString("CREATE TEMPORARY TABLE ")
		b.Ident(mergePairsTable)
		b.WriteString(" AS SELECT ")
		b.QualifiedIdent("src", mergeRowColumn)
		b.WriteString(" AS ")
		b.Ident(mergePairRow)
		if m.hasMatched {
			for i, ci := range m.meta.PK {
				b.WriteString(", ")
				b.QualifiedIdent(target.Qualifier(), m.meta.Columns[ci].Name)
				b.WriteString(" AS ")
				b.Ident(pk(i))
			}
			for i, a := range mp.sets {
				b.WriteString(", ")
				switch a.kind {
				case setValue:
					b.argFor(a.flags, a.val)
				case setExpr:
					b.exprFor(a.flags, a.expr)
				default:
					b.WriteString("NULL")
				}
				b.WriteString(" AS ")
				b.Ident(val(i))
			}
		}
		b.WriteString(" FROM ")
		target.appendFrom(b)
		b.WriteString(", ")
		from(b, mergeSnapshotTable, "src")
		b.WriteString(" WHERE ")
		appendAnd(b, m.on)
		if mysql && m.hasMatched {
			b.WriteString(" FOR UPDATE")
		}
	})
	if m.hasMatched {
		add(mergeStepCardinality, func(b *Builder) {
			b.WriteString("SELECT 1 FROM ")
			b.Ident(mergePairsTable)
			b.WriteString(" GROUP BY ")
			for i := range m.meta.PK {
				if i > 0 {
					b.WriteString(", ")
				}
				b.Ident(pk(i))
			}
			b.WriteString(" HAVING COUNT(*) > 1 LIMIT 1")
		})
		add(mergeStepCount, func(b *Builder) {
			b.WriteString("SELECT COUNT(*) FROM ")
			b.Ident(mergePairsTable)
		})
		add(mergeStepUpdate, func(b *Builder) {
			qual := ""
			if mysql {
				qual = target.Qualifier()
			}
			b.WriteString("UPDATE ")
			target.appendFrom(b)
			if mysql {
				b.WriteString(", ")
				from(b, mergePairsTable, "n")
			}
			b.WriteString(" SET ")
			for i, a := range mp.sets {
				if i > 0 {
					b.WriteString(", ")
				}
				b.QualifiedIdent(qual, a.col)
				b.WriteString(" = ")
				b.QualifiedIdent("n", val(i))
			}
			if vi := m.meta.Version; vi >= 0 {
				b.WriteString(", ")
				b.QualifiedIdent(qual, m.meta.Columns[vi].Name)
				b.WriteString(" = ")
				b.QualifiedIdent(target.Qualifier(), m.meta.Columns[vi].Name)
				b.WriteString(" + 1")
			}
			if !mysql {
				b.WriteString(" FROM ")
				from(b, mergePairsTable, "n")
			}
			b.WriteString(" WHERE ")
			for i, ci := range m.meta.PK {
				if i > 0 {
					b.WriteString(" AND ")
				}
				b.QualifiedIdent(target.Qualifier(), m.meta.Columns[ci].Name)
				b.WriteString(" = ")
				b.QualifiedIdent("n", pk(i))
			}
		})
	}
	if m.hasInsert {
		add(mergeStepInsert, func(b *Builder) {
			b.WriteString("INSERT INTO ")
			target.appendFrom(b)
			b.WriteString(" (")
			for i, ci := range append(slices.Clone(mp.ip.cols), mp.ip.tsCols...) {
				if i > 0 {
					b.WriteString(", ")
				}
				b.Ident(m.meta.Columns[ci].Name)
			}
			b.WriteString(") SELECT ")
			for i, ci := range mp.ip.cols {
				if i > 0 {
					b.WriteString(", ")
				}
				b.exprFor(m.meta.Columns[ci].Flags, mp.ip.vals[i])
			}
			for _, a := range mp.ip.tsAssign {
				b.WriteString(", ")
				mp.ins.appendValue(b, a)
			}
			b.WriteString(" FROM ")
			from(b, mergeSnapshotTable, "src")
			b.WriteString(" WHERE NOT EXISTS (SELECT 1 FROM ")
			from(b, mergePairsTable, "n")
			b.WriteString(" WHERE ")
			b.QualifiedIdent("n", mergePairRow)
			b.WriteString(" = ")
			b.QualifiedIdent("src", mergeRowColumn)
			b.writeByte(')')
		})
	}
	add(mergeStepCleanup, drop(mergeSnapshotTable))
	add(mergeStepCleanup, drop(mergePairsTable))
	if buildErr != nil {
		return nil, buildErr
	}
	return steps, nil
}

// execEmulated runs the emulation in one atomic scope: a transaction on a DB, a
// savepoint on a Tx. A failure in any step rolls every earlier step back.
func (m MergeQuery[T]) execEmulated(ctx context.Context) (Result, error) {
	if m.q == nil {
		return Result{}, noQuerier("merge", m.meta.Table)
	}
	if err := m.intercept(ctx); err != nil {
		return Result{}, err
	}
	steps, err := m.emulation()
	if err != nil {
		return Result{}, err
	}
	if db, ok := m.q.(*DB); ok && db.pinned {
		return Result{}, m.unsupportedErr("MERGE on %s runs as several statements and needs a transaction; a pinned connection has none, so call it on a Tx or a DB", m.q.Dialect().Name)
	}
	var total int64
	err = runAtomic(ctx, m.q, "merge", m.meta.Table, func(q Querier) (err error) {
		cn := q.conn()
		defer func() {
			// MySQL does not roll a temporary table back with the transaction, so
			// the drops always run, even after a failure.
			for _, s := range steps {
				if s.kind != mergeStepCleanup {
					continue
				}
				if _, derr := cn.exec(context.WithoutCancel(ctx), "merge", m.meta.Table, s.c); derr != nil {
					err = errors.Join(err, derr)
				}
			}
		}()
		matched := int64(0)
		for _, s := range steps {
			switch s.kind {
			case mergeStepSetup:
				if _, err := cn.exec(ctx, "merge", m.meta.Table, s.c); err != nil {
					return err
				}
			case mergeStepCardinality:
				twice, err := m.hasRow(ctx, cn, s.c)
				if err != nil {
					return err
				}
				if twice {
					return &Error{Kind: ErrTooManyRows, Op: "merge", Table: m.meta.Table,
						Err: errors.New("MERGE command cannot affect row a second time: a target row matches more than one source row")}
				}
			case mergeStepCount:
				if matched, err = m.count(ctx, cn, s.c); err != nil {
					return err
				}
				total += matched
			case mergeStepUpdate:
				if matched == 0 {
					continue
				}
				if _, err := cn.exec(ctx, "merge", m.meta.Table, s.c); err != nil {
					return err
				}
			case mergeStepInsert:
				n, err := cn.exec(ctx, "merge", m.meta.Table, s.c)
				if err != nil {
					return err
				}
				total += n
			}
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{RowsAffected: total}, nil
}

// hasRow runs a query and reports whether it returned a row.
func (m MergeQuery[T]) hasRow(ctx context.Context, cn *execer, c Compiled) (bool, error) {
	r, err := cn.query(ctx, "merge", m.meta.Table, "", c)
	if err != nil {
		return false, err
	}
	defer func() { _ = r.Close() }()
	found := r.Next()
	if err := r.Err(); err != nil {
		return false, err
	}
	return found, r.Close()
}

// count runs a COUNT(*) query.
func (m MergeQuery[T]) count(ctx context.Context, cn *execer, c Compiled) (int64, error) {
	r, err := cn.query(ctx, "merge", m.meta.Table, "", c)
	if err != nil {
		return 0, err
	}
	defer func() { _ = r.Close() }()
	var n int64
	if r.Next() {
		if err := r.Scan(&n); err != nil {
			return 0, err
		}
	}
	if err := r.Err(); err != nil {
		return 0, err
	}
	return n, r.Close()
}
