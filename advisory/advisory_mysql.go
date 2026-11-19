// SPDX-License-Identifier: Apache-2.0

package advisory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// The session functions run GET_LOCK and RELEASE_LOCK on MySQL and MariaDB.
// Those locks have a text name of at most 64 characters and are global to the
// server, where a PostgreSQL advisory lock belongs to one database. The name
// therefore carries the current database:
//
//	momo:advisory:<db>:<key>
//
// <db> is the first 8 hex digits of sha256(DATABASE()), or "nodb" when the
// connection has no default database; the server computes it on each call, so a
// USE statement moves the connection to another lock space. <key> is the
// decimal int64 key, or "s:" plus the first 32 hex digits of the sha256 of a
// text key. The longest name is 14 + 8 + 1 + 34 = 57 characters. The mapping is
// fixed, so every process that uses this package agrees on it. The text hash is
// not the one PostgreSQL uses and WithHash64 has no effect on MySQL.

const (
	// mysqlName is the SQL that builds the lock name from the bound key part.
	mysqlName = "CONCAT('momo:advisory:', IFNULL(LEFT(SHA2(DATABASE(), 256), 8), 'nodb'), ':', ?)"
	// mysqlSlice is the timeout in seconds of one GET_LOCK call inside the
	// blocking Acquire. A negative timeout waits forever on MySQL and returns NULL
	// on MariaDB, so Acquire repeats a short wait until the lock is granted or
	// the context ends.
	mysqlSlice = 1
)

// mysqlKey maps an int64 or string key to the key part of the lock name.
func mysqlKey(key any) string {
	if s, ok := key.(string); ok {
		sum := sha256.Sum256([]byte(s))
		return "s:" + hex.EncodeToString(sum[:])[:32]
	}
	return strconv.FormatInt(key.(int64), 10)
}

func lockErr(kind error, err error) error {
	return &momo.Error{Kind: kind, Op: "advisory lock", Err: err}
}

// mysqlCall runs one lock function and returns its integer result. ok is false
// for SQL NULL.
func mysqlCall(ctx context.Context, db *momo.DB, sql string, args ...any) (v int64, ok bool, err error) {
	got, err := momo.Scalars[*int64](ctx, db, sql, args...)
	if err != nil {
		return 0, false, err
	}
	if len(got) != 1 {
		return 0, false, lockErr(momo.ErrNotFound, errors.New("the lock statement returned no row"))
	}
	if got[0] == nil {
		return 0, false, nil
	}
	return *got[0], true, nil
}

var errNullLock = errors.New("GET_LOCK returned NULL; the server could not take the lock")

// mysqlTry is GET_LOCK(name, 0): it never waits.
func mysqlTry(ctx context.Context, db *momo.DB, key any) (bool, error) {
	v, ok, err := mysqlCall(ctx, db, "SELECT GET_LOCK("+mysqlName+", 0)", mysqlKey(key))
	if err != nil {
		return false, err
	}
	if !ok {
		return false, lockErr(momo.ErrBusy, errNullLock)
	}
	return v == 1, nil
}

// mysqlLock waits for the lock in slices of mysqlSlice seconds until it is
// granted, the server fails, or ctx ends.
func mysqlLock(ctx context.Context, db *momo.DB, key any) error {
	k := mysqlKey(key)
	for {
		v, ok, err := mysqlCall(ctx, db, "SELECT GET_LOCK("+mysqlName+", ?)", k, mysqlSlice)
		if err != nil {
			return err
		}
		if !ok {
			return lockErr(momo.ErrBusy, errNullLock)
		}
		if v == 1 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return lockErr(momo.ErrCanceled, err)
		}
	}
}

// mysqlUnlock is RELEASE_LOCK: true when this session held the lock, false when
// it did not (the server returns 0 for a lock another session holds and NULL
// for one nobody holds).
func mysqlUnlock(ctx context.Context, db *momo.DB, key any) (bool, error) {
	v, ok, err := mysqlCall(ctx, db, "SELECT RELEASE_LOCK("+mysqlName+")", mysqlKey(key))
	if err != nil {
		return false, err
	}
	return ok && v == 1, nil
}

func isMySQL(db *momo.DB) bool { return db.Dialect().Name == dialect.MySQL }
