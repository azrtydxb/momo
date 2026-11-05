// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// copySender is the part of pgxpool.Pool, pgxpool.Conn and pgx.Tx that COPY
// needs.
type copySender interface {
	CopyFrom(ctx context.Context, table pgx.Identifier, cols []string, src pgx.CopyFromSource) (int64, error)
}

// batchSender is the part of pgx.Tx and pgxpool.Conn that batches need.
type batchSender interface {
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	Conn() *pgx.Conn
}

// copyFrom runs COPY on s. Table and column names are never concatenated into
// SQL: pgx quotes each identifier part. A dotted table name is split into
// schema and table.
func copyFrom(ctx context.Context, s copySender, table string, cols []string, rows [][]any) (int64, error) {
	n, err := s.CopyFrom(ctx, pgx.Identifier(strings.Split(table, ".")), cols, pgx.CopyFromRows(rows))
	if err != nil {
		return 0, copyErr(err)
	}
	return n, nil
}

// copyErr classifies a COPY error. pgx encode errors quote the offending value
// ("unable to encode ..."), so anything that is neither a server error, a
// context error nor already a momo error is reported without its text.
func copyErr(err error) error {
	var pe *pgconn.PgError
	// A client-side encode failure aborts the COPY; the server echoes pgx's
	// message, which quotes the value, as "COPY from stdin failed: ...".
	if errors.As(err, &pe) && strings.HasPrefix(pe.Message, "COPY from stdin failed") {
		return driverError{cause: err, msg: "pgx: copy failed (row values withheld)"}
	}
	err = ClassifyError(err)
	var me *momo.Error
	var de driverError
	switch {
	case errors.As(err, &me), errors.As(err, &pe), errors.As(err, &de),
		errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded),
		pgconn.Timeout(err), pgconn.SafeToRetry(err):
		return err
	}
	return driverError{cause: err, msg: "pgx: copy failed (row values withheld)"}
}

// sendBatch queues every statement into one pgx.Batch, runs it and reads every
// result eagerly, so the connection is free again when it returns. All results
// are closed on every path. A failing statement is reported by index, without
// its arguments.
func sendBatch(ctx context.Context, s batchSender, stmts []momo.Compiled) ([]momo.Rows, error) {
	if len(stmts) == 0 {
		return nil, nil
	}
	if s.Conn().Config().DefaultQueryExecMode == pgx.QueryExecModeSimpleProtocol {
		return nil, &momo.Error{
			Kind: momo.ErrUnsupported,
			Op:   "batch",
			Err:  errors.New("simple protocol interpolates arguments client-side"),
		}
	}
	b := &pgx.Batch{}
	for _, st := range stmts {
		b.Queue(st.SQL, st.Args...)
	}
	br := s.SendBatch(ctx, b)
	tm := s.Conn().TypeMap()
	out := make([]momo.Rows, 0, len(stmts))
	for i := range stmts {
		rows, err := br.Query()
		if err == nil {
			var buf *bufferedRows
			buf, err = bufferRows(rows, tm)
			if err == nil {
				out = append(out, buf)
				continue
			}
		}
		_ = br.Close()
		return nil, batchErr(i, err)
	}
	if err := br.Close(); err != nil {
		return nil, batchErr(-1, err)
	}
	return out, nil
}

// batchError reports a failed batch statement by index. It never contains the
// statement's arguments; the cause keeps the classification chain.
type batchError struct {
	index int // -1: the batch failed as a whole
	cause error
}

func (e batchError) Error() string {
	if e.index < 0 {
		return "pgx: batch failed: " + e.cause.Error()
	}
	return fmt.Sprintf("pgx: batch statement %d failed: %s", e.index, e.cause.Error())
}

func (e batchError) Unwrap() error { return e.cause }

func batchErr(index int, err error) error { return batchError{index: index, cause: copyErr(err)} }

// bufferedRows is one statement's result, read completely off the wire so the
// next statement of the batch can be read and the connection released.
type bufferedRows struct {
	fields []pgconn.FieldDescription
	data   [][][]byte
	tm     *pgtype.Map
	pos    int // index of the current row + 1
	closed bool
}

func bufferRows(rows pgx.Rows, tm *pgtype.Map) (*bufferedRows, error) {
	defer rows.Close()
	b := &bufferedRows{tm: tm}
	b.fields = append(b.fields, rows.FieldDescriptions()...)
	for rows.Next() {
		raw := rows.RawValues()
		row := make([][]byte, len(raw))
		for i, v := range raw {
			if v != nil {
				row[i] = append([]byte{}, v...)
			}
		}
		b.data = append(b.data, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return b, nil
}

func (r *bufferedRows) Next() bool {
	if r.closed || r.pos >= len(r.data) {
		return false
	}
	r.pos++
	return true
}

func (r *bufferedRows) Scan(dest ...any) error {
	if r.closed || r.pos == 0 {
		return errors.New("pgxdriver: Scan called without a current row")
	}
	row := r.data[r.pos-1]
	if len(dest) != len(row) {
		return fmt.Errorf("pgxdriver: Scan got %d destinations for %d columns", len(dest), len(row))
	}
	for i, d := range dest {
		f := r.fields[i]
		if err := r.tm.Scan(f.DataTypeOID, f.Format, row[i], d); err != nil {
			return fmt.Errorf("pgxdriver: column %d cannot be scanned into the destination", i)
		}
	}
	return nil
}

func (r *bufferedRows) Err() error   { return nil }
func (r *bufferedRows) Close() error { r.closed = true; return nil }

// CopyFrom implements momo.Copier with pgx's COPY protocol.
func (d *driver) CopyFrom(ctx context.Context, table string, cols []string, rows [][]any) (int64, error) {
	return copyFrom(ctx, d.pool, table, cols, rows)
}

// SendBatch implements momo.Batcher. It holds one pool connection for the
// batch and releases it before returning.
func (d *driver) SendBatch(ctx context.Context, stmts []momo.Compiled) ([]momo.Rows, error) {
	if d.modeErr != nil {
		return nil, d.modeErr
	}
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, ClassifyError(err)
	}
	defer conn.Release()
	return sendBatch(ctx, conn, stmts)
}

// CopyFrom implements momo.Copier on the transaction's connection.
func (t *pgxTx) CopyFrom(ctx context.Context, table string, cols []string, rows [][]any) (int64, error) {
	n, err := copyFrom(ctx, t.tx, table, cols, rows)
	if errors.Is(err, pgx.ErrTxClosed) {
		return 0, &momo.Error{Kind: momo.ErrTxDone, Op: "copy", Err: err}
	}
	return n, err
}

// SendBatch implements momo.Batcher on the transaction's connection.
func (t *pgxTx) SendBatch(ctx context.Context, stmts []momo.Compiled) ([]momo.Rows, error) {
	return sendBatch(ctx, t.tx, stmts)
}

// CopyFrom implements momo.Copier on the pinned session.
func (p *pinned) CopyFrom(ctx context.Context, table string, cols []string, rows [][]any) (int64, error) {
	c, err := p.live()
	if err != nil {
		return 0, err
	}
	return copyFrom(ctx, c, table, cols, rows)
}

// SendBatch implements momo.Batcher on the pinned session.
func (p *pinned) SendBatch(ctx context.Context, stmts []momo.Compiled) ([]momo.Rows, error) {
	c, err := p.live()
	if err != nil {
		return nil, err
	}
	return sendBatch(ctx, c, stmts)
}

var (
	_ momo.Copier  = (*driver)(nil)
	_ momo.Batcher = (*driver)(nil)
	_ momo.Copier  = (*pgxTx)(nil)
	_ momo.Batcher = (*pgxTx)(nil)
	_ momo.Copier  = (*pinned)(nil)
	_ momo.Batcher = (*pinned)(nil)
)
