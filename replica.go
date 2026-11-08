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
// to the primary, and count as a write of the unit of work. If the replica
// cannot be reached when a read starts, that read runs on the primary; there
// is no retry once rows have been returned.
//
// It returns ErrUnsupported when replica i is not configured, and when the DB
// was built with WithImplicitTx: that option runs every statement in its own
// primary transaction, so a Replica handle could never use the replica, and
// failing is clearer than silently reading the primary. A read on the replica
// runs outside any transaction, so WithTxSetup does not apply to it; do not
// rely on a setting that WithTxSetup installs (for example a row-level-security
// tenant) for replica reads.
func (db *DB) Replica(i int) (*DB, error) {
	if i < 0 || i >= len(db.replicas) {
		return nil, &Error{Kind: ErrUnsupported, Op: "replica", Err: errors.New("no such read replica configured")}
	}
	if db.implicitTx {
		return nil, &Error{Kind: ErrUnsupported, Op: "replica", Err: errors.New("WithImplicitTx runs every statement on the primary; a read replica handle cannot be combined with it")}
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
	if !readOnlySQL(sql) {
		// Not provably a read (INSERT ... RETURNING, SELECT nextval, a locking
		// read): run it on the primary and count it as a write, so later reads
		// of the same unit of work see its effects.
		markWrite(ctx)
		return r.Driver.Query(ctx, sql, args)
	}
	if uowWritten(ctx) && !staleOK(ctx) {
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

// readOnlyStart lists the statements that may start a read, after any EXPLAIN
// options.
var readOnlyStart = map[string]bool{"select": true, "with": true, "values": true, "table": true}

// explainOptions are skipped between EXPLAIN and the statement it explains.
var explainOptions = map[string]bool{"analyze": true, "analyse": true, "verbose": true}

// writeWords make a statement a write wherever they appear as a word outside
// literals, quoted identifiers and comments. The list is deliberately wide: a
// false positive only costs a read on the primary.
var writeWords = map[string]bool{
	// data changes and commands
	"insert": true, "update": true, "delete": true, "merge": true, "into": true, "copy": true,
	"create": true, "drop": true, "alter": true, "truncate": true, "refresh": true, "execute": true,
	"call": true, "do": true, "listen": true, "notify": true, "lock": true, "declare": true,
	"grant": true, "revoke": true, "vacuum": true, "analyze": true, "analyse": true, "cluster": true,
	"reindex": true, "comment": true, "security": true, "import": true,
	// functions with side effects
	"nextval": true, "setval": true, "lastval": true, "set_config": true, "pg_notify": true,
	"txid_current": true, "pg_terminate_backend": true, "pg_cancel_backend": true,
	"lo_import": true, "lo_export": true, "lo_unlink": true, "dblink_exec": true,
	"pg_reload_conf": true, "pg_switch_wal": true, "pg_logical_emit_message": true,
}

type sqlToken struct {
	word string // lower-cased identifier or keyword; empty for anything else
	punc byte   // the character of a punctuation token; 0 for words and literals
}

// tokenizeSQL splits sql into words, punctuation and opaque literals. String
// literals, quoted identifiers and comments are dropped (a literal becomes an
// empty token). It reports false for text it cannot read with confidence:
// an unterminated literal or comment, or a dollar-quoted body.
func tokenizeSQL(sql string) ([]sqlToken, bool) {
	var out []sqlToken
	for i := 0; i < len(sql); {
		c := sql[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
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
				return nil, false
			}
			out = append(out, sqlToken{})
			i = j + 1
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
				return nil, false
			}
		case c == '$' && i+1 < len(sql) && (sql[i+1] == '$' || sql[i+1] == '_' || unicode.IsLetter(rune(sql[i+1]))):
			return nil, false // dollar-quoted body: cannot be inspected
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i
			for j < len(sql) && (sql[j] == '_' || sql[j] == '$' || sql[j] >= 'a' && sql[j] <= 'z' || sql[j] >= 'A' && sql[j] <= 'Z' || sql[j] >= '0' && sql[j] <= '9') {
				j++
			}
			out = append(out, sqlToken{word: strings.ToLower(sql[i:j])})
			i = j
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		default:
			out = append(out, sqlToken{punc: c})
			i++
		}
	}
	return out, true
}

// readOnlySQL is a conservative classifier. It returns true only for one
// statement that starts with SELECT, WITH, VALUES or TABLE (after any
// parentheses), or with EXPLAIN, its options and then one of those, and that
// contains no word from writeWords, no FOR UPDATE, FOR SHARE, FOR NO KEY
// UPDATE or FOR KEY SHARE clause, no advisory-lock function and no
// dollar-quoted body. A semicolon followed by anything else (a second
// statement) makes it false. Whatever it cannot read with confidence is a
// write, so the statement goes to the primary: a wrong answer true would send
// a write to a read-only server, a wrong answer false only costs a read on the
// primary.
//
// It cannot see inside a user-defined function: a SELECT that calls a function
// that writes looks like a read. Run such a call on the primary handle or in a
// transaction.
func readOnlySQL(sql string) bool {
	toks, ok := tokenizeSQL(sql)
	if !ok {
		return false
	}
	for len(toks) > 0 && toks[len(toks)-1].punc == ';' { // trailing semicolons
		toks = toks[:len(toks)-1]
	}
	for _, t := range toks {
		if t.punc == ';' {
			return false // a second statement
		}
	}
	i := 0
	skipParens := func() {
		for i < len(toks) && toks[i].punc == '(' {
			i++
		}
	}
	skipParens()
	if i >= len(toks) || toks[i].word == "" {
		return false
	}
	if toks[i].word == "explain" {
		i++
		for i < len(toks) {
			switch {
			case toks[i].punc == '(': // EXPLAIN (ANALYZE, VERBOSE, ...)
				depth := 0
				for ; i < len(toks); i++ {
					if toks[i].punc == '(' {
						depth++
					} else if toks[i].punc == ')' {
						depth--
						if depth == 0 {
							i++
							break
						}
					}
				}
				if depth != 0 {
					return false
				}
				continue
			case explainOptions[toks[i].word]:
				i++
				continue
			}
			break
		}
		skipParens()
		if i >= len(toks) {
			return false
		}
	}
	if !readOnlyStart[toks[i].word] {
		return false
	}
	prev := ""
	for ; i < len(toks); i++ {
		w := toks[i].word
		if w == "" {
			if toks[i].punc != 0 {
				prev = ""
			}
			continue
		}
		if writeWords[w] || strings.HasPrefix(w, "pg_advisory") || strings.HasPrefix(w, "pg_try_advisory") {
			return false
		}
		if prev == "for" && (w == "share" || w == "no" || w == "key") {
			return false
		}
		prev = w
	}
	return true
}
