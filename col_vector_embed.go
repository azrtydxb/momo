// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

// EmbeddingMeta declares one vector column kept in sync with its source
// columns. Generated code sets it, for a field tagged embed_from, in
// Meta.Embeddings. The hash column holds the hash (vector.SourceHash) of the
// source values the stored embedding was computed from; the generator adds it
// to the table, so it is not a field of the model.
type EmbeddingMeta struct {
	// Vector is the index in Meta.Columns of the vector column.
	Vector int
	// Dim is the declared dimension of the vector column. Zero leaves the
	// dimension of an embedding unchecked.
	Dim int
	// HashColumn is the name of the column that holds the source hash.
	HashColumn string
	// Sources are the indexes in Meta.Columns of the source columns, in the
	// order the embed_from tag names them.
	Sources []int
}

// EmbedOptions tunes StaleEmbeddings and SyncEmbeddings. The zero value is
// valid.
type EmbedOptions struct {
	// Vector is the column of the embedding to bring up to date. It may be
	// empty when the model has exactly one embedding.
	Vector string
	// BatchSize is the number of rows passed to one Embedder call, and the
	// number of rows StaleEmbeddings reads ahead. Zero means 100.
	BatchSize int
	// Limit stops the call after this many stale rows. Zero means no limit,
	// so a run handles every stale row.
	Limit int
	// Text builds the text that is embedded from a row's source values, in
	// the order of the embed_from tag. A row whose text is empty has no
	// embedding: its vector is set to NULL. The default joins the sources
	// that are not NULL and not empty with a blank line.
	Text func(sources []sql.NullString) string
}

// EmbedResult counts what SyncEmbeddings did.
type EmbedResult struct {
	// Embedded is the number of rows whose embedding and hash were written.
	Embedded int
	// Cleared is the number of rows with no source text whose embedding was
	// set to NULL and whose hash was written.
	Cleared int
	// Conflicts is the number of rows left alone because they changed after
	// they were read, or because another runner wrote them first. They stay
	// stale when the source changed, and the next run handles them.
	Conflicts int
}

const (
	defaultEmbedBatch = 100
	minEmbedFetch     = 256
)

// joinEmbedText is the default EmbedOptions.Text.
func joinEmbedText(sources []sql.NullString) string {
	parts := make([]string, 0, len(sources))
	for _, s := range sources {
		if s.Valid && s.String != "" {
			parts = append(parts, s.String)
		}
	}
	return strings.Join(parts, "\n\n")
}

// sourceValue reads a source field as a nullable string. The generator only
// accepts string-kind fields (string, *string, sql.NullString and named string
// types), so any other value is a hand-written Meta error.
func sourceValue(v any) (sql.NullString, error) {
	if v == nil {
		return sql.NullString{}, nil
	}
	if vr, ok := v.(driver.Valuer); ok {
		x, err := vr.Value()
		if err != nil {
			return sql.NullString{}, err
		}
		switch s := x.(type) {
		case nil:
			return sql.NullString{}, nil
		case string:
			return sql.NullString{String: s, Valid: true}, nil
		case []byte:
			return sql.NullString{String: string(s), Valid: true}, nil
		}
		return sql.NullString{}, fmt.Errorf("%T holds %T, not text", v, x)
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return sql.NullString{}, nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.String {
		return sql.NullString{}, fmt.Errorf("%T is not a string type", v)
	}
	return sql.NullString{String: rv.String(), Valid: true}, nil
}

// staleRow is a row whose embedding is out of date, as it was read.
type staleRow[T any] struct {
	// model holds the key and the source fields; the other fields are zero.
	model  *T
	srcs   []sql.NullString
	stored sql.NullString // the stored hash; not Valid when the row was never embedded
	hash   string         // the hash of srcs
	text   string
}

// embedCursor walks a table in key order and collects the stale rows.
type embedCursor[T any] struct {
	q     Querier
	d     *dialect.Dialect
	meta  *Meta[T]
	spec  EmbeddingMeta
	table TableRef
	// cols are the columns read besides the hash: the key, then the sources.
	cols      []int
	text      func([]sql.NullString) string
	batch     int
	fetchSize int
	after     *T
	done      bool
	pending   []staleRow[T]
	limit     int
}

func newEmbedCursor[T any, PT Modeler[T]](q Querier, opts EmbedOptions) (*embedCursor[T], error) {
	meta := metaOf[T, PT]()
	if q == nil {
		return nil, noQuerier("embed", meta.Table)
	}
	if meta.Table == "" {
		return nil, validation("embed", "", "%T has no table", *new(T))
	}
	d := q.Dialect()
	if d == nil {
		return nil, noQuerier("embed", meta.Table)
	}
	if d.Name == dialect.MySQL {
		return nil, newError(ErrUnsupported, "embed", meta.Table, errors.New("keeping embeddings in sync is supported on PostgreSQL and SQLite, not on MySQL or MariaDB"))
	}
	spec, err := embeddingSpec(meta, opts.Vector)
	if err != nil {
		return nil, err
	}
	if opts.BatchSize < 0 || opts.Limit < 0 {
		return nil, validation("embed", meta.Table, "BatchSize and Limit must not be negative")
	}
	c := &embedCursor[T]{q: q, d: d, meta: meta, spec: spec, table: TableRef{Name: meta.Table, Schema: meta.Schema}, text: opts.Text, batch: opts.BatchSize, limit: opts.Limit}
	if c.text == nil {
		c.text = joinEmbedText
	}
	if c.batch == 0 {
		c.batch = defaultEmbedBatch
	}
	c.fetchSize = max(c.batch, minEmbedFetch)
	c.cols = slices.Clone(meta.PK)
	for _, s := range spec.Sources {
		if !slices.Contains(c.cols, s) {
			c.cols = append(c.cols, s)
		}
	}
	return c, nil
}

// embeddingSpec picks the embedding of meta named by vec and checks that its
// column indexes lie inside the metadata.
func embeddingSpec[T any](meta *Meta[T], vec string) (EmbeddingMeta, error) {
	inRange := func(i int) bool { return i >= 0 && i < len(meta.Columns) }
	var spec EmbeddingMeta
	switch {
	case len(meta.Embeddings) == 0:
		return spec, validation("embed", meta.Table, "%T has no field tagged embed_from", *new(T))
	case vec == "" && len(meta.Embeddings) == 1:
		spec = meta.Embeddings[0]
	case vec == "":
		return spec, validation("embed", meta.Table, "%T has %d embeddings; name one in EmbedOptions.Vector", *new(T), len(meta.Embeddings))
	default:
		found := false
		for _, e := range meta.Embeddings {
			if inRange(e.Vector) && meta.Columns[e.Vector].Name == vec {
				spec, found = e, true
				break
			}
		}
		if !found {
			return spec, validation("embed", meta.Table, "%T has no embedding in column %q", *new(T), vec)
		}
	}
	bad := !inRange(spec.Vector) || spec.HashColumn == "" || len(spec.Sources) == 0 || len(meta.PK) == 0
	for _, s := range spec.Sources {
		bad = bad || !inRange(s)
	}
	for _, k := range meta.PK {
		bad = bad || !inRange(k)
	}
	if bad {
		return spec, validation("embed", meta.Table, "%T has an incomplete embedding declaration: it needs a vector column, a hash column, source columns and a primary key", *new(T))
	}
	return spec, nil
}

// next returns up to want stale rows, fewer only when the table is exhausted.
func (c *embedCursor[T]) next(ctx context.Context, want int) ([]staleRow[T], error) {
	for len(c.pending) < want && !c.done {
		if err := c.fetch(ctx); err != nil {
			return nil, err
		}
	}
	n := min(want, len(c.pending))
	out := c.pending[:n:n]
	c.pending = c.pending[n:]
	return out, nil
}

// fetch reads one page of the table after the last row read and keeps the
// stale rows of it. The statement is finished before fetch returns, so no
// rows or connection stay open while the caller works on the page.
func (c *embedCursor[T]) fetch(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return newError(ErrCanceled, "embed", c.meta.Table, err)
	}
	comp, err := compile(c.d, c.renderPage)
	if err != nil {
		return err
	}
	ex := c.q.conn()
	rows, err := ex.query(ctx, "select", c.meta.Table, "", comp)
	if err != nil {
		return err
	}
	var last *T
	n := 0
	for rows.Next() {
		m := new(T)
		var stored sql.NullString
		var noVec int64
		dest := make([]any, 0, len(c.cols)+2)
		for _, ci := range c.cols {
			dest = append(dest, c.meta.Ptr(m, ci))
		}
		dest = append(dest, &stored, &noVec)
		if err := rows.Scan(dest...); err != nil {
			_ = rows.Close()
			return ex.wrapErr("select", c.meta.Table, err)
		}
		n++
		last = m
		row, err := c.classify(m, stored, noVec != 0)
		if err != nil {
			_ = rows.Close()
			return err
		}
		if row != nil {
			c.pending = append(c.pending, *row)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return ex.wrapErr("select", c.meta.Table, err)
	}
	if err := rows.Close(); err != nil {
		return ex.wrapErr("select", c.meta.Table, err)
	}
	if n < c.fetchSize {
		c.done = true
	}
	if last != nil {
		c.after = last
	}
	return nil
}

// classify returns the row when its embedding is stale, and nil when it is
// current. A row is current when its stored hash equals the hash of its source
// values and it has an embedding, unless it has no text and so needs none.
func (c *embedCursor[T]) classify(m *T, stored sql.NullString, noVec bool) (*staleRow[T], error) {
	srcs := make([]sql.NullString, len(c.spec.Sources))
	for k, ci := range c.spec.Sources {
		v, err := sourceValue(c.meta.Value(m, ci))
		if err != nil {
			return nil, validation("embed", c.meta.Table, "source column %q: %v", c.meta.Columns[ci].Name, err)
		}
		srcs[k] = v
	}
	text := c.text(srcs)
	hash := vector.SourceHash(srcs...)
	if stored.Valid && stored.String == hash && (!noVec || text == "") {
		return nil, nil
	}
	return &staleRow[T]{model: m, srcs: srcs, stored: stored, hash: hash, text: text}, nil
}

// renderPage writes the page statement: the key, the sources, the stored hash
// and whether the vector is NULL, of the live rows after the last row read, in
// key order.
func (c *embedCursor[T]) renderPage(b *Builder) {
	m := c.meta
	b.WriteString("SELECT ")
	for _, ci := range c.cols {
		b.Ident(m.Columns[ci].Name)
		b.WriteString(", ")
	}
	b.Ident(c.spec.HashColumn)
	b.WriteString(", CASE WHEN ")
	b.Ident(m.Columns[c.spec.Vector].Name)
	b.WriteString(" IS NULL THEN 1 ELSE 0 END FROM ")
	c.table.appendFrom(b)
	sep := " WHERE "
	if m.SoftDelete >= 0 && m.SoftDelete < len(m.Columns) {
		b.WriteString(sep)
		sep = " AND "
		softDeletePred(m, c.table.Qualifier(), false).AppendSQL(b)
	}
	if c.after != nil {
		b.WriteString(sep)
		c.renderAfter(b)
	}
	b.WriteString(" ORDER BY ")
	for i, ci := range m.PK {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(m.Columns[ci].Name)
	}
	b.WriteString(" LIMIT ")
	b.WriteString(strconv.Itoa(c.fetchSize))
}

// renderAfter writes the keyset condition "key > last key", expanded for a
// composite key as (a > ?) OR (a = ? AND b > ?) so it needs no row-value
// comparison.
func (c *embedCursor[T]) renderAfter(b *Builder) {
	m := c.meta
	b.WriteString("(")
	for i := range m.PK {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteString("(")
		for j := 0; j <= i; j++ {
			ci := m.PK[j]
			b.Ident(m.Columns[ci].Name)
			if j < i {
				b.WriteString(" = ")
			} else {
				b.WriteString(" > ")
			}
			b.ArgFor(m.Columns[ci].Flags, m.Value(c.after, ci))
			if j < i {
				b.WriteString(" AND ")
			}
		}
		b.WriteString(")")
	}
	b.WriteString(")")
}

// update writes one row's embedding, or NULL when drop is set, together with
// the hash of the sources it was computed from. It is a compare-and-set: the
// statement matches the row only while every source column and the hash still
// hold what was read, so a row edited while the Embedder ran, or written by
// another runner, is left alone. It reports whether the row was written.
func (c *embedCursor[T]) update(ctx context.Context, r staleRow[T], vec vector.Vector, drop bool) (bool, error) {
	m := c.meta
	q := c.table.Qualifier()
	comp, err := compile(c.d, func(b *Builder) {
		b.WriteString("UPDATE ")
		c.table.appendFrom(b)
		b.WriteString(" SET ")
		b.Ident(m.Columns[c.spec.Vector].Name)
		b.WriteString(" = ")
		if drop {
			b.WriteString("NULL")
		} else {
			b.Arg(vec)
		}
		b.WriteString(", ")
		b.Ident(c.spec.HashColumn)
		b.WriteString(" = ")
		b.Arg(r.hash)
		b.WriteString(" WHERE ")
		for i, ci := range m.PK {
			if i > 0 {
				b.WriteString(" AND ")
			}
			col := m.Columns[ci]
			updPKEq{q, col.Name, m.Value(r.model, ci), col.Flags}.AppendSQL(b)
		}
		for k, ci := range c.spec.Sources {
			b.WriteString(" AND ")
			NewColumn[sql.NullString](q, m.Columns[ci].Name, 0).IsNotDistinctFrom(r.srcs[k]).AppendSQL(b)
		}
		b.WriteString(" AND ")
		NewColumn[sql.NullString](q, c.spec.HashColumn, 0).IsNotDistinctFrom(r.stored).AppendSQL(b)
	})
	if err != nil {
		return false, err
	}
	n, err := c.q.conn().exec(ctx, "update", m.Table, comp)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// StaleEmbeddings yields the rows of T whose embedding is out of date: the
// vector is NULL and the row has source text, or the hash of the current
// source values (vector.SourceHash) differs from the stored one, or no hash is
// stored. Soft-deleted rows are not visited. A yielded model holds the key and
// the source fields; its other fields are zero. Rows come in primary-key
// order, read a page at a time with no statement open between pages, so the
// loop body may do slow work, including a call to SyncEmbeddings.
//
// The hash is computed in Go, so each call reads the key and the source
// columns of every live row. opts.Vector picks the embedding when the model
// has several. It fails with ErrUnsupported on MySQL and MariaDB and with
// ErrValidation when T has no field tagged embed_from. Statements bypass the
// interceptors, so a tenant filter does not apply: the iteration covers every
// tenant.
func StaleEmbeddings[T any, PT Modeler[T]](ctx context.Context, q Querier, opts EmbedOptions) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		c, err := newEmbedCursor[T, PT](q, opts)
		if err != nil {
			yield(*new(T), err)
			return
		}
		yielded := 0
		for {
			want := c.batch
			if c.limit > 0 {
				want = min(want, c.limit-yielded)
			}
			if want <= 0 {
				return
			}
			rows, err := c.next(ctx, want)
			if err != nil {
				yield(*new(T), err)
				return
			}
			if len(rows) == 0 {
				return
			}
			for _, r := range rows {
				yielded++
				if !yield(*r.model, nil) {
					return
				}
			}
		}
	}
}

// SyncEmbeddings brings the stale embeddings of T up to date. It reads the
// stale rows as StaleEmbeddings does, passes the text of opts.BatchSize rows
// at a time to e, and writes each vector back together with the hash of the
// source values it was computed from. A row whose text is empty is not
// embedded: its vector is set to NULL.
//
// No transaction is held and no statement is open during the Embedder call.
// Each row is written by one compare-and-set UPDATE that matches it only while
// its source columns and its stored hash are what was read, so any number of
// runners may work at once: a row edited while the Embedder ran is not marked
// fresh (it is counted in EmbedResult.Conflicts and stays stale), and a row
// two runners both embedded is written once. Runners that read the same rows
// call the Embedder for them twice; run one runner for each part of a large
// backfill if that cost matters. The run is resumable by construction: a
// failed or interrupted run leaves the rows it did not reach stale, and the
// next run starts from them.
//
// The write sets the vector and the hash column only. It does not change
// updated_at or a version column, and it does not run hooks. Pass a *DB, not a
// *Tx, so no transaction is open during the Embedder call.
//
// It fails with ErrUnsupported on MySQL and MariaDB, with ErrValidation when T
// has no field tagged embed_from or the Embedder returns the wrong number of
// vectors or a vector of the wrong dimension, and with the Embedder's own
// error, wrapped, when Embed fails. The result counts the rows handled before
// an error.
func SyncEmbeddings[T any, PT Modeler[T]](ctx context.Context, q Querier, e vector.Embedder, opts EmbedOptions) (EmbedResult, error) {
	var res EmbedResult
	c, err := newEmbedCursor[T, PT](q, opts)
	if err != nil {
		return res, err
	}
	if e == nil {
		return res, validation("embed", c.meta.Table, "the Embedder is nil")
	}
	if err := viewWriteErr("embed", c.meta); err != nil {
		return res, err
	}
	processed := 0
	for {
		want := c.batch
		if c.limit > 0 {
			want = min(want, c.limit-processed)
		}
		if want <= 0 {
			return res, nil
		}
		rows, err := c.next(ctx, want)
		if err != nil {
			return res, err
		}
		if len(rows) == 0 {
			return res, nil
		}
		processed += len(rows)
		if err := syncBatch(ctx, c, e, rows, &res); err != nil {
			return res, err
		}
	}
}

// syncBatch embeds the rows that have text with one Embedder call and writes
// every row of the batch.
func syncBatch[T any](ctx context.Context, c *embedCursor[T], e vector.Embedder, rows []staleRow[T], res *EmbedResult) error {
	var texts []string
	for _, r := range rows {
		if r.text != "" {
			texts = append(texts, r.text)
		}
	}
	var vecs []vector.Vector
	if len(texts) > 0 {
		var err error
		vecs, err = e.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("momo: embedder: %w", err)
		}
		if len(vecs) != len(texts) {
			return validation("embed", c.meta.Table, "the Embedder returned %d vectors for %d texts", len(vecs), len(texts))
		}
		if c.spec.Dim > 0 {
			for i, v := range vecs {
				if err := v.Validate(c.spec.Dim); err != nil {
					return validation("embed", c.meta.Table, "vector %d of the Embedder: %v", i, err)
				}
			}
		}
	}
	k := 0
	for _, r := range rows {
		drop := r.text == ""
		var vec vector.Vector
		if !drop {
			vec = vecs[k]
			k++
		}
		ok, err := c.update(ctx, r, vec, drop)
		switch {
		case err != nil:
			return err
		case !ok:
			res.Conflicts++
		case drop:
			res.Cleared++
		default:
			res.Embedded++
		}
	}
	return nil
}
