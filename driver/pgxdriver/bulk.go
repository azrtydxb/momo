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
	return copyFromSource(ctx, s, table, cols, pgx.CopyFromRows(rows))
}

// copyFromSource is copyFrom over a streaming source.
func copyFromSource(ctx context.Context, s copySender, table string, cols []string, src pgx.CopyFromSource) (int64, error) {
	n, err := s.CopyFrom(ctx, pgx.Identifier(strings.Split(table, ".")), cols, src)
	if err != nil {
		return 0, copyErr(err)
	}
	return n, nil
}

// copyErr classifies a COPY error. The redacted error still wraps the raw pgx
// error as its cause (reachable with errors.As), like ClassifyError does; only
// its text is withheld. pgx encode errors quote the offending value
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
//
// The driver's exec mode is honoured. pgx has no per-batch mode (a batch
// always uses the connection's DefaultQueryExecMode, which prepares statements
// by default), so for QueryExecModeExec, the driver default, the batch is sent
// through pgconn's extended protocol with unnamed statements: no prepared
// statement is created, which keeps it PgBouncer-safe. Any other mode is only
// accepted when it equals the connection's own default, else ErrUnsupported.
//
// Vector arguments follow the driver's VectorFormat, as for Query and Exec.
//
// Results are buffered eagerly and without bound: every row of every statement
// is held in memory until the caller closes the Rows.
func sendBatch(ctx context.Context, s batchSender, mode pgx.QueryExecMode, vf VectorFormat, stmts []momo.Compiled) ([]momo.Rows, error) {
	if len(stmts) == 0 {
		return nil, nil
	}
	stmts = convertVectorStmts(vf, stmts)
	if mode == pgx.QueryExecModeExec {
		return execBatch(ctx, s.Conn(), stmts)
	}
	if s.Conn().Config().DefaultQueryExecMode != mode {
		return nil, &momo.Error{
			Kind: momo.ErrUnsupported,
			Op:   "batch",
			Err:  errors.New("pgx batches use the connection's default exec mode; it differs from the driver's WithExecMode"),
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

// convertVectorStmts applies convertVectorArgs to every statement, so
// VectorText means text for batches too. The input slice is not modified.
func convertVectorStmts(vf VectorFormat, stmts []momo.Compiled) []momo.Compiled {
	var out []momo.Compiled
	for i, st := range stmts {
		args := convertVectorArgs(vf, st.Args)
		if len(args) == 0 || &args[0] == &st.Args[0] {
			continue
		}
		if out == nil {
			out = append([]momo.Compiled(nil), stmts...)
		}
		out[i].Args = args
	}
	if out == nil {
		return stmts
	}
	return out
}

// execBatch sends stmts as one pgconn batch of unnamed extended-protocol
// statements with bound parameters (text format, types inferred by the server).
func execBatch(ctx context.Context, conn *pgx.Conn, stmts []momo.Compiled) ([]momo.Rows, error) {
	tm := conn.TypeMap()
	pb := &pgconn.Batch{}
	for i, st := range stmts {
		params := make([][]byte, len(st.Args))
		formats := make([]int16, len(st.Args))
		for j, a := range st.Args {
			if a == nil {
				continue
			}
			v, err := tm.Encode(0, pgtype.TextFormatCode, a, nil)
			if err != nil {
				return nil, batchErr(i, errors.New("argument cannot be encoded"))
			}
			if v == nil {
				v = []byte{}
			}
			params[j] = v
		}
		pb.ExecParams(st.SQL, params, nil, formats, nil)
	}
	results, err := conn.PgConn().ExecBatch(ctx, pb).ReadAll()
	for i, r := range results {
		if r.Err != nil {
			return nil, batchErr(i, r.Err)
		}
	}
	if err != nil {
		return nil, batchErr(len(results), err)
	}
	out := make([]momo.Rows, 0, len(results))
	for _, r := range results {
		out = append(out, &bufferedRows{fields: r.FieldDescriptions, data: r.Rows, tm: tm})
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
	return sendBatch(ctx, conn, d.mode, d.vecFormat, stmts)
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
	return sendBatch(ctx, t.tx, t.d.mode, t.d.vecFormat, stmts)
}

// CopyFrom implements momo.Copier on the pinned session.
func (p *pinned) CopyFrom(ctx context.Context, table string, cols []string, rows [][]any) (int64, error) {
	return p.copyFromSource(ctx, table, cols, pgx.CopyFromRows(rows))
}

// copyFromSource counts as an executing call, so Release returns
// momo.ErrPinnedBusy while the copy streams.
func (p *pinned) copyFromSource(ctx context.Context, table string, cols []string, src pgx.CopyFromSource) (int64, error) {
	c, err := p.live()
	if err != nil {
		return 0, err
	}
	defer p.done()
	return copyFromSource(ctx, c, table, cols, src)
}

// SendBatch implements momo.Batcher on the pinned session.
func (p *pinned) SendBatch(ctx context.Context, stmts []momo.Compiled) ([]momo.Rows, error) {
	c, err := p.live()
	if err != nil {
		return nil, err
	}
	defer p.done()
	return sendBatch(ctx, c, p.d.mode, p.d.vecFormat, stmts)
}

var (
	_ momo.Copier  = (*driver)(nil)
	_ momo.Batcher = (*driver)(nil)
	_ momo.Copier  = (*pgxTx)(nil)
	_ momo.Batcher = (*pgxTx)(nil)
	_ momo.Copier  = (*pinned)(nil)
	_ momo.Batcher = (*pinned)(nil)
)
