// SPDX-License-Identifier: Apache-2.0

// Package grpc generates protobuf messages and CRUD/list services from momo
// models. A lock file records every field number, so regenerating never
// renumbers a field.
package grpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/azrtydxb/momo/gen"
)

// lockVersion is the lock file format this package reads and writes.
const lockVersion = 1

const (
	// maxFieldNumber is the largest protobuf field number.
	maxFieldNumber = 1<<29 - 1
	// The range 19000 to 19999 is reserved by the protobuf implementation.
	reservedLow, reservedHigh = 19000, 19999
)

// Lock records the protobuf field number of every column of every message.
type Lock struct {
	Version  int                     `json:"version"`
	Messages map[string]*MessageLock `json:"messages"`
}

// MessageLock holds the numbers of one message. Field identity is the
// database column name, so renaming a Go field that keeps its column keeps
// its number. A column that disappears moves its number to Reserved, and the
// number is never handed out again.
type MessageLock struct {
	Fields   map[string]int `json:"fields"`
	Reserved []int          `json:"reserved,omitempty"`
}

// LoadLock reads the lock at path. A missing file is an empty lock. A file
// that cannot be parsed, has a newer version or holds an invalid or repeated
// number is an error: a lock that cannot be trusted must not be regenerated
// over.
func LoadLock(path string) (*Lock, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- the lock path is the generator's own configured output
	if errors.Is(err, fs.ErrNotExist) {
		return &Lock{Version: lockVersion, Messages: map[string]*MessageLock{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("grpc: read lock: %w", err)
	}
	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("grpc: parse lock %s: %w", filepath.Base(path), err)
	}
	if l.Version != lockVersion {
		return nil, fmt.Errorf("grpc: lock %s has version %d; this momo reads version %d", filepath.Base(path), l.Version, lockVersion)
	}
	if l.Messages == nil {
		l.Messages = map[string]*MessageLock{}
	}
	for name, m := range l.Messages {
		if m == nil {
			return nil, fmt.Errorf("grpc: lock message %q is null", name)
		}
		if m.Fields == nil {
			m.Fields = map[string]int{}
		}
		seen := map[int]string{}
		check := func(n int, what string) error {
			if n < 1 || n > maxFieldNumber || (n >= reservedLow && n <= reservedHigh) {
				return fmt.Errorf("grpc: lock message %q: %s has invalid field number %d", name, what, n)
			}
			if prev, dup := seen[n]; dup {
				return fmt.Errorf("grpc: lock message %q: number %d is used by %s and %s", name, n, prev, what)
			}
			seen[n] = what
			return nil
		}
		cols := make([]string, 0, len(m.Fields))
		for c := range m.Fields {
			cols = append(cols, c)
		}
		sort.Strings(cols)
		for _, c := range cols {
			if err := check(m.Fields[c], "column "+c); err != nil {
				return nil, err
			}
		}
		for _, n := range m.Reserved {
			if err := check(n, "a reserved entry"); err != nil {
				return nil, err
			}
		}
	}
	return &l, nil
}

// Save writes the lock to path as indented JSON with sorted keys, through a
// temporary file in the same directory.
func (l *Lock) Save(path string) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(l); err != nil {
		return fmt.Errorf("grpc: encode lock: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 -- the lock file is committed beside the .proto
		return fmt.Errorf("grpc: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".momo-lock-*.tmp")
	if err != nil {
		return fmt.Errorf("grpc: %w", err)
	}
	name := tmp.Name()
	_, werr := tmp.Write(buf.Bytes())
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("grpc: write lock: %w", err)
	}
	if err := os.Chmod(name, 0o644); err != nil { // #nosec G302 -- committed file, not a secret
		_ = os.Remove(name)
		return fmt.Errorf("grpc: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("grpc: %w", err)
	}
	return nil
}

// Assign returns the field number of each column of message, recording new
// columns in the lock. An existing column keeps its number. A new column gets
// the smallest number above every number ever used for the message, reserved
// ones included, skipping the range protobuf reserves. A column recorded
// earlier but absent from columns moves its number to Reserved. A repeated
// column or an exhausted number space is a diagnostic.
func (l *Lock) Assign(message string, columns []string) (map[string]int, []gen.Diagnostic) {
	if l.Messages == nil {
		l.Messages = map[string]*MessageLock{}
	}
	if l.Version == 0 {
		l.Version = lockVersion
	}
	m := l.Messages[message]
	if m == nil {
		m = &MessageLock{Fields: map[string]int{}}
		l.Messages[message] = m
	}
	if m.Fields == nil {
		m.Fields = map[string]int{}
	}
	var diags []gen.Diagnostic
	present := map[string]bool{}
	for _, c := range columns {
		if present[c] {
			diags = append(diags, gen.Diagnostic{Msg: fmt.Sprintf("message %s: column %q is listed twice", message, c)})
		}
		present[c] = true
	}

	// Retire the columns that are gone, in a stable order.
	var gone []string
	for c := range m.Fields {
		if !present[c] {
			gone = append(gone, c)
		}
	}
	sort.Strings(gone)
	for _, c := range gone {
		m.Reserved = append(m.Reserved, m.Fields[c])
		delete(m.Fields, c)
	}
	sort.Ints(m.Reserved)

	next := 0
	for _, n := range m.Fields {
		next = max(next, n)
	}
	for _, n := range m.Reserved {
		next = max(next, n)
	}
	out := make(map[string]int, len(columns))
	for _, c := range columns {
		if n, ok := m.Fields[c]; ok {
			out[c] = n
			continue
		}
		next++
		if next >= reservedLow && next <= reservedHigh {
			next = reservedHigh + 1
		}
		if next > maxFieldNumber {
			diags = append(diags, gen.Diagnostic{Msg: fmt.Sprintf("message %s: out of protobuf field numbers", message)})
			break
		}
		m.Fields[c] = next
		out[c] = next
	}
	return out, diags
}
