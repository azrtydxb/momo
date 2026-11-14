// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"github.com/azrtydxb/momo/dialect"
)

var postgresTypes = map[string]string{
	"int":              "integer",
	"int4":             "integer",
	"integer":          "integer",
	"int8":             "bigint",
	"bigint":           "bigint",
	"int2":             "smallint",
	"smallint":         "smallint",
	"bool":             "boolean",
	"boolean":          "boolean",
	"varchar":          "character varying",
	"float8":           "double precision",
	"double precision": "double precision",
	"float4":           "real",
	"real":             "real",
	"decimal":          "numeric",
	"char":             "character",
}

// NormalizeType returns the canonical spelling of a SQL type so that the
// declared form and the form reported by a database compare equal. It is
// idempotent.
func NormalizeType(d dialect.Name, t string) string {
	s := strings.Join(strings.Fields(strings.ToLower(t)), " ")
	if elem, ok := strings.CutSuffix(s, "[]"); ok && d == dialect.Postgres {
		return NormalizeType(d, elem) + "[]"
	}
	base, args, rest := s, "", ""
	if i := strings.IndexByte(s, '('); i >= 0 {
		var balanced bool
		base = strings.TrimSpace(s[:i])
		args, rest, balanced = splitArgs(s[i:])
		if !balanced {
			// Not a type with an argument list: keep the collapsed text, so
			// a second pass cannot read appended words as arguments.
			if d == dialect.SQLite {
				return strings.ToUpper(s)
			}
			return s
		}
	}
	if d == dialect.Postgres {
		if out, ok := postgresTime(base, args, rest); ok {
			return out
		}
		if m, ok := postgresTypes[base]; ok {
			base = m
		}
	}
	if d == dialect.MySQL {
		base, args = mysqlType(base, args)
	}
	if rest != "" {
		rest = " " + rest
	}
	out := base + args + rest
	if d == dialect.SQLite {
		out = strings.ToUpper(out)
	}
	return out
}

// splitArgs splits "(args) rest" into the parenthesised argument list, with
// whitespace inside it removed, and the trimmed text after the closing
// parenthesis. An unbalanced list is reported with balanced false.
func splitArgs(s string) (args, rest string, balanced bool) {
	var sb strings.Builder
	depth := 0
	for i, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		case r == ' ' && depth > 0:
			continue
		}
		sb.WriteRune(r)
		if depth == 0 {
			return sb.String(), strings.TrimSpace(s[i+1:]), true
		}
	}
	return sb.String(), "", false
}

// postgresTime canonicalises the timestamp and time families, whose optional
// precision sits between the type name and the time zone clause:
// timestamp(3) with time zone.
func postgresTime(base, args, rest string) (string, bool) {
	head, tail, _ := strings.Cut(base, " ")
	if rest != "" {
		tail = strings.TrimSpace(tail + " " + rest)
	}
	var zone string
	switch head {
	case "timestamp", "time":
		zone = "without time zone"
	case "timestamptz", "timetz":
		zone = "with time zone"
		head = strings.TrimSuffix(head, "tz")
	default:
		return "", false
	}
	if tail != "" {
		if tail != "with time zone" && tail != "without time zone" {
			return "", false
		}
		zone = tail
	}
	return head + args + " " + zone, true
}

// NormalizeExpr returns the canonical spelling of a SQL expression: outside
// quoted text whitespace is collapsed and letters are lowercased, and
// redundant enclosing parentheses are removed. Quoted text is kept verbatim.
// It is idempotent.
func NormalizeExpr(d dialect.Name, e string) string {
	for {
		n := normalizeExprOnce(d, e)
		if n == e {
			return n
		}
		e = n
	}
}

func normalizeExprOnce(d dialect.Name, e string) string {
	var sb strings.Builder
	var quote rune
	escapes := false // inside an E'' string (or any MySQL string), where a backslash escapes the next character
	skip := false
	space := false
	for _, r := range e {
		if quote != 0 {
			sb.WriteRune(r)
			switch {
			case skip:
				skip = false
			case escapes && r == '\\':
				skip = true
			case r == quote:
				quote = 0
			}
			continue
		}
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space {
			if r != ')' && !strings.HasSuffix(sb.String(), "(") {
				sb.WriteByte(' ')
			}
			space = false
		}
		if r == '\'' || r == '"' {
			// MySQL strings, both quote kinds, always take backslash escapes.
			escapes = d == dialect.MySQL || r == '\'' && endsWithEscapePrefix(sb.String())
			quote = r
			sb.WriteRune(r)
			continue
		}
		sb.WriteRune(unicode.ToLower(r))
	}
	s := strings.TrimSpace(sb.String())
	for len(s) >= 2 && s[0] == '(' && enclosedByOnePair(s) {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	if d == dialect.Postgres {
		s = postgresRewrite(s)
	}
	if d == dialect.MySQL {
		s = mysqlRewrite(s)
	}
	return s
}

// mysqlFKAction folds RESTRICT into the default: on MySQL and MariaDB it is
// NO ACTION by another name, and MariaDB reports it for a clause never written.
func mysqlFKAction(a string) string {
	switch n := strings.ToUpper(strings.Join(strings.Fields(a), " ")); n {
	case "RESTRICT", "NO ACTION":
		return ""
	default:
		return n
	}
}

// mysqlAliases maps the alternative spellings of MySQL and MariaDB types to
// the name information_schema reports.
var mysqlAliases = map[string]string{
	"integer": "int", "int1": "tinyint", "int2": "smallint", "int3": "mediumint", "middleint": "mediumint",
	"int4": "int", "int8": "bigint", "float4": "float", "float8": "double", "real": "double",
	"numeric": "decimal", "dec": "decimal", "fixed": "decimal",
}

// mysqlType canonicalises a MySQL or MariaDB type name and its argument list:
// aliases collapse (integer, bool, double precision, ...), and the display width
// of an integer type is dropped, because MySQL 8.0.19 and later no longer
// report it while MariaDB still does. tinyint(1) keeps its width, the spelling
// of a boolean. base may carry words after the type name ("int unsigned").
func mysqlType(base, args string) (string, string) {
	head, tail, _ := strings.Cut(base, " ")
	if m, ok := mysqlAliases[head]; ok {
		head = m
	}
	if head == "bool" || head == "boolean" {
		head, args = "tinyint", "(1)"
	}
	if head == "double" {
		for tail == "precision" || strings.HasPrefix(tail, "precision ") {
			tail = strings.TrimSpace(strings.TrimPrefix(tail, "precision"))
		}
	}
	switch head {
	case "tinyint", "smallint", "mediumint", "int", "bigint":
		if head != "tinyint" || args != "(1)" {
			args = ""
		}
	}
	if tail != "" {
		head += " " + tail
	}
	return head, args
}

// mysqlRewrite undoes spellings the MySQL family substitutes for what was
// declared: it removes identifier quotes and the character set introducers
// (_utf8mb4'x') that the server adds to stored expressions. Text inside
// string literals is left alone.
func mysqlRewrite(s string) string {
	var sb strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			sb.WriteByte(c)
			switch {
			case c == '\\' && i+1 < len(s):
				i++
				sb.WriteByte(s[i])
			case c == quote:
				quote = 0
			}
			continue
		}
		switch {
		case c == '`':
			continue
		case c == ',':
			// The servers print a list without the space after the comma.
			sb.WriteByte(c)
			for i+1 < len(s) && s[i+1] == ' ' {
				i++
			}
			continue
		case c == '\'' || c == '"':
			quote = c
		case c == '_' && (i == 0 || !isWordByte(s[i-1])):
			if n := charsetIntroducer(s[i:]); n > 0 {
				i += n - 1
				continue
			}
		}
		sb.WriteByte(c)
	}
	return mysqlDefaultForms(stripGroups(strings.TrimSpace(sb.String())))
}

var mysqlTimeNow = regexp.MustCompile(`^(?:now|current_timestamp|localtime|localtimestamp)(?:\((\d*)\))?$`)

// mysqlDefaultForms maps the spellings of one default to a single form: the
// servers store NOW(), LOCALTIME and CURRENT_TIMESTAMP alike (MariaDB prints
// current_timestamp() for the bare keyword), and TRUE and FALSE as 1 and 0.
func mysqlDefaultForms(s string) string {
	switch s {
	case "true":
		return "1"
	case "false":
		return "0"
	}
	if m := mysqlTimeNow.FindStringSubmatch(s); m != nil {
		if m[1] == "" || m[1] == "0" {
			return "current_timestamp"
		}
		return "current_timestamp(" + m[1] + ")"
	}
	return s
}

// stripGroups removes parentheses the server puts around the parts of a stored
// CHECK expression, which no model writes: "((a > 1) and (b in (1,2)))" and
// "a > 1 and b in (1,2)" are one expression. It only removes a pair that holds
// a bare comparison (no and/or/xor/not, no comma at its top level), is not a
// function call or an IN list, and sits between nothing, an opening
// parenthesis or and/or/not on its left and nothing, a closing parenthesis or
// and/or/xor on its right; in those places a comparison binds tighter than
// its neighbours, so the meaning cannot change.
func stripGroups(s string) string {
	for {
		t, ok := stripOneGroup(s)
		if !ok {
			return s
		}
		s = strings.TrimSpace(t)
	}
}

func stripOneGroup(s string) (string, bool) {
	var stack []int
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			switch {
			case c == '\\' && i+1 < len(s):
				i++
			case c == quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(':
			stack = append(stack, i)
		case ')':
			if len(stack) == 0 {
				return s, false
			}
			open := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if groupStrippable(s, open, i) {
				return s[:open] + s[open+1:i] + s[i+1:], true
			}
		}
	}
	return s, false
}

func groupStrippable(s string, open, end int) bool {
	if open > 0 && isWordByte(s[open-1]) {
		return false // f(...)
	}
	p := open - 1
	for p >= 0 && s[p] == ' ' {
		p--
	}
	if p >= 0 && s[p] != '(' {
		// NOT is not accepted: with HIGH_NOT_PRECEDENCE it binds tighter than a
		// comparison, so "not (a > 1)" and "not a > 1" differ there.
		switch wordEndingAt(s, p) {
		case "and":
			if betweenAnd(s, open) {
				return false // the AND of BETWEEN x AND (...)
			}
		case "or", "xor":
		default:
			return false
		}
	}
	q := end + 1
	for q < len(s) && s[q] == ' ' {
		q++
	}
	if q < len(s) && s[q] != ')' {
		switch wordStartingAt(s, q) {
		case "and", "or", "xor":
		default:
			return false
		}
	}
	return hasBareComparison(s[open+1 : end])
}

// betweenAnd reports whether the last word before s[open] is the AND that
// closes a BETWEEN of the same parenthesis level.
func betweenAnd(s string, open int) bool {
	pending := []int{0}
	last := false
	var quote byte
	for i := 0; i < open; i++ {
		c := s[i]
		if quote != 0 {
			switch {
			case c == '\\' && i+1 < open:
				i++
			case c == quote:
				quote = 0
			}
			continue
		}
		switch {
		case c == ' ':
		case c == '\'' || c == '"':
			quote = c
			last = false
		case c == '(':
			pending = append(pending, 0)
			last = false
		case c == ')':
			if len(pending) > 1 {
				pending = pending[:len(pending)-1]
			}
			last = false
		case isWordByte(c):
			if i > 0 && isWordByte(s[i-1]) {
				continue
			}
			top := len(pending) - 1
			switch wordStartingAt(s, i) {
			case "between":
				pending[top]++
				last = false
			case "and":
				last = pending[top] > 0
				if last {
					pending[top]--
				}
			default:
				last = false
			}
		default:
			last = false
		}
	}
	return last
}

func wordEndingAt(s string, p int) string {
	i := p
	for i >= 0 && isWordByte(s[i]) {
		i--
	}
	return s[i+1 : p+1]
}

func wordStartingAt(s string, q int) string {
	j := q
	for j < len(s) && isWordByte(s[j]) {
		j++
	}
	return s[q:j]
}

// hasBareComparison reports whether the top level of e (outside parentheses
// and quotes) holds a comparison and none of: a comma, a logical operator.
func hasBareComparison(e string) bool {
	depth, cmp := 0, false
	var quote byte
	for i := 0; i < len(e); i++ {
		c := e[i]
		if quote != 0 {
			switch {
			case c == '\\' && i+1 < len(e):
				i++
			case c == quote:
				quote = 0
			}
			continue
		}
		switch {
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case depth != 0:
		case c == ',' || c == '|' || c == '&':
			return false
		case c == '=' || c == '<' || c == '>' || c == '!':
			cmp = true
		case isWordByte(c) && (i == 0 || !isWordByte(e[i-1])):
			switch wordStartingAt(e, i) {
			case "and", "or", "xor", "not":
				return false
			case "in", "like", "is", "regexp", "rlike", "between":
				cmp = true
			}
		}
	}
	return cmp
}

func isWordByte(c byte) bool {
	return c == '_' || c == '$' || c >= 0x80 || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// charsetIntroducer returns the length of a character set introducer such as
// _utf8mb4 at the start of s when a string literal follows it, otherwise 0.
func charsetIntroducer(s string) int {
	for _, cs := range []string{"_utf8mb4", "_utf8mb3", "_utf8", "_latin1", "_binary"} {
		if strings.HasPrefix(s, cs) && len(s) > len(cs) && s[len(cs)] == '\'' {
			return len(cs)
		}
	}
	return 0
}

// postgresRewrite undoes spellings PostgreSQL substitutes for what was
// declared: CURRENT_TIMESTAMP and the legacy 'now'::text default are reported
// as now(), and every string literal gets a ::type cast appended, which is
// dropped here.
func postgresRewrite(s string) string {
	switch s {
	case "current_timestamp", "'now'::text":
		return "now()"
	}
	return stripLiteralCasts(s)
}

// stripLiteralCasts removes a ::type cast that directly follows a
// single-quoted literal. The type is a name with optional multi-word
// suffixes, a length list and array brackets.
func stripLiteralCasts(s string) string {
	var sb strings.Builder
	inDouble := false
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case inDouble:
			inDouble = c != '"'
		case c == '"':
			inDouble = true
		case c == '\'':
			start := i
			escapes := endsWithEscapePrefix(s[:i])
			j := i + 1
			for j < len(s) && s[j] != '\'' {
				if escapes && s[j] == '\\' {
					j++
				}
				j++
			}
			if j < len(s) {
				j++
			}
			sb.WriteString(s[start:j])
			i = j
			if strings.HasPrefix(s[j:], "::") && j-start > 1 && s[j-1] == '\'' {
				i = j + 2 + castTypeLen(s[j+2:])
			}
			continue
		}
		sb.WriteByte(c)
		i++
	}
	return sb.String()
}

// endsWithEscapePrefix reports whether s ends with the E (or e) that turns the
// following quote into an escape string literal, as opposed to the last letter
// of an identifier.
func endsWithEscapePrefix(s string) bool {
	n := len(s)
	if n == 0 || s[n-1] != 'e' && s[n-1] != 'E' {
		return false
	}
	if n == 1 {
		return true
	}
	c := s[n-2]
	return !(c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c >= 0x80)
}

// castTypeLen returns the length of the type name at the start of s.
func castTypeLen(s string) int {
	n := 0
	isWord := func(c byte) bool {
		return c == '_' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
	}
	for n < len(s) && isWord(s[n]) {
		n++
	}
	for _, suffix := range []string{" varying", " precision", " with time zone", " without time zone"} {
		if strings.HasPrefix(s[n:], suffix) {
			n += len(suffix)
			break
		}
	}
	if n < len(s) && s[n] == '(' {
		if k := strings.IndexByte(s[n:], ')'); k >= 0 {
			n += k + 1
		}
	}
	for strings.HasPrefix(s[n:], "[]") {
		n += 2
	}
	return n
}

// enclosedByOnePair reports whether the opening parenthesis at s[0] closes
// at the very end of s.
func enclosedByOnePair(s string) bool {
	var quote byte
	depth := 0
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
				return i == len(s)-1
			}
		}
	}
	return false
}

// Equal reports whether two schemas are equivalent: order of named
// collections, the Concurrently migration option, and the spelling of types
// and expressions, and FormatVersion (a file-format marker) are ignored. An
// extension version counts only when both schemas set it: an empty version
// means "whatever the server installs". Neither argument is modified.
func Equal(a, b *Schema) bool {
	na, err := normalizedCopy(a)
	if err != nil {
		return false
	}
	nb, err := normalizedCopy(b)
	if err != nil {
		return false
	}
	na.FormatVersion, nb.FormatVersion = 0, 0
	for i := range na.Extensions {
		for j := range nb.Extensions {
			x, y := &na.Extensions[i], &nb.Extensions[j]
			if x.Name == y.Name && (x.Version == "" || y.Version == "") {
				x.Version, y.Version = "", ""
			}
		}
	}
	ca, err := json.Marshal(na)
	if err != nil {
		return false
	}
	cb, err := json.Marshal(nb)
	if err != nil {
		return false
	}
	return bytes.Equal(ca, cb)
}

// Normalized returns a deep copy of s with every type and expression
// rewritten to its canonical spelling, the Concurrently migration option
// cleared and the collections sorted. It is idempotent and does not modify s.
// A schema that cannot be JSON-encoded yields an empty schema; Equal reports
// such schemas as unequal.
func Normalized(s *Schema) *Schema {
	c, err := normalizedCopy(s)
	if err != nil {
		return &Schema{}
	}
	return c
}

func normalizedCopy(s *Schema) (*Schema, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var c Schema
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	d := c.Dialect
	for i := range c.Tables {
		t := &c.Tables[i]
		for j := range t.Columns {
			col := &t.Columns[j]
			col.Type = NormalizeType(d, col.Type)
			col.Default = NormalizeExpr(d, col.Default)
			col.Generated = NormalizeExpr(d, col.Generated)
		}
		for j := range t.Checks {
			t.Checks[j].Expr = NormalizeExpr(d, t.Checks[j].Expr)
		}
		if d == dialect.MySQL {
			for j := range t.ForeignKeys {
				fk := &t.ForeignKeys[j]
				fk.OnDelete, fk.OnUpdate = mysqlFKAction(fk.OnDelete), mysqlFKAction(fk.OnUpdate)
			}
		}
		normalizeIndexes(d, t.Columns, t.Indexes)
		if d == dialect.MySQL {
			foldMySQLUniques(t)
		}
		if d == dialect.SQLite {
			normalizeSQLiteIdentity(t)
		}
	}
	for i := range c.Views {
		v := &c.Views[i]
		v.Query = NormalizeExpr(d, v.Query)
		normalizeIndexes(d, nil, v.Indexes)
	}
	for i := range c.Functions {
		c.Functions[i].Body = NormalizeExpr(d, c.Functions[i].Body)
		c.Functions[i].Args = normalizeFuncArgs(d, c.Functions[i].Args)
		c.Functions[i].Returns = normalizeFuncResult(d, c.Functions[i].Returns)
	}
	for i := range c.Triggers {
		c.Triggers[i].When = NormalizeExpr(d, c.Triggers[i].When)
		c.Triggers[i].Definition = normalizeDefinition(d, c.Triggers[i].Definition)
	}
	for i := range c.Policies {
		c.Policies[i].Using = NormalizeExpr(d, c.Policies[i].Using)
		c.Policies[i].Check = NormalizeExpr(d, c.Policies[i].Check)
	}
	c.Sort()
	return &c, nil
}

// foldMySQLUniques gives a unique key of MySQL and MariaDB one spelling. The
// servers keep a UNIQUE constraint and a unique index as the same object, and
// inspection reports it as a Unique when every key part is a plain ascending
// column. A model that declares such a key as a unique Index (the generator's
// unique tag does) would otherwise differ from what the server reports, so it
// is folded into a Unique here. A unique index with an expression, a descending
// or prefixed part, a predicate or an access method stays an Index. It is
// idempotent.
func foldMySQLUniques(t *Table) {
	kept := t.Indexes[:0:0]
	for _, ix := range t.Indexes {
		cols, ok := plainUniqueColumns(ix)
		if !ok {
			kept = append(kept, ix)
			continue
		}
		t.Uniques = append(t.Uniques, Unique{Name: ix.Name, Columns: cols})
	}
	t.Indexes = kept
}

// plainUniqueColumns returns the columns of a unique index whose every part is
// a plain ascending column, and false for any other index.
func plainUniqueColumns(ix Index) ([]string, bool) {
	if !ix.Unique || ix.Where != "" || len(ix.With) > 0 || len(ix.Parts) == 0 || (ix.Method != "" && ix.Method != "btree") {
		return nil, false
	}
	cols := make([]string, len(ix.Parts))
	for i, p := range ix.Parts {
		if p.Column == "" || p.Expr != "" || p.Desc || p.Nulls != "" || p.OpClass != "" || strings.Contains(p.Column, "(") {
			return nil, false
		}
		cols[i] = p.Column
	}
	return cols, true
}

// normalizeSQLiteIdentity gives SQLite's one kind of generated key a single
// spelling. SQLite has no GENERATED ... AS IDENTITY: an INTEGER column that is
// the whole primary key of a rowid table is the rowid, assigned when an insert
// gives no value, which is what "by_default" means. So on SQLite that column is
// "by_default" whether the model said so or not (inspection reports it that
// way), and "always", which SQLite cannot enforce, is "by_default" too. It is
// idempotent.
func normalizeSQLiteIdentity(t *Table) {
	for i := range t.Columns {
		if t.Columns[i].Identity == "always" {
			t.Columns[i].Identity = "by_default"
		}
	}
	if t.WithoutRowID || len(t.PrimaryKey) != 1 {
		return
	}
	for i := range t.Columns {
		col := &t.Columns[i]
		if col.Name == t.PrimaryKey[0] && col.Identity == "" && NormalizeType(dialect.SQLite, col.Type) == "INTEGER" {
			col.Identity = "by_default"
		}
	}
}

// normalizeDefinition canonicalises a SQLite trigger's CREATE TRIGGER text so
// that the text a model declares and the text sqlite_schema stores compare
// equal: whitespace and case outside quotes are normalised as for an
// expression, and a trailing semicolon and IF NOT EXISTS (which SQLite does
// not store) are dropped. Other dialects have no Definition and it is returned
// as it is. It is idempotent.
func normalizeDefinition(d dialect.Name, def string) string {
	if d != dialect.SQLite || def == "" {
		return def
	}
	for {
		n := strings.TrimRight(strings.TrimSpace(NormalizeExpr(d, def)), "; \t\r\n")
		if rest, ok := strings.CutPrefix(n, "create trigger if not exists "); ok {
			n = "create trigger " + rest
		}
		if n == def {
			return n
		}
		def = n
	}
}
