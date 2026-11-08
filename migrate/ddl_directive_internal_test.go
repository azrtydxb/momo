// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"strings"
	"testing"
)

func TestApplyDDLDirective(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"no directive", "ALTER TABLE t ADD COLUMN c int", "ALTER TABLE t ADD COLUMN c int"},
		{"algorithm and lock", "-- momo:ddl ALGORITHM=INSTANT, LOCK=NONE\nALTER TABLE t ADD COLUMN c int",
			"-- momo:ddl ALGORITHM=INSTANT, LOCK=NONE\nALTER TABLE t ADD COLUMN c int\n, ALGORITHM=INSTANT, LOCK=NONE"},
		{"one option", "-- momo:ddl ALGORITHM=INPLACE\nalter table t drop column c",
			"-- momo:ddl ALGORITHM=INPLACE\nalter table t drop column c\n, ALGORITHM=INPLACE"},
		{"other comments around it", "-- why\n-- momo:ddl LOCK=NONE\n/* more */\nALTER TABLE t ADD KEY k (c);",
			"-- why\n-- momo:ddl LOCK=NONE\n/* more */\nALTER TABLE t ADD KEY k (c)\n, LOCK=NONE"},
		{"statement ends in a line comment", "-- momo:ddl LOCK=NONE\nALTER TABLE t ADD c int -- note",
			"-- momo:ddl LOCK=NONE\nALTER TABLE t ADD c int -- note\n, LOCK=NONE"},
		{"other directives are left alone", "-- momo:generated\nCREATE TABLE t (id int)", "-- momo:generated\nCREATE TABLE t (id int)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := applyDDLDirective(c.in, "mysql")
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestApplyDDLDirectiveRefuses(t *testing.T) {
	cases := []struct{ name, in, dialect, want string }{
		{"not an alter", "-- momo:ddl ALGORITHM=INSTANT\nCREATE TABLE t (id int)", "mysql", "ALTER TABLE"},
		{"injection", "-- momo:ddl ALGORITHM=INSTANT; DROP TABLE t\nALTER TABLE t ADD c int", "mysql", "want ALGORITHM"},
		{"unknown option", "-- momo:ddl FORCE\nALTER TABLE t ADD c int", "mysql", "want ALGORITHM"},
		{"empty", "-- momo:ddl\nALTER TABLE t ADD c int", "mysql", "want ALGORITHM"},
		{"twice", "-- momo:ddl LOCK=NONE\n-- momo:ddl LOCK=NONE\nALTER TABLE t ADD c int", "mysql", "twice"},
		{"postgres", "-- momo:ddl LOCK=NONE\nALTER TABLE t ADD c int", "postgres", "mysql only"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := applyDDLDirective(c.in, c.dialect)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

// The directive is a statement comment, so a file may carry it right after the
// header without ParseDirectives rejecting it.
func TestParseDirectivesAcceptsDDL(t *testing.T) {
	d, err := ParseDirectives([]byte("-- momo:ddl ALGORITHM=INSTANT\nALTER TABLE t ADD c int;\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d != (Directives{}) {
		t.Fatalf("the statement-scoped directive set a file directive: %+v", d)
	}
}
