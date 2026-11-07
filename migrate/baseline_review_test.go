// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"context"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// privDialect is the PostgreSQL dialect whose Canonicalize reports the missing
// database privilege, as it does for a role without CREATE on the database.
type baseDialect = migrate.Dialect

type privDialect struct{ baseDialect }

func (privDialect) Canonicalize(context.Context, momo.PinnedConn, *schema.Schema) error {
	return fmt.Errorf("%w: needs CREATE on the database", migrate.ErrCanonicalizePrivilege)
}
