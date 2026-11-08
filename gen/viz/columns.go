// SPDX-License-Identifier: Apache-2.0

package viz

import "github.com/azrtydxb/momo/schema"

// keyMarks returns the PK, FK and UK markers of each column of t, keyed by
// column name. A column is UK when it is the only column of a unique
// constraint or of a unique index.
func keyMarks(t schema.Table) map[string][]string {
	pk := set(t.PrimaryKey)
	fk := map[string]bool{}
	for _, f := range t.ForeignKeys {
		for _, c := range f.Columns {
			fk[c] = true
		}
	}
	uk := map[string]bool{}
	for _, u := range t.Uniques {
		if len(u.Columns) == 1 {
			uk[u.Columns[0]] = true
		}
	}
	for _, ix := range t.Indexes {
		if ix.Unique && len(ix.Parts) == 1 && ix.Parts[0].Column != "" && ix.Where == "" {
			uk[ix.Parts[0].Column] = true
		}
	}
	marks := map[string][]string{}
	for _, c := range t.Columns {
		var m []string
		if pk[c.Name] {
			m = append(m, "PK")
		}
		if fk[c.Name] {
			m = append(m, "FK")
		}
		if uk[c.Name] {
			m = append(m, "UK")
		}
		marks[c.Name] = m
	}
	return marks
}

func set(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
