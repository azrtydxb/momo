// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"testing"
)

// sslRequestCode is the protocol code of a Postgres SSLRequest packet.
const sslRequestCode = 80877103

// wireCounter wraps a net.Conn and parses the frontend byte stream (pgx to
// server) for extended-protocol messages that create or use a named
// prepared statement: Parse and Bind with a non-empty statement name, and
// Describe/Close of a statement ('S'). It is independent of how the pooler is
// configured, so it holds even if PgBouncer would tolerate named statements.
type wireCounter struct {
	net.Conn
	mu      sync.Mutex
	buf     []byte
	started bool
	named   *atomic.Int64
	total   *atomic.Int64
}

func (w *wireCounter) Write(p []byte) (int, error) {
	n, err := w.Conn.Write(p)
	w.mu.Lock()
	w.buf = append(w.buf, p[:n]...)
	w.drain()
	w.mu.Unlock()
	return n, err
}

// drain consumes complete messages from w.buf.
func (w *wireCounter) drain() {
	for {
		if !w.started { // startup packet: int32 length, no type byte
			if len(w.buf) < 4 {
				return
			}
			l := int(binary.BigEndian.Uint32(w.buf))
			if l < 4 { // malformed frame: stop parsing rather than panic
				w.buf = nil
				return
			}
			if len(w.buf) < l {
				return
			}
			if l >= 8 && binary.BigEndian.Uint32(w.buf[4:8]) == sslRequestCode {
				// An encrypted stream cannot be parsed, so the detector
				// would silently count nothing.
				panic("pgbouncer wire counter: SSLRequest seen; the test needs sslmode=disable")
			}
			w.buf, w.started = w.buf[l:], true
			continue
		}
		if len(w.buf) < 5 {
			return
		}
		l := int(binary.BigEndian.Uint32(w.buf[1:5]))
		if l < 4 { // malformed frame: stop parsing rather than panic
			w.buf = nil
			return
		}
		if len(w.buf) < 1+l {
			return
		}
		typ, body := w.buf[0], w.buf[5:1+l]
		w.buf = w.buf[1+l:]
		switch typ {
		case 'P': // Parse: statement name, query, ...
			w.total.Add(1)
			if len(body) > 0 && body[0] != 0 {
				w.named.Add(1)
			}
		case 'B': // Bind: portal name, statement name, ...
			if i := bytes.IndexByte(body, 0); i >= 0 && i+1 < len(body) && body[i+1] != 0 {
				w.named.Add(1)
			}
		case 'D', 'C': // Describe/Close: 'S' = statement, then its name
			if len(body) > 1 && body[0] == 'S' && body[1] != 0 {
				w.named.Add(1)
			}
		}
	}
}

// TestWireCounterDrainGuards covers the parser itself: an SSLRequest fails
// loudly and a frame shorter than its own length field stops parsing.
func TestWireCounterDrainGuards(t *testing.T) {
	short := &wireCounter{named: new(atomic.Int64), total: new(atomic.Int64), started: true, buf: []byte{'P', 0, 0, 0, 0}}
	short.drain() // must not panic
	if len(short.buf) != 0 {
		t.Fatalf("malformed frame left %d bytes buffered", len(short.buf))
	}
	ssl := &wireCounter{named: new(atomic.Int64), total: new(atomic.Int64), buf: []byte{0, 0, 0, 8, 4, 210, 22, 47}}
	defer func() {
		if recover() == nil {
			t.Fatal("SSLRequest did not fail loudly")
		}
	}()
	ssl.drain()
}
