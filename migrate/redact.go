// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"regexp"
	"strings"
)

var (
	urlUserinfo = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^/?#\s]*@`)
	urlInText   = regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mysql|mariadb)://[^\s"']+`)
	// kvPassword matches a key=value DSN password: password=x, password='a b',
	// password='it\'s' and environment-style names such as PGPASSWORD=x.
	kvPassword = regexp.MustCompile(`(?i)\b(\w*(?:password|passwd|pwd))(\s*=\s*)('(?:\\.|[^'\\])*'|"(?:\\.|[^"\\])*"|[^\s'"&]+)`)
	// mysqlDSN matches the go-sql-driver form user:password@tcp(host:port)/db.
	mysqlDSN = regexp.MustCompile(`([^\s:/@"']+):([^\s@"']*)@([A-Za-z][A-Za-z0-9]*\()`)
)

// redactedError shows err's message with URLs and passwords removed while
// Unwrap keeps errors.Is and errors.As working on the original chain.
type redactedError struct{ err error }

func (e redactedError) Error() string { return RedactURLs(e.err.Error()) }
func (e redactedError) Unwrap() error { return e.err }

// redactErr wraps err so that its message is safe to print or log. It returns
// nil for nil and leaves an already wrapped error alone.
func redactErr(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(redactedError); ok {
		return err
	}
	return redactedError{err}
}

// RedactURLs removes passwords and DSNs from msg: every secret given, the
// userinfo of any URL, any URL-looking text that carries a query, and the value
// of a password= key in a key=value DSN. The engine applies it to the error
// text it stores in the history table, and the momo CLI to everything it
// prints.
//
// It is best effort. Known limits: a URL password that contains a raw "/" or a
// space ends the match early and the tail stays visible (percent-encode it), and
// a MySQL DSN password containing "@" is cut at the first "@".
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
	msg = mysqlDSN.ReplaceAllString(msg, "${1}:***@${3}")
	return kvPassword.ReplaceAllString(msg, "${1}${2}***")
}
