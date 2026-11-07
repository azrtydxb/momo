// SPDX-License-Identifier: Apache-2.0

package momo

import (
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
// must return a non-empty string and be safe for concurrent use. A model that
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
			if fn, ok := e.db.idGens[name]; ok && fn != nil {
				return fn, true
			}
		}
	}
	fn, ok := builtinIDGens[name]
	return fn, ok
}

// idGenDefaults resolves the generators of m into Go-side defaults, so a
// column left out of the statement (Omit, Values) still gets an id.
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
		out[g.Column] = func(*T) any { return fn() }
	}
	return out, nil
}

// generateIDs fills the id fields that hold their zero value in the models of
// q. With writeBack it also stores each generated id in the caller's model,
// so the caller reads it after Exec or Rows. It is a no-op for a model
// without id generators.
func (q InsertQuery[T]) generateIDs(writeBack bool) (InsertQuery[T], error) {
	if q.q == nil || len(q.meta.IDGens) == 0 || len(q.models) == 0 || q.meta.Ptr == nil || q.meta.Value == nil {
		return q, nil
	}
	cloned := false
	for _, g := range q.meta.IDGens {
		ci := g.Column
		if ci < 0 || ci >= len(q.meta.Columns) || !q.writable(ci) || slices.Contains(q.i.omit, q.meta.Columns[ci].Name) {
			continue // an omitted column gets its id through the Go-side default
		}
		fn, ok := idGenerator(q.q, g.Name)
		if !ok {
			return q, q.verr(q.meta.Columns[ci].Name, "unknown id generator %q: use cuid, uuid, ulid or register it with WithIDGenerator", g.Name)
		}
		for r := range q.models {
			v := q.meta.Value(&q.models[r], ci)
			unset, ok := idUnset(v)
			if !ok {
				return q, q.verr(q.meta.Columns[ci].Name, "idgen needs a string column, got %T", v)
			}
			if !unset {
				continue
			}
			id := fn()
			if id == "" {
				return q, q.verr(q.meta.Columns[ci].Name, "id generator %q returned an empty id", g.Name)
			}
			if !cloned {
				q.models = slices.Clone(q.models)
				cloned = true
			}
			if !setID(q.meta.Ptr(&q.models[r], ci), id) {
				return q, q.verr(q.meta.Columns[ci].Name, "idgen needs a string column, got %T", q.meta.Value(&q.models[r], ci))
			}
			if writeBack && r < len(q.back) && q.back[r] != nil {
				setID(q.meta.Ptr(q.back[r], ci), id)
			}
		}
	}
	return q, nil
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
// order, also within the same millisecond.
func newULID() string {
	ulidState.Lock()
	ms := uint64(time.Now().UnixMilli())
	if ms > ulidState.ms {
		ulidState.ms = ms
		randBytes(ulidState.rand[:])
	} else {
		ms = ulidState.ms // same millisecond or a clock step back: stay monotonic
		for i := len(ulidState.rand) - 1; i >= 0; i-- {
			ulidState.rand[i]++
			if ulidState.rand[i] != 0 {
				break
			}
		}
	}
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[0:8], ms<<16)
	copy(raw[6:], ulidState.rand[:])
	ulidState.Unlock()
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
