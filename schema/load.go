// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrNewerFormat is returned when a snapshot was written by a newer momo.
var ErrNewerFormat = errors.New("schema: snapshot format is newer than this momo")

// ErrMissingFormatVersion is returned when a snapshot carries no format_version.
var ErrMissingFormatVersion = errors.New("schema: snapshot has no format_version")

// readers decodes each supported snapshot format. A new format adds an entry
// and keeps the older ones.
var readers = map[int]func([]byte) (*Schema, error){1: decodeV1}

// Load reads a schema snapshot of any supported format version.
func Load(r io.Reader) (*Schema, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("schema: read snapshot: %w", err)
	}
	var hdr struct {
		FormatVersion int `json:"format_version"`
	}
	if err := json.Unmarshal(data, &hdr); err != nil {
		return nil, fmt.Errorf("schema: decode snapshot header: %w", err)
	}
	v := hdr.FormatVersion
	if v == 0 {
		return nil, ErrMissingFormatVersion
	}
	if v > FormatVersion {
		return nil, fmt.Errorf("%w: format_version %d; this momo reads format_version %d and earlier, upgrade momo to read it", ErrNewerFormat, v, FormatVersion)
	}
	read, ok := readers[v]
	if !ok {
		return nil, fmt.Errorf("schema: no reader for format_version %d", v)
	}
	return read(data)
}

func decodeV1(data []byte) (*Schema, error) {
	var s Schema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("schema: decode snapshot: %w", err)
	}
	s.Sort()
	return &s, nil
}
