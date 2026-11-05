// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// typeStartWords begin a multi-word type name, so a leading word from this
// set is part of the type and not an argument name.
var typeStartWords = map[string]bool{
	"double": true, "character": true, "timestamp": true, "time": true,
	"bit": true, "national": true, "interval": true,
}

var argModes = map[string]string{"in": "", "out": "OUT", "inout": "INOUT", "variadic": "VARIADIC"}

// normalizeFuncArgs canonicalizes a pg_get_function_arguments style list:
// every argument type goes through NormalizeType, argument names are kept
// and the default IN mode is dropped, so "x int" and "IN x integer" match.
func normalizeFuncArgs(d dialect.Name, args string) string {
	if strings.TrimSpace(args) == "" {
		return ""
	}
	parts := splitTopLevel(args)
	for i, p := range parts {
		parts[i] = normalizeFuncArg(d, p)
	}
	return strings.Join(parts, ", ")
}

// normalizeFuncResult canonicalizes a pg_get_function_result text:
// "SETOF t", "TABLE(name type, ...)" or a plain type.
func normalizeFuncResult(d dialect.Name, r string) string {
	r = strings.TrimSpace(r)
	if rest, ok := cutPrefixFold(r, "setof "); ok {
		return "SETOF " + NormalizeType(d, rest)
	}
	if rest, ok := cutPrefixFold(r, "table("); ok && strings.HasSuffix(rest, ")") {
		return "TABLE(" + normalizeFuncArgs(d, strings.TrimSuffix(rest, ")")) + ")"
	}
	return NormalizeType(d, r)
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

func normalizeFuncArg(d dialect.Name, a string) string {
	a = strings.TrimSpace(a)
	def := ""
	if i := topLevelDefault(a); i >= 0 {
		def = a[i:]
		a = strings.TrimSpace(a[:i])
	}
	mode := ""
	if first, rest, ok := strings.Cut(a, " "); ok {
		if m, isMode := argModes[strings.ToLower(first)]; isMode {
			mode, a = m, strings.TrimSpace(rest)
		}
	}
	name := ""
	if strings.HasPrefix(a, `"`) {
		if end := strings.Index(a[1:], `"`); end >= 0 && end+2 < len(a) {
			name, a = a[:end+2], strings.TrimSpace(a[end+2:])
		}
	} else if first, rest, ok := strings.Cut(a, " "); ok && !typeStartWords[strings.ToLower(first)] {
		name, a = first, strings.TrimSpace(rest)
	}
	out := NormalizeType(d, a)
	if name != "" {
		out = name + " " + out
	}
	if mode != "" {
		out = mode + " " + out
	}
	if def != "" {
		kw, expr := "=", strings.TrimPrefix(def, "=")
		if rest, ok := cutPrefixFold(def, "default"); ok {
			kw, expr = "DEFAULT", rest
		}
		out += " " + kw + " " + NormalizeExpr(d, expr)
	}
	return out
}

// topLevelDefault returns the index of a DEFAULT keyword or "=" outside
// parentheses and quotes, or -1.
func topLevelDefault(s string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case depth == 0 && c == '=':
			return i
		case depth == 0 && (c == 'd' || c == 'D') && i > 0 && s[i-1] == ' ' &&
			len(s) >= i+8 && strings.EqualFold(s[i:i+7], "default") && s[i+7] == ' ':
			return i
		}
	}
	return -1
}

// splitTopLevel splits on commas outside parentheses and quotes.
func splitTopLevel(s string) []string {
	var parts []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == ',' && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}
