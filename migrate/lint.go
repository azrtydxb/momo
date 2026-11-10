// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Lint rule names.
const (
	// RuleNotNullWithoutDefault: ADD COLUMN ... NOT NULL without a default, or SET NOT NULL, on an existing table.
	RuleNotNullWithoutDefault = "not-null-without-default"
	// RuleColumnTypeChange: ALTER COLUMN ... TYPE on an existing table.
	RuleColumnTypeChange = "column-type-change"
	// RuleBlockingIndex: CREATE INDEX without CONCURRENTLY on an existing table.
	RuleBlockingIndex = "blocking-index"
	// RuleForeignKeyWithoutNotValid: a foreign key or check constraint added without NOT VALID.
	RuleForeignKeyWithoutNotValid = "foreign-key-without-not-valid"
	// RuleSQLiteRebuild: a statement that rebuilds a table.
	RuleSQLiteRebuild = "sqlite-rebuild"
	// RuleEnumAddAndUse: a new enum value used in the migration that adds it.
	RuleEnumAddAndUse = "enum-add-and-use"
	// RuleDestructiveWithoutDirective: a destructive statement in a migration without -- momo:destructive.
	RuleDestructiveWithoutDirective = "destructive-without-directive"
)

// Severities of a lint Finding.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

var (
	lintLineComment  = regexp.MustCompile(`--[^\n]*`)
	lintBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lintSpace        = regexp.MustCompile(`\s+`)

	lintAlterTable   = regexp.MustCompile(`^alter table (?:if exists )?(?:only )?(\S+) (.*)$`)
	lintAddColumn    = regexp.MustCompile(`\badd (?:column )?(?:if not exists )?\S+ [^,]*\bnot null\b`)
	lintDefault      = regexp.MustCompile(`\bdefault\b`)
	lintSetNotNull   = regexp.MustCompile(`\balter (?:column )?\S+ set not null\b`)
	lintTypeChange   = regexp.MustCompile(`\balter (?:column )?\S+ (?:set data )?type\b`)
	lintAddConstr    = regexp.MustCompile(`\badd (?:constraint \S+ )?(?:foreign key|check)\b`)
	lintCreateIndex  = regexp.MustCompile(`^create (?:unique )?index (concurrently )?(?:if not exists )?(?:\S+ )?on (?:only )?(\S+)`)
	lintAddEnumValue = regexp.MustCompile(`(?i)^alter type \S+ add value (?:if not exists )?'((?:[^']|'')*)'`)
	lintDropTable    = regexp.MustCompile(`^(?:drop table|drop schema|drop type|truncate)\b`)
	lintDropColumn   = regexp.MustCompile(`\bdrop column\b`)
)

// lintClean strips comments, collapses whitespace and trims the trailing
// semicolon. It returns the text lowercased, for keyword matching, and as
// written, for string literals.
func lintClean(sql string) (collapsed, original string) {
	s := lintBlockComment.ReplaceAllString(lintLineComment.ReplaceAllString(sql, " "), " ")
	s = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(lintSpace.ReplaceAllString(s, " ")), ";"))
	return strings.ToLower(s), s
}

// lintTable normalizes a table reference from SQL text: quotes removed,
// lowercased. It returns the reference and its bare name.
func lintTable(ref string) (full, bare string) {
	ref, _, _ = strings.Cut(ref, "(")
	ref = strings.ToLower(strings.NewReplacer(`"`, "", "`", "").Replace(strings.TrimRight(ref, ";,")))
	bare = ref
	if i := strings.LastIndexByte(ref, '.'); i >= 0 {
		bare = ref[i+1:]
	}
	return ref, bare
}

// LintSQL checks the statements of one migration for operations that lock a
// table for long, rewrite it, or lose data, and reports each with a severity
// and a safer alternative. parentTables holds the tables that exist before the
// migration runs (from the parent snapshot): a table the migration creates
// itself is empty and never reported. A key is a bare table name or
// "schema.name"; case is ignored. dialectName selects the rules: R1 to R4 and R6
// are PostgreSQL rules, R5 reads the "rebuild" Hint a SQLite plan carries, and
// R7 applies to every dialect.
//
// The rules are text heuristics on the comment-free, whitespace-collapsed
// statement and have limits: a statement built by a DO block or a function is
// not seen, an ALTER TABLE with several comma-separated actions is matched as
// a whole, and NOT NULL, TYPE and constraint keywords inside a string literal
// can cause a false report. Finding.Statement is the 1-based position of the
// statement. LintSQL treats the migration as lacking -- momo:destructive;
// Engine.Lint reads the directive.
func LintSQL(version string, stmts []Statement, parentTables map[string]bool, dialectName string) []Finding {
	return lintStatements(version, stmts, parentTables, dialectName, false)
}

func lintStatements(version string, stmts []Statement, parentTables map[string]bool, dialectName string, destructiveOK bool) []Finding {
	parents := map[string]bool{}
	for k := range parentTables {
		if parentTables[k] {
			full, _ := lintTable(k)
			parents[full] = true
		}
	}
	existing := func(ref string) bool {
		full, bare := lintTable(ref)
		return parents[full] || parents[bare]
	}
	pg := dialectName == "postgres"
	var out []Finding
	add := func(n int, sev Severity, rule, msg, alt string) {
		out = append(out, Finding{Severity: sev, Rule: rule, Version: version, Statement: n + 1, Message: msg, Alternative: alt})
	}
	var addedValues []string
	for i, st := range stmts {
		if st.Hint == "rebuild" {
			add(i, SeverityWarning, RuleSQLiteRebuild,
				"this statement rebuilds a table: every row is copied to a new table",
				"batch the data copy or split the table")
			continue
		}
		low, orig := lintClean(st.SQL)
		if low == "" {
			continue
		}
		for _, v := range addedValues {
			if strings.Contains(orig, "'"+v+"'") {
				add(i, SeverityError, RuleEnumAddAndUse,
					"the enum value "+v+" is used in the migration that adds it",
					"move the use to a later migration; a new enum value is unusable until its transaction commits")
			}
		}
		if pg {
			if m := lintAddEnumValue.FindStringSubmatch(orig); m != nil {
				addedValues = append(addedValues, m[1])
			}
			lintPostgres(i, low, existing, add)
		}
		if !destructiveOK {
			destructive := lintDropTable.MatchString(low)
			if m := lintAlterTable.FindStringSubmatch(low); m != nil && lintDropColumn.MatchString(m[2]) {
				destructive = true
			}
			if destructive {
				add(i, SeverityError, RuleDestructiveWithoutDirective,
					"the statement drops or truncates data and the migration has no -- momo:destructive directive",
					"add -- momo:destructive after a review")
			}
		}
	}
	return out
}

func lintPostgres(i int, low string, existing func(string) bool, add func(int, Severity, string, string, string)) {
	if m := lintAlterTable.FindStringSubmatch(low); m != nil && existing(m[1]) {
		rest := m[2]
		switch {
		case lintAddColumn.MatchString(rest) && !lintDefault.MatchString(rest) && !strings.Contains(rest, "generated") &&
			!strings.Contains(rest, " serial") && !strings.Contains(rest, " bigserial") && !strings.Contains(rest, " smallserial"):
			add(i, SeverityError, RuleNotNullWithoutDefault,
				"ADD COLUMN ... NOT NULL without a DEFAULT fails on a table with rows",
				"add the column nullable, backfill in a separate migration, then SET NOT NULL")
		case lintSetNotNull.MatchString(rest):
			add(i, SeverityWarning, RuleNotNullWithoutDefault,
				"SET NOT NULL scans the whole table under an ACCESS EXCLUSIVE lock and fails when a row is NULL",
				"backfill first (momo migrate diff --stage, a data migration, then --enforce-not-null); on a large table add CHECK (col IS NOT NULL) NOT VALID and VALIDATE it first")
		}
		if lintTypeChange.MatchString(rest) {
			add(i, SeverityError, RuleColumnTypeChange,
				"ALTER COLUMN ... TYPE rewrites the table under an ACCESS EXCLUSIVE lock",
				"add a new column, dual-write, backfill, switch reads, drop the old column in a later migration")
		}
		if lintAddConstr.MatchString(rest) && !strings.Contains(rest, "not valid") {
			add(i, SeverityError, RuleForeignKeyWithoutNotValid,
				"adding a foreign key or check constraint validates every row under a lock",
				"add NOT VALID, then VALIDATE CONSTRAINT in a separate migration")
		}
	}
	if m := lintCreateIndex.FindStringSubmatch(low); m != nil && m[1] == "" && existing(m[2]) {
		add(i, SeverityError, RuleBlockingIndex,
			"CREATE INDEX without CONCURRENTLY blocks writes to the table while it builds",
			"CREATE INDEX CONCURRENTLY in a -- momo:no-transaction migration")
	}
}

// Lint checks the migrations that are pending in the database (see Plan) and
// returns the findings in version order. It needs a database connection and
// changes nothing. Go migrations are not linted.
func (e *Engine) Lint(ctx context.Context) ([]Finding, error) {
	pending, err := e.Plan(ctx)
	if err != nil {
		return nil, err
	}
	return e.lintPending(pending)
}

// LintAll checks every SQL migration of the directory, applied or not, and
// returns the findings in version order. It reads the directory only and needs
// no database. Use Lint to check only what is pending.
func (e *Engine) LintAll() ([]Finding, error) {
	pending, err := e.loadPending(map[string]bool{})
	if err != nil {
		return nil, err
	}
	return e.lintPending(pending)
}

func (e *Engine) lintPending(pending []Pending) ([]Finding, error) {
	files, err := Discover(e.dir)
	if err != nil {
		return nil, err
	}
	dirs := map[string]Directives{}
	var snaps []File
	for _, f := range files {
		switch f.Kind {
		case "up":
			d, err := ParseDirectives(f.Body)
			if err != nil {
				return nil, fmt.Errorf("migrate: %s: %w", f.Path, err)
			}
			dirs[f.Version] = d
		case "snapshot":
			snaps = append(snaps, f)
		}
	}
	name := string(e.opts.dialect.Dialect().Name)
	var out []Finding
	for _, p := range pending {
		if p.Kind != "sql" {
			continue
		}
		parent, err := parentTableNames(snaps, p.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, lintStatements(p.Version, p.Statements, parent, name, dirs[p.Version].Destructive)...)
	}
	return out, nil
}

// parentTableNames returns the tables of the latest snapshot older than
// version, keyed by bare name and by schema.name; nil when there is none.
func parentTableNames(snaps []File, version string) (map[string]bool, error) {
	var best *File
	for i := range snaps {
		if snaps[i].Version < version && (best == nil || snaps[i].Version > best.Version) {
			best = &snaps[i]
		}
	}
	if best == nil {
		return nil, nil
	}
	s, err := ReadSnapshot(strings.NewReader(string(best.Body)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", best.Path, err)
	}
	out := map[string]bool{}
	for _, t := range s.Schema.Tables {
		out[t.Name] = true
		if t.Schema != "" {
			out[t.Schema+"."+t.Name] = true
		}
	}
	return out, nil
}

// HasErrors reports whether any finding has error severity.
func HasErrors(fs []Finding) bool {
	return slices.ContainsFunc(fs, func(f Finding) bool { return f.Severity == SeverityError })
}
