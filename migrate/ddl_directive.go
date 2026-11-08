// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	// ddlClause is what a -- momo:ddl directive may carry: one or more
	// ALGORITHM= and LOCK= options separated by commas. Nothing else reaches
	// the statement, so a directive cannot inject SQL.
	ddlClause = regexp.MustCompile(`(?i)^(?:ALGORITHM|LOCK)\s*=\s*[A-Za-z_]+(?:\s*,\s*(?:ALGORITHM|LOCK)\s*=\s*[A-Za-z_]+)*$`)
	alterStmt = regexp.MustCompile(`(?is)^alter\s+table\b`)
)

// applyDDLDirective applies a `-- momo:ddl <options>` comment found before the
// SQL of one statement (stmt keeps the comments that precede it). The options
// are appended to the ALTER TABLE statement as its trailing alter options:
//
//	-- momo:ddl ALGORITHM=INSTANT, LOCK=NONE
//	ALTER TABLE big ADD COLUMN note varchar(40) NULL;
//
// runs as ALTER TABLE big ADD COLUMN note varchar(40) NULL, ALGORITHM=INSTANT,
// LOCK=NONE, and the server rejects the statement when it cannot honour them.
// A statement without the directive is returned unchanged, and no ALGORITHM or
// LOCK clause is ever added without it. The directive is MySQL and MariaDB
// only, belongs to an ALTER TABLE, and may appear once per statement.
func applyDDLDirective(stmt, dialectName string) (string, error) {
	clause, body, found, err := splitDDLDirective(stmt)
	if err != nil || !found {
		return stmt, err
	}
	if dialectName != "mysql" {
		return "", fmt.Errorf("-- momo:ddl is supported on mysql only, not %s", dialectName)
	}
	if !alterStmt.MatchString(body) {
		return "", errors.New("-- momo:ddl must come directly before an ALTER TABLE statement")
	}
	if !ddlClause.MatchString(clause) {
		return "", fmt.Errorf("-- momo:ddl %q: want ALGORITHM=<name> and/or LOCK=<name>, separated by commas", clause)
	}
	return strings.TrimRight(stmt, " \t\r\n;") + ", " + clause, nil
}

// splitDDLDirective finds the -- momo:ddl comment in the leading trivia of
// stmt and returns its options and the SQL after the trivia.
func splitDDLDirective(stmt string) (clause, body string, found bool, err error) {
	rest := stmt
	for {
		trimmed := strings.TrimLeft(rest, " \t\r\n\uFEFF")
		switch {
		case strings.HasPrefix(trimmed, "/*"):
			end := strings.Index(trimmed, "*/")
			if end < 0 {
				return "", trimmed, found, nil
			}
			rest = trimmed[end+2:]
		case strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "#"):
			line, after, _ := strings.Cut(trimmed, "\n")
			text := strings.TrimSpace(strings.TrimLeft(line, "-#"))
			if opts, ok := strings.CutPrefix(text, "momo:ddl"); ok && (opts == "" || opts[0] == ' ' || opts[0] == '\t') {
				if found {
					return "", "", false, errors.New("-- momo:ddl appears twice before one statement")
				}
				clause, found = strings.TrimSpace(opts), true
			}
			rest = after
		default:
			return clause, trimmed, found, nil
		}
	}
}
