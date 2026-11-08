// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlshared_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/sqlitepure"
	"github.com/azrtydxb/momo/internal/sqlshared"
	"github.com/azrtydxb/momo/internal/testdb"
)

type opener struct {
	name string
	open func(string, sqlitepure.Config) (*sql.DB, momo.Driver, error)
}

// openers lists the drivers built on sqlshared; cgo_integration_test.go adds
// the cgo one. Every test here runs against each.
var openers = []opener{{"sqlitepure", sqlitepure.Open}}

func eachDriver(t *testing.T, f func(t *testing.T, o opener)) {
	t.Helper()
	for _, o := range openers {
		t.Run(o.name, func(t *testing.T) { f(t, o) })
	}
}

func openAt(t *testing.T, o opener, cfg sqlitepure.Config) (*sql.DB, momo.Driver) {
	t.Helper()
	db, d, err := o.open(testdb.SQLitePath(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, d
}

// When COMMIT fails and the ROLLBACK that follows fails too, the connection's
// state is unknown, so it must not return to the pool (ErrBadConn).
func TestReadOnlyCommitFailureDiscardsConnection(t *testing.T) {
	eachDriver(t, func(t *testing.T, o opener) {
		db, drv := openAt(t, o, sqlitepure.Config{})
		ctx := context.Background()
		tx, err := drv.Begin(ctx, momo.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		sqlshared.ExecStmtHook = func(stmt string) error {
			if stmt == "COMMIT" || stmt == "ROLLBACK" {
				return errors.New("injected failure")
			}
			return nil
		}
		t.Cleanup(func() { sqlshared.ExecStmtHook = nil })
		if err := tx.Commit(ctx); err == nil {
			t.Fatal("Commit succeeded although the statement failed")
		}
		sqlshared.ExecStmtHook = nil
		if st := db.Stats(); st.OpenConnections != 0 {
			t.Fatalf("OpenConnections = %d; the suspect connection was returned to the pool", st.OpenConnections)
		}
		// The pool opens a fresh, healthy connection for the next statement.
		if _, err := drv.Exec(ctx, "CREATE TABLE after_discard (x int)", nil); err != nil {
			t.Fatalf("the pool is unusable after the discard: %v", err)
		}
	})
}

// A failed COMMIT whose ROLLBACK succeeds returns the connection in autocommit
// mode with query_only reset.
func TestReadOnlyCommitFailureRollsBack(t *testing.T) {
	eachDriver(t, func(t *testing.T, o opener) {
		_, drv := openAt(t, o, sqlitepure.Config{})
		ctx := context.Background()
		if _, err := drv.Exec(ctx, "CREATE TABLE rb (x int)", nil); err != nil {
			t.Fatal(err)
		}
		tx, err := drv.Begin(ctx, momo.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		sqlshared.ExecStmtHook = func(stmt string) error {
			if stmt == "COMMIT" {
				return errors.New("injected failure")
			}
			return nil
		}
		t.Cleanup(func() { sqlshared.ExecStmtHook = nil })
		if err := tx.Commit(ctx); err == nil {
			t.Fatal("Commit succeeded although the statement failed")
		}
		sqlshared.ExecStmtHook = nil
		if _, err := drv.Exec(ctx, "INSERT INTO rb VALUES (1)", nil); err != nil {
			t.Fatalf("the connection was returned read-only or inside a transaction: %v", err)
		}
	})
}

// The settings travel in the DSN, so every pooled connection has them, not
// only the first.
func TestPragmasOnEveryPooledConnection(t *testing.T) {
	eachDriver(t, func(t *testing.T, o opener) {
		_, drv := openAt(t, o, sqlitepure.Config{MaxOpenConns: 2})
		ctx := context.Background()
		var pins []momo.PinnedConn
		for range 2 {
			p, err := drv.(momo.Pinner).Pin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			pins = append(pins, p)
			t.Cleanup(func() { _ = p.Release() })
		}
		if st := drv.(momo.PoolStater).PoolStats(); st.AcquiredConns != 2 {
			t.Fatalf("AcquiredConns = %d, want two distinct connections", st.AcquiredConns)
		}
		for i, p := range pins {
			var mode string
			var syncMode, fk, busy int
			for _, c := range []struct {
				pragma string
				dest   any
			}{{"journal_mode", &mode}, {"synchronous", &syncMode}, {"foreign_keys", &fk}, {"busy_timeout", &busy}} {
				rows, err := p.Query(ctx, "PRAGMA "+c.pragma, nil)
				if err != nil {
					t.Fatal(err)
				}
				if !rows.Next() {
					t.Fatalf("connection %d: PRAGMA %s returned no row", i, c.pragma)
				}
				if err := rows.Scan(c.dest); err != nil {
					t.Fatal(err)
				}
				_ = rows.Close()
			}
			if mode != "wal" || syncMode != 1 || fk != 1 || busy != 5000 {
				t.Errorf("connection %d: journal=%s synchronous=%d foreign_keys=%d busy_timeout=%d", i, mode, syncMode, fk, busy)
			}
		}
	})
}

const endlessCTE = "WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c) SELECT count(*) FROM c"

// Cancelling the context interrupts a statement that is running, and both
// drivers report the cancellation the same way: errors.Is with
// context.Canceled (or momo.ErrCanceled), not a bare SQLITE_INTERRUPT.
func TestCancelInterruptsRunningStatement(t *testing.T) {
	eachDriver(t, func(t *testing.T, o opener) {
		_, drv := openAt(t, o, sqlitepure.Config{})
		isCanceled := func(err error) bool {
			return errors.Is(err, context.Canceled) || errors.Is(err, momo.ErrCanceled)
		}
		for _, mode := range []string{"query", "exec"} {
			ctx, cancel := context.WithCancel(context.Background())
			timer := time.AfterFunc(200*time.Millisecond, cancel)
			start := time.Now()
			var err error
			switch mode {
			case "exec":
				_, err = drv.Exec(ctx, endlessCTE, nil)
			default:
				var rows momo.Rows
				rows, err = drv.Query(ctx, endlessCTE, nil)
				if err == nil {
					for rows.Next() {
					}
					err = rows.Err()
					_ = rows.Close()
				}
			}
			timer.Stop()
			cancel()
			if err == nil || !isCanceled(err) {
				t.Errorf("%s: err = %v, want a cancellation", mode, err)
			}
			if d := time.Since(start); d > 10*time.Second {
				t.Errorf("%s: the statement ran %v after the cancel", mode, d)
			}
			// The connection is usable again.
			if _, err := drv.Exec(context.Background(), "SELECT 1", nil); err != nil {
				t.Errorf("%s: pool unusable after the cancel: %v", mode, err)
			}
		}
	})
}

// The shared package must not pull in either SQLite implementation.
func TestSharedPackageImportsNoSQLiteDriver(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.HasPrefix(pkg, "modernc.org/sqlite") || strings.Contains(pkg, "mattn/go-sqlite3") || strings.Contains(pkg, "sqlite-vec") || pkg == "C" {
			t.Errorf("internal/sqlshared depends on %s", pkg)
		}
	}
}

// Times with a zone offset and nanoseconds survive a round trip, through a
// text column and through columns declared DATETIME and TIMESTAMP.
func TestTimeRoundTripWithOffsetAndNanoseconds(t *testing.T) {
	eachDriver(t, func(t *testing.T, o opener) {
		_, drv := openAt(t, o, sqlitepure.Config{})
		ctx := context.Background()
		if _, err := drv.Exec(ctx, "CREATE TABLE tm (at text, dt DATETIME, ts TIMESTAMP)", nil); err != nil {
			t.Fatal(err)
		}
		want := time.Date(2026, 11, 8, 10, 11, 12, 123456789, time.FixedZone("", 5*3600+1800))
		if _, err := drv.Exec(ctx, "INSERT INTO tm VALUES (?, ?, ?)", []any{want, want, want}); err != nil {
			t.Fatal(err)
		}
		rows, err := drv.Query(ctx, "SELECT at, dt, ts, dt FROM tm", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if !rows.Next() {
			t.Fatal("no row")
		}
		var at, dt, ts time.Time
		var nt sql.NullTime
		if err := rows.Scan(&at, &dt, &ts, &nt); err != nil {
			t.Fatal(err)
		}
		for name, got := range map[string]time.Time{"text": at, "DATETIME": dt, "TIMESTAMP": ts, "NullTime": nt.Time} {
			if !got.Equal(want) {
				t.Errorf("%s column: got %v, want the instant %v", name, got, want)
			}
		}
		if !nt.Valid {
			t.Error("NullTime not valid")
		}
	})
}
