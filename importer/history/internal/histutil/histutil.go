// SPDX-License-Identifier: Apache-2.0

// Package histutil holds the small helpers the tool-specific history readers
// share.
package histutil

import (
	"errors"
	"fmt"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// QuoteTable quotes a table name read from the caller for use in a SELECT on
// PostgreSQL. A dot separates a schema from the table; every part is quoted.
// It refuses an empty name or an empty part.
func QuoteTable(name string) (string, error) {
	if name == "" {
		return "", errors.New("history: the source history table name is empty")
	}
	d := dialect.For(dialect.Postgres)
	parts := strings.Split(name, ".")
	for i, p := range parts {
		if p == "" {
			return "", fmt.Errorf("history: %q is not a table name", name)
		}
		parts[i] = d.QuoteIdent(p)
	}
	return strings.Join(parts, "."), nil
}

// CheckDuplicates fails when two version strings (digits only) have the same
// numeric value, such as "01" and "1": the source tool reads them as one
// version, so the import cannot tell which file is meant.
func CheckDuplicates(versions []string) error {
	seen := map[string]string{}
	for _, v := range versions {
		k := strings.TrimLeft(v, "0")
		if k == "" {
			k = "0"
		}
		if prev, ok := seen[k]; ok {
			return fmt.Errorf("history: versions %q and %q are the same number; the source tool treats them as one version", prev, v)
		}
		seen[k] = v
	}
	return nil
}

// LessNumeric orders two digit strings by their numeric value without
// converting them (shorter after trimming leading zeros sorts first).
func LessNumeric(a, b string) bool {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}
