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
// number is never handed out again, and its name goes to ReservedNames while
// no column uses it, so the .proto can reserve both.
//
// Table is the table the message came from. A model whose Go type is renamed
// keeps its table, so the new message adopts the numbers of the old one. Types
// records the protobuf type of each field ("string", "optional int64",
// "repeated float"); a column whose type changes keeps its name and gets a new
// number, and the old number is reserved, because a reader of the old schema
// would decode the new bytes as the old type.
type MessageLock struct {
	Table         string            `json:"table,omitempty"`
	Types         map[string]string `json:"types,omitempty"`
	Fields        map[string]int    `json:"fields"`
	Reserved      []int             `json:"reserved,omitempty"`
	ReservedNames []string          `json:"reserved_names,omitempty"`
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
	if err := json.Unmarshal(stripHeader(data), &l); err != nil {
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
		for c := range m.Types {
			if _, ok := m.Fields[c]; !ok {
				return nil, fmt.Errorf("grpc: lock message %q: type recorded for %q, which is not a field", name, c)
			}
		}
		names := map[string]bool{}
		for _, n := range m.ReservedNames {
			switch {
			case !identRE.MatchString(n):
				return nil, fmt.Errorf("grpc: lock message %q: reserved name %q is not an identifier", name, n)
			case names[n]:
				return nil, fmt.Errorf("grpc: lock message %q: reserved name %q is listed twice", name, n)
			}
			if _, used := m.Fields[n]; used {
				return nil, fmt.Errorf("grpc: lock message %q: reserved name %q is also a field", name, n)
			}
			names[n] = true
		}
	}
	return &l, nil
}

// stripHeader removes the leading "//" comment lines of a lock file: the
// generated-file header the plugin writes in front of the JSON, which the
// writer needs to tell a generated file from a handwritten one.
func stripHeader(data []byte) []byte {
	for bytes.HasPrefix(data, []byte("//")) {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			return nil
		}
		data = data[i+1:]
	}
	return data
}

// lockFile is the content of the lock file: the generated-file header, which
// LoadLock skips, followed by the JSON of Marshal.
func lockFile(marshaled []byte) []byte {
	return append([]byte(gen.Header()+"\n"), marshaled...)
}

// Marshal returns the lock as indented JSON with sorted keys and a final
// newline. Generator code never writes files itself: the caller hands the
// bytes to the gen.Writer.
func (l *Lock) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(l); err != nil {
		return nil, fmt.Errorf("grpc: encode lock: %w", err)
	}
	return buf.Bytes(), nil
}

// Assign returns the field number of each column of message, recording new
// columns in the lock. An existing column keeps its number. A new column gets
// the smallest number above every number ever used for the message, reserved
// ones included, skipping the range protobuf reserves. A column recorded
// earlier but absent from columns moves its number to Reserved. A repeated
// column or an exhausted number space is a diagnostic.
func (l *Lock) Assign(message string, columns []string) (map[string]int, []gen.Diagnostic) {
	return l.AssignKeeping(message, columns, nil)
}

// AssignKeeping is Assign for a model that has columns it does not put in the
// message (sensitive columns, or columns a diagnostic left out). Those hidden
// columns keep any number already recorded, and are neither numbered nor
// retired: only a column that is gone from the model altogether is retired, so
// toggling a column's visibility never frees its number. A retired column's
// name is recorded in ReservedNames until a column uses it again.
func (l *Lock) AssignKeeping(message string, columns, hidden []string) (map[string]int, []gen.Diagnostic) {
	return l.AssignTyped(message, "", nil, columns, nil, hidden)
}

// AssignTyped is AssignKeeping for a message that comes from table and whose
// columns have the protobuf types in types (a missing entry is not checked).
// When the lock has no entry for message but has one recorded for the same
// table under a name that is not in live, the message adopts that entry, so
// renaming a Go type keeps its field numbers. A column whose recorded type
// differs from its type now moves its old number to Reserved and gets a new
// one.
func (l *Lock) AssignTyped(message, table string, live map[string]bool, columns []string, types map[string]string, hidden []string) (map[string]int, []gen.Diagnostic) {
	if l.Messages == nil {
		l.Messages = map[string]*MessageLock{}
	}
	if l.Version == 0 {
		l.Version = lockVersion
	}
	m := l.Messages[message]
	if m == nil && table != "" {
		var olds []string
		for k, old := range l.Messages {
			if old != nil && old.Table == table && !live[k] {
				olds = append(olds, k)
			}
		}
		sort.Strings(olds)
		if len(olds) > 0 {
			m = l.Messages[olds[0]]
			l.Messages[message] = m
			delete(l.Messages, olds[0])
		}
	}
	if m == nil {
		m = &MessageLock{Fields: map[string]int{}}
		l.Messages[message] = m
	}
	if m.Fields == nil {
		m.Fields = map[string]int{}
	}
	if table != "" {
		m.Table = table
	}
	if m.Types == nil {
		m.Types = map[string]string{}
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
	kept := map[string]bool{}
	for _, c := range hidden {
		kept[c] = true
	}
	var gone []string
	for c := range m.Fields {
		if !present[c] && !kept[c] {
			gone = append(gone, c)
		}
	}
	sort.Strings(gone)
	for _, c := range gone {
		m.Reserved = append(m.Reserved, m.Fields[c])
		m.ReservedNames = append(m.ReservedNames, c)
		delete(m.Fields, c)
		delete(m.Types, c)
	}
	// A column whose type changed gives up its number: it keeps its name.
	for _, c := range columns {
		if old, ok := m.Types[c]; ok && types[c] != "" && old != types[c] {
			if n, has := m.Fields[c]; has {
				m.Reserved = append(m.Reserved, n)
				delete(m.Fields, c)
			}
		}
	}
	sort.Ints(m.Reserved)
	// A name in use again is no longer reserved: protoc refuses a field whose
	// name is reserved. Its old number stays reserved.
	names := m.ReservedNames[:0]
	for _, n := range m.ReservedNames {
		if !present[n] {
			names = append(names, n)
		}
	}
	m.ReservedNames = names
	sort.Strings(m.ReservedNames)

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
	for _, c := range columns {
		if t := types[c]; t != "" {
			if _, numbered := m.Fields[c]; numbered {
				m.Types[c] = t
			}
		}
	}
	if len(m.Types) == 0 {
		m.Types = nil
	}
	return out, diags
}
