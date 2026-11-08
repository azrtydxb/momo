// SPDX-License-Identifier: Apache-2.0

//go:build integration

package migrate_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/schema"
)

func twoVersions() fstest.MapFS {
	return fstest.MapFS{
		"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")},
		"20260101000001_b.up.sql": {Data: []byte("CREATE TABLE b (x int);")},
	}
}

func appliedCount(t *testing.T, db *momo.DB) int {
	t.Helper()
	rows, err := readHistory(context.Background(), t, db)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range rows {
		if r.Status == migrate.StatusApplied {
			n++
		}
	}
	return n
}

// TestConcurrentApply: two engines on one database and one history table; the
// lock lets only one run at a time and each version is applied exactly once.
func TestConcurrentApply(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := twoVersions()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e, err := migrate.NewEngine(db.Driver(), fsys, migrate.WithDialect(mpg.New()), migrate.WithoutSum(), migrate.WithLockWait(0))
			if err != nil {
				errs <- err
				return
			}
			_, err = e.Apply(ctx, migrate.ApplyOptions{})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, migrate.ErrLocked) {
			t.Fatal(err)
		}
	}
	if got := appliedCount(t, db); got != 2 {
		t.Fatalf("each version must be applied exactly once, applied rows = %d", got)
	}
}

// TestConcurrentApplyWaits: with a wait budget every caller succeeds, the
// others finding nothing left to do, and the history is whole.
func TestConcurrentApplyWaits(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := twoVersions()
	var wg sync.WaitGroup
	reports := make(chan *migrate.Report, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
			rep, err := e.Apply(ctx, migrate.ApplyOptions{LockWait: 20 * time.Second})
			reports <- rep
			errs <- err
		}()
	}
	wg.Wait()
	close(reports)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	for r := range reports {
		total += len(r.Applied)
	}
	if total != 2 || appliedCount(t, db) != 2 {
		t.Fatalf("2 versions must be applied once across 4 callers: reports applied %d, history applied %d", total, appliedCount(t, db))
	}
}

func lockKey(t *testing.T) string { return "momo:" + testdb.Schema(t) + ".momo_migrations" }

func holdLock(t *testing.T, db *momo.DB) (release func()) {
	t.Helper()
	c := pinned(t, db)
	unlock, _, err := mpg.New().Lock(context.Background(), c, lockKey(t))
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := unlock(); err != nil {
			t.Error(err)
		}
	}
}

func TestApplyLockWaitTimesOutWithHolder(t *testing.T) {
	db := testopen.Postgres(t)
	release := holdLock(t, db)
	defer release()
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), twoVersions())
	start := time.Now()
	_, err := e.Apply(context.Background(), migrate.ApplyOptions{LockWait: 600 * time.Millisecond})
	if !errors.Is(err, migrate.ErrLocked) || !strings.Contains(err.Error(), "held by pid=") {
		t.Fatalf("want ErrLocked naming the holder, got %v", err)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Fatalf("Apply gave up after %v; it must wait for LockWait", time.Since(start))
	}
	if tableExists(t, db, "a") {
		t.Fatal("nothing may be applied without the lock")
	}
}

func TestApplyWaitsForLockRelease(t *testing.T) {
	db := testopen.Postgres(t)
	release := holdLock(t, db)
	var mu sync.Mutex
	var log []string
	d := recorder{inner: mpg.New(), mu: &mu, log: &log, locked: make(chan struct{}, 1)}
	go func() { <-d.locked; release() }() // release once Apply has found the lock held
	e := migratetest.NewEngine(t, d, db.Driver(), twoVersions())
	if _, err := e.Apply(context.Background(), migrate.ApplyOptions{LockWait: 10 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if appliedCount(t, db) != 2 {
		t.Fatal("both versions must apply once the lock is released")
	}
}

func TestApplyCancelDuringLockWait(t *testing.T) {
	db := testopen.Postgres(t)
	release := holdLock(t, db)
	defer release()
	var mu sync.Mutex
	var log []string
	d := recorder{inner: mpg.New(), mu: &mu, log: &log, locked: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-d.locked; cancel() }() // cancel once Apply is waiting for the lock
	e := migratetest.NewEngine(t, d, db.Driver(), twoVersions())
	start := time.Now()
	_, err := e.Apply(ctx, migrate.ApplyOptions{LockWait: time.Minute})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled from the lock wait, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("cancel took %v to stop the lock wait", time.Since(start))
	}
}

func TestApplyTo(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := twoVersions()
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{To: "20260101000000"}); err != nil {
		t.Fatal(err)
	}
	plan, err := e.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 1 || plan[0].Version != "20260101000001" {
		t.Fatalf("after --to the second version must remain pending: %+v", plan)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{To: "20990101000000"}); err == nil {
		t.Fatal("a --to version without a migration file must be refused")
	}
}

func TestChecksumMismatchRefusesApply(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	orig := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), orig)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	edited := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int, y int);")}}
	e2 := migratetest.NewEngine(t, mpg.New(), db.Driver(), edited)
	_, err := e2.Apply(ctx, migrate.ApplyOptions{})
	if !errors.Is(err, migrate.ErrChecksumMismatch) || !strings.Contains(err.Error(), "20260101000000_a.up.sql") {
		t.Fatalf("want ErrChecksumMismatch naming the file, got %v", err)
	}
}

func TestSumFileMismatchRefusesApply(t *testing.T) {
	db := testopen.Postgres(t)
	fsys := migratetest.WithSum(t, twoVersions()).(fstest.MapFS)
	fsys["20260101000001_b.up.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE b (x int, evil int);")}
	e := sumEngine(t, db, fsys)
	_, err := e.Apply(context.Background(), migrate.ApplyOptions{})
	if !errors.Is(err, migrate.ErrChecksumMismatch) || !strings.Contains(err.Error(), "20260101000001_b.up.sql") {
		t.Fatalf("want ErrChecksumMismatch naming the edited file, got %v", err)
	}
	// A refused apply writes nothing, so the history table may not exist at all.
	if tableExists(t, db, testdb.Schema(t)+".momo_migrations") && appliedCount(t, db) != 0 {
		t.Fatal("nothing may run when momo.sum disagrees with the directory")
	}
}

// sumEngine is an engine that enforces momo.sum (migratetest.NewEngine opts out).
func sumEngine(t *testing.T, db *momo.DB, fsys fs.FS, opts ...migrate.Option) *migrate.Engine {
	t.Helper()
	e, err := migrate.NewEngine(db.Driver(), fsys, append([]migrate.Option{migrate.WithDialect(mpg.New())}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestMissingSumRefusesApplyAndStatus(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	e := sumEngine(t, db, twoVersions()) // no momo.sum
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil || !strings.Contains(err.Error(), "momo migrate hash") {
		t.Fatalf("Apply without momo.sum must name momo migrate hash, got %v", err)
	}
	if _, err := e.Status(ctx); err == nil || !strings.Contains(err.Error(), "momo migrate hash") {
		t.Fatalf("Status without momo.sum must name momo migrate hash, got %v", err)
	}
	if tableExists(t, db, "a") {
		t.Fatal("nothing may run without momo.sum")
	}
	// The explicit opt-out skips the requirement.
	opt := sumEngine(t, db, twoVersions(), migrate.WithoutSum())
	if _, err := opt.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("WithoutSum must skip the momo.sum check: %v", err)
	}
	// With a matching momo.sum the checking engine applies nothing new and is happy.
	ok := sumEngine(t, db, migratetest.WithSum(t, twoVersions()))
	if _, err := ok.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestPlanAndStatusAreReadOnly(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), twoVersions())
	plan, err := e.Plan(ctx)
	if err != nil || len(plan) != 2 {
		t.Fatalf("a fresh database has every migration pending: %v %+v", err, plan)
	}
	st, err := e.Status(ctx)
	if err != nil || len(st) != 2 || st[0].State != "pending" {
		t.Fatalf("status on a fresh database: %v %+v", err, st)
	}
	if tableExists(t, db, "momo_migrations") {
		t.Fatal("Plan and Status must not create the history table")
	}
}

// TestPlanDuringFirstApply: Plan and Status polling while the first Apply creates
// the history table never fail.
func TestPlanDuringFirstApply(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), twoVersions())
	stop := make(chan struct{})
	errs := make(chan error, 64)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := e.Plan(ctx); err != nil {
					errs <- err
					return
				}
				if _, err := e.Status(ctx); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	_, err := e.Apply(ctx, migrate.ApplyOptions{})
	close(stop)
	wg.Wait()
	close(errs)
	if err != nil {
		t.Fatal(err)
	}
	for err := range errs {
		t.Fatalf("Plan or Status failed during the first Apply: %v", err)
	}
}

func TestResolveTakesTheLock(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	bad := fstest.MapFS{"20260101000000_t.up.sql": {Data: []byte("-- momo:no-transaction\nCREATE TABLE a (x int);\nINSERT INTO missing VALUES (1);")}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), bad, migrate.WithLockWait(0))
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("the failing migration must fail Apply")
	}
	release := holdLock(t, db)
	err := e.Resolve(ctx, "20260101000000", migrate.ResolveRolledBack)
	release()
	if !errors.Is(err, migrate.ErrLocked) {
		t.Fatalf("Resolve must take the migration lock, got %v", err)
	}
	if err := e.Resolve(ctx, "20260101000000", migrate.ResolveRolledBack); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRefusesDirtyAndReleasesLock(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	bad := fstest.MapFS{"20260101000000_t.up.sql": {Data: []byte("-- momo:no-transaction\nCREATE TABLE a (x int);\nINSERT INTO missing VALUES (1);")}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), bad)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("the failing migration must fail Apply")
	}
	_, err := e.Apply(ctx, migrate.ApplyOptions{})
	if !errors.Is(err, migrate.ErrDirty) || !strings.Contains(err.Error(), "20260101000000") {
		t.Fatalf("want ErrDirty naming the version, got %v", err)
	}
	// Both runs released the lock.
	release := holdLock(t, db)
	release()
}

// recorder wraps the PostgreSQL dialect and logs the order of reset and unlock.
type recorder struct {
	inner    mpg.Dialect
	mu       *sync.Mutex
	log      *[]string
	resetErr error
	locked   chan struct{} // receives a value each time the lock was found held
}

func (r recorder) Dialect() *dialect.Dialect { return r.inner.Dialect() }

func (r recorder) Inspect(ctx context.Context, c momo.PinnedConn, o migrate.InspectOptions) (*schema.Schema, error) {
	return r.inner.Inspect(ctx, c, o)
}

func (r recorder) Render(p []migrate.Change) ([]migrate.Statement, error) { return r.inner.Render(p) }

func (r recorder) HistoryDDL() []string { return r.inner.HistoryDDL() }

func (r recorder) note(s string) { r.mu.Lock(); *r.log = append(*r.log, s); r.mu.Unlock() }

func (r recorder) Lock(ctx context.Context, c momo.PinnedConn, key string) (func() error, func(context.Context) (string, error), error) {
	unlock, holder, err := r.inner.Lock(ctx, c, key)
	if err != nil {
		if r.locked != nil {
			select {
			case r.locked <- struct{}{}:
			default:
			}
		}
		return unlock, holder, err
	}
	return func() error { r.note("unlock"); return unlock() }, holder, nil
}

func (r recorder) Setup(ctx context.Context, c momo.PinnedConn, s migrate.SessionOptions) (func() error, error) {
	reset, err := r.inner.Setup(ctx, c, s)
	if err != nil {
		return nil, err
	}
	return func() error { r.note("reset"); return errors.Join(reset(), r.resetErr) }, nil
}

func TestApplyResetsBeforeUnlock(t *testing.T) {
	db := testopen.Postgres(t)
	var mu sync.Mutex
	var log []string
	d := recorder{inner: mpg.New(), mu: &mu, log: &log}
	e := migratetest.NewEngine(t, d, db.Driver(), twoVersions(), migrate.WithConfig(migrate.Config{StatementTimeout: time.Minute}))
	if _, err := e.Apply(context.Background(), migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(log, ","); got != "reset,unlock" {
		t.Fatalf("cleanup order = %q, want reset,unlock", got)
	}
}

func TestApplyJoinsResetFailureAndStillUnlocks(t *testing.T) {
	db := testopen.Postgres(t)
	var mu sync.Mutex
	var log []string
	boom := errors.New("reset boom")
	d := recorder{inner: mpg.New(), mu: &mu, log: &log, resetErr: boom}
	e := migratetest.NewEngine(t, d, db.Driver(), twoVersions())
	rep, err := e.Apply(context.Background(), migrate.ApplyOptions{})
	if !errors.Is(err, boom) {
		t.Fatalf("a failed reset must be reported, got %v", err)
	}
	if rep == nil || len(rep.Applied) != 2 {
		t.Fatalf("the report of the work done must survive a cleanup error: %+v", rep)
	}
	if got := strings.Join(log, ","); got != "reset,unlock" {
		t.Fatalf("unlock must still run after a failed reset, log = %q", got)
	}
}

func TestApplyCancelledStillUnlocks(t *testing.T) {
	db := testopen.Postgres(t)
	ctx, cancel := context.WithCancel(context.Background())
	marker := "momo-cancel-" + testdb.Schema(t)
	files := fstest.MapFS{"20260101000000_slow.up.sql": {Data: []byte("SELECT pg_sleep(30) /* " + marker + " */;")}}
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), files)
	cancelWhenRunning(t, db, marker, cancel) // cancel once the slow statement runs on the server
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("a cancelled apply must fail")
	}
	// The detached cleanup released the lock.
	c := pinned(t, db)
	unlock, _, err := mpg.New().Lock(context.Background(), c, lockKey(t))
	if err != nil {
		t.Fatalf("lock still held after a cancelled apply: %v", err)
	}
	_ = unlock()
}

func TestApplySchemasMustMatchCurrentSchema(t *testing.T) {
	db := testopen.Postgres(t)
	e := migratetest.NewEngine(t, mpg.New(), db.Driver(), twoVersions(), migrate.WithConfig(migrate.Config{Schemas: []string{"definitely_not_current"}}))
	_, err := e.Apply(context.Background(), migrate.ApplyOptions{})
	if err == nil || !strings.Contains(err.Error(), "definitely_not_current") || !strings.Contains(err.Error(), testdb.Schema(t)) {
		t.Fatalf("want a configuration error naming both schemas, got %v", err)
	}
}

func TestStatusStates(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := fstest.MapFS{
		"20260101000000_ok.up.sql":   {Data: []byte("CREATE TABLE ok (x int);")},
		"20260101000001_edit.up.sql": {Data: []byte("CREATE TABLE edit (x int);")},
		"20260101000002_gone.up.sql": {Data: []byte("CREATE TABLE gone (x int);")},
	}
	if _, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	c := pinned(t, db)
	mustExec(t, c, `INSERT INTO momo_migrations (version, name, checksum, status, error, applied_at) VALUES ('20260101000003', 'dirty', 'x', 'failed', 'boom', 'now')`)
	now := fstest.MapFS{
		"20260101000000_ok.up.sql":    fsys["20260101000000_ok.up.sql"],
		"20260101000001_edit.up.sql":  {Data: []byte("CREATE TABLE edit (x int, y int);")},
		"20260101000003_dirty.up.sql": {Data: []byte("CREATE TABLE d (x int);")},
		"20260101000004_new.up.sql":   {Data: []byte("CREATE TABLE n (x int);")},
	}
	got, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), now).Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]migrate.State{
		"20260101000000": "applied", "20260101000001": "mismatch", "20260101000002": "missing",
		"20260101000003": "dirty", "20260101000004": "pending",
	}
	if len(got) != len(want) {
		t.Fatalf("status = %+v", got)
	}
	for _, s := range got {
		if want[s.Version] != s.State {
			t.Errorf("%s: state %q, want %q", s.Version, s.State, want[s.Version])
		}
		if s.Version == "20260101000003" && s.Error != "boom" {
			t.Errorf("dirty status must carry the error, got %q", s.Error)
		}
	}
}

// TestMovingAppliedMigrationIntoArchiveKeepsHistory: history is keyed by version,
// so moving an applied migration into a nested directory (and refreshing
// momo.sum) neither makes it pending again nor missing, and the checksum gate
// still passes because the content did not change.
func TestMovingAppliedMigrationIntoArchiveKeepsHistory(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := sumEngine(t, db, migratetest.WithSum(t, twoVersions())).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	moved := fstest.MapFS{
		"archive/2026/20260101000000_a.up.sql": twoVersions()["20260101000000_a.up.sql"],
		"20260101000001_b.up.sql":              twoVersions()["20260101000001_b.up.sql"],
	}
	e := sumEngine(t, db, migratetest.WithSum(t, moved))
	plan, err := e.Plan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 0 {
		t.Fatalf("nothing may be pending after the move: %+v", plan)
	}
	st, err := e.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 2 {
		t.Fatalf("status = %+v", st)
	}
	for _, s := range st {
		if s.State != "applied" {
			t.Errorf("%s: state %q, want applied", s.Version, s.State)
		}
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("the checksum gate must pass after the move: %v", err)
	}
	if appliedCount(t, db) != 2 {
		t.Fatal("the move must not apply anything again")
	}
}
