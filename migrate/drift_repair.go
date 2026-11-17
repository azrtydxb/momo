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
	// Uncovered lists, as DriftReport.Lines spells them, the differences of
	// Report that no change covers, so no statement can repair them: an index
	// PostgreSQL marks invalid, an unmanaged extension or schema, anything the
	// differ does not express. Fix them by hand or hide them with
	// Config.Exclude. Differences of text Drift could not compare
	// (DriftReport.Unverified) are not listed here, because Drift does not
	// count them as drift either.
	Uncovered []string
	// Staged lists the NOT NULL columns the plan adds nullable because the table
	// may hold rows. Each entry names the column and the statement to run once
	// it is backfilled. Until then the database still differs from the snapshot.
	Staged []string
	// Hints carries the other notes of the differ, such as the risks of a step.
	Hints []string
	// Transactional reports whether the dialect rolls DDL back: false on MySQL
	// and MariaDB, where a failing statement leaves the ones before it applied.
	Transactional bool
}

// Complete reports whether the plan is the whole repair: nothing was withheld,
// nothing is unsupported, no difference is uncovered and no column was added
// nullable instead of NOT NULL. Running Statements then leaves no drift.
func (r *Repair) Complete() bool {
	return len(r.Withheld) == 0 && len(r.Unsupported) == 0 && len(r.Uncovered) == 0 && len(r.Staged) == 0
}

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
// compares it with: the one RepairOptions.Version names, else the latest applied
// migration's, else, for a database without momo history, the latest state of
// the migration directory (Repair.Report.SnapshotVersion says which). It
// compares the two exactly as Drift does and renders the difference (live to
// snapshot) with the dialect's renderer, ordered as Engine.Diff orders a
// migration.
//
// It is read-only and never executes the statements. The caller reviews them
// and runs them (Repair.SQL spells them as a script). PostgreSQL and SQLite
// roll DDL back, except that a SQLite table rebuild needs foreign-key
// enforcement off, which cannot be set inside a transaction; MySQL and MariaDB
// do not roll DDL back (Repair.Transactional), so a failing step leaves the
// earlier ones applied.
//
// Destructive steps are not rendered unless RepairOptions.AllowDestructive is
// set: every drop (including an index or constraint present in the database
// but not in the snapshot, and a whole table the snapshot does not know) and
// every column type change. They are returned in Repair.Withheld together
// with the steps that depend on them, and with the creates that would replace
// a withheld drop (a changed view, trigger, policy, index or constraint is a
// drop and a create). A step the dialect cannot render is returned in
// Repair.Unsupported, never as wrong SQL. A difference no change covers (an
// invalid index, an unmanaged extension) is returned in Repair.Uncovered. A
// NOT NULL column that is missing from a table is added nullable (the table
// may hold rows) and listed in Repair.Staged. Repair.Complete is false while
// any of these lists is not empty.
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
	r := &Repair{Report: s.report(), Transactional: e.opts.dialect.Dialect().TransactionalDDL}
	if r.Report.Clean() {
		return r, nil
	}
	changes, hints, err := Diff(s.live, s.want, DiffOptions{AllowDestructive: true, Stage: true, EnforceNotNull: true})
	if err != nil {
		return nil, fmt.Errorf("migrate: plan the repair: %w", err)
	}
	r.Staged, r.Hints = splitStagedHints(hints)
	r.Uncovered = uncovered(r.Report, changes)
	kept, withheld := e.splitDestructive(s.live, changes, o.AllowDestructive)
	kept, withheld = skipDependents(kept, withheld)
	r.Withheld = withheld
	ordered, err := OrderWithViews(kept, s.live.Views, s.want.Views)
	if err != nil {
		return nil, fmt.Errorf("migrate: order the repair: %w", err)
	}
	r.Changes, r.Statements, r.Unsupported, err = renderRepair(e.opts.dialect, s.live, s.want, ordered)
	if err != nil {
		return nil, fmt.Errorf("migrate: render the repair: %w", err)
	}
	return r, nil
}

// renderRepair renders the ordered changes through d. A step d has no
// rendering for (an error wrapping momo.ErrUnsupported), and the steps that
// depend on it, are returned in unsupported instead; the rest is rendered and
// returned with its changes.
func renderRepair(d Dialect, live, want *schema.Schema, ordered []Change) (changes []Change, stmts []Statement, unsupported []Skipped, err error) {
	stmts, err = renderPlan(d, live, want, ordered)
	if !errors.Is(err, momo.ErrUnsupported) {
		return ordered, stmts, nil, err
	}
	var supported []Change
	for _, ch := range ordered {
		if _, one := renderPlan(d, live, want, []Change{ch}); errors.Is(one, momo.ErrUnsupported) {
			unsupported = append(unsupported, Skipped{Change: ch, Reason: "the " + string(d.Dialect().Name) + " dialect cannot render it: " + one.Error()})
		} else {
			supported = append(supported, ch)
		}
	}
	supported, unsupported = skipDependents(supported, unsupported)
	stmts, err = renderPlan(d, live, want, supported)
	return supported, stmts, unsupported, err
}

// stagedHintLead and stagedHintTail frame the differ's hint for a NOT NULL
// column it added nullable, which names a migrate diff flag a repair has no use for.
const (
	stagedHintLead = "staged: "
	stagedHintTail = "; backfill it, then run migrate diff with --enforce-not-null to emit "
)

// splitStagedHints separates the differ's staged hints, reworded for a repair
// ("backfill it, then run ALTER TABLE ... SET NOT NULL"), from the other hints.
func splitStagedHints(hints []string) (staged, other []string) {
	for _, h := range hints {
		rest, ok := strings.CutPrefix(h, stagedHintLead)
		if !ok {
			other = append(other, h)
			continue
		}
		if what, stmt, ok := strings.Cut(rest, stagedHintTail); ok {
			rest = what + "; backfill it, then run " + stmt
		}
		staged = append(staged, rest)
	}
	return staged, other
}

// uncovered returns the lines of the differences in rep that no change covers.
func uncovered(rep *DriftReport, changes []Change) []string {
	var out []string
	for _, d := range rep.Changed {
		if !covers(changes, d.Object) {
			out = append(out, d.Line())
		}
	}
	for _, o := range rep.Missing {
		if !covers(changes, o) {
			out = append(out, "missing: "+o.Kind+" "+objectName(o))
		}
	}
	for _, o := range rep.Unmanaged {
		if !covers(changes, o) {
			out = append(out, "unmanaged: "+o.Kind+" "+objectName(o))
		}
	}
	return out
}

// covers reports whether some change acts on the object ref names: for a table
// column, index or constraint, a change with that table and name (or of the
// table's own); for anything else, a change with that name.
func covers(changes []Change, ref ObjectRef) bool {
	sameSchema := func(c Change) bool {
		switch ref.Kind {
		case "schema", "extension", "enum":
			return true
		}
		return c.Schema == ref.Schema
	}
	switch ref.Kind {
	case "column", "unique", "index", "fk", "check", "trigger", "policy":
		i := strings.LastIndexByte(ref.Name, '.')
		if i < 0 {
			return false
		}
		tbl, name := ref.Name[:i], ref.Name[i+1:]
		return slices.ContainsFunc(changes, func(c Change) bool {
			return sameSchema(c) && (c.Table == tbl && c.Name == name || c.Table == "" && c.Name == tbl || c.Table == tbl && c.Name == tbl)
		})
	case "table":
		return slices.ContainsFunc(changes, func(c Change) bool {
			return sameSchema(c) && (c.Table == ref.Name || c.Table == "" && c.Name == ref.Name)
		})
	}
	name, _, _ := strings.Cut(ref.Name, "(")
	return slices.ContainsFunc(changes, func(c Change) bool { return sameSchema(c) && c.Name == name })
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
			dropped[dropKey(ch.Kind, ch)] = ch
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
		case replacedDrop(ch, dropped) != nil:
			withheld = append(withheld, Skipped{Change: ch, Reason: "it replaces " + describeChange(*replacedDrop(ch, dropped)) + ", which is withheld"})
		case rd != nil && tables[ch.Schema+"\x00"+ch.Table] && rd.NeedsRebuild(live, ch):
			withheld = append(withheld, Skipped{Change: ch, Reason: "it needs a rebuild of table " + ch.Table + ", which would also apply a withheld destructive step"})
		default:
			next = append(next, ch)
		}
	}
	return next, withheld
}

// dropsFor lists, for each kind of change that creates an object, the kinds of
// drop that remove an object of that kind. A changed object the differ cannot
// alter in place (a view, a trigger, a policy, an index, a constraint) is a drop
// followed by a create of the same name.
var dropsFor = map[ChangeKind][]ChangeKind{
	CreateTable:    {DropTable},
	CreateEnum:     {DropEnum},
	CreateSequence: {DropSequence},
	CreateView:     {DropView},
	CreateFunction: {DropFunction},
	CreateTrigger:  {DropTrigger},
	CreatePolicy:   {DropPolicy},
	CreateIndex:    {DropIndex},
	AddPrimaryKey:  {DropPrimaryKey},
	AddUnique:      {DropUnique},
	AddCheck:       {DropCheck},
	AlterCheckIn:   {DropCheck},
	AddFK:          {DropFK},
}

// dropKey identifies the object a change of kind drops or creates.
func dropKey(kind ChangeKind, ch Change) string {
	return string(kind) + "\x00" + ch.Schema + "\x00" + ch.Table + "\x00" + ch.Name
}

// replacedDrop returns the withheld drop whose object ch creates again under
// the same name, or nil. Creating it while the drop is withheld would fail with
// "already exists" (and on MySQL leave the earlier statements applied), so the
// create is withheld with it.
func replacedDrop(ch Change, dropped map[string]Change) *Change {
	for _, k := range dropsFor[ch.Kind] {
		if d, ok := dropped[dropKey(k, ch)]; ok {
			return &d
		}
	}
	return nil
}
