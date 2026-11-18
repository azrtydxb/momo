// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"slices"
	"time"
)

// insertChunk is one statement of a planned insert: its state, and, when sub is
// set, the models lo..hi of the query it holds (all of them otherwise). back says
// that the caller's models are sliced the same way.
type insertChunk struct {
	r      insertRef
	lo, hi int
	sub    bool
	back   bool
}

// insertChunks is the outcome of planning: the statement itself (one) when no
// split is needed, else the chunks (many). The common single statement is kept
// out of a slice so planning allocates nothing for it.
type insertChunks struct {
	one  insertChunk
	many []insertChunk
}

// plan resolves the statement once and splits a bulk model insert into chunks
// that each stay under the dialect's bind-parameter limit. A single chunk is
// the statement itself, also when rendering will report the error, so
// validation errors keep their single-statement form.
func (r insertRef) plan(a insAccess) (insertChunks, error) {
	r, err := r.generateIDs(true, a)
	if err != nil {
		return insertChunks{}, err
	}
	r.idsDone = true
	if r.q != nil {
		if err := r.checkExplicit(); err != nil {
			return insertChunks{}, err
		}
		if err := r.checkExplicitZero(r.q.Dialect(), a); err != nil { // every row, before the first chunk runs
			return insertChunks{}, err
		}
	}
	r = r.withDBD(a) // after hooks and ids: the plan is computed once and sliced per chunk
	runs, err := r.defaultRuns(a)
	if err != nil {
		return insertChunks{}, err
	}
	if runs == nil {
		return r.planBulk(a)
	}
	var out []insertChunk
	for _, run := range runs {
		ra := a
		ra.off = a.off + run.lo
		cs, err := run.r.planBulk(ra)
		if err != nil {
			return insertChunks{}, err
		}
		list := cs.many
		if list == nil {
			list = []insertChunk{cs.one}
		}
		for _, c := range list {
			if c.sub {
				c.lo += run.lo
				c.hi += run.lo
			} else {
				c.lo, c.hi, c.sub, c.back = run.lo, run.hi, run.sub, run.back
			}
			out = append(out, c)
		}
	}
	return insertChunks{many: out}, nil
}

// planBulk splits a bulk model insert over the bind-parameter limit; see plan.
// The chunks' models are relative to the models of r.
func (r insertRef) planBulk(a insAccess) (insertChunks, error) {
	one := insertChunk{r: r, lo: 0, hi: r.n}
	if r.i.unnest {
		return insertChunks{one: one}, nil // one array parameter per column: the bind count does not grow with the rows
	}
	if r.q == nil || r.i.err != nil || r.m.Table == "" || r.n < 2 {
		return insertChunks{one: one}, nil
	}
	d := r.q.Dialect()
	if d.MaxParams <= 0 {
		return insertChunks{one: one}, nil
	}
	rr, err := r.resolveUpsert(a)
	if err != nil {
		return insertChunks{}, err
	}
	rr.i.upsert = false // resolved: chunks keep target, set and identity decisions
	if r.ferr != nil {
		return insertChunks{one: one}, nil // rendering reports it
	}
	cols, err := rr.modelCols(a)
	if err != nil || len(cols) == 0 {
		return insertChunks{one: one}, nil
	}
	rr.i.fixedCols = cols  // every chunk writes the columns of the whole set
	if rr.clock.IsZero() { // an updated_at bump in resolveUpsert already read it
		rr.clock = r.q.conn().now()
	}
	// Compute and validate every row before the first chunk runs, so a bad
	// row late in a bulk insert executes nothing. The computed values are
	// kept and bound by the chunks, so a non-deterministic default cannot
	// pass here and change afterwards.
	if a.fields.n > 0 {
		rr.pre = rr.modelValues(cols, rr.clock, a)
		if err := rr.validateModelRows(cols, rr.pre, a); err != nil {
			return insertChunks{}, err
		}
		one.r.pre = rr.pre // a fallback to the single statement binds the same values
		one.r.clock = rr.clock
	}
	probe := rr
	probe.n = 1
	probe.dbd = rr.dbd.slice(0, 1)
	if rr.pre != nil {
		probe.pre = rr.pre[:1]
	}
	c1, err := compile(d, func(b *Builder) { probe.render(b, a) })
	if err != nil {
		return insertChunks{one: one}, nil
	}
	bound := 0 // cells the probe row binds: a dbdefault cell writes DEFAULT, no parameter
	for _, ci := range cols {
		if !probe.dbDefaultRow(0, ci) {
			bound++
		}
	}
	fixed := len(c1.Args) - bound // parameters outside the VALUES rows
	per := (d.MaxParams - fixed) / len(cols)
	if per < 1 {
		return insertChunks{}, &Error{Kind: ErrLimitExceeded, Op: "insert", Table: r.m.Table,
			Err: fmt.Errorf("one row binds %d parameters, over the %s limit of %d", len(cols)+fixed, d.Name, d.MaxParams)}
	}
	if rr.n <= per {
		return insertChunks{one: insertChunk{r: rr, lo: 0, hi: rr.n}}, nil
	}
	if len(rr.i.with) > 0 {
		// A WITH clause would run once per chunk, repeating a data-modifying CTE.
		return insertChunks{}, rr.verr("", "With: the insert needs %d statements at the %s parameter limit and a WITH clause would repeat in each; insert fewer rows", (rr.n+per-1)/per, d.Name)
	}
	var out []insertChunk
	for lo := 0; lo < rr.n; lo += per {
		c := rr
		hi := min(lo+per, rr.n)
		ch := insertChunk{lo: lo, hi: hi, sub: true, back: rr.nBack == rr.n}
		c.n = hi - lo
		c.dbd = rr.dbd.slice(lo, hi)
		if ch.back {
			c.nBack = hi - lo
		}
		if rr.pre != nil {
			c.pre = rr.pre[lo:hi]
		}
		ch.r = c
		out = append(out, ch)
	}
	return insertChunks{many: out}, nil
}

// defaultRuns splits a multi-row insert on a dialect whose VALUES list cannot
// hold DEFAULT (SQLite) into runs of consecutive rows that leave the same
// columns to the database default. Within a run every column is either default
// in every row, so it leaves the column list and the column default applies, or
// given in every row, so each run is an ordinary statement. Runs are
// consecutive, never regrouped, so rows are stored and returned in the order
// the caller gave them. It returns nil when no split is needed: the dialect
// takes DEFAULT, Unnest, a single row or a uniform statement.
func (r insertRef) defaultRuns(a insAccess) ([]insertChunk, error) {
	if r.q == nil || r.i.unnest || r.i.err != nil || r.m.Table == "" || r.q.Dialect().DefaultInValues {
		return nil, nil
	}
	var sigs [][]bool
	if r.i.hasRows {
		sigs = r.rowsDefaultSigs(a)
	} else if r.n > 1 {
		sigs = r.dbd.rows
	}
	if len(sigs) < 2 {
		return nil, nil
	}
	var bounds []int // the start of each run
	for row := range sigs {
		if row == 0 || !slices.Equal(sigs[row], sigs[row-1]) {
			bounds = append(bounds, row)
		}
	}
	if len(bounds) < 2 {
		return nil, nil
	}
	if len(r.i.with) > 0 {
		return nil, r.verr("", "With: the rows leave different columns to the database default, so the insert needs %d statements on %s and a WITH clause would repeat in each; give every row the same columns", len(bounds), r.q.Dialect().Name)
	}
	if r.clock.IsZero() {
		r.clock = r.q.conn().now() // one statement time for every run
	}
	out := make([]insertChunk, 0, len(bounds))
	for i, lo := range bounds {
		hi := len(sigs)
		if i+1 < len(bounds) {
			hi = bounds[i+1]
		}
		c := r
		ch := insertChunk{lo: lo, hi: hi}
		if r.i.hasRows {
			c.i.rows = r.i.rows[lo:hi:hi]
		} else {
			ch.sub = true
			c.n = hi - lo
			c.dbd = r.dbd.slice(lo, hi)
			ch.back = r.nBack == r.n
			if ch.back {
				c.nBack = hi - lo
			}
			if r.pre != nil {
				c.pre = r.pre[lo:hi:hi]
			}
		}
		ch.r = c
		out = append(out, ch)
	}
	return out, nil
}

// rowsDefaultSigs returns, per ValuesRows row, which columns of the statement
// the row leaves to the database default: a column some row assigns that this
// row omits or sets with SetDefault, unless a Go-side default or an automatic
// timestamp fills it. It returns nil when the rows cannot be resolved; render
// reports that.
func (r insertRef) rowsDefaultSigs(a insAccess) [][]bool {
	if len(r.i.rows) < 2 {
		return nil
	}
	if r.ferr != nil {
		return nil
	}
	n := len(r.m.Columns)
	kinds := make([]map[int]setKind, len(r.i.rows))
	union := make([]bool, n)
	for ri, row := range r.i.rows {
		kinds[ri] = map[int]setKind{}
		for _, as := range row {
			ci := r.column(as.col)
			if ci < 0 {
				return nil
			}
			kinds[ri][ci] = as.kind
			union[ci] = true
		}
	}
	sigs := make([][]bool, len(r.i.rows))
	for ri := range r.i.rows {
		seen := make(map[int]struct{}, len(kinds[ri]))
		for ci := range kinds[ri] {
			seen[ci] = struct{}{}
		}
		stamped, _, err := r.timestampAssigns(seen, time.Time{}, a) // presence only
		if err != nil {
			return nil
		}
		sig := make([]bool, n)
		for ci := range n {
			kind, assigned := kinds[ri][ci]
			switch {
			case assigned:
				sig[ci] = kind == setDefault
			case !union[ci]:
			case a.fields.hasDef(ci) && r.writable(ci):
			case slices.Contains(stamped, ci):
			default:
				sig[ci] = true
			}
		}
		sigs[ri] = sig
	}
	return sigs
}
