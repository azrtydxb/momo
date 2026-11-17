// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

// maxMismatchLines caps how many differing objects the error of a refused
// baseline names; the rest are counted.
const maxMismatchLines = 50

// BaselineMismatchError is the error Baseline returns when the live database
// does not match the snapshot. It wraps ErrBaselineMismatch, so errors.Is
// keeps working, and carries the full report: its message names the first 50
// differing objects (kind, table, column or constraint, and for a changed
// object the definition each side holds), and Report holds every one.
type BaselineMismatchError struct {
	Report *DriftReport
}

// Error names the differing object kinds, their counts and the differing
// objects themselves, one per line.
func (e *BaselineMismatchError) Error() string {
	r := e.Report
	var b strings.Builder
	fmt.Fprintf(&b, "%s: differing object kinds: %s (%d changed, %d missing, %d unmanaged); fix the database (Engine.DriftRepair or momo migrate drift --fix-sql prints the DDL), adjust Config.Exclude, or force the baseline",
		ErrBaselineMismatch, strings.Join(r.kinds(), ", "), len(r.Changed), len(r.Missing), len(r.Unmanaged))
	for _, l := range r.Lines(maxMismatchLines) {
		b.WriteString("\n  ")
		b.WriteString(l)
	}
	return b.String()
}

// Unwrap returns ErrBaselineMismatch.
func (e *BaselineMismatchError) Unwrap() error { return ErrBaselineMismatch }

// objectName spells an object as schema.name, or name in the current schema.
func objectName(o ObjectRef) string {
	if o.Schema == "" {
		return o.Name
	}
	return o.Schema + "." + o.Name
}

// Line spells the difference on one line: the kind, the qualified name
// (table.column, table.constraint), what differs and, when known, the
// definition each side holds.
func (d ObjectDiff) Line() string {
	s := fmt.Sprintf("changed: %s %s: %s", d.Object.Kind, objectName(d.Object), d.Detail)
	if d.Live != "" {
		s += "; live: " + d.Live
	}
	if d.Snapshot != "" {
		s += "; snapshot: " + d.Snapshot
	}
	return s
}

// Lines names every differing object, one line each: the changed objects with
// their live and snapshot definitions, then the missing ones, then the
// unmanaged ones. A limit above zero keeps the first limit lines and ends the
// list with "and N more"; zero or less lists all of them.
func (d *DriftReport) Lines(limit int) []string {
	var out []string
	for _, c := range d.Changed {
		out = append(out, c.Line())
	}
	for _, o := range d.Missing {
		out = append(out, fmt.Sprintf("missing: %s %s", o.Kind, objectName(o)))
	}
	for _, o := range d.Unmanaged {
		out = append(out, fmt.Sprintf("unmanaged: %s %s", o.Kind, objectName(o)))
	}
	if limit > 0 && len(out) > limit {
		more := len(out) - limit
		out = append(out[:limit:limit], fmt.Sprintf("and %d more", more))
	}
	return out
}

// RepairOptions controls DriftRepair.
type RepairOptions struct {
	DriftOptions
	// AllowDestructive renders the destructive steps too: every drop (a table,
	// column, index, constraint, view, enum, ...) and every column type change.
	// Without it they are listed in Repair.Withheld and not rendered.
	AllowDestructive bool
}

// Repair is the plan that brings a live database to the latest snapshot.
type Repair struct {
	// Report is the drift the plan repairs.
	Report *DriftReport
	// Changes holds the changes the statements carry, in execution order.
	Changes []Change
	// Statements brings the live database to the snapshot, in an order that is
	// safe to run in one transaction (the ones with NoTx set excepted).
	Statements []Statement
	// Withheld lists the destructive steps that were not rendered because
	// AllowDestructive was off, and the steps that depend on one, each with
	// its reason.
	Withheld []Skipped
	// Unsupported lists the steps the dialect cannot render, and the steps that
	// depend on one, each with its reason. No statement is written for them.
	Unsupported []Skipped
	// Hints carries the notes of the differ: a NOT NULL column that was added
	// nullable because the table may hold rows (backfill it, then SET NOT NULL),
	// the risks of a step.
	Hints []string
}

// Complete reports whether every step of the plan is in Statements: nothing
// was withheld and nothing is unsupported. Running Statements then leaves no
// drift, except for text-only differences that cannot be rendered.
func (r *Repair) Complete() bool { return len(r.Withheld) == 0 && len(r.Unsupported) == 0 }

// SQL spells Statements as a script: the statements that run in a transaction,
// each ended by a semicolon and separated by a blank line, then, behind a
// comment, the ones that must run outside a transaction. A SQLite table
// rebuild is preceded by a comment saying that foreign-key enforcement must be
// off. It is empty when there is nothing to do.
func (r *Repair) SQL() string {
	var tx, noTx []string
	rebuild := false
	for _, s := range r.Statements {
		text := strings.TrimRight(strings.TrimSpace(s.SQL), "; \n\t") + ";"
		if isForeignKeyCheck(s) {
			rebuild = true
		}
		if s.NoTx {
			noTx = append(noTx, text)
		} else {
			tx = append(tx, text)
		}
	}
	var parts []string
	if rebuild {
		parts = append(parts, "-- A table rebuild: run it with foreign-key enforcement off (PRAGMA foreign_keys = OFF, outside a transaction).")
	}
	parts = append(parts, tx...)
	if len(noTx) > 0 {
		parts = append(parts, "-- The statements below must run outside a transaction.")
		parts = append(parts, noTx...)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// DriftRepair plans the DDL that brings the live database to the snapshot Drift
// compares it with: the latest applied migration's, or, for a database without
// momo history, the latest state of the migration directory. It compares the
// two exactly as Drift does and renders the difference (live to snapshot) with
// the dialect's renderer, ordered as Engine.Diff orders a migration.
//
// It is read-only and never executes the statements. The caller reviews them
// and runs them (Repair.SQL spells them as a script); a MySQL or MariaDB
// database does not roll DDL back, so run the script knowing that.
//
// Destructive steps are not rendered unless RepairOptions.AllowDestructive is
// set: every drop (including an index or constraint present in the database
// but not in the snapshot, and a whole table the snapshot does not know) and
// every column type change. They are returned in Repair.Withheld together
// with the steps that depend on them, so a caller can refuse to go on. A step
// the dialect cannot render is returned in Repair.Unsupported, never as wrong
// SQL. A NOT NULL column that is missing from a table is added nullable (the
// table may hold rows); Repair.Hints says to backfill it and set NOT NULL.
// On SQLite a change that needs a table rebuild is withheld while another step
// of the same table is, because the rebuild would also apply that one.
func (e *Engine) DriftRepair(ctx context.Context, o RepairOptions) (*Repair, error) {
	c, err := e.pin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Release() }()
	s, err := e.driftSides(ctx, c, o.DriftOptions)
	if err != nil {
		return nil, err
	}
	r := &Repair{Report: s.report()}
	if r.Report.Clean() {
		return r, nil
	}
	changes, hints, err := Diff(s.live, s.want, DiffOptions{AllowDestructive: true, Stage: true, EnforceNotNull: true})
	if err != nil {
		return nil, fmt.Errorf("migrate: plan the repair: %w", err)
	}
	r.Hints = hints
	kept, withheld := e.splitDestructive(s.live, changes, o.AllowDestructive)
	kept, withheld = skipDependents(kept, withheld)
	r.Withheld = withheld
	ordered, err := OrderWithViews(kept, s.live.Views, s.want.Views)
	if err != nil {
		return nil, fmt.Errorf("migrate: order the repair: %w", err)
	}
	stmts, err := renderPlan(e.opts.dialect, s.live, s.want, ordered)
	if errors.Is(err, momo.ErrUnsupported) {
		// Find the steps the dialect has no rendering for, and render the rest.
		var supported []Change
		var unsupported []Skipped
		for _, ch := range ordered {
			if _, one := renderPlan(e.opts.dialect, s.live, s.want, []Change{ch}); errors.Is(one, momo.ErrUnsupported) {
				unsupported = append(unsupported, Skipped{Change: ch, Reason: "the " + string(e.opts.dialect.Dialect().Name) + " dialect cannot render it: " + one.Error()})
			} else {
				supported = append(supported, ch)
			}
		}
		supported, unsupported = skipDependents(supported, unsupported)
		r.Unsupported = unsupported
		ordered = supported
		stmts, err = renderPlan(e.opts.dialect, s.live, s.want, ordered)
	}
	if err != nil {
		return nil, fmt.Errorf("migrate: render the repair: %w", err)
	}
	r.Changes, r.Statements = ordered, stmts
	return r, nil
}

// withholdReason returns why a repair must not render ch without
// AllowDestructive, or "" when it may.
func withholdReason(ch Change) string {
	switch {
	case ch.Destructive && ch.Kind == AlterColumn:
		return "destructive: the column type changes and may lose or reject data"
	case ch.Destructive:
		return "destructive: it drops data"
	case strings.HasPrefix(string(ch.Kind), "drop_"):
		return "destructive: it drops an object the snapshot does not have"
	}
	return ""
}

// splitDestructive separates the changes a repair may render from the ones it
// withholds. With allow every change is kept.
func (e *Engine) splitDestructive(live *schema.Schema, changes []Change, allow bool) (kept []Change, withheld []Skipped) {
	if allow {
		return changes, nil
	}
	dropped := map[string]Change{}
	for _, ch := range changes {
		if reason := withholdReason(ch); reason != "" {
			withheld = append(withheld, Skipped{Change: ch, Reason: reason})
			dropped[ch.Schema+"\x00"+ch.Table+"\x00"+ch.Name] = ch
			continue
		}
		kept = append(kept, ch)
	}
	if len(withheld) == 0 {
		return kept, nil
	}
	tables := map[string]bool{}
	for _, w := range withheld {
		tables[w.Change.Schema+"\x00"+w.Change.Table] = true
	}
	rd, _ := e.opts.dialect.(RebuildDetector)
	var next []Change
	for _, ch := range kept {
		switch {
		case replaces(ch, dropped):
			withheld = append(withheld, Skipped{Change: ch, Reason: "it replaces " + describeChange(dropped[ch.Schema+"\x00"+ch.Table+"\x00"+ch.Name]) + ", which is withheld"})
		case rd != nil && tables[ch.Schema+"\x00"+ch.Table] && rd.NeedsRebuild(live, ch):
			withheld = append(withheld, Skipped{Change: ch, Reason: "it needs a rebuild of table " + ch.Table + ", which would also apply a withheld destructive step"})
		default:
			next = append(next, ch)
		}
	}
	return next, withheld
}

// replaces reports whether ch adds an object under the name of a dropped one,
// so that adding it while the drop is withheld would collide.
func replaces(ch Change, dropped map[string]Change) bool {
	if !slices.Contains([]ChangeKind{AddCheck, AddUnique, AddFK, AddPrimaryKey, CreateIndex, AlterCheckIn}, ch.Kind) {
		return false
	}
	d, ok := dropped[ch.Schema+"\x00"+ch.Table+"\x00"+ch.Name]
	return ok && d.Kind != ch.Kind && strings.HasPrefix(string(d.Kind), "drop_")
}
