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
