// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"io/fs"
	"time"

	iredact "github.com/azrtydxb/momo/internal/redact"
	"github.com/azrtydxb/momo/internal/sqlerr"
	"github.com/azrtydxb/momo/internal/sqlsplit"
)

// RedactURLs removes passwords and DSNs from msg: every secret given, the
// userinfo of any URL, any URL-looking text that carries a query, and the
// value of a password= key in a key=value DSN.
//
// Deprecated: not part of the migration API. Removed in the next release.
func RedactURLs(msg string, secrets ...string) string { return iredact.RedactURLs(msg, secrets...) }

// SplitStatements splits a migration body into statements.
//
// Deprecated: the engine splits migration files itself. Removed in the next
// release.
func SplitStatements(body []byte, dialectName string) ([]string, error) {
	return sqlsplit.Split(body, dialectName)
}

// IsNoSuchTable reports whether err is SQLite's "no such table" error for the
// named table.
//
// Deprecated: not part of the migration API. Removed in the next release.
func IsNoSuchTable(err error, name string) bool { return sqlerr.IsNoSuchTable(err, name) }

// ParseDirectives reads the directives in the leading comment block of body.
//
// Deprecated: the engine reads directives itself. Removed in the next release.
func ParseDirectives(body []byte) (Directives, error) { return readDirectives(body) }

// Discover lists the migration files in fsys.
//
// Deprecated: use ValidateOffline to check a migration directory. Removed in
// the next release.
func Discover(fsys fs.FS) ([]File, error) { return discover(fsys) }

// NextVersion returns the version a new migration in fsys gets.
//
// Deprecated: Engine.Diff names new migrations. Removed in the next release.
func NextVersion(now func() time.Time, fsys fs.FS) (string, error) { return nextVersion(now, fsys) }

// Chain returns the snapshot chain of fsys, oldest first.
//
// Deprecated: use ValidateOffline to check the chain and LatestSnapshot to
// read its newest snapshot. Removed in the next release.
func Chain(fsys fs.FS) ([]SnapshotFile, error) { return snapshotChain(fsys) }

// RefuseGoRollback returns ErrGoMigrationNotReversible for the first row of
// kind go in rows.
//
// Deprecated: Engine.Rollback applies this check itself. Removed in the next
// release.
func RefuseGoRollback(rows []HistoryRow) error { return refuseGoRollback(rows) }
