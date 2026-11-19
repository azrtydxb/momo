// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// plainIdent matches an unquoted identifier as pg_get_constraintdef writes it:
// it quotes everything else.
var plainIdent = regexp.MustCompile(`^[a-z_][a-z0-9_$]*$`)

// excludeFromDef reads an exclusion constraint from the text
// pg_get_constraintdef returns for it, such as
//
//	EXCLUDE USING gist (room_id WITH =, during WITH &&)
//
// The deferral flags come from pg_constraint, not from the text. Only what the
// schema model holds is accepted: plain columns, each with its operator and the
// default operator class and ordering. An expression, an operator class, an
// ordering, a WHERE predicate, an INCLUDE list or a storage parameter is
// refused with an error that names the constraint, because skipping it would
// let Baseline and drift report a database that does not match its model.
func excludeFromDef(table, name, def string, deferrable, deferred bool) (schema.Exclude, error) {
	refuse := func(why string) (schema.Exclude, error) {
		return schema.Exclude{}, fmt.Errorf("exclusion constraint %q of table %q is not modelled (%s): momo models EXCLUDE USING <method> (<column> WITH <operator>, ...) over plain columns; declare it outside momo and exclude the table from inspection (introspect exclude) to proceed; the constraint is %s", name, table, why, def)
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(def), "EXCLUDE USING ")
	if !ok {
		return refuse("it does not start with EXCLUDE USING")
	}
	method, rest, ok := strings.Cut(rest, " ")
	if !ok || !excludeMethods[method] {
		return refuse("its index method is not gist, spgist, btree or hash")
	}
	rest = strings.TrimSpace(rest)
	end := closingParen(rest)
	if end < 0 {
		return refuse("its element list cannot be read")
	}
	// What follows the element list: nothing, or the deferral words the flags
	// already carry. A WHERE predicate, INCLUDE or WITH list is something else.
	for _, w := range strings.Fields(rest[end+1:]) {
		switch w {
		case "DEFERRABLE", "NOT", "INITIALLY", "DEFERRED", "IMMEDIATE":
		default:
			return refuse("it has a " + w + " clause")
		}
	}
	x := schema.Exclude{Name: name, Method: method, Deferrable: deferrable, InitiallyDeferred: deferred}
	for _, el := range splitTopLevel(rest[1:end]) {
		i := strings.LastIndex(el, " WITH ")
		if i < 0 {
			return refuse("the element " + strings.TrimSpace(el) + " has no operator")
		}
		col, ok := identName(strings.TrimSpace(el[:i]))
		if !ok {
			return refuse("the element " + strings.TrimSpace(el) + " is not a plain column with the default operator class and ordering")
		}
		op := strings.TrimSpace(el[i+len(" WITH "):])
		if !operatorText.MatchString(op) {
			return refuse("the operator " + op + " is not a plain operator")
		}
		x.Elements = append(x.Elements, schema.ExcludeElement{Column: col, Operator: op})
	}
	if len(x.Elements) == 0 {
		return refuse("it has no elements")
	}
	return x, nil
}

// identName reads s as one identifier: an unquoted lower-case name or a quoted
// one with "" for a quote. ok is false for anything else (an expression, or a
// name followed by an operator class or an ordering).
func identName(s string) (string, bool) {
	if plainIdent.MatchString(s) {
		return s, true
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		// Every quote inside must be doubled, or the string is more than one
		// token.
		if strings.Contains(strings.ReplaceAll(inner, `""`, ""), `"`) {
			return "", false
		}
		return strings.ReplaceAll(inner, `""`, `"`), true
	}
	return "", false
}

// closingParen returns the index of the parenthesis that closes the one s starts
// with, or -1. Double-quoted identifiers and single-quoted strings are skipped.
func closingParen(s string) int {
	if s == "" || s[0] != '(' {
		return -1
	}
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopLevel splits s at the commas that are not inside parentheses or
// quotes.
func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
