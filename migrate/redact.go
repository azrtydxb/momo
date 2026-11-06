// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"regexp"
	"strings"
)

var (
	urlUserinfo = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/?#\s]*@`)
	urlInText   = regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mysql|mariadb)://[^\s"']+`)
	// kvPassword matches a key=value DSN password: password=x, password='a b'.
	kvPassword = regexp.MustCompile(`(?i)\b(password|passwd|pwd)(\s*=\s*)('[^']*'|"[^"]*"|[^\s'"&]+)`)
)

// RedactURLs removes passwords and DSNs from msg: every secret given, the
// userinfo of any URL, any URL-looking text that carries a query, and the value
// of a password= key in a key=value DSN. The engine applies it to the error
// text it stores in the history table, and the momo CLI to everything it
// prints.
func RedactURLs(msg string, secrets ...string) string {
	for _, s := range secrets {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "***")
		}
	}
	msg = urlUserinfo.ReplaceAllString(msg, "${1}***@")
	msg = urlInText.ReplaceAllStringFunc(msg, func(m string) string {
		if i := strings.IndexAny(m, "?"); i >= 0 {
			return m[:i] + "?***"
		}
		return m
	})
	return kvPassword.ReplaceAllString(msg, "${1}${2}***")
}
