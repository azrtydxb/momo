// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"regexp"
	"strings"
	"sync"
)

// urlPatterns holds the compiled expressions of RedactURLs. They compile on
// the first call, not at package init, because the root package imports this
// one and most programs never redact a message.
type urlPatterns struct {
	userinfo *regexp.Regexp
	inText   *regexp.Regexp
	// kvPassword matches a key=value DSN password: password=x, password='a b',
	// password='it\'s' and environment-style names such as PGPASSWORD=x.
	kvPassword *regexp.Regexp
	// kvNextKey matches the next key=value pair that an empty value, as in
	// "password= host=h", would otherwise swallow; kvPlaceholder matches a bind
	// placeholder ($1 or ?), which is not a secret.
	kvNextKey     *regexp.Regexp
	kvPlaceholder *regexp.Regexp
	// mysqlDSN matches the go-sql-driver form user:password@tcp(host:port)/db.
	mysqlDSN *regexp.Regexp
}

var patterns = sync.OnceValue(func() *urlPatterns {
	return &urlPatterns{
		userinfo:      regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/?#\s]*@`),
		inText:        regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mysql|mariadb)://[^\s"']+`),
		kvPassword:    regexp.MustCompile(`(?i)\b(\w*(?:password|passwd|pwd))(\s*=)(\s*)('(?:\\.|[^'\\])*'|"(?:\\.|[^"\\])*"|[^\s'"&]+)`),
		kvNextKey:     regexp.MustCompile(`^\w+=`),
		kvPlaceholder: regexp.MustCompile(`^(?:\$\d+|\?)$`),
		mysqlDSN:      regexp.MustCompile(`([^\s:/@"']+):([^\s"']*)@([A-Za-z][A-Za-z0-9]*\()`),
	}
})

// RedactURLs removes passwords and DSNs from msg: every secret given, the
// userinfo of any URL, any URL-looking text that carries a query, and the value
// of a password= key in a key=value DSN. The migration engine applies it to the
// error text it stores in the history table, and the momo CLI to everything it
// prints.
//
// It is best effort. Known limits: a URL password that contains a raw "/" or a
// space ends the match early and the tail stays visible (percent-encode it), and
// a MySQL DSN password containing a space or a quote is not matched.
func RedactURLs(msg string, secrets ...string) string {
	for _, s := range secrets {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "***")
		}
	}
	p := patterns()
	msg = p.userinfo.ReplaceAllString(msg, "${1}***@")
	msg = p.inText.ReplaceAllStringFunc(msg, func(m string) string {
		if i := strings.IndexAny(m, "?"); i >= 0 {
			return m[:i] + "?***"
		}
		return m
	})
	msg = p.mysqlDSN.ReplaceAllString(msg, "${1}:***@${3}")
	return p.kvPassword.ReplaceAllStringFunc(msg, func(m string) string {
		g := p.kvPassword.FindStringSubmatch(m)
		key, eq, space, val := g[1], g[2], g[3], g[4]
		if p.kvPlaceholder.MatchString(val) || space != "" && p.kvNextKey.MatchString(val) {
			return m
		}
		return key + eq + space + "***"
	})
}
