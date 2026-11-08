// SPDX-License-Identifier: Apache-2.0

package sqlite

import "strings"

// columnText is what the table text says about a column that the pragmas do
// not: its collation and its generated expression.
type columnText struct {
	collate, generated string
}

// checkText is one CHECK clause of a table: column-level and table-level
// clauses alike. name is empty when the clause is unnamed.
type checkText struct {
	name, expr string
}

// tableDef is the parsed CREATE TABLE text of a table.
type tableDef struct {
	columns      map[string]columnText
	checks       []checkText
	uniqueNames  map[string]string // key columns joined by NUL -> constraint name
	fkNames      map[string]string // key columns joined by NUL, SOH, lower-case parent -> constraint name
	withoutRowID bool
}

// parseTableSQL reads the CREATE TABLE text SQLite stores in sqlite_schema.
// The pragmas give the columns, keys and indexes; the text is the only place
// that holds constraint names, CHECK expressions, generated expressions,
// collations and WITHOUT ROWID. Text it cannot make sense of yields an empty
// result for that part and never an error: the pragmas still describe the
// table.
func parseTableSQL(createSQL string) tableDef {
	def := tableDef{columns: map[string]columnText{}, uniqueNames: map[string]string{}, fkNames: map[string]string{}}
	toks := tokenize(createSQL)
	gi := -1
	for i, t := range toks {
		if t.kind == tokGroup {
			gi = i
			break
		}
	}
	if gi < 0 {
		return def
	}
	for i := gi + 1; i+1 < len(toks); i++ {
		if toks[i].upper() == "WITHOUT" && toks[i+1].upper() == "ROWID" {
			def.withoutRowID = true
		}
	}
	body := groupInner(toks[gi])
	for _, part := range splitCommas(tokenize(body)) {
		if len(part) == 0 {
			continue
		}
		switch part[0].upper() {
		case "CONSTRAINT", "PRIMARY", "UNIQUE", "CHECK", "FOREIGN":
			def.tableConstraint(part)
		default:
			def.columnItem(part)
		}
	}
	return def
}

// tableConstraint records the name and expression of a table-level constraint.
func (d *tableDef) tableConstraint(part []token) {
	name, i := "", 0
	if part[0].upper() == "CONSTRAINT" && len(part) >= 3 {
		name, i = part[1].name(), 2
	}
	switch part[i].upper() {
	case "UNIQUE":
		if i+1 < len(part) && part[i+1].kind == tokGroup && name != "" {
			d.uniqueNames[strings.Join(groupNames(part[i+1]), "\x00")] = name
		}
	case "CHECK":
		if i+1 < len(part) && part[i+1].kind == tokGroup {
			d.checks = append(d.checks, checkText{name: name, expr: strings.TrimSpace(groupInner(part[i+1]))})
		}
	case "FOREIGN":
		// FOREIGN KEY (cols) REFERENCES parent [(cols)] ...
		if i+4 < len(part) && part[i+1].upper() == "KEY" && part[i+2].kind == tokGroup && part[i+3].upper() == "REFERENCES" && name != "" {
			key := strings.Join(groupNames(part[i+2]), "\x00") + "\x01" + strings.ToLower(part[i+4].name())
			d.fkNames[key] = name
		}
	}
}

// columnItem reads one column definition: its collation, generated expression,
// CHECK clauses, and the names of its column-level UNIQUE and REFERENCES
// constraints.
func (d *tableDef) columnItem(part []token) {
	col := part[0].name()
	var ct columnText
	pending := ""
	for i := 1; i < len(part); i++ {
		switch part[i].upper() {
		case "CONSTRAINT":
			if i+1 < len(part) {
				pending = part[i+1].name()
				i++
			}
			continue
		case "CHECK":
			if i+1 < len(part) && part[i+1].kind == tokGroup {
				d.checks = append(d.checks, checkText{name: pending, expr: strings.TrimSpace(groupInner(part[i+1]))})
				i++
			}
		case "COLLATE":
			if i+1 < len(part) {
				ct.collate = part[i+1].name()
				i++
			}
		case "AS":
			if i+1 < len(part) && part[i+1].kind == tokGroup {
				ct.generated = strings.TrimSpace(groupInner(part[i+1]))
				i++
			}
		case "UNIQUE":
			if pending != "" {
				d.uniqueNames[col] = pending
			}
		case "REFERENCES":
			if pending != "" && i+1 < len(part) {
				d.fkNames[col+"\x01"+strings.ToLower(part[i+1].name())] = pending
			}
		}
		pending = ""
	}
	d.columns[col] = ct
}

// enumCheck recognises the CHECK expression that realises an enum column,
// "col" IN ('a', 'b'), and returns the column and the values. Parentheses
// around the whole expression are ignored.
func enumCheck(expr string) (col string, values []string, ok bool) {
	toks := tokenize(expr)
	for len(toks) == 1 && toks[0].kind == tokGroup {
		toks = tokenize(groupInner(toks[0]))
	}
	if len(toks) != 3 || toks[0].kind != tokWord && toks[0].kind != tokIdent || toks[1].upper() != "IN" || toks[2].kind != tokGroup {
		return "", nil, false
	}
	for _, p := range splitCommas(tokenize(groupInner(toks[2]))) {
		if len(p) != 1 || p[0].kind != tokString {
			return "", nil, false
		}
		values = append(values, unquoteString(p[0].text))
	}
	if len(values) == 0 {
		return "", nil, false
	}
	return toks[0].name(), values, true
}

// parseIndexSQL reads a stored CREATE INDEX statement and returns the text of
// each key (a column name or an expression, without COLLATE and sort order) and
// the WHERE predicate of a partial index.
func parseIndexSQL(createSQL string) (parts []string, where string) {
	toks := tokenize(createSQL)
	grp := -1
	for i := 0; i+2 < len(toks); i++ {
		if toks[i].upper() == "ON" && toks[i+2].kind == tokGroup {
			grp = i + 2
			break
		}
	}
	if grp < 0 {
		return nil, ""
	}
	inner := groupInner(toks[grp])
	for _, p := range splitCommas(tokenize(inner)) {
		for len(p) > 0 {
			last := p[len(p)-1].upper()
			switch {
			case last == "ASC" || last == "DESC":
				p = p[:len(p)-1]
				continue
			case len(p) >= 2 && p[len(p)-2].upper() == "COLLATE":
				p = p[:len(p)-2]
				continue
			}
			break
		}
		parts = append(parts, spelled(inner, p))
	}
	if grp+1 < len(toks) && toks[grp+1].upper() == "WHERE" {
		where = strings.TrimRight(strings.TrimSpace(createSQL[toks[grp+1].end:]), "; \t\r\n")
	}
	return parts, where
}
