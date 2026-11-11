// SPDX-License-Identifier: Apache-2.0

package naming

import "strings"

// gormInitialisms is the list gorm title-cases before snake-casing, so an
// initialism becomes its own word (HTTPURL becomes http_url).
var gormInitialisms = []string{
	"API", "ASCII", "CPU", "CSS", "DNS", "EOF", "GUID", "HTML", "HTTP", "HTTPS",
	"ID", "IP", "JSON", "LHS", "QPS", "RAM", "RHS", "RPC", "SLA", "SMTP", "SSH",
	"TLS", "TTL", "UID", "UI", "UUID", "URI", "URL", "UTF8", "VM", "XML",
	"XSRF", "XSS",
}

// gormReplacer pairs each initialism with its title-cased form. Like gorm's
// own, it replaces unconditionally and, at one position, prefers the earlier
// pair (HTTP wins over HTTPS), so UserIDs becomes UserIds.
var gormReplacer = newInitialismReplacer(gormInitialisms)

func newInitialismReplacer(list []string) *strings.Replacer {
	pairs := make([]string, 0, 2*len(list))
	for _, in := range list {
		if in == "" { // strings.NewReplacer would match the empty string everywhere
			continue
		}
		pairs = append(pairs, in, titleASCII(in))
	}
	return strings.NewReplacer(pairs...)
}

// titleASCII upper-cases the first byte and lower-cases the rest.
func titleASCII(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.ToUpper(s[:1]))
	b.WriteString(strings.ToLower(s[1:]))
	return b.String()
}

// snakeInitialisms is gorm's snake_case conversion: initialisms are
// title-cased first, then a '_' goes before an upper-case letter that starts a
// word, and digits after an upper-case letter do not split it.
func snakeInitialisms(name string) string {
	if name == "" {
		return ""
	}
	value := gormReplacer.Replace(name)
	var (
		buf                            strings.Builder
		lastCase, nextCase, nextNumber bool // upper case == true
		curCase                        = isUpperByte(value[0])
	)
	for i := 0; i < len(value)-1; i++ {
		v := value[i]
		nextCase = isUpperByte(value[i+1])
		nextNumber = value[i+1] >= '0' && value[i+1] <= '9'
		switch {
		case !curCase:
			buf.WriteByte(v)
		case lastCase && (nextCase || nextNumber):
			buf.WriteByte(v + 32)
		default:
			if i > 0 && value[i-1] != '_' && value[i+1] != '_' {
				buf.WriteByte('_')
			}
			buf.WriteByte(v + 32)
		}
		lastCase = curCase
		curCase = nextCase
	}
	last := value[len(value)-1]
	if curCase {
		if !lastCase && len(value) > 1 {
			buf.WriteByte('_')
		}
		buf.WriteByte(last + 32)
	} else {
		buf.WriteByte(last)
	}
	return buf.String()
}

func isUpperByte(c byte) bool { return c >= 'A' && c <= 'Z' }

// goInitialisms is the word list of the Go-style snake_case and of the
// --from-db field names: a word in it is written in capitals inside a Go name
// (UserID, not UserId), and a plural s after it belongs to it (IDs, URLs).
var goInitialisms = map[string]bool{
	"ACL": true, "API": true, "ASCII": true, "CPU": true, "CSS": true,
	"DNS": true, "EOF": true, "GUID": true, "HTML": true, "HTTP": true,
	"HTTPS": true, "ID": true, "IP": true, "JSON": true, "LHS": true,
	"QPS": true, "RAM": true, "RHS": true, "RPC": true, "SLA": true,
	"SMTP": true, "SQL": true, "SSH": true, "TCP": true, "TLS": true,
	"TTL": true, "UDP": true, "UI": true, "UID": true, "URI": true,
	"URL": true, "UUID": true, "VM": true, "XML": true, "XMPP": true,
	"XSRF": true, "XSS": true,
}

// maxInitialism is the length of the longest word in goInitialisms.
const maxInitialism = 5

// GoInitialism reports whether word, in any letter case, is an initialism
// of the word list (id, URL) or its plural (ids, Urls), and returns its Go
// spelling: ID, URL, IDs, URLs. It is the list ToSnake and the --from-db
// field names share, so a name round-trips between a column and a Go field.
func GoInitialism(word string) (string, bool) {
	up := strings.ToUpper(word)
	if goInitialisms[up] {
		return up, true
	}
	if n := len(up); n > 1 && up[n-1] == 'S' && goInitialisms[up[:n-1]] {
		return up[:n-1] + "s", true
	}
	return "", false
}

// initialismSuffix returns the length of the longest word of the list that
// the upper-case run ends with, or 0.
func initialismSuffix(run []rune) int {
	for n := min(len(run), maxInitialism); n > 0; n-- {
		if goInitialisms[string(run[len(run)-n:])] {
			return n
		}
	}
	return 0
}
