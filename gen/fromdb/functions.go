// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"regexp"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// functions declares the database functions. With introspect.functions: all
// every function goes on the first model. Otherwise a function is written only
// when a modeled table uses it, on the model of the first table (in model
// order) that does: the functions its triggers, column defaults, generated
// expressions, checks, index expressions and predicates and policies call, and
// the functions those functions call in turn (their bodies and argument
// defaults), whether by a call or by a regproc or regprocedure literal. SQL
// built at run time, such as EXECUTE with a name put together from pieces,
// cannot be followed: a function that only such code reaches is not written, so
// use introspect.functions: all for a database that does that. A view is a global object that
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
				if claimed[i] == nil && usesFunction(text, fns[i].Name) {
					claimed[i] = m
					// A function's argument defaults can call functions too.
					queue = append(queue, fns[i].Body, fns[i].Args)
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

// usesFunction reports whether text refers to the function name: a call, or a
// regproc or regprocedure literal that names it.
func usesFunction(text, name string) bool {
	return callsFunction(text, name) || regprocNames(text)[strings.ToLower(name)]
}

// regprocLiteral matches a single-quoted literal cast to regproc or
// regprocedure, with or without the pg_catalog qualifier. toRegprocCall matches
// to_regproc('name') and to_regprocedure('name(args)'). The literal may be
// schema qualified and hold quoted identifiers. Not recognized: a name built at
// run time, an escape-string or dollar-quoted literal.
var (
	regprocLiteral = regexp.MustCompile(`(?is)'((?:[^']|'')*)'\s*::\s*(?:pg_catalog\s*\.\s*)?regproc(?:edure)?\b`)
	toRegprocCall  = regexp.MustCompile(`(?is)\bto_regproc(?:edure)?\s*\(\s*'((?:[^']|'')*)'`)
)

// regprocNames returns the lower-cased bare function names that text names in
// regproc and regprocedure literals.
func regprocNames(text string) map[string]bool {
	out := map[string]bool{}
	for _, re := range []*regexp.Regexp{regprocLiteral, toRegprocCall} {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			out[bareFunctionName(strings.ReplaceAll(m[1], "''", "'"))] = true
		}
	}
	return out
}

// bareFunctionName is the lower-cased last identifier of a function reference
// such as `public."My Fn"(integer)`: quotes are honoured (a dot inside quotes
// belongs to the name) and the argument list is dropped.
func bareFunctionName(ref string) string {
	var cur strings.Builder
	quoted := false
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		switch {
		case quoted && c == '"' && i+1 < len(ref) && ref[i+1] == '"':
			cur.WriteByte('"')
			i++
		case quoted && c == '"':
			quoted = false
		case quoted:
			cur.WriteByte(c)
		case c == '"':
			quoted = true
		case c == '.':
			cur.Reset()
		case c == '(':
			return strings.ToLower(strings.TrimSpace(cur.String()))
		default:
			cur.WriteByte(c)
		}
	}
	return strings.ToLower(strings.TrimSpace(cur.String()))
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
