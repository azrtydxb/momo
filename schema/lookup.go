// SPDX-License-Identifier: Apache-2.0

package schema

// Index returns the index with the given name.
func (t *Table) Index(name string) (*Index, bool) {
	for i := range t.Indexes {
		if t.Indexes[i].Name == name {
			return &t.Indexes[i], true
		}
	}
	return nil, false
}
