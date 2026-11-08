// SPDX-License-Identifier: Apache-2.0

package viz

import (
	"html"
	"strings"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/schema"
)

// DOT renders tables and edges as a Graphviz digraph. The first line is the
// generated-file header as a comment.
//
// Each table is a plaintext node whose HTML-like label is a title row and one
// row per column (name, type, markers); all text is HTML-escaped. An edge
// from the child to the parent carries a crow's foot at the child end and a
// tee at the parent end; a many-to-many edge has a crow's foot and a dot at
// both ends. Tables and edges appear in the order given.
func DOT(tables []schema.Table, edges []Edge) []byte {
	var b strings.Builder
	b.WriteString(gen.Header() + "\n")
	b.WriteString("digraph momo {\n")
	b.WriteString("    rankdir=LR;\n")
	b.WriteString("    node [shape=plaintext];\n")
	for _, t := range tables {
		marks := keyMarks(t)
		b.WriteString("    " + dotID(t.Name) + " [label=<<TABLE BORDER=\"0\" CELLBORDER=\"1\" CELLSPACING=\"0\">")
		b.WriteString("<TR><TD COLSPAN=\"3\"><B>" + html.EscapeString(t.Name) + "</B></TD></TR>")
		for _, c := range t.Columns {
			b.WriteString("<TR><TD ALIGN=\"LEFT\">" + html.EscapeString(c.Name) + "</TD><TD ALIGN=\"LEFT\">" +
				html.EscapeString(c.Type) + "</TD><TD>" + html.EscapeString(strings.Join(marks[c.Name], " ")) + "</TD></TR>")
		}
		b.WriteString("</TABLE>>];\n")
	}
	for _, e := range edges {
		head, tail := "tee", "crow"
		switch e.Cardinality {
		case OneToOne:
			tail = "tee"
		case ManyToMany:
			head, tail = "crowodot", "crowodot"
		}
		b.WriteString("    " + dotID(e.From) + " -> " + dotID(e.To) + " [label=" + dotID(e.Label) +
			", arrowhead=" + head + ", arrowtail=" + tail + ", dir=both];\n")
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

// dotID quotes s as a DOT string.
func dotID(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return `"` + s + `"`
}
