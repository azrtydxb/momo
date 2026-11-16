// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
)

const (
	// Template databases and clones. Database names are cluster-wide, so the
	// locks that guard them are taken on one fixed database (lockDatabase).
	templateDBPrefix = "momotest_tpldb_"
	cloneDBPrefix    = "momotest_db_"
	lockDatabase     = "postgres"

	templateDBLockKey = "momotest:templatedb"
	cloneLockKey      = "momotest:clone"
	// readyPrefix starts the comment of a finished template database; the rest
	// is the Unix time of its last use, which the sweep reads.
	readyPrefix = "momotest:ready:"
	// sweepAfter is how long a template database may go unused before the next
	// template build drops it.
	sweepAfter = 7 * 24 * time.Hour
	// cloneTimeout bounds one CREATE DATABASE ... TEMPLATE, including the wait
	// for a template whose last connection is still closing.
	cloneTimeout = 2 * time.Minute

	// sqlstateObjectInUse is "source database is being accessed by other users".
	sqlstateObjectInUse = "55006"
	// sqlstateInsufficientPrivilege is "permission denied".
	sqlstateInsufficientPrivilege = "42501"
	// sqlstateDuplicateDatabase is "database already exists".
	sqlstateDuplicateDatabase = "42P04"

	// forceDropVersion is the first server version with DROP DATABASE ... WITH
	// (FORCE) (PostgreSQL 13).
	forceDropVersion = 130000
)

var (
	// databaseTemplateReady holds raw URL + NUL + schema template name -> the
	// ready template database name, after this process has found or built it.
	databaseTemplateReady sync.Map
	// noCreateDB holds raw URL -> reason, for a server on which the role cannot
	// create or copy databases, so later tests go straight to the schema path.
	noCreateDB sync.Map
	// pathLogged holds the messages t.Log already printed in this process.
	pathLogged sync.Map
)

// logOnce prints msg through t.Log the first time this process sees it.
func logOnce(t testing.TB, msg string) {
	t.Helper()
	if _, loaded := pathLogged.LoadOrStore(msg, struct{}{}); !loaded {
		t.Log(msg)
	}
}

// databaseURL returns raw with its database replaced by db.
func databaseURL(raw, db string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return "", errors.New("momotest: a postgres:// URL is required to name another database")
	}
	u.Path = "/" + db
	u.RawPath = ""
	return u.String(), nil
}

func hasPgCode(err error, code string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == code
}

// lockConn opens the administrator connection that takes the template and clone
// locks. Advisory locks are per database but template and clone names are
// cluster-wide, so every process locks on the same database: postgres, or the
// URL's own when the role cannot connect there.
func lockConn(ctx context.Context, t testing.TB, raw string) *pgx.Conn {
	t.Helper()
	if u, err := databaseURL(raw, lockDatabase); err == nil {
		if conn, err := pgx.Connect(ctx, u); err == nil {
			return conn
		}
	}
	return connect(ctx, t, raw)
}

// hashTemplateDB names the template database. On top of the schema template
// name (migrations, extension list, momo version) it covers the server version
// and the version of every listed extension the server would install, because
// a template built by another server or extension release is not the schema a
// test on this one expects.
func hashTemplateDB(schemaName, serverVersion string, extVersions []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "momotest-templatedb-v1\x00%s\x00%s\x00%s", schemaName, serverVersion, strings.Join(extVersions, ","))
	return templateDBPrefix + hex.EncodeToString(h.Sum(nil))[:16]
}

// databaseTemplateName reads the server version and the extension versions
// through conn and names the template database.
func (c *config) databaseTemplateName(ctx context.Context, conn *pgx.Conn, schemaName string) (string, error) {
	var version string
	if err := conn.QueryRow(ctx, "SELECT current_setting('server_version_num')").Scan(&version); err != nil {
		return "", err
	}
	var exts []string
	if len(c.extensions) > 0 {
		rows, err := conn.Query(ctx, "SELECT name || '=' || coalesce(default_version, '') FROM pg_available_extensions WHERE name = ANY($1) ORDER BY name", c.extensions)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return "", err
			}
			exts = append(exts, s)
		}
		if err := rows.Err(); err != nil {
			return "", err
		}
	}
	return hashTemplateDB(schemaName, version, exts), nil
}

// templateDatabase returns the name of the ready template database for this
// configuration, building it first under an advisory lock. When the role cannot
// create databases it returns an empty name and the reason.
func (c *config) templateDatabase(t testing.TB, raw string) (name, reason string) {
	t.Helper()
	schemaName, err := c.preparedTemplate()
	if err != nil {
		t.Fatalf("momotest: %v", err)
	}
	key := raw + "\x00" + schemaName
	cached := func() (string, string, bool) {
		if v, ok := noCreateDB.Load(raw); ok {
			return "", v.(string), true
		}
		if v, ok := databaseTemplateReady.Load(key); ok {
			return v.(string), "", true
		}
		return "", "", false
	}
	if n, r, ok := cached(); ok {
		return n, r
	}
	templateMu.Lock()
	defer templateMu.Unlock()
	if n, r, ok := cached(); ok {
		return n, r
	}
	name, reason = c.buildTemplateDatabase(t, raw, schemaName)
	if reason != "" {
		noCreateDB.Store(raw, reason)
		return "", reason
	}
	databaseTemplateReady.Store(key, name)
	return name, ""
}

// templateState reports whether the template database name carries the ready
// comment and whether it exists at all.
func templateState(ctx context.Context, conn *pgx.Conn, name string) (ready, exists bool, err error) {
	var comment *string
	err = conn.QueryRow(ctx,
		"SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname = $1", name).Scan(&comment)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, false, nil
	case err != nil:
		return false, false, err
	}
	return comment != nil && strings.HasPrefix(*comment, readyPrefix), true, nil
}

// markReady stamps the template database with the ready comment and now.
func markReady(ctx context.Context, conn *pgx.Conn, name string) error {
	return ddl(ctx, conn, "COMMENT ON DATABASE %s IS '"+readyPrefix+strconv.FormatInt(time.Now().Unix(), 10)+"'", name)
}

// waitReady polls until another process has finished building name.
func waitReady(ctx context.Context, conn *pgx.Conn, name string) error {
	for {
		ready, _, err := templateState(ctx, conn, name)
		if err != nil || ready {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("template database %s was never marked ready: %w", name, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// dropDatabaseOn drops name through conn. PostgreSQL 13 and newer disconnect
// its sessions with WITH (FORCE); older servers get their backends terminated
// first.
func dropDatabaseOn(ctx context.Context, conn *pgx.Conn, name string) error {
	var version int
	if err := conn.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); err != nil {
		return err
	}
	if version >= forceDropVersion {
		return ddl(ctx, conn, "DROP DATABASE IF EXISTS %s WITH (FORCE)", name)
	}
	if _, err := conn.Exec(ctx, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()", name); err != nil {
		return err
	}
	return ddl(ctx, conn, "DROP DATABASE IF EXISTS %s", name)
}

// sweepTemplates drops the template databases, other than keep, that no process
// has used for sweepAfter. Best effort: a failure leaves them for the next run.
func sweepTemplates(ctx context.Context, conn *pgx.Conn, keep string) {
	rows, err := conn.Query(ctx, `SELECT datname, coalesce(shobj_description(oid, 'pg_database'), '') FROM pg_database
WHERE datname LIKE 'momotest\_tpldb\_%' AND datname <> $1`, keep)
	if err != nil {
		return
	}
	var stale []string
	for rows.Next() {
		var name, comment string
		if err := rows.Scan(&name, &comment); err != nil {
			break
		}
		stamp, ok := strings.CutPrefix(comment, readyPrefix)
		if !ok {
			continue
		}
		if at, err := strconv.ParseInt(stamp, 10, 64); err == nil && time.Since(time.Unix(at, 0)) > sweepAfter {
			stale = append(stale, name)
		}
	}
	rows.Close()
	for _, name := range stale {
		_ = dropDatabaseOn(ctx, conn, name)
	}
}

// buildTemplateDatabase names the template database, creates and migrates it
// unless a complete one exists, stamps it as used now and sweeps old ones. A
// non-empty reason means the role cannot do it.
func (c *config) buildTemplateDatabase(t testing.TB, raw, schemaName string) (name, reason string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	conn := lockConn(ctx, t, raw)
	defer conn.Close(ctx)

	var can bool
	if err := conn.QueryRow(ctx, "SELECT rolcreatedb OR rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&can); err != nil {
		t.Fatalf("momotest: read the role's privileges: %v", err)
	}
	if !can {
		return "", "the role lacks CREATEDB"
	}
	name, err := c.databaseTemplateName(ctx, conn, schemaName)
	if err != nil {
		t.Fatalf("momotest: name the template database: %v", err)
	}
	err = withLock(ctx, conn, templateDBLockKey, func() error {
		ready, exists, err := templateState(ctx, conn, name)
		if err != nil {
			return err
		}
		if !ready {
			if exists {
				// A killed run left a half-built template; rebuild it.
				if err := dropDatabaseOn(ctx, conn, name); err != nil {
					return err
				}
			}
			if reason, err = c.createTemplate(ctx, conn, raw, name); err != nil || reason != "" {
				return err
			}
		}
		// Stamping is bookkeeping: a template that another role owns stays as it is.
		if err := markReady(ctx, conn, name); err != nil && !hasPgCode(err, sqlstateInsufficientPrivilege) {
			return err
		}
		sweepTemplates(ctx, conn, name)
		return nil
	})
	if err != nil {
		t.Fatalf("momotest: build template database %s: %v", name, err)
	}
	return name, reason
}

// createTemplate creates database name, fills it and marks it ready. A database
// of that name that appears meanwhile belongs to a process that locks elsewhere;
// createTemplate waits for it to be ready instead of touching it.
func (c *config) createTemplate(ctx context.Context, conn *pgx.Conn, raw, name string) (reason string, err error) {
	err = ddl(ctx, conn, "CREATE DATABASE %s", name)
	switch {
	case hasPgCode(err, sqlstateDuplicateDatabase):
		return "", waitReady(ctx, conn, name)
	case hasPgCode(err, sqlstateInsufficientPrivilege):
		return "the server refused CREATE DATABASE", nil
	case err != nil:
		return "", err
	}
	if err := c.fillTemplateDatabase(ctx, raw, name); err != nil {
		_ = dropDatabaseOn(context.WithoutCancel(ctx), conn, name)
		return "", err
	}
	return "", markReady(ctx, conn, name)
}

// fillTemplateDatabase installs the extensions in the new database and applies
// the migrations to its public schema. Every connection it opens is closed
// before it returns, because CREATE DATABASE ... TEMPLATE needs the template to
// have none.
func (c *config) fillTemplateDatabase(ctx context.Context, raw, name string) error {
	u, err := databaseURL(raw, name)
	if err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, u)
	if err != nil {
		return err
	}
	err = ensureExtensions(ctx, conn, c.extensions)
	_ = conn.Close(ctx)
	if err != nil {
		return err
	}
	return c.applyMigrations(ctx, u, "public")
}

// cloneDatabase creates database name from template. It retries while the
// template still has a connection that is closing, and serializes with other
// clones through an advisory lock, so tests and binaries cloning at the same
// time do not trip over the template.
func cloneDatabase(ctx context.Context, conn *pgx.Conn, template, name string) error {
	return withLock(ctx, conn, cloneLockKey, func() error {
		for {
			// Both identifiers are generated here and quoted; the template name
			// is hex and underscores, so it adds no verb to the format.
			err := ddl(ctx, conn, "CREATE DATABASE %s TEMPLATE "+pgx.Identifier{template}.Sanitize(), name)
			if !hasPgCode(err, sqlstateObjectInUse) {
				return err
			}
			select {
			case <-ctx.Done():
				return err
			case <-time.After(100 * time.Millisecond):
			}
		}
	})
}

// dropDatabase drops the clone name. Where WITH (FORCE) exists it disconnects
// any session the test leaked and takes no lock per relation, so the cost does
// not grow with the number of tables.
func dropDatabase(t testing.TB, raw, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), dropTimeout)
	defer cancel()
	conn, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Errorf("momotest: connect to drop database %s: %v", name, err)
		return
	}
	defer conn.Close(ctx)
	switch err := dropDatabaseOn(ctx, conn, name); {
	case hasPgCode(err, sqlstateInsufficientPrivilege):
		t.Logf("momotest: the role may not drop database %s: %v", name, err)
	case err != nil:
		t.Errorf("momotest: drop database %s: %v", name, err)
	}
}

// newClone returns a *momo.DB on a database cloned from the template database,
// or nil after logging why the schema path must run instead.
func (c *config) newClone(t testing.TB, raw string) *momo.DB {
	t.Helper()
	template, reason := c.templateDatabase(t, raw)
	if template == "" {
		logOnce(t, "momotest: New rebuilds a schema per test ("+reason+"; the migrations run for every test)")
		return nil
	}
	name := randomName(cloneDBPrefix)
	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	defer cancel()
	conn := lockConn(ctx, t, raw)
	defer conn.Close(ctx)
	if err := cloneDatabase(ctx, conn, template, name); err != nil {
		if hasPgCode(err, sqlstateInsufficientPrivilege) {
			// The template belongs to another role: this one cannot copy it.
			noCreateDB.Store(raw, "the role may not copy the template database")
			logOnce(t, "momotest: New rebuilds a schema per test (the role may not copy the template database; the migrations run for every test)")
			return nil
		}
		t.Fatalf("momotest: clone database %s from %s: %v", name, template, err)
	}
	logOnce(t, "momotest: New clones a template database per test (CREATE DATABASE ... TEMPLATE)")
	// The pool, registered later, is closed first.
	t.Cleanup(func() { dropDatabase(t, raw, name) })
	u, err := databaseURL(raw, name)
	if err != nil {
		t.Fatal(err)
	}
	pool := c.openPool(t, u, "public", 4)
	return momo.New(pgxdriver.New(pool), c.momoOpts...)
}
