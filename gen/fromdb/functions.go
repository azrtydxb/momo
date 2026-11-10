// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// functions declares the database functions. With introspect.functions: all
// every function goes on the first model. Otherwise a function is written only
// when a modeled table uses it, on the model of the first table (in model
// order) that does: the functions its triggers, column defaults, generated
// expressions, checks, index expressions and predicates and policies call, and
// the functions those functions call in turn. A view is a global object that
// lives on the first model, so the functions a view calls go there too, as do
// those of a trigger or policy whose table has no model. A function nothing
// modeled uses is not written, so the models do not carry database-wide SQL
// that has nothing to do with them and their text does not change when an
// unrelated function does.
func (g *generator) functions(host *model) {
	fns := g.clean.Functions
	if g.o.Config.Functions == "all" {
		for _, f := range fns {
			host.global("Function", f, g, f.Name)
		}
		return
	}
	claimed := make([]*model, len(fns))
	claim := func(m *model, seeds []string) {
		queue := seeds
		for len(queue) > 0 {
			text := queue[0]
			queue = queue[1:]
			for i := range fns {
				if claimed[i] == nil && callsFunction(text, fns[i].Name) {
					claimed[i] = m
					queue = append(queue, fns[i].Body)
				}
			}
		}
	}
	for _, m := range g.models {
		claim(m, g.tableTexts(m))
	}
	var viewTexts []string
	for _, v := range g.clean.Views {
		viewTexts = append(viewTexts, v.Query)
	}
	// A trigger or policy whose table has no model sits on the first model too.
	for _, x := range g.clean.Triggers {
		if g.owner(x.Schema, x.Table, nil) == nil {
			viewTexts = append(viewTexts, x.Function, x.When, x.Definition)
		}
	}
	for _, x := range g.clean.Policies {
		if g.owner(x.Schema, x.Table, nil) == nil {
			viewTexts = append(viewTexts, x.Using, x.Check)
		}
	}
	claim(host, viewTexts)
	var kept []schema.Function
	for i, f := range fns {
		if claimed[i] != nil {
			kept = append(kept, f)
		}
	}
	for _, m := range g.models {
		for i, f := range fns {
			if claimed[i] == m {
				m.global("Function", f, g, f.Name)
			}
		}
	}
	g.clean.Functions = kept
}

// tableTexts are the SQL texts of a modeled table that can call a function.
func (g *generator) tableTexts(m *model) []string {
	t := m.t
	var out []string
	for _, c := range t.Columns {
		out = append(out, c.Default, c.Generated)
	}
	for _, c := range t.Checks {
		out = append(out, c.Expr)
	}
	for _, ix := range t.Indexes {
		out = append(out, ix.Where)
		for _, p := range ix.Parts {
			out = append(out, p.Expr)
		}
	}
	for _, x := range g.clean.Triggers {
		if g.owner(x.Schema, x.Table, nil) == m {
			out = append(out, x.Function, x.When, x.Definition)
			out = append(out, x.Args...)
		}
	}
	for _, x := range g.clean.Policies {
		if g.owner(x.Schema, x.Table, nil) == m {
			out = append(out, x.Using, x.Check)
		}
	}
	return out
}

// callsFunction reports whether text calls the function name: the name, quoted
// or not and optionally schema qualified, directly followed by an opening
// parenthesis. It errs towards writing a function that is not needed (a call
// in a comment or a string counts) rather than leaving one out.
func callsFunction(text, name string) bool {
	if text == "" || name == "" {
		return false
	}
	text, name = strings.ToLower(text), strings.ToLower(name)
	for from := 0; ; {
		i := strings.Index(text[from:], name)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(name)
		from = start + 1
		if start > 0 && identByte(text[start-1]) {
			continue
		}
		if end < len(text) && text[end] == '"' {
			end++
		}
		for end < len(text) && (text[end] == ' ' || text[end] == '\t' || text[end] == '\n' || text[end] == '\r') {
			end++
		}
		if end < len(text) && text[end] == '(' {
			return true
		}
	}
}

// identByte reports whether b can be part of an unquoted identifier.
func identByte(b byte) bool {
	return b == '_' || b == '$' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 0x80
}
