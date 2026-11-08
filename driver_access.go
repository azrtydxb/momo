// SPDX-License-Identifier: Apache-2.0

package momo

// CopierOf returns the Copier behind q: the driver for a *DB, the transaction
// for a *Tx. For a *DB made with OnConn it is the pinned connection, so COPY
// runs on the pinned session.
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
