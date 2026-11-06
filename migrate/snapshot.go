// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/azrtydxb/momo/schema"
)

// SnapshotFormat is the snapshot wrapper format version this momo writes and
// reads up to. It is independent of schema.FormatVersion, which versions the
// embedded schema.
const SnapshotFormat = 1

// SnapshotFile is the content of <version>_<name>.snapshot.json: the schema
// after a migration, plus the version of the migration it follows.
type SnapshotFile struct {
	Format        int            `json:"format_version"` // SnapshotFormat
	Version       string         `json:"version"`
	ParentVersion string         `json:"parent"` // empty for the first migration
	Schema        *schema.Schema `json:"schema"`
}

// rawSnapshot decodes the wrapper while deferring the schema to schema.Load,
// which owns the schema format_version check and the version dispatch.
type rawSnapshot struct {
	Format  int             `json:"format_version"`
	Version string          `json:"version"`
	Parent  string          `json:"parent"`
	Schema  json.RawMessage `json:"schema"`
}

// ReadSnapshot decodes a snapshot file. It tolerates a UTF-8 BOM and CRLF line
// endings. It returns an error wrapping ErrNewerFormat when the wrapper or the
// embedded schema was written by a newer momo, and an error for corrupt,
// truncated or incomplete input.
func ReadSnapshot(r io.Reader) (*SnapshotFile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("migrate: read snapshot: %w", err)
	}
	data = bytes.TrimPrefix(data, utf8BOM)
	dec := json.NewDecoder(bytes.NewReader(data))
	var raw rawSnapshot
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("migrate: decode snapshot: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("migrate: decode snapshot: unexpected data after the JSON document")
	}
	switch {
	case raw.Format == 0:
		return nil, errors.New("migrate: snapshot has no format_version")
	case raw.Format < 0:
		return nil, fmt.Errorf("migrate: snapshot has invalid format_version %d", raw.Format)
	case raw.Format > SnapshotFormat:
		return nil, fmt.Errorf("%w: snapshot format_version %d; this momo reads format_version %d and earlier, upgrade momo to read it", ErrNewerFormat, raw.Format, SnapshotFormat)
	}
	if len(raw.Schema) == 0 || string(raw.Schema) == "null" {
		return nil, errors.New("migrate: snapshot has no schema")
	}
	s, err := schema.Load(bytes.NewReader(raw.Schema))
	if err != nil {
		return nil, fmt.Errorf("migrate: snapshot schema: %w", err)
	}
	return &SnapshotFile{Format: raw.Format, Version: raw.Version, ParentVersion: raw.Parent, Schema: s}, nil
}

// WriteSnapshot encodes s as indented JSON with a trailing newline. It stamps
// the current SnapshotFormat and schema.FormatVersion and sorts the schema, so
// equal input always yields identical bytes. It does not modify s.
func WriteSnapshot(w io.Writer, s SnapshotFile) error {
	if s.Schema == nil {
		return errors.New("migrate: write snapshot: nil schema")
	}
	// Deep-copy through JSON so Sort cannot reorder the caller's slices.
	stamped := *s.Schema
	stamped.FormatVersion = schema.FormatVersion
	b, err := json.Marshal(&stamped)
	if err != nil {
		return fmt.Errorf("migrate: write snapshot: %w", err)
	}
	cp, err := schema.Load(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("migrate: write snapshot: %w", err)
	}
	cp.Sort()
	s.Format = SnapshotFormat
	s.Schema = cp
	out, err := json.MarshalIndent(&s, "", "  ")
	if err != nil {
		return fmt.Errorf("migrate: write snapshot: %w", err)
	}
	if _, err := w.Write(append(out, '\n')); err != nil {
		return fmt.Errorf("migrate: write snapshot: %w", err)
	}
	return nil
}

// LatestSnapshot returns the snapshot with the greatest version in the top
// level of fsys, or nil and ErrNoSnapshot when there is none. A repeated
// version, a snapshot whose recorded version differs from its file name, and a
// snapshot that names itself as its parent are errors naming the file.
func LatestSnapshot(fsys fs.FS) (*SnapshotFile, error) {
	files, err := Discover(fsys)
	if err != nil {
		return nil, err
	}
	var best *SnapshotFile
	seen := map[string]string{}
	for _, f := range files {
		if f.Kind != "snapshot" {
			continue
		}
		if prev, ok := seen[f.Version]; ok {
			return nil, fmt.Errorf("migrate: %s and %s share snapshot version %s", prev, f.Path, f.Version)
		}
		seen[f.Version] = f.Path
		s, err := ReadSnapshot(strings.NewReader(string(f.Body)))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		if s.Version != f.Version {
			return nil, fmt.Errorf("migrate: %s records version %q but its file name says %s", f.Path, s.Version, f.Version)
		}
		if s.ParentVersion == s.Version {
			return nil, fmt.Errorf("migrate: %s names itself as its parent", f.Path)
		}
		if best == nil || s.Version > best.Version {
			best = s
		}
	}
	if best == nil {
		return nil, ErrNoSnapshot
	}
	return best, nil
}
