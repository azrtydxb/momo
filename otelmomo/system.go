// SPDX-License-Identifier: Apache-2.0

package otelmomo

import (
	"go.opentelemetry.io/otel/semconv/v1.43.0/dbconv"

	"github.com/azrtydxb/momo/dialect"
)

// systemName maps a Momo dialect to the db.system.name value.
func systemName(d *dialect.Dialect) dbconv.SystemNameAttr {
	if d == nil {
		return dbconv.SystemNameOtherSQL
	}
	switch d.Name {
	case dialect.Postgres:
		return dbconv.SystemNamePostgreSQL
	case dialect.SQLite:
		return dbconv.SystemNameSQLite
	case dialect.MySQL:
		if d.Flavor == dialect.FlavorMariaDB {
			return dbconv.SystemNameMariaDB
		}
		return dbconv.SystemNameMySQL
	}
	return dbconv.SystemNameOtherSQL
}
