// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func TestForeignKeyCheckStatement(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		hint string
		want bool
		stmt string // the text that is run
	}{
		{"plain", "PRAGMA foreign_key_check", "", true, "PRAGMA foreign_key_check"},
		{"semicolon and case", "pragma Foreign_Key_Check ;\n", "", true, "pragma Foreign_Key_Check"},
		{"leading line comment", "-- rebuild step\nPRAGMA foreign_key_check;", "", true, "PRAGMA foreign_key_check"},
		{"leading block comment", "/* check */  \n PRAGMA foreign_key_check", "", true, "PRAGMA foreign_key_check"},
		{"several comments", "-- a\n-- b\n/* c */\nPRAGMA foreign_key_check", "", true, "PRAGMA foreign_key_check"},
		{"schema", "PRAGMA main.foreign_key_check", "", true, "PRAGMA main.foreign_key_check"},
		{"quoted schema", `PRAGMA "main".foreign_key_check`, "", true, `PRAGMA "main".foreign_key_check`},
		{"table argument", "PRAGMA foreign_key_check(items)", "", true, "PRAGMA foreign_key_check(items)"},
		{"quoted table argument", `PRAGMA foreign_key_check( "items" );`, "", true, `PRAGMA foreign_key_check( "items" )`},
		{"schema and table", "PRAGMA main.foreign_key_check('items')", "", true, "PRAGMA main.foreign_key_check('items')"},
		{"hint without text match", "SELECT 1", "fk-check", true, ""},
		{"trailing line comment", "PRAGMA foreign_key_check; -- done", "", true, "PRAGMA foreign_key_check"},
		{"trailing block comment", "PRAGMA foreign_key_check /* done */;", "", true, "PRAGMA foreign_key_check"},
		{"comment between words", "PRAGMA /*x*/ foreign_key_check", "", true, "PRAGMA   foreign_key_check"},
		{"dashes in an argument", "PRAGMA foreign_key_check('a--b')", "", true, "PRAGMA foreign_key_check('a--b')"},
		{"another pragma", "PRAGMA foreign_keys = OFF", "", false, ""},
		{"integrity check", "PRAGMA integrity_check", "", false, ""},
		{"trailing statement", "PRAGMA foreign_key_check; DROP TABLE t", "", false, ""},
		{"inside a comment only", "-- PRAGMA foreign_key_check", "", false, ""},
		{"unterminated comment", "/* PRAGMA foreign_key_check", "", false, ""},
		{"set form", "PRAGMA foreign_key_check = 1", "", false, ""},
		{"plain statement", "CREATE TABLE foreign_key_check (a int)", "", false, ""},
	}
	for _, c := range cases {
		if got := isForeignKeyCheck(Statement{SQL: c.sql, Hint: c.hint}); got != c.want {
			t.Errorf("%s: isForeignKeyCheck = %v, want %v", c.name, got, c.want)
		}
		if c.want && c.hint == "" {
			if stmt, ok := foreignKeyCheckSQL(c.sql); !ok || stmt != c.stmt {
				t.Errorf("%s: foreignKeyCheckSQL = %q, %v; want %q", c.name, stmt, ok, c.stmt)
			}
		}
	}
}

// recordingExecer records the statements it is asked to run.
type recordingExecer struct{ ran []string }

func (r *recordingExecer) Exec(_ context.Context, sql string, _ []any) (int64, error) {
	r.ran = append(r.ran, sql)
	return 0, nil
}

// TestForeignKeyCheckFailsClosed checks that a statement naming foreign_key_check
// in a form the matcher does not know is refused instead of run without its
// check, and that unrelated statements run.
func TestForeignKeyCheckFailsClosed(t *testing.T) {
	for _, sql := range []string{
		"PRAGMA foreign_key_check; DROP TABLE t",
		"PRAGMA foreign_key_check = 1",
		"-- c\nPRAGMA foreign_key_check(a, b)",
		"SELECT * FROM pragma_foreign_key_check",
		"SELECT * FROM pragma_foreign_key_check('items')",
		"select count(*) from PRAGMA_FOREIGN_KEY_CHECK where 'x' = 'x'",
	} {
		x := &recordingExecer{}
		err := execStatement(context.Background(), x, Statement{SQL: sql})
		if !errors.Is(err, momo.ErrValidation) || len(x.ran) != 0 {
			t.Errorf("%q: err = %v, ran = %v; want ErrValidation and nothing run", sql, err, x.ran)
		}
	}
	for _, sql := range []string{
		"CREATE TABLE foreign_key_check (a int)",
		"INSERT INTO log (msg) VALUES ('PRAGMA foreign_key_check')",
		"SELECT 'pragma_foreign_key_check'",
		"PRAGMA foreign_keys = OFF",
	} {
		x := &recordingExecer{}
		if err := execStatement(context.Background(), x, Statement{SQL: sql}); err != nil || len(x.ran) != 1 {
			t.Errorf("%q is unrelated and must run: %v %v", sql, err, x.ran)
		}
	}
}

type stateError struct{ state string }

func (e stateError) Error() string    { return "boom " + e.state }
func (e stateError) SQLState() string { return e.state }

func TestIsUndefinedTable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{stateError{"42P01"}, true},
		{stateError{"42S02"}, true},
		{stateError{"23505"}, false},
		{errors.New("no such table: momo_migrations"), true},
		{fmt.Errorf("migrate: read history: %w", errors.New("no such table: main.momo_migrations")), true},
		{errors.New("no such table: users"), false},
		{errors.New("no such column: momo_migrations"), false},
	}
	for _, c := range cases {
		if got := isUndefinedTable(c.err); got != c.want {
			t.Errorf("isUndefinedTable(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestIsNoSuchTable(t *testing.T) {
	for _, c := range []struct {
		err  error
		name string
		want bool
	}{
		{nil, "t", false},
		{errors.New("no such table: t"), "t", true},
		{fmt.Errorf("wrapped: %w", errors.New("no such table: main.t")), "t", true},
		{errors.New("no such table: other"), "t", false},
	} {
		if got := IsNoSuchTable(c.err, c.name); got != c.want {
			t.Errorf("IsNoSuchTable(%v, %q) = %v, want %v", c.err, c.name, got, c.want)
		}
	}
}

// violationRows is one foreign_key_check row: child, rowid, parent, fkid.
type violationRows struct{ done bool }

func (r *violationRows) Next() bool {
	if r.done {
		return false
	}
	r.done = true
	return true
}

func (r *violationRows) Scan(dest ...any) error {
	*dest[0].(*sql.NullString) = sql.NullString{String: "child", Valid: true}
	*dest[1].(*sql.NullInt64) = sql.NullInt64{Int64: 7, Valid: true}
	*dest[2].(*sql.NullString) = sql.NullString{String: "parent", Valid: true}
	*dest[3].(*sql.NullInt64) = sql.NullInt64{Int64: 0, Valid: true}
	return nil
}
func (*violationRows) Err() error   { return nil }
func (*violationRows) Close() error { return nil }

// violationConn answers every query with one violation; it is a connection.
type violationConn struct{}

func (violationConn) Exec(context.Context, string, []any) (int64, error) { return 0, nil }
func (violationConn) Query(context.Context, string, []any) (momo.Rows, error) {
	return &violationRows{}, nil
}

// violationTx is the same inside a transaction.
type violationTx struct{ violationConn }

func (violationTx) Commit(context.Context) error { return nil }

// A foreign_key_check violation in a no-transaction run must not claim a
// rollback; in a transaction it says so.
func TestForeignKeyCheckMessageMatchesMode(t *testing.T) {
	stmt := Statement{SQL: "PRAGMA foreign_key_check"}
	err := execStatement(context.Background(), violationTx{}, stmt)
	if !errors.Is(err, momo.ErrForeignKeyViolation) || !strings.Contains(err.Error(), "the transaction is rolled back") {
		t.Fatalf("in a transaction: %v", err)
	}
	err = execStatement(context.Background(), violationConn{}, stmt)
	if !errors.Is(err, momo.ErrForeignKeyViolation) {
		t.Fatalf("without a transaction: %v", err)
	}
	if strings.Contains(err.Error(), "the transaction is rolled back") || !strings.Contains(err.Error(), "already committed") {
		t.Fatalf("without a transaction the message must not claim a rollback: %v", err)
	}
}

// namedDialect is a Dialect stub that only reports its name.
type namedDialect struct{ name dialect.Name }

func (d namedDialect) Dialect() *dialect.Dialect { return dialect.For(d.name) }
func (namedDialect) Inspect(context.Context, momo.PinnedConn, InspectOptions) (*schema.Schema, error) {
	return nil, errors.New("unused")
}
func (namedDialect) Render([]Change) ([]Statement, error) { return nil, errors.New("unused") }
func (namedDialect) HistoryDDL() []string                 { return nil }
func (namedDialect) Setup(context.Context, momo.PinnedConn, SessionOptions) (func() error, error) {
	return nil, errors.New("setup must not run")
}
func (namedDialect) Lock(context.Context, momo.PinnedConn, string) (func() error, func(context.Context) (string, error), error) {
	return nil, nil, errors.New("unused")
}

// The sqlite-foreign-keys-off directive is refused where it has no meaning.
func TestForeignKeysOffRefusedOffSQLite(t *testing.T) {
	for _, name := range []dialect.Name{dialect.Postgres, dialect.MySQL} {
		e := &Engine{opts: options{dialect: namedDialect{name}}}
		restore, err := e.foreignKeysOff(context.Background(), nil, true)
		if !errors.Is(err, momo.ErrValidation) || restore != nil {
			t.Errorf("%s: err = %v, restore set = %v; want ErrValidation", name, err, restore != nil)
		}
		if restore, err := e.foreignKeysOff(context.Background(), nil, false); err != nil || restore == nil {
			t.Errorf("%s: off=false must do nothing: %v", name, err)
		}
	}
}
