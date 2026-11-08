// SPDX-License-Identifier: Apache-2.0

package momotest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/azrtydxb/momo"
)

// fixtureRow is one row of a fixture file, ready to insert.
type fixtureRow struct {
	table string
	// n is the 1-based position of the row in its table's list.
	n    int
	cols []string
	vals []any
}

// LoadFixtures inserts the rows of a YAML file into q, which is a *momo.DB or a
// *momo.Tx. The file is a mapping of table name to a list of rows, each a
// mapping of column name to value:
//
//	authors:
//	  - id: 1
//	    name: Ada
//	documents:
//	  - id: 1
//	    author_id: 1
//	    title: First
//	  - id: 2
//	    author_id: null
//	    title: Orphan
//
// Tables are inserted in file order and the rows of a table in list order, so a
// foreign key is satisfied by listing the parent table first. Columns of a row
// are written sorted by name. A YAML null becomes SQL NULL. Table and column
// names are quoted with momo.Ident and values are bound, never spliced into the
// SQL. Each row is its own statement; wrap the call in a transaction (pass the
// *momo.Tx of NewTx, or a Tx opened with db.Tx) when a failed load must leave
// nothing behind. A file that is not of this shape fails the test with a
// message that names the file and the table.
func LoadFixtures(tb testing.TB, q momo.Querier, path string) {
	tb.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- the path is the fixture file the test names
	if err != nil {
		tb.Fatalf("momotest.LoadFixtures: %v", err)
		return
	}
	rows, err := parseFixtures(raw)
	if err != nil {
		tb.Fatalf("momotest.LoadFixtures %s: %v", path, err)
		return
	}
	for _, r := range rows {
		sql, args := insertStatement(r)
		if _, err := momo.Exec(context.Background(), q, sql, args...); err != nil {
			tb.Fatalf("momotest.LoadFixtures %s: table %s row %d: %v", path, r.table, r.n, err)
			return
		}
	}
}

// parseFixtures reads the YAML of a fixture file into rows, in file order.
func parseFixtures(raw []byte) ([]fixtureRow, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("top level must be a mapping of table name to rows")
	}
	top := doc.Content[0]
	var out []fixtureRow
	for i := 0; i+1 < len(top.Content); i += 2 {
		table, rows := top.Content[i].Value, top.Content[i+1]
		if rows.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("table %s must be a list of rows", table)
		}
		for n, rn := range rows.Content {
			var row map[string]any
			if err := rn.Decode(&row); err != nil {
				return nil, fmt.Errorf("table %s row %d: %w", table, n+1, err)
			}
			if len(row) == 0 {
				return nil, fmt.Errorf("table %s row %d: a row needs at least one column", table, n+1)
			}
			cols := make([]string, 0, len(row))
			for c := range row {
				cols = append(cols, c)
			}
			sort.Strings(cols)
			vals := make([]any, len(cols))
			for j, c := range cols {
				vals[j] = row[c]
			}
			out = append(out, fixtureRow{table: table, n: n + 1, cols: cols, vals: vals})
		}
	}
	return out, nil
}

// insertStatement renders the INSERT of one row as raw SQL with identifier and
// value arguments, in the order the placeholders appear.
func insertStatement(r fixtureRow) (string, []any) {
	args := make([]any, 0, 1+2*len(r.cols))
	args = append(args, momo.Ident(r.table))
	for _, c := range r.cols {
		args = append(args, momo.Ident(c))
	}
	args = append(args, r.vals...)
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(r.cols)), ", ")
	return "INSERT INTO ? (" + ph + ") VALUES (" + ph + ")", args
}
