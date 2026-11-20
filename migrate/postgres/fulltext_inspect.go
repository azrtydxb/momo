// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"regexp"
	"strings"

	"github.com/azrtydxb/momo/internal/schemautil"
	"github.com/azrtydxb/momo/schema"
)

const pgIdentPattern = `(?:[a-z_][a-z0-9_$]*|"(?:[^"]|"")+")`

var (
	// termRE is one term of a generation expression as pg_get_expr writes it
	// (or as NormalizeExpr leaves it): to_tsvector over COALESCE of one column,
	// optionally wrapped in setweight.
	termRE = regexp.MustCompile(`(?is)^(?:(setweight)\s*\(\s*)?to_tsvector\s*\(\s*'([A-Za-z_][A-Za-z0-9_]*)'(?:::regconfig)?\s*,\s*coalesce\s*\(\s*(` +
		pgIdentPattern + `)\s*,\s*''(?:::text)?\s*\)\s*\)(?:\s*,\s*'([A-D])'(?:::"char")?\s*\))?$`)
	// vectorCallRE is the to_tsvector call of an expression index.
	vectorCallRE = regexp.MustCompile(`(?is)^to_tsvector\s*\(\s*'([A-Za-z_][A-Za-z0-9_]*)'(?:::regconfig)?\s*,\s*(.*)\)\s*$`)
	coalesceRE   = regexp.MustCompile(`(?is)^coalesce\s*\(\s*(` + pgIdentPattern + `)\s*,\s*''(?:::text)?\s*\)$`)
	spaceLitRE   = regexp.MustCompile(`(?s)^' '(?:::text)?$`)
)

// parseGenerationExpr reads the expression of a generated tsvector column back
// into the declaration that produced it. It accepts exactly the shape
// generationExpr writes: one or more to_tsvector terms over COALESCE of a
// column, each optionally in setweight, joined by ||. Any other expression is
// not a full-text declaration and returns false.
func parseGenerationExpr(expr string) (schema.FullText, bool) {
	var ft schema.FullText
	terms := flattenConcat(expr)
	weighted := 0
	for _, term := range terms {
		m := termRE.FindStringSubmatch(strings.TrimSpace(term))
		if m == nil || (m[1] != "") != (m[4] != "") {
			return schema.FullText{}, false
		}
		if ft.Config == "" {
			ft.Config = m[2]
		} else if ft.Config != m[2] {
			return schema.FullText{}, false
		}
		ft.Columns = append(ft.Columns, unquoteIdent(m[3]))
		if m[4] != "" {
			weighted++
			ft.Weights = append(ft.Weights, strings.ToUpper(m[4]))
		}
	}
	if len(terms) == 0 || (weighted != 0 && weighted != len(terms)) {
		return schema.FullText{}, false
	}
	if ft.Validate() != nil {
		return schema.FullText{}, false
	}
	return ft, true
}

// parseIndexExpression reads the expression key of a GIN index back into an
// expression declaration: to_tsvector over the COALESCE of one column, or of
// several joined by a single space.
func parseIndexExpression(expr string) (schema.FullText, bool) {
	m := vectorCallRE.FindStringSubmatch(strings.TrimSpace(expr))
	if m == nil {
		return schema.FullText{}, false
	}
	ft := schema.FullText{Config: m[1], Expression: true}
	elems := flattenConcat(m[2])
	if len(elems)%2 == 0 {
		return schema.FullText{}, false
	}
	for i, e := range elems {
		e = strings.TrimSpace(e)
		if i%2 == 1 {
			if !spaceLitRE.MatchString(e) {
				return schema.FullText{}, false
			}
			continue
		}
		c := coalesceRE.FindStringSubmatch(e)
		if c == nil {
			return schema.FullText{}, false
		}
		ft.Columns = append(ft.Columns, unquoteIdent(c[1]))
	}
	if ft.Validate() != nil {
		return schema.FullText{}, false
	}
	return ft, true
}

func unquoteIdent(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strings.ReplaceAll(s[1:len(s)-1], `""`, `"`)
	}
	return s
}

// flattenConcat splits s at every top-level || operator, looking through
// redundant parentheses, so ((a || b) || c) yields a, b, c.
func flattenConcat(s string) []string {
	s = unwrapParens(strings.TrimSpace(s))
	parts := splitConcat(s)
	if len(parts) == 1 {
		return []string{s}
	}
	var out []string
	for _, p := range parts {
		out = append(out, flattenConcat(p)...)
	}
	return out
}

// unwrapParens removes parentheses that enclose the whole of s.
func unwrapParens(s string) string {
	for len(s) >= 2 && s[0] == '(' && closingOf(s) == len(s)-1 {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

// closingOf returns the index of the parenthesis that closes the one at s[0].
func closingOf(s string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitConcat splits s at the || operators outside quotes and parentheses.
func splitConcat(s string) []string {
	var out []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
		case '|':
			if depth == 0 && i+1 < len(s) && s[i+1] == '|' {
				out = append(out, strings.TrimSpace(s[start:i]))
				i++
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// foldFullText turns the GIN indexes of t that a full-text declaration would
// have created back into that declaration: a GIN index over the generated
// tsvector column named for it, or over a to_tsvector expression. The text of
// the generated column and of the expression key is written from the parsed
// declaration, the same way the expansion writes it, so an inspected table
// compares equal to the expanded declared one. Anything that does not parse
// stays a plain column and a plain GIN index.
func foldFullText(t *schema.Table) {
	for i := range t.Indexes {
		ix := &t.Indexes[i]
		if ix.Method != "gin" || ix.Unique || ix.Where != "" || len(ix.With) > 0 || len(ix.Parts) != 1 {
			continue
		}
		p := ix.Parts[0]
		if p.Desc || p.Nulls != "" || p.OpClass != "" {
			continue
		}
		switch {
		case p.Column != "":
			if p.Column != schemautil.TSVectorColumn(ix.Name) {
				continue
			}
			for j := range t.Columns {
				c := &t.Columns[j]
				if c.Name != p.Column || c.Generated == "" || c.GeneratedVirtual || c.Type != "tsvector" {
					continue
				}
				ft, ok := parseGenerationExpr(c.Generated)
				if !ok {
					break
				}
				c.Generated = generationExpr(ft)
				ix.Method, ix.FullText = "fulltext", &ft
				break
			}
		case p.Expr != "":
			ft, ok := parseIndexExpression(p.Expr)
			if !ok {
				continue
			}
			ix.Method, ix.FullText = "fulltext", &ft
			ix.Parts = []schema.IndexPart{{Expr: expressionIndexExpr(ft)}}
		}
	}
}

// fullTextGenerated reports whether column is the generated tsvector column of
// a stored-form full-text index of t.
func fullTextGenerated(t *schema.Table, column string) bool {
	for _, ix := range t.Indexes {
		if ix.FullText != nil && !ix.FullText.Expression && schemautil.TSVectorColumn(ix.Name) == column {
			return true
		}
	}
	return false
}
