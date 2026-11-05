// SPDX-License-Identifier: Apache-2.0

package momo

// OnConn returns a handle whose statements all run on pc, a connection pinned
// with Pinner.Pin. It is a copy of db: it keeps db's hooks, error classifier
// and options (WithHook, WithArgLogging, ...). Begin and Tx on it return
// ErrUnsupported with Op "begin": a pinned connection is not a pool, so
// transactions are not started on it. Non-transactional Go migrations and
// session-scoped work use it. The caller still owns pc and releases it.
func (db *DB) OnConn(pc PinnedConn) *DB {
	nd := new(DB)
	*nd = *db
	nd.pinned = true
	nd.ex = execer{db: nd, q: pc, d: db.ex.d, classify: db.ex.classify, owner: nd}
	return nd
}
