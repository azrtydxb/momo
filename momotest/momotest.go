// SPDX-License-Identifier: Apache-2.0

// Package momotest gives an application's tests a real PostgreSQL database
// with the project's migrations applied, isolated per test.
//
// Two modes cover the usual needs:
//
//   - New returns a *momo.DB on a schema private to the test. The schema is
//     created, the migrations are applied to it, and it is dropped when the
//     test ends. Use it when the code under test commits transactions.
//   - NewTx returns a *momo.Tx that is rolled back when the test ends. The
//     migrations are applied once to a shared template schema, so each test
//     only pays for a BEGIN. Use it for the fast path.
//
// Both are safe with t.Parallel and the race detector: every call takes its
// own connection pool, and the shared setup runs under a PostgreSQL advisory
// lock, so tests and test binaries running at the same time do not collide.
//
// The server comes from an environment variable, MOMO_TEST_POSTGRES_URL by
// default (WithEnv changes it). The URL must be a postgres:// or postgresql://
// URL and its role must be able to create schemas. When the variable is unset
// the test is skipped, unless MOMO_REQUIRE_INTEGRATION=1, where it fails
// instead, so a CI job cannot go green by skipping every database test.
//
// Tests that call this package open a database, so keep them in files with the
// integration build tag (or a tag of your own) so a plain go test stays
// database-free.
package momotest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
)

const (
	// DefaultEnv is the environment variable that holds the server URL.
	DefaultEnv = "MOMO_TEST_POSTGRES_URL"
	// RequireEnv, set to 1, turns "the server URL is not set" from a skip
	// into a failure.
	RequireEnv = "MOMO_REQUIRE_INTEGRATION"

	// Advisory lock keys; the server hashes them (hashtext) to lock ids.
	extLockKey      = "momotest:extensions"
	templateLockKey = "momotest:template"
	readyMarker     = "momotest:ready"
	opTimeout       = 30 * time.Second
	// dropTimeout bounds DROP SCHEMA of a large schema on a busy server.
	dropTimeout  = 3 * time.Minute
	applyTimeout = 5 * time.Minute
)

type config struct {
	env        string
	migrations fs.FS
	momoOpts   []momo.Option
	extensions []string
	maxConns   int32
}

// Option configures New and NewTx.
type Option func(*config)

// WithEnv reads the server URL from the environment variable name instead of
// MOMO_TEST_POSTGRES_URL.
func WithEnv(name string) Option { return func(c *config) { c.env = name } }

// WithMigrations applies the migrations in fsys, typically an embed.FS
// narrowed with fs.Sub to the migrations directory, to the database before
// the test sees it. The directory needs no momo.sum. Without this option the
// database starts empty.
func WithMigrations(fsys fs.FS) Option { return func(c *config) { c.migrations = fsys } }

// WithMomoOptions passes opts to momo.New, for example momo.WithHook.
func WithMomoOptions(opts ...momo.Option) Option {
	return func(c *config) { c.momoOpts = append(c.momoOpts, opts...) }
}

// WithExtensions creates the named extensions, such as "vector" or "pgcrypto",
// in the public schema before the migrations run. The role needs the right to
// create them; an extension that exists is left alone. With "vector" the pool
// also registers the binary vector codec.
func WithExtensions(names ...string) Option {
	return func(c *config) { c.extensions = append(c.extensions, names...) }
}

// WithMaxConns sets the pool size. The default is 4 for New and 2 for NewTx.
func WithMaxConns(n int32) Option { return func(c *config) { c.maxConns = n } }

func newConfig(opts []Option) *config {
	c := &config{env: DefaultEnv}
	for _, o := range opts {
		o(c)
	}
	// Extension names are matched and created in lower case, so
	// WithExtensions("Vector") means vector.
	for i, e := range c.extensions {
		c.extensions[i] = strings.ToLower(e)
	}
	return c
}

var errSkip = errors.New("momotest: server URL not set")

// resolve returns the value of env, errSkip when it is unset and require is
// false, and a descriptive error when it is unset and require is true.
func resolve(env string, lookup func(string) (string, bool), require bool) (string, error) {
	if v, ok := lookup(env); ok && v != "" {
		return v, nil
	}
	if !require {
		return "", errSkip
	}
	return "", fmt.Errorf("momotest: %s is not set and %s=1", env, RequireEnv)
}

// serverURL returns the administrator URL, skipping or failing t when the
// variable is unset.
func (c *config) serverURL(t testing.TB) string {
	t.Helper()
	v, err := resolve(c.env, os.LookupEnv, os.Getenv(RequireEnv) == "1")
	switch {
	case errors.Is(err, errSkip):
		t.Skip(err.Error() + ": " + c.env)
	case err != nil:
		t.Fatal(err)
	}
	return v
}

// randomName returns prefix followed by 16 lowercase hex characters.
func randomName(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("momotest: crypto/rand failed: " + err.Error())
	}
	return prefix + hex.EncodeToString(b[:])
}

// withSearchPath returns raw with search_path=name,public. raw must be a
// postgres:// or postgresql:// URL; a keyword/value DSN cannot carry the
// parameter and would silently lose isolation.
func withSearchPath(raw, name string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return "", errors.New("momotest: a postgres:// URL is required to set search_path")
	}
	q := u.Query()
	q.Set("search_path", name+",public")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ddl runs format with the identifier name quoted into its single %s. The
// identifier is generated here or comes from the caller's extension list, and
// is quoted either way.
func ddl(ctx context.Context, conn *pgx.Conn, format, name string) error {
	_, err := conn.Exec(ctx, fmt.Sprintf(format, pgx.Identifier{name}.Sanitize()))
	return err
}

// withLock runs fn on conn while holding the session advisory lock named key.
// The lock id is hashtext(key) on the server, so it needs no number reserved
// in the application's own lock space.
func withLock(ctx context.Context, conn *pgx.Conn, key string, fn func() error) (err error) {
	if _, err = conn.Exec(ctx, "SELECT pg_advisory_lock(hashtext($1))", key); err != nil {
		return err
	}
	defer func() {
		if _, uerr := conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock(hashtext($1))", key); err == nil {
			err = uerr
		}
	}()
	return fn()
}

func ensureExtensions(ctx context.Context, conn *pgx.Conn, names []string) error {
	if len(names) == 0 {
		return nil
	}
	return withLock(ctx, conn, extLockKey, func() error {
		for _, n := range names {
			if err := ddl(ctx, conn, "CREATE EXTENSION IF NOT EXISTS %s SCHEMA public", n); err != nil {
				return fmt.Errorf("create extension %s: %w", n, err)
			}
		}
		return nil
	})
}

// connect opens an administrator connection.
func connect(ctx context.Context, t testing.TB, raw string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatalf("momotest: connect: %v", err)
	}
	return conn
}

func dropSchema(t testing.TB, raw, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), dropTimeout)
	defer cancel()
	conn, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Errorf("momotest: connect to drop schema %s: %v", name, err)
		return
	}
	defer conn.Close(ctx)
	if err := ddl(ctx, conn, "DROP SCHEMA IF EXISTS %s CASCADE", name); err != nil {
		t.Errorf("momotest: drop schema %s: %v", name, err)
	}
}

// openPool opens a pool whose connections have schema first on the search
// path, and closes it in t.Cleanup.
func (c *config) openPool(t testing.TB, raw, schema string, defaultConns int32) *pgxpool.Pool {
	t.Helper()
	u, err := withSearchPath(raw, schema)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(u)
	if err != nil {
		t.Fatalf("momotest: parse URL: %v", err)
	}
	cfg.MaxConns = defaultConns
	if c.maxConns > 0 {
		cfg.MaxConns = c.maxConns
	}
	pgxdriver.InstrumentPool(cfg)
	for _, e := range c.extensions {
		if e == "vector" {
			pgxdriver.ConfigureVectors(cfg)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("momotest: open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

var (
	createExtension = regexp.MustCompile(`(?is)\bcreate\s+extension\s+(?:if\s+not\s+exists\s+)?"?([a-z0-9_-]+)"?`)
	createSchema    = regexp.MustCompile(`(?is)\bcreate\s+schema\b`)
)

// checkMigrations fails early on migration SQL that would escape the per-test
// schema.
//
// An extension is created once per database, not once per schema, so a
// migration that creates one lands it in whichever test's schema ran first,
// and DROP SCHEMA removes it under the other tests. List the extension in
// WithExtensions instead; it is then installed in public, which is on every
// search path. A migration that creates another schema would collide between
// parallel tests, so keep every object unqualified in the test schema.
//
// The check reads the SQL with comments and string literals removed (see
// stripSQL), so a comment that mentions the words does not trip it and a
// comment between the keywords does not hide them. Dollar-quoted bodies are
// checked as code: a DO block that creates an extension is caught, at the price
// of flagging a dollar-quoted string that merely contains such text.
func (c *config) checkMigrations() error {
	if c.migrations == nil {
		return nil
	}
	return fs.WalkDir(c.migrations, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".sql") {
			return err
		}
		b, err := fs.ReadFile(c.migrations, p)
		if err != nil {
			return err
		}
		b = stripSQL(b)
		for _, m := range createExtension.FindAllSubmatch(b, -1) {
			name := strings.ToLower(string(m[1]))
			if !slices.Contains(c.extensions, name) {
				return fmt.Errorf("migration %s creates extension %s; list it in momotest.WithExtensions so it is installed once in the public schema", p, name)
			}
		}
		if createSchema.Match(b) {
			return fmt.Errorf("migration %s creates a schema; momotest runs migrations in a per-test schema, so objects must be unqualified and no extra schema may be created", p)
		}
		return nil
	})
}

// applyTo runs the configured migrations through drv.
func (c *config) applyTo(ctx context.Context, drv momo.Driver) error {
	if c.migrations == nil {
		return nil
	}
	e, err := migrate.NewEngine(drv, c.migrations, migrate.WithDialect(mpg.New()), migrate.WithoutSum())
	if err != nil {
		return err
	}
	_, err = e.Apply(ctx, migrate.ApplyOptions{})
	return err
}

// New returns a *momo.DB on a schema private to t, with the migrations
// applied. The pool is closed and the schema dropped when t finishes. The
// test is skipped when the server URL variable is unset.
func New(t testing.TB, opts ...Option) *momo.DB {
	t.Helper()
	c := newConfig(opts)
	raw := c.serverURL(t)
	if err := c.checkMigrations(); err != nil {
		t.Fatalf("momotest: %v", err)
	}
	name := randomName("momotest_")

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()
	conn := connect(ctx, t, raw)
	defer conn.Close(ctx)
	if err := ensureExtensions(ctx, conn, c.extensions); err != nil {
		t.Fatalf("momotest: %v", err)
	}
	if err := ddl(ctx, conn, "CREATE SCHEMA %s", name); err != nil {
		t.Fatalf("momotest: create schema %s: %v", name, err)
	}
	// Cleanups run last in first out: the pool, registered later, is closed
	// before the schema is dropped.
	t.Cleanup(func() { dropSchema(t, raw, name) })

	pool := c.openPool(t, raw, name, 4)
	db := momo.New(pgxdriver.New(pool), c.momoOpts...)
	actx, acancel := context.WithTimeout(context.Background(), applyTimeout)
	defer acancel()
	if err := c.applyTo(actx, db.Driver()); err != nil {
		t.Fatalf("momotest: apply migrations: %v", err)
	}
	return db
}

// momoVersion returns the version of the momo module this binary links, so a
// template built by one engine version is not reused by another. It is "devel"
// when momo is the main module or the build carries no module information.
func momoVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "devel"
	}
	for _, d := range bi.Deps {
		if d.Path == "github.com/azrtydxb/momo" {
			return d.Version
		}
	}
	return "devel"
}

// templateName names the shared schema for this migration set: the hash covers
// the file names and contents of the migrations and the extension list, so a
// changed migration gets a fresh template and a stale one is never reused.
func (c *config) templateName() (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "momotest-template-v1\x00%s\x00%s\x00", momoVersion(), strings.Join(c.extensions, ","))
	if c.migrations != nil {
		err := fs.WalkDir(c.migrations, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := fs.ReadFile(c.migrations, p)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "%s\x00%d\x00", p, len(b))
			h.Write(b)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return "momotest_tpl_" + hex.EncodeToString(h.Sum(nil))[:16], nil
}

type preparedKey struct {
	fsys fs.FS
	exts string
}

var preparedCache sync.Map // preparedKey -> template name; successes only

// preparedTemplate checks the migrations and names their template. A success
// is remembered per migration FS value and extension list, so a suite that
// builds its FS once (a package variable or an embed.FS) walks and hashes the
// files once. Errors are never remembered, so a transient read error fails one
// test, not every later one.
//
// The key is the FS value itself. A value that cannot be hashed, even when its
// type looks comparable (a struct wrapping an fstest.MapFS), is recomputed on
// every call, as is one rebuilt on every call (fs.Sub returns a new value each
// time). Changing the files behind a remembered FS while the test binary runs,
// for example rewriting the directory an os.DirFS points at, is unsupported:
// the remembered name would be stale.
func (c *config) preparedTemplate() (string, error) {
	compute := func() (string, error) {
		if err := c.checkMigrations(); err != nil {
			return "", err
		}
		name, err := c.templateName()
		if err != nil {
			return "", fmt.Errorf("hash migrations: %w", err)
		}
		return name, nil
	}
	// Value.Comparable looks at the dynamic value, unlike the static
	// Type.Comparable, which would let an interface field holding a map through
	// to a panic in sync.Map.
	if c.migrations != nil && !reflect.ValueOf(c.migrations).Comparable() {
		return compute()
	}
	key := preparedKey{c.migrations, strings.Join(c.extensions, ",")}
	if v, ok := preparedCache.Load(key); ok {
		return v.(string), nil
	}
	name, err := compute()
	if err != nil {
		return "", err
	}
	preparedCache.Store(key, name)
	return name, nil
}

var (
	templateMu    sync.Mutex
	templateReady sync.Map // raw URL + NUL + template name -> struct{}
)

// ensureTemplate creates and migrates the template schema unless a complete
// one exists. It is idempotent and safe across goroutines and processes. After
// the first success in this process it returns at once, so later tests pay
// for no admin connection and no lock; a template dropped by hand while the
// test binary runs is not noticed until the next run.
func (c *config) ensureTemplate(t testing.TB, raw, name string) {
	t.Helper()
	key := raw + "\x00" + name
	if _, ok := templateReady.Load(key); ok {
		return
	}
	// One build at a time per process; the advisory lock below covers other
	// processes. Tests that wait here find the key stored and leave.
	templateMu.Lock()
	defer templateMu.Unlock()
	if _, ok := templateReady.Load(key); ok {
		return
	}
	c.buildOnce(t, raw, name)
	templateReady.Store(key, struct{}{})
}

func (c *config) buildOnce(t testing.TB, raw, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	conn := connect(ctx, t, raw)
	defer conn.Close(ctx)
	if err := ensureExtensions(ctx, conn, c.extensions); err != nil {
		t.Fatalf("momotest: %v", err)
	}
	err := withLock(ctx, conn, templateLockKey, func() error {
		var comment *string
		err := conn.QueryRow(ctx,
			"SELECT obj_description(oid, 'pg_namespace') FROM pg_namespace WHERE nspname = $1", name).Scan(&comment)
		switch {
		case err == nil && comment != nil && *comment == readyMarker:
			return nil
		case err == nil:
			// A killed run left a half-built template; rebuild it.
			if err := ddl(ctx, conn, "DROP SCHEMA %s CASCADE", name); err != nil {
				return err
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if err := ddl(ctx, conn, "CREATE SCHEMA %s", name); err != nil {
			return err
		}
		if err := c.buildTemplate(ctx, raw, name); err != nil {
			_ = ddl(context.WithoutCancel(ctx), conn, "DROP SCHEMA IF EXISTS %s CASCADE", name)
			return err
		}
		return ddl(ctx, conn, "COMMENT ON SCHEMA %s IS '"+readyMarker+"'", name)
	})
	if err != nil {
		t.Fatalf("momotest: build template schema %s: %v", name, err)
	}
}

func (c *config) buildTemplate(ctx context.Context, raw, name string) error {
	u, err := withSearchPath(raw, name)
	if err != nil {
		return err
	}
	cfg, err := pgxpool.ParseConfig(u)
	if err != nil {
		return err
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	return c.applyTo(ctx, pgxdriver.New(pool))
}

// NewTx returns a *momo.Tx that sees the migrated schema and is rolled back
// when t finishes, so nothing a test writes outlives it.
//
// The migrations are applied once to a template schema named after a hash of
// their contents and kept on the server between runs; each call only opens a
// transaction on it, which is much cheaper than New. A changed migration gets
// a new template, and the old ones can be dropped at will (they are named
// momotest_tpl_*).
//
// The returned Tx is adopted: Commit and Rollback on it return an error
// matching momo.ErrAdoptedTx, so a test cannot make its writes permanent.
// Tx on it opens savepoints as usual. Code that needs a real commit belongs
// in a test that uses New. Parallel tests do not see each other's rows, but
// two that write the same unique key wait for each other until one finishes;
// give them distinct keys or use New.
func NewTx(t testing.TB, opts ...Option) *momo.Tx {
	t.Helper()
	c := newConfig(opts)
	raw := c.serverURL(t)
	name, err := c.preparedTemplate()
	if err != nil {
		t.Fatalf("momotest: %v", err)
	}
	c.ensureTemplate(t, raw, name)

	pool := c.openPool(t, raw, name, 2)
	db := momo.New(pgxdriver.New(pool), c.momoOpts...)
	dtx, err := db.Driver().Begin(context.Background(), momo.TxOptions{})
	if err != nil {
		t.Fatalf("momotest: begin: %v", err)
	}
	// Registered after the pool's Close, so it runs first and the pool is
	// not waiting on a connection this transaction still holds.
	t.Cleanup(func() {
		rctx, rcancel := context.WithTimeout(context.Background(), opTimeout)
		defer rcancel()
		if err := dtx.Rollback(rctx); err != nil && !errors.Is(err, momo.ErrTxDone) {
			t.Errorf("momotest: rollback: %v", err)
		}
	})
	return db.AdoptTx(dtx)
}
