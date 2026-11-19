// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"slices"
	"strings"
)

// HistorySuffix is appended to a table name to name its audit history table.
const HistorySuffix = "_history"

// HistoryIDColumn is the identity primary key of a history table.
const HistoryIDColumn = "history_id"

// HistoryOperationColumn holds INSERT, UPDATE or DELETE.
const HistoryOperationColumn = "history_operation"

// HistoryChangedAtColumn holds the wall-clock time of the change
// (clock_timestamp(), so two changes of one transaction keep their order).
const HistoryChangedAtColumn = "history_changed_at"

// HistoryChangedByColumn holds the actor the transaction set, or NULL.
const HistoryChangedByColumn = "history_changed_by"

// HistoryActorSetting is the transaction-local PostgreSQL setting the history
// trigger reads into HistoryChangedByColumn.
const HistoryActorSetting = "momo.actor"

// maxHistoryIdent is the longest identifier PostgreSQL keeps.
const maxHistoryIdent = 63

// HistoryTableName returns the name of the history table of table.
func HistoryTableName(table string) string { return table + HistorySuffix }

// HistoryFunctionName returns the name of the trigger function that writes the
// history of table.
func HistoryFunctionName(table string) string { return table + HistorySuffix + "_fn" }

// HistoryTriggerName returns the name of the trigger that writes the history
// of table.
func HistoryTriggerName(table string) string { return table + HistorySuffix + "_trg" }

// HistoryIndexName returns the name of the index of the history table that
// serves the queries of one row's versions.
func HistoryIndexName(table string) string { return table + HistorySuffix + "_row_idx" }

// History is the set of schema objects that make up the audit history of one
// table: the history table, the trigger function and the trigger on the
// audited table. They are ordinary objects, so Diff, Baseline, drift and a
// snapshot treat them like any others.
type History struct {
	Table    Table
	Function Function
	Trigger  Trigger
}

// HistoryFor builds the audit history objects of t, for PostgreSQL.
//
// The history table repeats every column of t by name and type, nullability,
// default, enum and collation, with the identity, generated and comment parts
// left out, and adds history_id (identity primary key), history_operation,
// history_changed_at and history_changed_by. The trigger function records the
// new row on INSERT and UPDATE and the removed row on DELETE, so the table is
// the complete log of every state of every row. An index over the primary key
// of t and history_changed_at serves the lookups by row.
//
// A column added to or dropped from t changes the objects HistoryFor returns,
// so Diff mirrors the change on the history table and rewrites the function in
// the same plan.
//
// It returns an error for a table that cannot have history: one with no
// primary key, in a schema other than the current one, with a column named like
// a history column, or with a name so long that a derived name exceeds
// PostgreSQL's identifier limit.
func HistoryFor(t *Table) (History, error) {
	if t.Schema != "" {
		return History{}, fmt.Errorf("table %q is in the schema %q: audit history supports tables in the current schema only", t.Name, t.Schema)
	}
	if len(t.PrimaryKey) == 0 {
		return History{}, fmt.Errorf("table %q has no primary key: audit history needs one to find the versions of a row", t.Name)
	}
	reserved := []string{HistoryIDColumn, HistoryOperationColumn, HistoryChangedAtColumn, HistoryChangedByColumn}
	for _, c := range t.Columns {
		if slices.Contains(reserved, c.Name) {
			return History{}, fmt.Errorf("table %q has a column named %q, which audit history uses for its own column", t.Name, c.Name)
		}
	}
	for _, n := range []string{HistoryTableName(t.Name), HistoryFunctionName(t.Name), HistoryTriggerName(t.Name), HistoryIndexName(t.Name)} {
		if len(n) > maxHistoryIdent {
			return History{}, fmt.Errorf("the derived name %q is longer than %d bytes, which PostgreSQL would cut; use a shorter table name", n, maxHistoryIdent)
		}
	}
	h := Table{Name: HistoryTableName(t.Name)}
	var names []string
	for _, c := range t.Columns {
		h.Columns = append(h.Columns, Column{Name: c.Name, Type: c.Type, Nullable: c.Nullable, Default: c.Default, Enum: c.Enum, Collate: c.Collate})
		names = append(names, historyIdent(c.Name))
	}
	h.Columns = append(h.Columns,
		Column{Name: HistoryIDColumn, Type: "bigint", Identity: "always"},
		Column{Name: HistoryOperationColumn, Type: "text"},
		Column{Name: HistoryChangedAtColumn, Type: "timestamptz"},
		Column{Name: HistoryChangedByColumn, Type: "text", Nullable: true},
	)
	h.PrimaryKey = []string{HistoryIDColumn}
	var parts []IndexPart
	for _, c := range t.PrimaryKey {
		parts = append(parts, IndexPart{Column: c})
	}
	parts = append(parts, IndexPart{Column: HistoryChangedAtColumn}, IndexPart{Column: HistoryIDColumn})
	h.Indexes = []Index{{Name: HistoryIndexName(t.Name), Parts: parts}}

	insert := func(row string) string {
		var vals []string
		for _, c := range t.Columns {
			vals = append(vals, row+"."+historyIdent(c.Name))
		}
		vals = append(vals, "TG_OP", "clock_timestamp()", "nullif(current_setting('"+HistoryActorSetting+"', true), '')")
		cols := append(slices.Clone(names), historyIdent(HistoryOperationColumn), historyIdent(HistoryChangedAtColumn), historyIdent(HistoryChangedByColumn))
		return "INSERT INTO " + historyIdent(h.Name) + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(vals, ", ") + ");"
	}
	body := "BEGIN\n" +
		"  IF TG_OP = 'DELETE' THEN\n" +
		"    " + insert("OLD") + "\n" +
		"    RETURN OLD;\n" +
		"  END IF;\n" +
		"  " + insert("NEW") + "\n" +
		"  RETURN NEW;\n" +
		"END"
	fn := Function{Name: HistoryFunctionName(t.Name), Returns: "trigger", Language: "plpgsql", Volatility: "VOLATILE", Body: body}
	trg := Trigger{
		Table:      t.Name,
		Name:       HistoryTriggerName(t.Name),
		Timing:     "AFTER",
		Events:     []string{"DELETE", "INSERT", "UPDATE"},
		ForEachRow: true,
		Function:   fn.Name + "()",
	}
	return History{Table: h, Function: fn, Trigger: trg}, nil
}

// historyIdent quotes an identifier for the function body.
func historyIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
