// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo/schema"
)

// ValidateSchema refuses an index that carries the name of a foreign key of the
// same table. MySQL builds an index with the constraint's name for every key
// that has no usable index, so the declared index would collide with it when
// the table is created, and Inspect cannot tell a declared index from the one
// the server built (it hides the latter). Give the index another name. Diff and
// AutoMigrate call it on the desired schema.
func (Dialect) ValidateSchema(s *schema.Schema) error {
	var errs []error
	for _, t := range s.Tables {
		keys := make(map[string]bool, len(t.ForeignKeys))
		for _, fk := range t.ForeignKeys {
			keys[fk.Name] = true
		}
		for _, ix := range t.Indexes {
			if keys[ix.Name] {
				errs = append(errs, fmt.Errorf("mysql: table %s: the index %q has the name of a foreign key; MySQL builds an index with that name for the key, so the two collide: rename the index", t.Name, ix.Name))
			}
		}
	}
	return errors.Join(errs...)
}
