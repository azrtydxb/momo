// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo/migrate"
)

func sfEngine(t *testing.T, db *sfDB, fsys fstest.MapFS, opts ...migrate.Option) *migrate.Engine {
	t.Helper()
	opts = append([]migrate.Option{migrate.WithDialect(sfDialect{db}), migrate.WithoutSum(), migrate.WithLockWait(0)}, opts...)
	e, err := migrate.NewEngine(db, fsys, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func sfRan(db *sfDB, fragment string) bool {
	for _, s := range db.execLog() {
		if strings.Contains(s, fragment) {
			return true
		}
	}
	return false
}

func sfApplied(version, checksum string) migrate.HistoryRow {
	return migrate.HistoryRow{Version: version, Name: "x", Checksum: checksum, Status: migrate.StatusApplied, Kind: "sql"}
}

// A pending version older than the latest applied one means two branches were
// merged; apply must refuse and run nothing.
func TestApplyRefusesPendingOlderThanApplied(t *testing.T) {
	db := &sfDB{tableExists: true, hist: []migrate.HistoryRow{sfApplied("20260103000000", "c3")}}
	fsys := fstest.MapFS{
		"20260102000000_late.up.sql": {Data: []byte("CREATE TABLE late (x int);")},
	}
	_, err := sfEngine(t, db, fsys).Apply(context.Background(), migrate.ApplyOptions{})
	if !errors.Is(err, migrate.ErrOutOfOrder) {
		t.Fatalf("want ErrOutOfOrder, got %v", err)
	}
	if !strings.Contains(err.Error(), "20260102000000") || !strings.Contains(err.Error(), "20260103000000") {
		t.Fatalf("the error must name both versions: %v", err)
	}
	if sfRan(db, "CREATE TABLE late") {
		t.Fatal("apply ran the out-of-order migration")
	}
}

// A forked snapshot chain fails apply with the error validate reports.
func TestApplyRefusesForkedChain(t *testing.T) {
	db := &sfDB{tableExists: true}
	fsys := fstest.MapFS{
		"20260101000000_root.up.sql":        {Data: []byte("CREATE TABLE r (x int);")},
		"20260101000000_root.snapshot.json": {Data: []byte(sfSnap("20260101000000", ""))},
		"20260102000000_a.snapshot.json":    {Data: []byte(sfSnap("20260102000000", "20260101000000"))},
		"20260102000001_b.snapshot.json":    {Data: []byte(sfSnap("20260102000001", "20260101000000"))},
	}
	_, err := sfEngine(t, db, fsys).Apply(context.Background(), migrate.ApplyOptions{})
	if !errors.Is(err, migrate.ErrBranchConflict) {
		t.Fatalf("want ErrBranchConflict, got %v", err)
	}
	rep, verr := migrate.ValidateOffline(fsys)
	if verr != nil {
		t.Fatal(verr)
	}
	found := false
	for _, p := range rep.Problems {
		found = found || strings.Contains(err.Error(), p)
	}
	if !found {
		t.Fatalf("apply's error %q must contain a problem validate reports %q", err, rep.Problems)
	}
	if sfRan(db, "CREATE TABLE r") {
		t.Fatal("apply ran a migration on a forked chain")
	}
}

func TestApplyLinearChainStillApplies(t *testing.T) {
	db := &sfDB{}
	fsys := fstest.MapFS{
		"20260101000000_a.up.sql":        {Data: []byte("CREATE TABLE a (x int);")},
		"20260101000000_a.snapshot.json": {Data: []byte(sfSnap("20260101000000", ""))},
		"20260102000000_b.up.sql":        {Data: []byte("CREATE TABLE b (x int);")},
		"20260102000000_b.snapshot.json": {Data: []byte(sfSnap("20260102000000", "20260101000000"))},
	}
	rep, err := sfEngine(t, db, fsys).Apply(context.Background(), migrate.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Applied) != 2 || !sfRan(db, "CREATE TABLE a") || !sfRan(db, "CREATE TABLE b") {
		t.Fatalf("both migrations must run, report %+v", rep)
	}
}

func TestApplyNothingPendingWritesNothing(t *testing.T) {
	t.Run("no history table", func(t *testing.T) {
		db := &sfDB{}
		rep, err := sfEngine(t, db, fstest.MapFS{}).Apply(context.Background(), migrate.ApplyOptions{})
		if err != nil || rep == nil || len(rep.Applied) != 0 {
			t.Fatalf("want an empty report, got %+v, %v", rep, err)
		}
		if db.tableExists || len(db.execLog()) != 0 {
			t.Fatalf("a no-op apply wrote: table=%v statements=%q", db.tableExists, db.execLog())
		}
	})
	t.Run("everything applied", func(t *testing.T) {
		fsys := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}}
		sum, err := migrate.ComputeSum(fsys)
		if err != nil {
			t.Fatal(err)
		}
		db := &sfDB{tableExists: true, hist: []migrate.HistoryRow{sfApplied("20260101000000", sum.Files["20260101000000_a.up.sql"])}}
		if _, err := sfEngine(t, db, fsys).Apply(context.Background(), migrate.ApplyOptions{}); err != nil {
			t.Fatal(err)
		}
		if n := len(db.execLog()); n != 0 {
			t.Fatalf("a no-op apply wrote %d statements: %q", n, db.execLog())
		}
	})
}

func TestApplyWithPendingCreatesHistoryFirst(t *testing.T) {
	db := &sfDB{}
	fsys := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}}
	if _, err := sfEngine(t, db, fsys).Apply(context.Background(), migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	log := db.execLog()
	if len(log) == 0 || !strings.HasPrefix(log[0], "CREATE TABLE IF NOT EXISTS momo_migrations") {
		t.Fatalf("the history table must be created before anything else, got %q", log)
	}
	if r, ok := db.row("20260101000000"); !ok || r.Status != migrate.StatusApplied {
		t.Fatalf("history row = %+v", r)
	}
}

// A server error that echoes a URL must not reach the history table.
func TestHistoryErrorIsRedacted(t *testing.T) {
	const leak = `connection to postgres://admin:s3cretpw@db.internal:5432/prod?sslmode=disable failed`
	for name, sql := range map[string]string{
		"transactional": "CREATE TABLE a (x int);",
		"no-tx":         "-- momo:no-transaction\nCREATE TABLE a (x int);",
	} {
		t.Run(name, func(t *testing.T) {
			db := &sfDB{failOn: "CREATE TABLE a", failErr: errors.New(leak)}
			fsys := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte(sql)}}
			_, err := sfEngine(t, db, fsys).Apply(context.Background(), migrate.ApplyOptions{})
			if err == nil {
				t.Fatal("apply must fail")
			}
			r, ok := db.row("20260101000000")
			if !ok || r.Error == "" {
				t.Fatalf("a failure row with an error text is expected, got %+v", r)
			}
			if strings.Contains(r.Error, "s3cretpw") || strings.Contains(r.Error, "admin:") {
				t.Fatalf("history error leaks the URL: %q", r.Error)
			}
			if !strings.Contains(r.Error, "connection to") {
				t.Fatalf("redaction must keep the rest of the message: %q", r.Error)
			}
		})
	}
}

func TestRedactURLs(t *testing.T) {
	got := migrate.RedactURLs("dial postgres://u:pw@h/db?password=x and password=hunter2 end", "hunter2")
	for _, bad := range []string{"u:pw", "password=x", "hunter2"} {
		if strings.Contains(got, bad) {
			t.Fatalf("%q still contains %q", got, bad)
		}
	}
}

func TestSessionTimeouts(t *testing.T) {
	fsys := fstest.MapFS{}
	setup := func(t *testing.T, opts ...migrate.Option) migrate.SessionOptions {
		t.Helper()
		db := &sfDB{}
		if _, err := sfEngine(t, db, fsys, opts...).Apply(context.Background(), migrate.ApplyOptions{}); err != nil {
			t.Fatal(err)
		}
		if len(db.setups) != 1 {
			t.Fatalf("Setup calls = %d", len(db.setups))
		}
		return db.setups[0]
	}
	t.Run("absent keeps the defaults", func(t *testing.T) {
		s := setup(t)
		if s.LockTimeout != 5*time.Second || s.LockTimeoutSet || s.StatementTimeout != 0 || s.StatementTimeoutSet {
			t.Fatalf("got %+v", s)
		}
	})
	t.Run("config values apply", func(t *testing.T) {
		s := setup(t, migrate.WithConfig(migrate.Config{LockTimeout: 2 * time.Second, StatementTimeout: 30 * time.Second}))
		if s.LockTimeout != 2*time.Second || s.StatementTimeout != 30*time.Second || s.LockTimeoutSet || s.StatementTimeoutSet {
			t.Fatalf("got %+v", s)
		}
	})
	t.Run("explicit zero disables", func(t *testing.T) {
		s := setup(t, migrate.WithConfig(migrate.Config{LockTimeout: 2 * time.Second, StatementTimeout: 30 * time.Second}),
			migrate.WithLockTimeout(0), migrate.WithStatementTimeout(0))
		if s.LockTimeout != 0 || !s.LockTimeoutSet || s.StatementTimeout != 0 || !s.StatementTimeoutSet {
			t.Fatalf("got %+v", s)
		}
	})
	t.Run("explicit zero survives a later WithConfig", func(t *testing.T) {
		s := setup(t, migrate.WithLockTimeout(0), migrate.WithConfig(migrate.Config{}))
		if s.LockTimeout != 0 || !s.LockTimeoutSet {
			t.Fatalf("got %+v", s)
		}
	})
}

func TestRehash(t *testing.T) {
	const v1, v2 = "20260101000000", "20260102000000"
	setup := func() (*sfDB, fstest.MapFS) {
		fsys := fstest.MapFS{
			v1 + "_a.up.sql": {Data: []byte("CREATE TABLE a (x int);-- edited")},
			v2 + "_b.up.sql": {Data: []byte("CREATE TABLE b (x int);")},
		}
		sum, err := migrate.ComputeSum(fsys)
		if err != nil {
			t.Fatal(err)
		}
		return &sfDB{tableExists: true, hist: []migrate.HistoryRow{
			sfApplied(v1, "old-checksum"),
			sfApplied(v2, sum.Files[v2+"_b.up.sql"]),
		}}, fsys
	}
	ctx := context.Background()

	t.Run("without accept nothing changes", func(t *testing.T) {
		db, fsys := setup()
		r, err := sfEngine(t, db, fsys).Rehash(ctx, v1, false)
		if err != nil {
			t.Fatal(err)
		}
		if !r.Changed || r.Accepted || r.Recorded != "old-checksum" || r.Current == "" || r.Current == r.Recorded {
			t.Fatalf("result = %+v", r)
		}
		if row, _ := db.row(v1); row.Checksum != "old-checksum" || len(db.execLog()) != 0 {
			t.Fatalf("a dry run wrote: %+v %q", row, db.execLog())
		}
	})

	t.Run("accept updates that version only and clears the mismatch", func(t *testing.T) {
		db, fsys := setup()
		e := sfEngine(t, db, fsys)
		if _, err := e.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrChecksumMismatch) {
			t.Fatalf("before rehash apply must report a mismatch, got %v", err)
		}
		other, _ := db.row(v2)
		r, err := e.Rehash(ctx, v1, true)
		if err != nil {
			t.Fatal(err)
		}
		if !r.Accepted {
			t.Fatalf("result = %+v", r)
		}
		row, _ := db.row(v1)
		if row.Checksum != r.Current || !strings.Contains(row.Error, "accepted by rehash") || !strings.Contains(row.Error, "old-checksum") {
			t.Fatalf("v1 row = %+v", row)
		}
		if after, _ := db.row(v2); after != other {
			t.Fatalf("another version's row changed: %+v -> %+v", other, after)
		}
		if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatalf("apply after rehash: %v", err)
		}
		st, err := e.Status(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range st {
			if s.State != "applied" {
				t.Fatalf("status after rehash = %+v", st)
			}
		}
	})

	t.Run("errors", func(t *testing.T) {
		db, fsys := setup()
		e := sfEngine(t, db, fsys)
		if _, err := e.Rehash(ctx, "20269999999999", true); err == nil {
			t.Fatal("an unknown version must fail")
		}
		delete(fsys, v1+"_a.up.sql")
		if _, err := e.Rehash(ctx, v1, true); err == nil {
			t.Fatal("a version without a file must fail")
		}
	})
}
