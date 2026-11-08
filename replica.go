// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"syscall"
	"unicode"
)

// WithReplicas registers read-replica drivers. Replicas are used only through
// DB.Replica; the primary handle never reads from them.
func WithReplicas(replicas ...Driver) Option {
	return func(db *DB) { db.replicas = append(db.replicas, replicas...) }
}

// Replica returns a read handle whose plain reads go to replica i, unless the
// context is a unit of work that has written: then reads go to the primary.
// Writes, transactions, INSERT/UPDATE/DELETE ... RETURNING, locking reads
// (FOR UPDATE and FOR SHARE), and any statement that readOnlySQL cannot prove
// read-only, such as a WITH that contains DML or a call to nextval, always go
// to the primary. If the replica cannot be reached when a read starts, that
// read runs on the primary; there is no retry once rows have been returned. It returns ErrUnsupported
// when replica i is not configured.
func (db *DB) Replica(i int) (*DB, error) {
	if i < 0 || i >= len(db.replicas) {
		return nil, &Error{Kind: ErrUnsupported, Op: "replica", Err: errors.New("no such read replica configured")}
	}
	nd := new(DB)
	*nd = *db
	nd.replicas = nil // replicas do not chain
	nd.drv = routingDriver{Driver: db.drv, replica: db.replicas[i]}
	nd.ex = execer{db: nd, q: nd.drv, d: db.ex.d, classify: db.ex.classify, owner: nd}
	return nd, nil
}

type uowKey struct{}
type staleOKKey struct{}

// uowState records whether a write happened in one unit of work.
type uowState struct{ written atomic.Bool }

// NewUnitOfWork returns a context in which every write made through any
// handle is recorded. A Replica handle reads from the primary once a write has
// been recorded in that context, so a request reads its own writes.
func NewUnitOfWork(ctx context.Context) context.Context {
	return context.WithValue(ctx, uowKey{}, &uowState{})
}

// AllowStaleReads returns a context that opts reads of a Replica handle back
// to the replica even after a write in the same unit of work.
func AllowStaleReads(ctx context.Context) context.Context {
	return context.WithValue(ctx, staleOKKey{}, true)
}

// markWrite records a write in the context's unit of work, if it has one.
func markWrite(ctx context.Context) {
	if s, ok := ctx.Value(uowKey{}).(*uowState); ok {
		s.written.Store(true)
	}
}

func uowWritten(ctx context.Context) bool {
	s, ok := ctx.Value(uowKey{}).(*uowState)
	return ok && s.written.Load()
}

func staleOK(ctx context.Context) bool {
	v, _ := ctx.Value(staleOKKey{}).(bool)
	return v
}

// routingDriver sends statements that are provably reads to the replica and
// everything else to the embedded primary.
type routingDriver struct {
	Driver
	replica Driver
}

// Query routes one statement. It goes to the replica only when readOnlySQL
// accepts it and the context is not a unit of work that has written (unless
// AllowStaleReads is set). When the replica fails with a connection-class
// error before returning rows, the statement runs on the primary instead;
// errors raised after rows were returned, and every other kind of error,
// reach the caller unchanged.
func (r routingDriver) Query(ctx context.Context, sql string, args []any) (Rows, error) {
	if !readOnlySQL(sql) || (uowWritten(ctx) && !staleOK(ctx)) {
		return r.Driver.Query(ctx, sql, args)
	}
	rows, err := r.replica.Query(ctx, sql, args)
	if err != nil && ctx.Err() == nil && connectionClassError(err) {
		return r.Driver.Query(ctx, sql, args)
	}
	return rows, err
}

// sqlStater is implemented by driver errors that carry a SQLSTATE, such as
// pgconn.PgError.
type sqlStater interface{ SQLState() string }

// connectionClassError reports whether err says the server could not be
// reached or went away, so retrying the read elsewhere is safe and useful.
func connectionClassError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, driver.ErrBadConn) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var st sqlStater
	if errors.As(err, &st) {
		c := st.SQLState()
		// 08 connection exception, 57P01..57P03 shutdown and startup, 53300 too many connections.
		return strings.HasPrefix(c, "08") || c == "57P01" || c == "57P02" || c == "57P03" || c == "53300"
	}
	return false
}

// readOnlyFirst lists the statements that may start a read.
var readOnlyFirst = map[string]bool{"select": true, "with": true, "values": true, "explain": true}

// writeWords make a statement a write wherever they appear as a word outside
// literals, quoted identifiers and comments.
var writeWords = map[string]bool{
	"insert": true, "update": true, "delete": true, "merge": true, "into": true, "copy": true,
	"nextval": true, "setval": true, "lastval": true, "set_config": true, "pg_notify": true,
	"txid_current": true, "pg_terminate_backend": true, "pg_cancel_backend": true,
}

// readOnlySQL is a conservative classifier: it returns true only for a
// statement that starts with SELECT, WITH, VALUES or EXPLAIN and contains no
// data-changing keyword (INSERT, UPDATE, DELETE, MERGE, INTO, COPY), no row
// lock clause (FOR UPDATE, FOR SHARE, FOR NO KEY UPDATE, FOR KEY SHARE), no
// sequence or session-state function (nextval, setval, lastval, set_config,
// advisory locks) and no dollar-quoted body. Anything it cannot read with
// confidence is a write, so the statement goes to the primary. A false
// negative on the replica side would send a write to a read-only server; a
// false positive only costs a read on the primary.
func readOnlySQL(sql string) bool {
	first := true
	prev := ""
	for i := 0; i < len(sql); {
		c := sql[i]
		switch {
		case c == '\'' || c == '"':
			j := i + 1
			for j < len(sql) {
				if sql[j] == c {
					if j+1 < len(sql) && sql[j+1] == c { // doubled quote
						j += 2
						continue
					}
					break
				}
				j++
			}
			if j >= len(sql) {
				return false // unterminated
			}
			i = j + 1
			if first { // a statement cannot start with a literal
				return false
			}
			prev = ""
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			depth := 1
			i += 2
			for i < len(sql) && depth > 0 {
				switch {
				case strings.HasPrefix(sql[i:], "/*"):
					depth++
					i += 2
				case strings.HasPrefix(sql[i:], "*/"):
					depth--
					i += 2
				default:
					i++
				}
			}
			if depth > 0 {
				return false
			}
		case c == '$':
			if i+1 < len(sql) && (sql[i+1] == '$' || sql[i+1] == '_' || unicode.IsLetter(rune(sql[i+1]))) {
				return false // dollar-quoted body: cannot be inspected
			}
			i++
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i
			for j < len(sql) && (sql[j] == '_' || sql[j] == '$' || sql[j] >= 'a' && sql[j] <= 'z' || sql[j] >= 'A' && sql[j] <= 'Z' || sql[j] >= '0' && sql[j] <= '9') {
				j++
			}
			w := strings.ToLower(sql[i:j])
			i = j
			if first {
				if !readOnlyFirst[w] {
					return false
				}
				first = false
			}
			if writeWords[w] || strings.HasPrefix(w, "pg_advisory") || strings.HasPrefix(w, "pg_try_advisory") {
				return false
			}
			if prev == "for" && (w == "update" || w == "share" || w == "no" || w == "key") {
				return false
			}
			prev = w
		default:
			i++
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '(' {
				if first {
					return false
				}
			}
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
				prev = ""
			}
		}
	}
	return !first
}
