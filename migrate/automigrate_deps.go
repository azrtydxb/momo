// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"regexp"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// stableLiteralDefault matches defaults PostgreSQL stores in the catalog
// without rewriting the table: constant literals (optionally cast), NULL,
// booleans, and the stable now() and CURRENT_TIMESTAMP family.
var stableLiteralDefault = regexp.MustCompile(`(?i)^(?:` +
	`'(?:[^']|'')*'|` +
	`[+-]?(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?|` +
	`true|false|null|` +
	`now\(\)|current_timestamp|current_date` +
	`)(?:::[a-z_][a-z0-9_ ]*(?:\(\d+(?:,\s*\d+)?\))?(?:\[\])*)?$`)

// defaultNeedsNoRewrite reports whether adding a column with default d is a
// metadata-only change. It is conservative: anything it cannot classify as a
// constant is treated as volatile.
func defaultNeedsNoRewrite(d string) bool {
	d = strings.TrimSpace(d)
	return d == "" || stableLiteralDefault.MatchString(d)
}

// addColumnRewriteReason returns why adding col to an existing table would
// rewrite or scan it, or "" when it is cheap.
func addColumnRewriteReason(col *schema.Column) string {
	switch {
	case col.Generated != "":
		return "stored generated column rewrites the table and requires a migration"
	case col.Identity != "":
		return "identity column rewrites the table and requires a migration"
	case !defaultNeedsNoRewrite(col.Default):
		return "default is not a constant, so adding the column rewrites the table and requires a migration"
	}
	return ""
}

func dkey(kind string, parts ...string) string {
	return kind + "\x00" + strings.Join(parts, "\x00")
}

// provided returns the keys a skipped change leaves undefined: objects the
// desired schema has but the live database will not get.
func provided(ch Change) []string {
	switch ch.Kind {
	case AddColumn:
		return []string{dkey("col", ch.Schema, ch.Table, ch.Name)}
	case AlterColumn:
		if ch.Destructive { // type change: the desired schema assumes the new type
			return []string{dkey("col", ch.Schema, ch.Table, ch.Name)}
		}
	case RenameColumn:
		return []string{dkey("col", ch.Schema, ch.Table, ch.Name)}
	case CreateTable:
		return []string{dkey("tbl", ch.Schema, ch.Name)}
	case RenameTable:
		return []string{dkey("tbl", ch.Schema, ch.Name)}
	case CreateEnum:
		return []string{dkey("enum", ch.Name)}
	case CreateSchema:
		return []string{dkey("ns", ch.Name)}
	case CreateExtension:
		return []string{dkey("ext", ch.Name)}
	}
	return nil
}

func colRefs(sch, table string, names ...string) []string {
	out := []string{dkey("tbl", sch, table)}
	for _, n := range names {
		out = append(out, dkey("col", sch, table, n))
	}
	return out
}

func columnTypeRefs(c *schema.Column) []string {
	var out []string
	if c.Enum != "" {
		out = append(out, dkey("enum", c.Enum))
	}
	base := strings.ToLower(strings.TrimSpace(c.Type))
	if i := strings.IndexAny(base, "( "); i >= 0 {
		base = base[:i]
	}
	if base != "" {
		out = append(out, dkey("enum", c.Type), dkey("ext", base))
	}
	return out
}

// requires returns the keys the change needs to exist.
func requires(ch Change) []string {
	var out []string
	if ch.Schema != "" {
		out = append(out, dkey("ns", ch.Schema))
	}
	switch ch.Kind {
	case CreateTable:
		if t, ok := ch.After.(*schema.Table); ok {
			for i := range t.Columns {
				out = append(out, columnTypeRefs(&t.Columns[i])...)
			}
			for _, fk := range t.ForeignKeys {
				out = append(out, colRefs(t.Schema, fk.RefTable, fk.RefColumns...)...)
			}
		}
	case AddColumn:
		out = append(out, colRefs(ch.Schema, ch.Table)...)
		if c, ok := ch.After.(*schema.Column); ok {
			out = append(out, columnTypeRefs(c)...)
		}
	case CreateIndex:
		if ix, ok := ch.After.(*schema.Index); ok {
			var cols []string
			for _, p := range ix.Parts {
				if p.Column != "" {
					cols = append(cols, p.Column)
				}
			}
			out = append(out, colRefs(ch.Schema, ch.Table, cols...)...)
		} else {
			out = append(out, colRefs(ch.Schema, ch.Table)...)
		}
	case AddFK:
		if fk, ok := ch.After.(*schema.ForeignKey); ok {
			out = append(out, colRefs(ch.Schema, ch.Table, fk.Columns...)...)
			out = append(out, colRefs(ch.Schema, fk.RefTable, fk.RefColumns...)...)
		} else {
			out = append(out, colRefs(ch.Schema, ch.Table)...)
		}
	case SetRLS, CreatePolicy:
		out = append(out, colRefs(ch.Schema, ch.Table)...)
	}
	return out
}

func describeChange(ch Change) string {
	return string(ch.Kind) + " " + qualified(ch.Schema, ch.Table, ch.Name)
}

// skipDependents moves every kept change that needs an object introduced or
// altered by a skipped change into skipped, until no kept change does.
// Skipping a change can taint further objects (a skipped add_column taints its
// column), so it iterates to a fixed point.
func skipDependents(kept []Change, skipped []Skipped) ([]Change, []Skipped) {
	tainted := map[string]Change{}
	for _, s := range skipped {
		for _, k := range provided(s.Change) {
			if _, ok := tainted[k]; !ok {
				tainted[k] = s.Change
			}
		}
	}
	for changed := true; changed; {
		changed = false
		var next []Change
		for _, ch := range kept {
			var by *Change
			for _, k := range requires(ch) {
				if src, ok := tainted[k]; ok {
					by = &src
					break
				}
			}
			if by == nil {
				next = append(next, ch)
				continue
			}
			skipped = append(skipped, Skipped{Change: ch, Reason: "depends on a skipped change: " + describeChange(*by)})
			for _, k := range provided(ch) {
				if _, ok := tainted[k]; !ok {
					tainted[k] = ch
				}
			}
			changed = true
		}
		kept = next
	}
	return kept, skipped
}
