// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

// IDGen declares a Go-side id generator for one column of a model: the
// generated code sets it from the idgen tag or the IDGen companion call. Name
// is a built-in generator (cuid, uuid, ulid) or one registered with
// WithIDGenerator.
type IDGen struct {
	// Column is the index into Meta.Columns.
	Column int
	// Name is the generator name.
	Name string
}

// WithIDGenerator registers the id generator name on this DB, or replaces the
// built-in of that name (cuid, uuid, ulid). A model column declared with
// idgen:name takes its id from fn when an insert writes the zero value. fn
// must return a non-empty string and be safe for concurrent use. An empty
// id, a panic in fn and a nil fn fail the insert with ErrValidation. A model that
// names a generator that is neither built in nor registered fails with
// ErrValidation when it is inserted.
func WithIDGenerator(name string, fn func() string) Option {
	return func(db *DB) {
		if db.idGens == nil {
			db.idGens = map[string]func() string{}
		}
		db.idGens[name] = fn
	}
}

var builtinIDGens = map[string]func() string{
	"cuid": newCUID,
	"uuid": newUUID,
	"ulid": newULID,
}

// idGenerator resolves a generator name against the DB of q, then the
// built-ins.
func idGenerator(q Querier, name string) (func() string, bool) {
	if q != nil {
		if e := q.conn(); e != nil && e.db != nil {
			if fn, ok := e.db.idGens[name]; ok {
				return fn, true
			}
		}
	}
	fn, ok := builtinIDGens[name]
	return fn, ok
}

// idGenFail carries an id generator error out of a Go-side default, which
// cannot return one; the insert builder turns it back into an error.
type idGenFail struct{ err error }

// callIDGen runs a generator and turns a nil function, a panic and an empty id
// into an error.
func callIDGen(name string, fn func() string) (id string, err error) {
	if fn == nil {
		return "", fmt.Errorf("id generator %q is registered with a nil function", name)
	}
	defer func() {
		if p := recover(); p != nil {
			id, err = "", fmt.Errorf("id generator %q panicked: %v", name, p)
		}
	}()
	id = fn()
	if id == "" {
		err = fmt.Errorf("id generator %q returned an empty id", name)
	}
	return id, err
}

// idGenDefaults resolves the generators of m into Go-side defaults, so a
// column left out of a Values statement still gets an id.
func idGenDefaults[T any](q Querier, m *Meta[T]) (map[int]func(*T) any, error) {
	var out map[int]func(*T) any
	for _, g := range m.IDGens {
		if g.Column < 0 || g.Column >= len(m.Columns) {
			return nil, &Error{Kind: ErrValidation, Op: "insert", Table: m.Table,
				Err: fmt.Errorf("id generator column index %d is outside the %d columns of %s", g.Column, len(m.Columns), m.Table)}
		}
		fn, ok := idGenerator(q, g.Name)
		if !ok {
			return nil, &Error{Kind: ErrValidation, Op: "insert", Table: m.Table, Column: m.Columns[g.Column].Name,
				Err: fmt.Errorf("unknown id generator %q: use cuid, uuid, ulid or register it with WithIDGenerator", g.Name)}
		}
		if out == nil {
			out = map[int]func(*T) any{}
		}
		name, col := g.Name, m.Columns[g.Column].Name
		out[g.Column] = func(*T) any {
			id, err := callIDGen(name, fn)
			if err != nil {
				panic(idGenFail{&Error{Kind: ErrValidation, Op: "insert", Table: m.Table, Column: col, Err: err}})
			}
			return id
		}
	}
	return out, nil
}

// valueRowsSafe is valueRows with a failing id generator reported as an error.
func (q InsertQuery[T]) valueRowsSafe(d *dialect.Dialect, now time.Time, fp fieldPlan[T]) (cols []int, rows []map[int]Assignment, err error) {
	defer func() {
		if p := recover(); p != nil {
			f, ok := p.(idGenFail)
			if !ok {
				panic(p)
			}
			cols, rows, err = nil, nil, f.err
		}
	}()
	return q.valueRows(d, now, fp)
}

// isNonPKIDGen reports whether column ci has an id generator and is not part of
// the primary key, e.g. a public id: a conflict must keep the stored value.
func (q InsertQuery[T]) isNonPKIDGen(ci int) bool {
	if slices.Contains(q.meta.PK, ci) {
		return false
	}
	for _, g := range q.meta.IDGens {
		if g.Column == ci {
			return true
		}
	}
	return false
}

// execSingle runs the only chunk of an insert. A statement that reconciles ids
// (or reads dbdefault columns back) runs in its own transaction (or savepoint),
// so a RETURNING row that matches no model rolls the write back instead of
// leaving it committed.
func (q InsertQuery[T]) execSingle(ctx context.Context, c InsertQuery[T]) (Result, error) {
	if db, ok := q.q.(*DB); !(c.reconcile || len(c.dbDefaultReadBack()) > 0) || (ok && db.pinned) {
		return c.execOne(ctx)
	}
	var res Result
	err := runAtomic(ctx, q.q, "insert", q.meta.Table, func(sq Querier) error {
		c.q = sq
		var e error
		res, e = c.execOne(ctx)
		return e
	})
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// keepsExistingRow reports whether the statement can leave a conflicting row
// in place under another id than the one generated: it has an ON CONFLICT
// action on something other than the primary key.
func (q InsertQuery[T]) keepsExistingRow() bool {
	rq, err := q.resolveUpsert()
	if err != nil || rq.i.conflict == conflictNone {
		return false
	}
	for _, g := range rq.meta.IDGens {
		if !slices.Contains(rq.meta.PK, g.Column) {
			return true // e.g. a public_id: the row that is kept has its own
		}
	}
	if len(rq.i.target) != len(rq.meta.PK) {
		return true
	}
	for _, pk := range rq.meta.PK {
		if !slices.Contains(rq.i.target, rq.meta.Columns[pk].Name) {
			return true
		}
	}
	return false
}

// generateIDs fills the id fields of the models of q that hold their zero
// value, and every id column listed in Omit. With writeBack it also stores
// each generated id in the caller's model, so the caller reads it after Exec
// or Rows. It is a no-op for a model without id generators.
//
// When the statement can keep an existing row under its own id (ON CONFLICT on
// a column other than the primary key) the generated id is not stored in the
// caller's model up front: Exec sets reconcile and reads the stored ids back
// with RETURNING instead (see execReconcile).
func (q InsertQuery[T]) generateIDs(writeBack bool) (InsertQuery[T], error) {
	if q.idsDone || q.q == nil || len(q.meta.IDGens) == 0 || len(q.models) == 0 || q.meta.Ptr == nil || q.meta.Value == nil {
		return q, nil
	}
	eager := writeBack
	if writeBack && q.keepsExistingRow() {
		eager = false
	}
	cloned, generated := false, false
	var undoOmit []string
	for _, g := range q.meta.IDGens {
		ci := g.Column
		if ci < 0 || ci >= len(q.meta.Columns) || !q.writable(ci) {
			continue
		}
		name := q.meta.Columns[ci].Name
		omitted := slices.Contains(q.i.omit, name)
		fn, ok := idGenerator(q.q, g.Name)
		if !ok {
			return q, q.verr(name, "unknown id generator %q: use cuid, uuid, ulid or register it with WithIDGenerator", g.Name)
		}
		for r := range q.models {
			v := q.meta.Value(&q.models[r], ci)
			unset, ok := idUnset(v)
			if !ok {
				return q, q.verr(name, "idgen needs a string column, got %T", v)
			}
			if !unset && !omitted {
				continue // an Omit column always takes a fresh id
			}
			id, err := callIDGen(g.Name, fn)
			if err != nil {
				return q, q.verr(name, "%v", err)
			}
			if !cloned {
				q.models = slices.Clone(q.models)
				cloned = true
			}
			setID(q.meta.Ptr(&q.models[r], ci), id)
			generated = true
			if eager && r < len(q.back) && q.back[r] != nil {
				setID(q.meta.Ptr(q.back[r], ci), id)
			}
		}
		if omitted {
			undoOmit = append(undoOmit, name) // the generated id is bound from the model
		}
	}
	if len(undoOmit) > 0 {
		q.i.omit = slices.DeleteFunc(slices.Clone(q.i.omit), func(n string) bool { return slices.Contains(undoOmit, n) })
	}
	if generated && writeBack && !eager && q.q.Dialect().Returning {
		q.reconcile = true
	}
	return q, nil
}

// execReconcile runs an insert that may keep conflicting rows under their own
// ids, asking RETURNING for the id and conflict-target columns, and writes the
// id each stored row has into the matching caller model. A row the statement
// skipped (DO NOTHING) comes back empty and its model keeps what the caller
// had.
func (q InsertQuery[T]) execReconcile(ctx context.Context) (Result, error) {
	rq, err := q.resolveUpsert()
	if err != nil {
		return Result{}, err
	}
	idCols := make([]int, 0, len(q.meta.IDGens))
	var refs []ColumnRef
	for _, g := range q.meta.IDGens {
		idCols = append(idCols, g.Column)
		refs = append(refs, NewColumn[string](q.meta.Table, q.meta.Columns[g.Column].Name, 0))
	}
	// Rows are matched on the conflict target when there is one, otherwise on
	// the generated id (a skipped row never comes back, an inserted one does).
	keyCols := idCols
	if len(rq.i.target) > 0 {
		keyCols = keyCols[:0:0]
		for _, n := range rq.i.target {
			ci := q.column(n)
			if ci < 0 {
				return Result{}, q.verr(n, "OnConflict: column %q is not mapped by %s", n, q.meta.Table)
			}
			keyCols = append(keyCols, ci)
			if !slices.Contains(idCols, ci) {
				refs = append(refs, NewColumn[string](q.meta.Table, n, 0))
			}
		}
	}
	dbCols := q.dbDefaultReadBack() // dbdefault columns come back with the ids
	for _, ci := range keyCols {
		if slices.Contains(dbCols, ci) {
			return Result{}, q.verr(q.meta.Columns[ci].Name, "the conflict-target column %q takes its database default in some rows, so the stored rows cannot be matched to the models: give it a value or use another conflict target", q.meta.Columns[ci].Name)
		}
	}
	for _, ci := range dbCols {
		if !slices.Contains(idCols, ci) {
			refs = append(refs, NewColumn[string](q.meta.Table, q.meta.Columns[ci].Name, 0))
		}
	}
	q2 := q
	q2.i.returning = returningSet{cols: refs}
	c, err := q2.SQL()
	if err != nil {
		return Result{}, err
	}
	rows, err := runReturning(ctx, q.q, "insert", q.meta, q2.i.returning, c)
	if err != nil {
		return Result{}, err
	}
	rows.noHooks = true // internal read: the models are half-filled
	byKey := make(map[string]int, len(q.models))
	for i := range q.models {
		k := idKey(q.meta, &q.models[i], keyCols)
		if _, dup := byKey[k]; !dup {
			byKey[k] = i // the first model of a duplicate key is the one DO NOTHING inserts
		}
	}
	var n int64
	for rows.Next() {
		var got T
		if err := rows.ScanModel(&got); err != nil {
			_ = rows.Close()
			return Result{}, err
		}
		n++
		i, ok := byKey[idKey(q.meta, &got, keyCols)]
		if !ok {
			_ = rows.Close()
			return Result{}, q.verr("", "idgen: a RETURNING row of %s matched no model on %s: the conflict-target columns must read back exactly as written (citext, char(n) and numeric columns may not); the write is rolled back unless the handle is a pinned OnConn one", q.meta.Table, strings.Join(keyNames(q.meta, keyCols), ", "))
		}
		if i >= len(q.back) || q.back[i] == nil {
			continue
		}
		for _, ci := range dbCols {
			if q.dbDefaultRow(i, ci) {
				dst, src, col := q.back[i], got, ci
				q.later(func() { copyField(q.meta, dst, &src, col) })
			}
		}
		for _, ci := range idCols {
			if id, ok := idString(q.meta.Value(&got, ci)); ok {
				back, col := q.back[i], ci
				q.later(func() { setID(q.meta.Ptr(back, col), id) }) // after the statement commits
			}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return Result{}, err
	}
	if err := rows.Close(); err != nil {
		return Result{}, err
	}
	if len(dbCols) > 0 && int(n) < len(q.models) {
		if err := q.shortfall(int(n)); err != nil {
			return Result{}, err
		}
	}
	return Result{RowsAffected: n}, nil
}

// keyNames lists the column names of the given indexes.
func keyNames[T any](m *Meta[T], cols []int) []string {
	out := make([]string, len(cols))
	for i, ci := range cols {
		out[i] = m.Columns[ci].Name
	}
	return out
}

// idKey joins the values of the given columns of m into a comparable key.
func idKey[T any](meta *Meta[T], m *T, cols []int) string {
	var sb strings.Builder
	for _, ci := range cols {
		v := reflect.ValueOf(meta.Value(m, ci))
		for v.Kind() == reflect.Pointer && !v.IsNil() {
			v = v.Elem()
		}
		fmt.Fprintf(&sb, "%v\x00", v)
	}
	return sb.String()
}

// idString reads a string-kind field value, or the string a *string points to.
func idString(v any) (string, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.String {
		return "", false
	}
	return rv.String(), true
}

// idUnset reports whether an id field value counts as not assigned: an empty
// string or a nil or empty string pointer. ok is false when the value is not
// a string kind, which no generator can fill.
func idUnset(v any) (unset, ok bool) {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		return rv.Len() == 0, true
	case reflect.Pointer:
		if rv.Type().Elem().Kind() != reflect.String {
			return false, false
		}
		return rv.IsNil() || rv.Elem().Len() == 0, true
	}
	return false, false
}

// setID stores id through p, a pointer to a string-kind field or to a pointer
// to one. It reports false for any other field type.
func setID(p any, id string) bool {
	rv := reflect.ValueOf(p)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return false
	}
	e := rv.Elem()
	switch {
	case e.Kind() == reflect.String:
		e.SetString(id)
		return true
	case e.Kind() == reflect.Pointer && e.Type().Elem().Kind() == reflect.String:
		s := reflect.New(e.Type().Elem())
		s.Elem().SetString(id)
		e.Set(s)
		return true
	}
	return false
}

// randBytes fills b from the system random source; it cannot fail on the
// supported platforms.
func randBytes(b []byte) {
	if _, err := rand.Read(b); err != nil {
		panic("momo: system random source failed: " + err.Error())
	}
}

// newUUID returns a random (version 4) UUID in its 36-character text form.
func newUUID() string {
	var u [16]byte
	randBytes(u[:])
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	const hex = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, b := range u {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hex[b>>4], hex[b&0x0f])
	}
	return string(out)
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var ulidState struct {
	sync.Mutex
	ms   uint64
	rand [10]byte
}

// newULID returns a ULID: 48 bits of millisecond time and 80 random bits in
// 26 Crockford base32 characters. Ids from one process sort in creation
// order, also within the same millisecond. It returns the empty string, which
// the insert reports as an error, if the random part would overflow.
func newULID() string {
	raw, ok := nextULIDBytes()
	if !ok {
		return ""
	}
	// Encode the 128 bits as 26 groups of 5 bits, least significant first;
	// the leading group holds only 3 bits.
	var out [26]byte
	hi := binary.BigEndian.Uint64(raw[0:8])
	lo := binary.BigEndian.Uint64(raw[8:16])
	for i := 25; i >= 0; i-- {
		out[i] = crockford[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out[:])
}

// nextULIDBytes advances the monotonic state and returns the next 128 bits. It
// reports false if the random part would overflow within one millisecond.
func nextULIDBytes() (raw [16]byte, ok bool) {
	ulidState.Lock()
	defer ulidState.Unlock()
	ms := uint64(time.Now().UnixMilli())
	if ms > ulidState.ms {
		ulidState.ms = ms
		randBytes(ulidState.rand[:])
	} else {
		ms = ulidState.ms // same millisecond or a clock step back: stay monotonic
		prev := ulidState.rand
		i := len(ulidState.rand) - 1
		for ; i >= 0; i-- {
			ulidState.rand[i]++
			if ulidState.rand[i] != 0 {
				break
			}
		}
		if i < 0 { // 2^80 ids in one millisecond: refuse instead of wrapping
			ulidState.rand = prev
			return raw, false
		}
	}
	binary.BigEndian.PutUint64(raw[0:8], ms<<16)
	copy(raw[6:], ulidState.rand[:])
	return raw, true
}

var cuidState struct {
	sync.Mutex
	counter uint32
	init    bool
}

const cuidBlock = 36 * 36 * 36 * 36 // values of a 4-character base36 block

var cuidFingerprint = sync.OnceValue(func() string {
	host, _ := os.Hostname()
	h := fnv.New32a()
	_, _ = h.Write([]byte(host + "/" + strconv.Itoa(os.Getpid())))
	return pad36(uint64(h.Sum32()%cuidBlock), 4)
})

// pad36 renders n in base 36, left-padded with zeros to width characters and
// keeping the low-order characters when longer.
func pad36(n uint64, width int) string {
	s := strconv.FormatUint(n, 36)
	if len(s) < width {
		return strings.Repeat("0", width-len(s)) + s
	}
	return s[len(s)-width:]
}

func cuidRandom() string {
	var b [8]byte
	randBytes(b[:])
	return pad36(binary.BigEndian.Uint64(b[:])%cuidBlock, 4)
}

// newCUID returns a collision-resistant id in the original cuid layout that
// Prisma's cuid default writes: "c", the millisecond time, a counter, a
// process fingerprint and 8 random characters, 25 lowercase base36 characters
// in all. Existing Prisma ids and new ones therefore look alike.
func newCUID() string {
	cuidState.Lock()
	if !cuidState.init {
		var b [4]byte
		randBytes(b[:])
		cuidState.counter = binary.BigEndian.Uint32(b[:]) % cuidBlock
		cuidState.init = true
	}
	c := cuidState.counter
	cuidState.counter = (cuidState.counter + 1) % cuidBlock
	cuidState.Unlock()
	return "c" + pad36(uint64(time.Now().UnixMilli()), 8) + pad36(uint64(c), 4) + cuidFingerprint() + cuidRandom() + cuidRandom()
}
