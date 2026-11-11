// SPDX-License-Identifier: Apache-2.0

package mysqldriver

import (
	"context"
	"database/sql"

	"github.com/azrtydxb/momo"
)

// execInsertID reports the rows affected and the auto-increment key the
// server generated (Result.LastInsertId) for an INSERT. The key is meaningful
// for a single-row insert only: for a multi-row statement MySQL reports the
// first generated id and the rest depend on innodb_autoinc_lock_mode, which is
// why the root package refuses multi-row inserts that need keys.
func execInsertID(res sql.Result, err error, stmt string) (int64, int64, error) {
	if err != nil {
		return 0, 0, classify(err, stmt)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, 0, classify(err, stmt)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, 0, classify(err, stmt)
	}
	return n, id, nil
}

// ExecInsertID implements momo.InsertIDExecer.
func (d *driver) ExecInsertID(ctx context.Context, q string, args []any) (int64, int64, error) {
	if d.err != nil {
		return 0, 0, d.err
	}
	if err := sqlshared.CheckArgs("exec", args); err != nil {
		return 0, 0, err
	}
	res, err := d.db.ExecContext(ctx, q, args...)
	return execInsertID(res, err, q)
}

// ExecInsertID implements momo.InsertIDExecer.
func (t *sqlTx) ExecInsertID(ctx context.Context, q string, args []any) (int64, int64, error) {
	if err := sqlshared.CheckArgs("exec", args); err != nil {
		return 0, 0, err
	}
	res, err := t.tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, 0, txErr("exec", err, q)
	}
	return execInsertID(res, nil, q)
}

// ExecInsertID implements momo.InsertIDExecer.
func (p *pinned) ExecInsertID(ctx context.Context, q string, args []any) (int64, int64, error) {
	if err := sqlshared.CheckArgs("exec", args); err != nil {
		return 0, 0, err
	}
	c, err := p.live()
	if err != nil {
		return 0, 0, err
	}
	defer p.done()
	res, err := c.ExecContext(ctx, q, args...)
	return execInsertID(res, err, q)
}

var (
	_ momo.InsertIDExecer = (*driver)(nil)
	_ momo.InsertIDExecer = (*sqlTx)(nil)
	_ momo.InsertIDExecer = (*pinned)(nil)
)
