// SPDX-License-Identifier: Apache-2.0

package momo

// CopierOf returns the Copier behind q: the driver for a *DB, the transaction
// for a *Tx. For a *DB made with OnConn it is the pinned connection, so COPY
// runs on the pinned session. A *DB made with WithReplicas or Replica always
// reports a Copier, because the router forwards COPY to the primary; its
// CopyFrom returns ErrUnsupported when the primary has none. Insert.Copy checks
// the primary itself and fails before it runs hooks or opens a transaction.
func CopierOf(q Querier) (Copier, bool) {
	switch h := q.(type) {
	case *DB:
		if bypassesTx(h) {
			return nil, false
		}
		c, ok := h.ex.q.(Copier)
		return c, ok
	case *Tx:
		c, ok := h.dtx.(Copier)
		return c, ok
	}
	return nil, false
}

// hasCopier reports whether the driver behind x really has a COPY path, looking
// through the replica router, which always implements Copier but forwards to
// the primary.
func hasCopier(x any) bool {
	if r, ok := x.(routingDriver); ok {
		x = r.Driver
	}
	_, ok := x.(Copier)
	return ok
}

// BatcherOf returns the Batcher behind q: the driver for a *DB, the
// transaction for a *Tx. For a *DB made with OnConn it is the pinned
// connection, so the batch runs on the pinned session.
func BatcherOf(q Querier) (Batcher, bool) {
	switch h := q.(type) {
	case *DB:
		if bypassesTx(h) {
			return nil, false
		}
		b, ok := h.ex.q.(Batcher)
		return b, ok
	case *Tx:
		b, ok := h.dtx.(Batcher)
		return b, ok
	}
	return nil, false
}
