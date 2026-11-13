// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
)

// HistoryDDL returns the momo_migrations DDL for the bare table name, which
// lives in the first schema of the session's search_path. The engine uses
// HistoryDDLFor with the qualified reference from HistoryTable instead.
func (d Dialect) HistoryDDL() []string { return d.HistoryDDLFor("momo_migrations") }

// HistoryDDLFor returns the history table DDL for table, a SQL-ready
// reference such as the one HistoryTable returns.
func (Dialect) HistoryDDLFor(table string) []string {
	return []string{`CREATE TABLE IF NOT EXISTS ` + table + ` (
version TEXT PRIMARY KEY,
name TEXT NOT NULL,
checksum TEXT NOT NULL,
status TEXT NOT NULL,
statements_applied INTEGER NOT NULL DEFAULT 0,
error TEXT NOT NULL DEFAULT '',
applied_at TEXT NOT NULL,
execution_ms BIGINT NOT NULL DEFAULT 0,
apply_id TEXT NOT NULL DEFAULT '',
kind TEXT NOT NULL DEFAULT 'sql'
)`}
}

// HistoryTable returns the quoted, schema-qualified reference of the
// momo_migrations table, resolved once against the pinned connection. An
// existing table is found the way earlier momo versions found it, through the
// connection's search_path, so installs made with the unqualified name keep
// working and no second table is created. The resolved object must be a
// regular table in a permanent schema, owned by the connecting role or by a
// role whose privileges it holds (pg_has_role USAGE; a superuser connecting
// role also requires the owner to be itself or a superuser): a view, a foreign table, a
// temporary table or a table owned by an unrelated role is refused with an
// error wrapping momo.ErrValidation, because writing history through it would
// run with the migrator's privileges against an object someone else controls.
// When no table exists yet, the reference names current_schema, the schema the
// CREATE TABLE uses; when there is no current schema either, the error wraps
// migrate.ErrNoHistoryTable. Every later statement uses the returned
// reference, so a migration that changes search_path cannot redirect the
// history writes. With WithHistorySchema the lookup and the creation schema are
// that schema and the search_path plays no part.
func (d Dialect) HistoryTable(ctx context.Context, c momo.PinnedConn) (string, error) {
	lookup := "momo_migrations"
	if d.historySchema != "" {
		lookup = quoteIdent(d.historySchema) + ".momo_migrations"
	}
	var schemaName, kind, persistence, owner string
	var usable, superTrust bool
	found := false
	// usable: the connecting role holds the privileges of the owner. A
	// superuser holds those of every role, so for a superuser migrator that
	// is not enough: superTrust is false when current_user is a superuser and
	// the owner is neither current_user nor a superuser, because a table (and
	// its triggers and rules) planted by an ordinary role would run with
	// superuser rights.
	err := forEach(ctx, c, `SELECT n.nspname::text, c.relkind::text, c.relpersistence::text,
 pg_catalog.pg_get_userbyid(c.relowner)::text, pg_catalog.pg_has_role(current_user, c.relowner, 'USAGE'),
 NOT COALESCE((SELECT me.rolsuper FROM pg_catalog.pg_roles me WHERE me.rolname = current_user), false)
  OR c.relowner = (SELECT me.oid FROM pg_catalog.pg_roles me WHERE me.rolname = current_user)
  OR COALESCE((SELECT o.rolsuper FROM pg_catalog.pg_roles o WHERE o.oid = c.relowner), false)
FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE c.oid = pg_catalog.to_regclass($1)`, []any{lookup}, func(r momo.Rows) error {
		found = true
		return r.Scan(&schemaName, &kind, &persistence, &owner, &usable, &superTrust)
	})
	if err != nil {
		return "", fmt.Errorf("migrate/postgres: locate the history table: %w", err)
	}
	if found {
		if (kind != "r" && kind != "p") || persistence != "p" {
			return "", &momo.Error{Kind: momo.ErrValidation, Op: "migrate/postgres", Err: fmt.Errorf("momo_migrations resolves to %s, which is not a regular permanent table; remove it from the search_path or drop it", describeRelation(schemaName, kind, persistence))}
		}
		if !usable {
			return "", &momo.Error{Kind: momo.ErrValidation, Op: "migrate/postgres", Table: "momo_migrations", Err: fmt.Errorf("the table %s.momo_migrations is owned by role %s, and the connecting role does not hold the privileges of that role; refusing to write migration history through it. Fix the ownership, or remove that schema from the search_path", quoteIdent(schemaName), quoteIdent(owner))}
		}
		if !superTrust {
			return "", &momo.Error{Kind: momo.ErrValidation, Op: "migrate/postgres", Table: "momo_migrations", Err: fmt.Errorf("the table %s.momo_migrations is owned by role %s, which is not a superuser, and the connecting role is a superuser; a superuser only trusts a history table that it or another superuser owns, because the table's triggers and rules would run with superuser rights. Fix the ownership, or remove that schema from the search_path", quoteIdent(schemaName), quoteIdent(owner))}
		}
		return quoteIdent(schemaName) + "." + quoteIdent("momo_migrations"), nil
	}
	if d.historySchema != "" {
		return quoteIdent(d.historySchema) + "." + quoteIdent("momo_migrations"), nil
	}
	var cur string
	if err := queryOne(ctx, c, `SELECT COALESCE(current_schema(), '')`, nil, &cur); err != nil {
		return "", fmt.Errorf("migrate/postgres: read current_schema: %w", err)
	}
	if cur == "" {
		return "", fmt.Errorf("%w: the connection has no current schema; set search_path to the schema that should hold momo_migrations", migrate.ErrNoHistoryTable)
	}
	return quoteIdent(cur) + "." + quoteIdent("momo_migrations"), nil
}

// HistoryLockKey returns the advisory-lock key for the history table reference
// that HistoryTable returned: "momo:<schema>.momo_migrations" with the
// unquoted schema name, the key earlier versions used for the same table.
func (Dialect) HistoryLockKey(table string) string {
	// table is quoteIdent(schema) + "." + quoteIdent("momo_migrations").
	rest := strings.TrimPrefix(table, `"`)
	var sb strings.Builder
	for i := 0; i < len(rest); i++ {
		if rest[i] == '"' {
			if i+1 < len(rest) && rest[i+1] == '"' {
				sb.WriteByte('"')
				i++
				continue
			}
			break
		}
		sb.WriteByte(rest[i])
	}
	return "momo:" + sb.String() + ".momo_migrations"
}

// describeRelation names a relation by kind for an error message.
func describeRelation(schemaName, kind, persistence string) string {
	what := map[string]string{"r": "table", "p": "partitioned table", "v": "view", "m": "materialized view", "f": "foreign table", "S": "sequence", "i": "index", "c": "composite type"}[kind]
	if what == "" {
		what = "relation of kind " + kind
	}
	if persistence == "t" {
		what = "temporary " + what
	} else if persistence == "u" {
		what = "unlogged " + what
	}
	article := "a"
	if strings.ContainsRune("ui", rune(what[0])) {
		article = "an"
	}
	return article + " " + what + " in schema " + quoteIdent(schemaName)
}

// quoteIdent quotes a PostgreSQL identifier.
func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// Setup applies lock_timeout and statement_timeout at session scope on the
// pinned connection. A zero duration leaves that setting untouched unless
// SessionOptions says it was set explicitly, which sends SET ... = 0 (off). The
// returned reset restores both to the session defaults (RESET), so nothing
// leaks into the pool when the connection is released. reset is idempotent and
// ignores cancellation of ctx. When RESET fails, reset discards the connection
// (momo.Discarder) so the timeouts cannot leak into the pool. Run reset before
// the unlock returned by Lock. If Setup itself fails part-way, the settings it
// already applied are reset before it returns.
func (Dialect) Setup(ctx context.Context, c momo.PinnedConn, s migrate.SessionOptions) (reset func() error, err error) {
	reset = func() error {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockTimeout)
		defer cancel()
		_, e1 := c.Exec(rctx, `RESET lock_timeout`, nil)
		_, e2 := c.Exec(rctx, `RESET statement_timeout`, nil)
		err := errors.Join(e1, e2)
		if err != nil {
			// The session still carries the timeouts; destroy it rather than
			// return it to the pool.
			err = errors.Join(err, discard(c))
		}
		return err
	}
	for _, st := range []struct {
		name string
		d    time.Duration
		set  bool
	}{{"lock_timeout", s.LockTimeout, s.LockTimeoutSet}, {"statement_timeout", s.StatementTimeout, s.StatementTimeoutSet}} {
		if st.d < 0 || st.d == 0 && !st.set {
			continue
		}
		// SET takes no bind parameters; the value is a formatted integer. A
		// positive value below 1ms would become 0, which switches the timeout
		// off, so it is rounded up.
		ms := st.d.Milliseconds()
		if st.d > 0 && ms == 0 {
			ms = 1
		}
		if _, err := c.Exec(ctx, fmt.Sprintf("SET %s = %d", st.name, ms), nil); err != nil {
			return nil, errors.Join(fmt.Errorf("migrate/postgres: set %s: %w", st.name, err), reset())
		}
	}
	return reset, nil
}

// discard destroys the pinned connection when its driver supports it, and
// reports why when it does not.
func discard(c momo.PinnedConn) error {
	d, ok := c.(momo.Discarder)
	if !ok {
		return errors.New("migrate/postgres: pinned connection cannot be discarded; its session state may leak into the pool")
	}
	return d.Discard()
}
