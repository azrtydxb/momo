// SPDX-License-Identifier: Apache-2.0

package naming

import "strings"

// initialisms are the acronyms gorm and ent split off as their own word, so
// HTTPURL becomes http_url although a plain acronym run is otherwise one word.
// Longer entries precede their prefixes (HTTPS before HTTP).
var initialisms = []string{
	"ASCII", "HTTPS", "HTML", "HTTP", "JSON", "SMTP", "UUID", "GUID", "UTF8",
	"XSRF", "API", "CPU", "CSS", "DNS", "EOF", "LHS", "QPS", "RAM", "RHS",
	"RPC", "SLA", "SSH", "TLS", "TTL", "UID", "URI", "URL", "XML", "XSS",
	"ID", "IP", "UI", "VM",
}

// snakeInitialisms is ToSnake after title-casing each known initialism that
// ends at a word boundary, which makes the acronym a separate word.
func snakeInitialisms(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		matched := false
		for _, in := range initialisms {
			end := i + len(in)
			if strings.HasPrefix(s[i:], in) && (end == len(s) || !isLowerByte(s[end])) {
				b.WriteString(in[:1] + strings.ToLower(in[1:]))
				i = end
				matched = true
				break
			}
		}
		if !matched {
			b.WriteByte(s[i])
			i++
		}
	}
	return ToSnake(b.String())
}

func isLowerByte(c byte) bool { return c >= 'a' && c <= 'z' }
