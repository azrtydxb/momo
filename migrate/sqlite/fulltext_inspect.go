// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"regexp"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

var createFTS5RE = regexp.MustCompile(`(?is)^\s*CREATE\s+VIRTUAL\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?(?:"(?:[^"]|"")+"|\x60[^\x60]+\x60|\[[^\]]+\]|[^\s(]+)\s+USING\s+fts5\s*\((.*)\)\s*;?\s*$`)

// parseFTS5 reads the CREATE VIRTUAL TABLE text of an external-content FTS5
// table back into the declaration behind it: the content table, the indexed
// columns and the tokenizer. A virtual table of another module, an FTS5 table
// without content=, one with a column option or with an option the declaration
// has no field for is not a full-text declaration and returns false.
func parseFTS5(sql string) (content string, ft schema.FullText, ok bool) {
	m := createFTS5RE.FindStringSubmatch(sql)
	if m == nil {
		return "", schema.FullText{}, false
	}
	for _, arg := range splitTopLevel(m[1]) {
		key, val, isOpt := cutOption(arg)
		if !isOpt {
			col := unquoteSQL(strings.TrimSpace(arg))
			if col == "" || strings.ContainsAny(strings.TrimSpace(arg), " \t\r\n") && !quoted(strings.TrimSpace(arg)) {
				return "", schema.FullText{}, false
			}
			ft.Columns = append(ft.Columns, col)
			continue
		}
		switch strings.ToLower(key) {
		case "content":
			content = val
		case "content_rowid":
		case "tokenize":
			ft.Tokenizer = val
		default:
			return "", schema.FullText{}, false
		}
	}
	if content == "" || len(ft.Columns) == 0 {
		return "", schema.FullText{}, false
	}
	return content, ft, true
}

// splitTopLevel splits s at the commas outside quotes.
func splitTopLevel(s string) []string {
	var out []string
	var quote rune
	start := 0
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"' || r == '`':
			quote = r
		case r == '[':
			quote = ']'
		case r == ',':
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// cutOption splits key = value outside quotes.
func cutOption(arg string) (key, val string, ok bool) {
	var quote rune
	for i, r := range arg {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"' || r == '`':
			quote = r
		case r == '=':
			return strings.TrimSpace(arg[:i]), unquoteSQL(strings.TrimSpace(arg[i+1:])), true
		}
	}
	return "", "", false
}

func quoted(s string) bool {
	if len(s) < 2 {
		return false
	}
	switch s[0] {
	case '"', '\'', '`':
		return s[len(s)-1] == s[0]
	case '[':
		return s[len(s)-1] == ']'
	}
	return false
}

// unquoteSQL removes one level of SQL quoting from an identifier or literal.
func unquoteSQL(s string) string {
	if !quoted(s) {
		return s
	}
	q := s[0]
	body := s[1 : len(s)-1]
	if q == '[' {
		return body
	}
	return strings.ReplaceAll(body, string(q)+string(q), string(q))
}

// ftsIndexes adds, to each content table of tables, the full-text index that
// an external-content FTS5 table of rows stands for. The index carries the
// FTS5 table's name and lists its columns in order, as the expanded
// declaration does.
func ftsIndexes(rows []masterRow, tables []schema.Table) {
	for _, m := range rows {
		if m.typ != "table" {
			continue
		}
		content, ft, ok := parseFTS5(m.sql)
		if !ok {
			continue
		}
		for i := range tables {
			if !strings.EqualFold(tables[i].Name, content) {
				continue
			}
			parts := make([]schema.IndexPart, len(ft.Columns))
			for j, c := range ft.Columns {
				parts[j] = schema.IndexPart{Column: c}
			}
			ft := ft
			tables[i].Indexes = append(tables[i].Indexes, schema.Index{Name: m.name, Method: "fulltext", Parts: parts, FullText: &ft})
		}
	}
}
