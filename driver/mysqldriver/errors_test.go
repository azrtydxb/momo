// SPDX-License-Identifier: Apache-2.0

package mysqldriver

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/go-sql-driver/mysql"
)

func containsValue(s, v string) bool { return strings.Contains(s, v) }

func TestMySQLErrorClassification(t *testing.T) {
	cases := []struct {
		num  uint16
		msg  string
		kind error
		name string
	}{
		{1062, "Duplicate entry 'a@example.com' for key 'users.email'", momo.ErrUniqueViolation, "users.email"},
		{1452, "Cannot add or update a child row: a foreign key constraint fails (`app`.`orders`, CONSTRAINT `fk_orders_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`))", momo.ErrForeignKeyViolation, "fk_orders_user"},
		{1048, "Column 'name' cannot be null", momo.ErrNotNullViolation, ""},
		{3819, "Check constraint 'positive' is violated.", momo.ErrCheckViolation, "positive"},
		{4025, "CONSTRAINT `positive` failed for `app`.`checked`", momo.ErrCheckViolation, "positive"},
		{1213, "Deadlock found when trying to get lock", momo.ErrSerialization, ""},
		{1205, "Lock wait timeout exceeded", momo.ErrBusy, ""},
	}
	for _, c := range cases {
		err := classifier{}.ClassifyError(&mysql.MySQLError{Number: c.num, Message: c.msg})
		if !errors.Is(err, c.kind) {
			t.Errorf("%d: errors.Is(%v) false", c.num, err)
		}
		var me *mysql.MySQLError
		if !errors.As(err, &me) || me.Number != c.num {
			t.Errorf("%d: driver error not reachable with errors.As", c.num)
		}
		var me2 *momo.Error
		if !errors.As(err, &me2) {
			t.Errorf("%d: not a *momo.Error", c.num)
			continue
		}
		if c.name != "" && me2.Constraint != c.name {
			t.Errorf("%d: constraint = %q, want %q", c.num, me2.Constraint, c.name)
		}
		if containsValue(err.Error(), "a@example.com") {
			t.Errorf("%d: Error() leaked the offending value", c.num)
		}
		if me2.Detail() != c.msg {
			t.Errorf("%d: Detail() = %q, want the server message", c.num, me2.Detail())
		}
	}
	if got := (classifier{}).ClassifyError(errors.New("boom")); got.Error() != "boom" {
		t.Errorf("unknown errors must pass through unchanged, got %v", got)
	}
}

func TestTableFromServerMessageAndStatement(t *testing.T) {
	fk := classify(&mysql.MySQLError{Number: 1452, Message: "Cannot add or update a child row: a foreign key constraint fails (`app`.`orders`, CONSTRAINT `fk` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`))"}, "")
	var me *momo.Error
	if !errors.As(fk, &me) || me.Table != "orders" {
		t.Fatalf("fk table = %+v, want orders", me)
	}
	dup := classify(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'x' for key 'documents_title_uq'"}, "INSERT INTO `documents` (`title`) VALUES (?)")
	if !errors.As(dup, &me) || me.Table != "documents" {
		t.Fatalf("statement fallback table = %+v, want documents", me)
	}
}

func TestValueErrorsAreRedacted(t *testing.T) {
	err := classify(&mysql.MySQLError{Number: 1366, Message: "Incorrect integer value: 'secret' for column 'n' at row 1"}, "")
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("Error() leaked the value: %v", err)
	}
	var se *mysql.MySQLError
	if !errors.As(err, &se) {
		t.Fatal("server error not reachable")
	}
}

// Catches: a server error with no mapping (1525 incorrect DATETIME value, 1367
// illegal value, 1406 data too long) rendering the value the server quotes.
func TestUnmappedServerErrorsAreRedacted(t *testing.T) {
	for _, num := range []uint16{1525, 1300, 1367, 1406, 9999} {
		msg := "Incorrect DATETIME value: 'secret-value'"
		err := classify(&mysql.MySQLError{Number: num, Message: msg}, "SELECT 1")
		if strings.Contains(err.Error(), "secret-value") {
			t.Errorf("%d: Error() leaked the value: %v", num, err)
		}
		var se *mysql.MySQLError
		if !errors.As(err, &se) || se.Number != num {
			t.Errorf("%d: server error not reachable with errors.As", num)
		}
		d, ok := err.(interface{ Detail() string })
		if !ok || d.Detail() != msg {
			t.Errorf("%d: Detail() does not return the server message", num)
		}
	}
}

func TestNewRejectsUnknownFlavor(t *testing.T) {
	d := New(nil, "oracle")
	if _, err := d.Exec(t.Context(), "SELECT 1", nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

// Catches: a DSN that turns on client-side interpolation, which would put bound
// values into the SQL text, or multi-statement execution.
func TestConfigureDSN(t *testing.T) {
	cfg, err := ConfigureDSN("u:p@tcp(h:3306)/d?multiStatements=true&interpolateParams=true")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ParseTime || !cfg.ClientFoundRows || cfg.MultiStatements || cfg.InterpolateParams || cfg.Loc.String() != "UTC" {
		t.Fatalf("config = %+v", cfg)
	}
	if cfg.Params["time_zone"] != "'+00:00'" {
		t.Fatalf("time_zone param = %q", cfg.Params["time_zone"])
	}
}
