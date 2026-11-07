// SPDX-License-Identifier: Apache-2.0

package dialect

// mysqlBase holds the capabilities shared by both flavors. RETURNING,
// ON CONFLICT, NULLS FIRST/LAST, ILIKE, MERGE and transactional DDL are
// absent: NULL ordering is emulated with an IS NULL sort key and ILIKE with
// LOWER().
var mysqlBase = Dialect{
	Name: MySQL, MaxParams: 65535,
	OnDuplicateKey: true, Locking: true, DefaultInUpdate: true, DefaultInValues: true,
	RecursiveCTE: true, RowValues: true, Savepoints: true, IsolationLevels: true,
	WindowFunctions: true,
}

// MySQLFlavor returns the MySQL-family dialect for the given flavor, or nil
// for an unknown flavor. Every call returns a fresh value.
//
// MySQL Community has VECTOR storage from 9.0 but no distance function, so it
// lists no metrics and no index. MariaDB 11.8 has VEC_DISTANCE_EUCLIDEAN and
// VEC_DISTANCE_COSINE and VECTOR INDEX, and supports INSERT ... RETURNING but
// not UPDATE or DELETE ... RETURNING. A server-version gate in migrate/mysql
// rejects VECTOR on MySQL 8.4.
func MySQLFlavor(f Flavor) *Dialect {
	d := mysqlBase
	d.Flavor = f
	switch f {
	case FlavorMySQL:
		d.Vector = VectorCaps{Storage: true}
	case FlavorMariaDB:
		d.InsertReturning = true
		d.Vector = VectorCaps{
			Storage: true,
			Metrics: []Metric{MetricL2, MetricCosine},
			Indexes: []string{"vector"},
		}
	default:
		return nil
	}
	return &d
}
