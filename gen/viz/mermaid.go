// SPDX-License-Identifier: Apache-2.0

package viz

import (
	"strings"
	"unicode"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/schema"
)

// Mermaid renders tables and edges as a Mermaid erDiagram. The first line is
// the generated-file header as a Mermaid comment, then erDiagram.
//
// Each table is an entity with one attribute line per column, the SQL type
// (reduced to letters, digits and underscores, so vector(1536) is
// vector_1536), the column name and its PK, FK and UK markers. Mermaid reads
// "A ||--o{ B" as one A to many B, so a OneToOne or OneToMany edge, which
// runs from the child to the parent, is drawn parent first. Tables and edges
// appear in the order given.
func Mermaid(tables []schema.Table, edges []Edge) []byte {
	var b strings.Builder
	b.WriteString("%% " + strings.TrimPrefix(gen.Header(), "// ") + "\n")
	b.WriteString("erDiagram\n")
	for _, t := range tables {
		marks := keyMarks(t)
		b.WriteString("    " + mermaidName(t.Name) + " {\n")
		for _, c := range t.Columns {
			b.WriteString("        " + mermaidType(c.Type) + " " + mermaidType(c.Name))
			if m := marks[c.Name]; len(m) > 0 {
				b.WriteString(" " + strings.Join(m, ", "))
			}
			b.WriteString("\n")
		}
		b.WriteString("    }\n")
	}
	for _, e := range edges {
		left, marker, right := mermaidName(e.To), "||--o{", mermaidName(e.From)
		switch e.Cardinality {
		case OneToOne:
			marker = "||--||"
		case ManyToMany:
			left, marker, right = mermaidName(e.From), "}o--o{", mermaidName(e.To)
		}
		b.WriteString("    " + left + " " + marker + " " + right + " : " + mermaidLabel(e.Label) + "\n")
	}
	return []byte(b.String())
}

// mermaidType reduces a SQL type to the characters Mermaid accepts in an
// attribute type.
func mermaidType(sqlType string) string {
	out := make([]byte, 0, len(sqlType))
	for i := 0; i < len(sqlType); i++ {
		c := sqlType[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	s := strings.TrimRight(string(out), "_")
	if s == "" {
		return "unknown"
	}
	return s
}

// mermaidName returns an entity or attribute name, quoted when it holds a
// character outside letters, digits, underscore and hyphen.
func mermaidName(name string) string {
	name = oneLine(name)
	plain := name != ""
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			plain = false
		}
	}
	if plain {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, "#quot;") + `"`
}

// isLineControl reports whether r is a control character or a Unicode line or
// paragraph separator: anything a parser may treat as a line break.
func isLineControl(r rune) bool {
	return unicode.IsControl(r) || r == ' ' || r == ' '
}

// oneLine replaces every control character and Unicode line separator, which
// could end a Mermaid statement, with a space.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if isLineControl(r) {
			return ' '
		}
		return r
	}, s)
}

func mermaidLabel(label string) string {
	label = oneLine(label)
	return `"` + strings.ReplaceAll(label, `"`, "#quot;") + `"`
}
