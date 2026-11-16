// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"context"
	"errors"
	"net/url"
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
	// Template databases and clones. The template name carries the same hash as
	// a template schema (see templateName), so a changed migration, extension
	// list or momo version builds a new one.
	templateDBPrefix = "momotest_tpldb_"
	cloneDBPrefix    = "momotest_db_"

	templateDBLockKey = "momotest:templatedb"
	cloneLockKey      = "momotest:clone"
	// cloneTimeout bounds one CREATE DATABASE ... TEMPLATE, including the wait
	// for a template whose last connection is still closing.
	cloneTimeout = 2 * time.Minute

	// sqlstateObjectInUse is "source database is being accessed by other users".
	sqlstateObjectInUse = "55006"
	// sqlstateInsufficientPrivilege is "permission denied to create database".
	sqlstateInsufficientPrivilege = "42501"
)

var (
	// databaseTemplateReady holds raw URL + NUL + template database name, after
	// this process has found or built the template database.
	databaseTemplateReady sync.Map
	// noCreateDB holds raw URL -> reason, for a server whose role cannot create
	// databases, so later tests go straight to the schema path.
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

// templateDatabase returns the name of the ready template database for this
// configuration, building it first under an advisory lock. When the role cannot
// create databases it returns an empty name and the reason.
func (c *config) templateDatabase(t testing.TB, raw string) (name, reason string) {
	t.Helper()
	schemaName, err := c.preparedTemplate()
	if err != nil {
		t.Fatalf("momotest: %v", err)
	}
	name = templateDBPrefix + strings.TrimPrefix(schemaName, "momotest_tpl_")
	key := raw + "\x00" + name
	if _, ok := databaseTemplateReady.Load(key); ok {
		return name, ""
	}
	if v, ok := noCreateDB.Load(raw); ok {
		return "", v.(string)
	}
	templateMu.Lock()
	defer templateMu.Unlock()
	if _, ok := databaseTemplateReady.Load(key); ok {
		return name, ""
	}
	if v, ok := noCreateDB.Load(raw); ok {
		return "", v.(string)
	}
	if reason = c.buildTemplateDatabase(t, raw, name); reason != "" {
		noCreateDB.Store(raw, reason)
		return "", reason
	}
	databaseTemplateReady.Store(key, struct{}{})
	return name, ""
}

// buildTemplateDatabase creates and migrates the template database unless a
// complete one exists. A non-empty result is the reason the role cannot do it.
func (c *config) buildTemplateDatabase(t testing.TB, raw, name string) (reason string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	conn := connect(ctx, t, raw)
	defer conn.Close(ctx)

	var can bool
	if err := conn.QueryRow(ctx, "SELECT rolcreatedb OR rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&can); err != nil {
		t.Fatalf("momotest: read the role's privileges: %v", err)
	}
	if !can {
		return "the role lacks CREATEDB"
	}
	err := withLock(ctx, conn, templateDBLockKey, func() error {
		var comment *string
		err := conn.QueryRow(ctx,
			"SELECT shobj_description(oid, 'pg_database') FROM pg_database WHERE datname = $1", name).Scan(&comment)
		switch {
		case err == nil && comment != nil && *comment == readyMarker:
			return nil
		case err == nil:
			// A killed run left a half-built template; rebuild it.
			if err := ddl(ctx, conn, "DROP DATABASE %s WITH (FORCE)", name); err != nil {
				return err
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if err := ddl(ctx, conn, "CREATE DATABASE %s", name); err != nil {
			if hasPgCode(err, sqlstateInsufficientPrivilege) {
				reason = "the server refused CREATE DATABASE"
				return nil
			}
			return err
		}
		if err := c.fillTemplateDatabase(ctx, raw, name); err != nil {
			_ = ddl(context.WithoutCancel(ctx), conn, "DROP DATABASE IF EXISTS %s WITH (FORCE)", name)
			return err
		}
		return ddl(ctx, conn, "COMMENT ON DATABASE %s IS '"+readyMarker+"'", name)
	})
	if err != nil {
		t.Fatalf("momotest: build template database %s: %v", name, err)
	}
	return reason
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

// dropDatabase drops the clone name. WITH (FORCE) disconnects any session the
// test leaked and takes no lock per relation, so the cost does not grow with the
// number of tables.
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
	if err := ddl(ctx, conn, "DROP DATABASE IF EXISTS %s WITH (FORCE)", name); err != nil {
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
	logOnce(t, "momotest: New clones a template database per test (CREATE DATABASE ... TEMPLATE)")

	name := randomName(cloneDBPrefix)
	ctx, cancel := context.WithTimeout(context.Background(), cloneTimeout)
	defer cancel()
	conn := connect(ctx, t, raw)
	defer conn.Close(ctx)
	// Registered before the CREATE, so a half-made clone is dropped too. The
	// pool, registered later, is closed first.
	t.Cleanup(func() { dropDatabase(t, raw, name) })
	if err := cloneDatabase(ctx, conn, template, name); err != nil {
		t.Fatalf("momotest: clone database %s from %s: %v", name, template, err)
	}
	u, err := databaseURL(raw, name)
	if err != nil {
		t.Fatal(err)
	}
	pool := c.openPool(t, u, "public", 4)
	return momo.New(pgxdriver.New(pool), c.momoOpts...)
}
