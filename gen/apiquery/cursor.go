// SPDX-License-Identifier: Apache-2.0

// Package apiquery holds the request-side helpers the GraphQL, gRPC and
// OpenAPI generators share: opaque keyset cursors, page-size limits and
// filters resolved through the generated column allowlist.
package apiquery

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
)

const (
	// DefaultLimit is the page size used when a request names none.
	DefaultLimit = 50
	// MaxLimit is the largest page size a request may ask for.
	MaxLimit = 200
)

// ClampLimit returns DefaultLimit for n <= 0 and MaxLimit for n > MaxLimit.
func ClampLimit(n int) int {
	if n <= 0 {
		return DefaultLimit
	}
	if n > MaxLimit {
		return MaxLimit
	}
	return n
}

// Cursor is the decoded form of an opaque page cursor: the sort-key values of
// the last row of a page, as text, in sort order.
type Cursor struct {
	Keys []string
}

type cursorWire struct {
	K []string `json:"k"`
}

// EncodeCursor returns the opaque cursor for keys: base64url, without
// padding, of the JSON object {"k":[...]}.
func EncodeCursor(keys ...string) string {
	if keys == nil {
		keys = []string{}
	}
	b, err := json.Marshal(cursorWire{K: keys})
	if err != nil { // a []string always marshals
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses a cursor made by EncodeCursor. A malformed cursor
// returns an error that matches momo.ErrValidation and does not echo s.
func DecodeCursor(s string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, invalidCursor()
	}
	var w cursorWire
	if err := json.Unmarshal(raw, &w); err != nil || w.K == nil {
		return Cursor{}, invalidCursor()
	}
	return Cursor{Keys: w.K}, nil
}

func invalidCursor() error {
	return &momo.Error{Kind: momo.ErrValidation, Op: "cursor", Err: errors.New("invalid page cursor")}
}

func invalidf(op, format string, args ...any) error {
	return &momo.Error{Kind: momo.ErrValidation, Op: op, Err: fmt.Errorf(format, args...)}
}
