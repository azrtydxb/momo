// SPDX-License-Identifier: Apache-2.0

//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/azrtydxb/momo"
	testdb "github.com/azrtydxb/momo/internal/testdb"
	testopen "github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

// historySchemas creates empty schemas named after the test, drops them when the
// test ends and returns their names.
func historySchemas(t *testing.T, c momo.PinnedConn, suffixes ...string) []string {
	t.Helper()
	var out []string
	for _, s := range suffixes {
		name := testdb.Schema(t) + "_" + s
		out = append(out, name)
		if _, err := c.Exec(context.Background(), `CREATE SCHEMA `+name, nil); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = c.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+name+` CASCADE`, nil) })
	}
	t.Cleanup(func() { _, _ = c.Exec(context.Background(), `RESET search_path`, nil) })
	return out
}

func setPath(t *testing.T, c momo.PinnedConn, path string) {
	t.Helper()
	if _, err := c.Exec(context.Background(), `SET search_path = `+path, nil); err != nil {
		t.Fatal(err)
	}
}

func relationCount(t *testing.T, c momo.PinnedConn, schemaName string) int {
	t.Helper()
	var n int
	mustQueryRow(t, c, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = '`+schemaName+`' AND c.relname = 'momo_migrations'`, &n)
	return n
}

// A database written by an earlier momo holds an unqualified table in whatever
// schema was first on search_path. It must be found again, wherever that is, and
// no second table may appear in the current schema.
func TestHistoryTableFindsExistingUnqualifiedInstall(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	s := historySchemas(t, c, "old", "cur")
	old, cur := s[0], s[1]

	setPath(t, c, old)
	for _, ddl := range mpg.New().HistoryDDL() { // the unqualified DDL earlier versions ran
		if _, err := c.Exec(ctx, ddl, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Exec(ctx, `INSERT INTO momo_migrations (version, name, checksum, status, applied_at) VALUES ('20250101000000', 'init', 'x', 'applied', '2025-01-01T00:00:00Z')`, nil); err != nil {
		t.Fatal(err)
	}

	setPath(t, c, cur+", "+old) // current_schema() is cur, the table lives in old
	ref, err := mpg.New().HistoryTable(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"` + old + `"."momo_migrations"`; ref != want {
		t.Fatalf("HistoryTable = %s, want %s", ref, want)
	}
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	if relationCount(t, c, cur) != 0 {
		t.Fatal("a second history table was created in the current schema")
	}

	p := migrate.Pending{Version: "20260101000000", Name: "next", Statements: []migrate.Statement{{SQL: "SELECT 1"}}}
	env := migrate.RunEnv{Conn: c, Dialect: mpg.New(), Clock: time.Now}
	if _, err := migrate.RunMigration(ctx, env, p, "a1"); err != nil {
		t.Fatal(err)
	}
	var n int
	mustQueryRow(t, c, `SELECT count(*) FROM "`+old+`".momo_migrations WHERE status = 'applied'`, &n)
	if n != 2 {
		t.Fatalf("want the old row and the new one in the existing table, got %d applied rows", n)
	}
	if relationCount(t, c, cur) != 0 {
		t.Fatal("the apply created a history table in the current schema")
	}
}

// With no table anywhere, the table is created in current_schema(), qualified.
func TestHistoryTableNewInstallIsQualified(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	s := historySchemas(t, c, "fresh")
	setPath(t, c, s[0])
	ref, err := mpg.New().HistoryTable(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"` + s[0] + `"."momo_migrations"`; ref != want {
		t.Fatalf("HistoryTable = %s, want %s", ref, want)
	}
	ddl := strings.Join(mpg.New().HistoryDDLFor(ref), "\n")
	if !strings.Contains(ddl, "CREATE TABLE IF NOT EXISTS "+ref+" (") {
		t.Fatalf("DDL is not qualified: %s", ddl)
	}
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	if relationCount(t, c, s[0]) != 1 {
		t.Fatal("the history table is not in the current schema")
	}
}

// A view, or a temporary table, named momo_migrations earlier on search_path must
// not be used, even when the real table sits in a later schema.
func TestHistoryTableRefusesShadowingObject(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	s := historySchemas(t, c, "shadow", "real")
	shadow, live := s[0], s[1]

	setPath(t, c, live)
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Exec(ctx, `CREATE VIEW "`+shadow+`".momo_migrations AS SELECT 'x'::text AS version, 'x'::text AS name, 'x'::text AS checksum, 'applied'::text AS status`, nil); err != nil {
		t.Fatal(err)
	}
	setPath(t, c, shadow+", "+live)
	if _, err := mpg.New().HistoryTable(ctx, c); err == nil || !strings.Contains(err.Error(), "view") {
		t.Fatalf("a view on the search_path must be refused, got %v", err)
	}
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err == nil {
		t.Fatal("EnsureHistory must refuse a shadowing view")
	}

	// Remove the view; a temporary table now shadows the real one.
	if _, err := c.Exec(ctx, `DROP VIEW "`+shadow+`".momo_migrations`, nil); err != nil {
		t.Fatal(err)
	}
	setPath(t, c, live)
	if _, err := c.Exec(ctx, `CREATE TEMP TABLE momo_migrations (version text, name text, checksum text, status text, applied_at text)`, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = c.Exec(context.Background(), `DROP TABLE IF EXISTS pg_temp.momo_migrations`, nil) })
	if _, err := mpg.New().HistoryTable(ctx, c); err == nil || !strings.Contains(err.Error(), "temporary") {
		t.Fatalf("a temporary table must be refused, got %v", err)
	}
}

// qi quotes an identifier.
func qi(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// quotedSchema creates a schema with a name that needs quoting and drops it when
// the test ends.
func quotedSchema(t *testing.T, c momo.PinnedConn, name string) {
	t.Helper()
	if _, err := c.Exec(context.Background(), `CREATE SCHEMA `+qi(name), nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = c.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+qi(name)+` CASCADE`, nil) })
	t.Cleanup(func() { _, _ = c.Exec(context.Background(), `RESET search_path`, nil) })
}

// hookDriver pins connections whose search_path is path. flip maps the number k
// of a history lookup (a query naming to_regclass) to a search_path that is set
// just before the k-th lookup runs, which simulates the table moving under the
// engine between two lookups. Release resets the search_path.
type hookDriver struct {
	momo.Driver
	path  string
	flip  map[int]string
	locks *[]int64 // when set, collects the id of every advisory-lock attempt
}

func (d hookDriver) Pin(ctx context.Context) (momo.PinnedConn, error) {
	c, err := d.Driver.(momo.Pinner).Pin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := c.Exec(ctx, `SET search_path = `+d.path, nil); err != nil {
		_ = c.Release()
		return nil, err
	}
	return &hookConn{PinnedConn: c, flip: d.flip, locks: d.locks}, nil
}

type hookConn struct {
	momo.PinnedConn
	flip    map[int]string
	locks   *[]int64
	lookups int
}

func (c *hookConn) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	if strings.Contains(sql, "to_regclass") {
		c.lookups++
		if p, ok := c.flip[c.lookups]; ok {
			if _, err := c.PinnedConn.Exec(ctx, `SET search_path = `+p, nil); err != nil {
				return nil, err
			}
		}
	}
	if c.locks != nil && strings.Contains(sql, "pg_try_advisory_lock") && len(args) == 1 {
		if id, ok := args[0].(int64); ok {
			*c.locks = append(*c.locks, id)
		}
	}
	return c.PinnedConn.Query(ctx, sql, args)
}

func (c *hookConn) Release() error {
	_, _ = c.PinnedConn.Exec(context.Background(), `RESET search_path`, nil)
	return c.PinnedConn.Release()
}

func historyCount(t *testing.T, c momo.PinnedConn, ref string) int {
	t.Helper()
	var n int
	mustQueryRow(t, c, `SELECT count(*) FROM `+ref+` WHERE status = 'applied'`, &n)
	return n
}

// A migration that moves search_path must not redirect the history writes that
// follow it (progress, finish): they land in the table resolved at the start.
func TestHistoryWritesSurviveSearchPathChange(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	other := historySchemas(t, c, "moved")[0]
	cur := testdb.Schema(t)
	fsys := fstest.MapFS{
		"20260101000000_move.up.sql":  {Data: []byte(`SET search_path = ` + qi(other) + `;`)},
		"20260101000001_table.up.sql": {Data: []byte(`CREATE TABLE moved_here (x int);`)},
		"20260101000002_back.up.sql":  {Data: []byte(`RESET search_path;`)},
	}
	if _, err := migratetest.NewEngine(t, mpg.New(), db.Driver(), fsys).Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if n := historyCount(t, c, qi(cur)+".momo_migrations"); n != 3 {
		t.Fatalf("want 3 applied rows in the original schema, got %d", n)
	}
	if relationCount(t, c, other) != 0 {
		t.Fatal("a history table appeared in the schema the migration switched to")
	}
	var n int
	mustQueryRow(t, c, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = '`+other+`' AND c.relname = 'moved_here'`, &n)
	if n != 1 {
		t.Fatal("the migration did not run under the switched search_path")
	}
}

// With the same table name in two schemas, the first on the search_path wins,
// the way an unqualified name resolves.
func TestHistoryTableFirstOnSearchPathWins(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	s := historySchemas(t, c, "a", "b")
	for _, name := range s {
		setPath(t, c, name)
		if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ path, want string }{{s[0] + ", " + s[1], s[0]}, {s[1] + ", " + s[0], s[1]}} {
		setPath(t, c, tc.path)
		ref, err := mpg.New().HistoryTable(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if want := qi(tc.want) + `."momo_migrations"`; ref != want {
			t.Fatalf("path %s: HistoryTable = %s, want %s", tc.path, ref, want)
		}
	}
}

// Schema names that need quoting (upper case, an embedded double quote) work for
// the table reference, the DDL, the history writes and the lock key.
func TestHistoryTableQuotedSchemaNames(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	for _, name := range []string{"Mixed_" + testdb.Schema(t), `we"ird_` + testdb.Schema(t)} {
		quotedSchema(t, c, name)
		setPath(t, c, qi(name))
		ref, err := mpg.New().HistoryTable(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if want := qi(name) + `."momo_migrations"`; ref != want {
			t.Fatalf("HistoryTable = %s, want %s", ref, want)
		}
		if got, want := mpg.New().HistoryLockKey(ref), "momo:"+name+".momo_migrations"; got != want {
			t.Fatalf("HistoryLockKey = %q, want %q", got, want)
		}
		if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
			t.Fatal(err)
		}
		p := migrate.Pending{Version: "20260101000000", Name: "x", Statements: []migrate.Statement{{SQL: "SELECT 1"}}}
		if _, err := migrate.RunMigration(ctx, migrate.RunEnv{Conn: c, Dialect: mpg.New(), Clock: time.Now}, p, "a1"); err != nil {
			t.Fatal(err)
		}
		if n := historyCount(t, c, ref); n != 1 {
			t.Fatalf("schema %q: want 1 applied row, got %d", name, n)
		}
	}
}

// A table owned by a role whose privileges the connecting role does not hold is
// refused, both when it is found and when EnsureHistory checks it. A superuser
// connecting role trusts only tables owned by itself or by another superuser.
func TestHistoryTableRefusesForeignOwner(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	s := historySchemas(t, c, "owned")[0]
	owner, user := "momo_own_"+testdb.Schema(t), "momo_usr_"+testdb.Schema(t)
	for _, r := range []string{owner, user} {
		if _, err := c.Exec(ctx, `CREATE ROLE `+qi(r)+` NOLOGIN`, nil); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = c.Exec(context.Background(), `RESET ROLE`, nil)
		for _, r := range []string{owner, user} {
			_, _ = c.Exec(context.Background(), `DROP OWNED BY `+qi(r), nil)
			_, _ = c.Exec(context.Background(), `DROP ROLE IF EXISTS `+qi(r), nil)
		}
	})
	setPath(t, c, s)
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`ALTER TABLE ` + qi(s) + `.momo_migrations OWNER TO ` + qi(owner),
		`GRANT USAGE ON SCHEMA ` + qi(s) + ` TO ` + qi(user),
		`SET ROLE ` + qi(user),
	} {
		if _, err := c.Exec(ctx, q, nil); err != nil {
			t.Fatal(err)
		}
	}
	for name, call := range map[string]func() error{
		"HistoryTable":  func() error { _, err := mpg.New().HistoryTable(ctx, c); return err },
		"EnsureHistory": func() error { return migrate.EnsureHistory(ctx, c, mpg.New()) },
	} {
		err := call()
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("%s: want ErrValidation, got %v", name, err)
		}
		for _, want := range []string{s, "momo_migrations", owner} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("%s: error %q does not name %q", name, err, want)
			}
		}
	}
	// The superuser migrator holds every role's privileges, but still refuses
	// a table owned by an ordinary role.
	if _, err := c.Exec(ctx, `RESET ROLE`, nil); err != nil {
		t.Fatal(err)
	}
	_, err := mpg.New().HistoryTable(ctx, c)
	if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), owner) || !strings.Contains(err.Error(), "superuser") {
		t.Fatalf("a superuser must refuse a table owned by a non-superuser, got %v", err)
	}
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("EnsureHistory must refuse it for a superuser too, got %v", err)
	}
	// Owned by another superuser: accepted. Owned by the superuser itself: accepted.
	super := "momo_sup_" + testdb.Schema(t)
	if _, err := c.Exec(ctx, `CREATE ROLE `+qi(super)+` NOLOGIN SUPERUSER`, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = c.Exec(context.Background(), `DROP OWNED BY `+qi(super), nil)
		_, _ = c.Exec(context.Background(), `DROP ROLE IF EXISTS `+qi(super), nil)
	})
	if _, err := c.Exec(ctx, `ALTER TABLE `+qi(s)+`.momo_migrations OWNER TO `+qi(super), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mpg.New().HistoryTable(ctx, c); err != nil {
		t.Fatalf("a table owned by another superuser must be accepted, got %v", err)
	}
	if _, err := c.Exec(ctx, `ALTER TABLE `+qi(s)+`.momo_migrations OWNER TO current_user`, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mpg.New().HistoryTable(ctx, c); err != nil {
		t.Fatalf("a table owned by the connecting superuser must be accepted, got %v", err)
	}
}

// With an empty search_path there is no current schema: the read-only calls
// report an empty history, the writers fail with a clear message.
func TestHistoryNoCurrentSchema(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	fsys := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("SELECT 1;")}}
	e := migratetest.NewEngine(t, mpg.New(), hookDriver{Driver: db.Driver(), path: `''`}, fsys)
	pend, err := e.Plan(ctx)
	if err != nil || len(pend) != 1 {
		t.Fatalf("Plan = %v, %v; want one pending migration", pend, err)
	}
	st, err := e.Status(ctx)
	if err != nil || len(st) != 1 {
		t.Fatalf("Status = %v, %v; want one pending version", st, err)
	}
	c, err := hookDriver{Driver: db.Driver(), path: `''`}.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Release() }()
	if _, err := mpg.New().HistoryTable(ctx, c); !errors.Is(err, migrate.ErrNoHistoryTable) {
		t.Fatalf("want ErrNoHistoryTable, got %v", err)
	}
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err == nil || !strings.Contains(err.Error(), "no current schema") {
		t.Fatalf("Apply must fail naming the missing current schema, got %v", err)
	}
}

// The table appearing in another schema between the CREATE TABLE and the
// re-resolve in ensureHistory is reported, not written through.
func TestEnsureHistoryDetectsMoveAfterCreate(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	old := historySchemas(t, c, "after")[0]
	cur := testdb.Schema(t)
	setPath(t, c, qi(old))
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	setPath(t, c, qi(cur))
	fsys := fstest.MapFS{"20260101000000_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}}
	// Lookups: 1 and 2 around the lock, 3 in Apply, 4 right after the CREATE.
	flip := map[int]string{4: qi(old) + ", " + qi(cur)}
	e := migratetest.NewEngine(t, mpg.New(), hookDriver{Driver: db.Driver(), path: qi(cur), flip: flip}, fsys)
	_, err := e.Apply(ctx, migrate.ApplyOptions{})
	if err == nil || !strings.Contains(err.Error(), "after it was created") {
		t.Fatalf("want the moved-after-create error, got %v", err)
	}
	if n := historyCount(t, c, qi(old)+".momo_migrations"); n != 0 {
		t.Fatalf("nothing may be written through the other table, got %d rows", n)
	}
}

// Rehash writes through the qualified name of a table that is not in the current
// schema, and the advisory lock is keyed on that table's schema.
func TestHistoryRehashAndLockKeyUseResolvedTable(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	old := historySchemas(t, c, "old")[0]
	cur := testdb.Schema(t)
	const v = "20260101000000"
	fsys := fstest.MapFS{v + "_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}}

	first := migratetest.NewEngine(t, mpg.New(), hookDriver{Driver: db.Driver(), path: qi(old)}, fsys)
	if _, err := first.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	e := migratetest.NewEngine(t, mpg.New(), hookDriver{Driver: db.Driver(), path: qi(cur) + ", " + qi(old)}, fsys, migrate.WithLockWait(300*time.Millisecond))

	// The lock named for the table's own schema blocks Apply; the lock named
	// for the current schema does not.
	for key, wantLocked := range map[string]bool{"momo:" + old + ".momo_migrations": true, "momo:" + cur + ".momo_migrations": false} {
		unlock, _, err := mpg.New().Lock(ctx, c, key)
		if err != nil {
			t.Fatal(err)
		}
		_, aerr := e.Apply(ctx, migrate.ApplyOptions{})
		if errors.Is(aerr, migrate.ErrLocked) != wantLocked {
			t.Fatalf("key %s: want locked=%v, got %v", key, wantLocked, aerr)
		}
		if err := unlock(); err != nil {
			t.Fatal(err)
		}
	}

	fsys[v+"_a.up.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE a (x int); -- reformatted")}
	res, err := e.Rehash(ctx, v, true)
	if err != nil || !res.Accepted {
		t.Fatalf("Rehash = %+v, %v", res, err)
	}
	var note string
	mustQueryRow(t, c, `SELECT error FROM `+qi(old)+`.momo_migrations WHERE version = '`+v+`'`, &note)
	if !strings.Contains(note, "accepted by rehash") {
		t.Fatalf("the row in the resolved table was not rehashed: %q", note)
	}
	if relationCount(t, c, cur) != 0 {
		t.Fatal("Rehash created a history table in the current schema")
	}
}

// When the table appears in an earlier schema between the lookup and the lock,
// the lock is released and retaken for the new name once; a second move fails
// and leaves no lock behind.
func TestHistoryLockRetriesWhenTableMoves(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	c := pinned(t, db)
	old := historySchemas(t, c, "late")[0]
	cur := testdb.Schema(t)
	const v = "20260101000000"
	fsys := fstest.MapFS{v + "_a.up.sql": {Data: []byte("CREATE TABLE a (x int);")}}
	setPath(t, c, qi(old))
	if err := migrate.EnsureHistory(ctx, c, mpg.New()); err != nil {
		t.Fatal(err)
	}
	setPath(t, c, qi(cur))

	// Lookups: 1 before the lock (path cur: no table, name in cur), 2 after it
	// (flipped to old first: name differs), 3 and 4 on the retry (no flip).
	flip := map[int]string{2: qi(old) + ", " + qi(cur)}
	var locks []int64
	e := migratetest.NewEngine(t, mpg.New(), hookDriver{Driver: db.Driver(), path: qi(cur), flip: flip, locks: &locks}, fsys)
	if _, err := e.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatalf("one move must be retried: %v", err)
	}
	if len(locks) != 2 || locks[0] == locks[1] {
		t.Fatalf("want two lock attempts on different keys (the retry), got %v", locks)
	}
	if n := historyCount(t, c, qi(old)+".momo_migrations"); n != 1 {
		t.Fatalf("want the row in the table that appeared, got %d", n)
	}

	// A second move on the retry fails.
	fsys["20260101000001_b.up.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE b (x int);")}
	flip = map[int]string{2: qi(old) + ", " + qi(cur), 4: qi(cur)}
	e = migratetest.NewEngine(t, mpg.New(), hookDriver{Driver: db.Driver(), path: qi(cur), flip: flip}, fsys)
	_, err := e.Apply(ctx, migrate.ApplyOptions{})
	if err == nil || !strings.Contains(err.Error(), "moved while taking the migration lock") {
		t.Fatalf("want the moved-table error, got %v", err)
	}
	for _, key := range []string{"momo:" + old + ".momo_migrations", "momo:" + cur + ".momo_migrations"} {
		unlock, _, lerr := mpg.New().Lock(ctx, c, key)
		if lerr != nil {
			t.Fatalf("lock %s left behind: %v", key, lerr)
		}
		_ = unlock()
	}
}
