// SPDX-License-Identifier: Apache-2.0

package mysqldriver

import (
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// TestSQLStateReachesTheMigrationEngine: the engine reads SQLState() from the
// error chain to recognise a missing history table (42S02) and to withhold the
// message of a data exception (class 22). The driver redacts server text, so
// the state must survive the redaction.
func TestSQLStateReachesTheMigrationEngine(t *testing.T) {
	cases := []struct {
		name  string
		err   *mysql.MySQLError
		state string
	}{
		{"unmapped table error", &mysql.MySQLError{Number: 1146, SQLState: [5]byte{'4', '2', 'S', '0', '2'}, Message: "Table 'a.b' doesn't exist"}, "42S02"},
		{"mapped unique error", &mysql.MySQLError{Number: 1062, SQLState: [5]byte{'2', '3', '0', '0', '0'}, Message: "Duplicate entry 'x' for key 'k'"}, "23000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := classifier{}.ClassifyError(c.err)
			var s interface{ SQLState() string }
			if !errors.As(err, &s) {
				t.Fatalf("%T has no SQLState method in its chain", err)
			}
			if got := s.SQLState(); got != c.state {
				t.Fatalf("SQLState = %q, want %q", got, c.state)
			}
		})
	}
}
