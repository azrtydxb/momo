// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

// mariaDBFunctions maps each metric MariaDB 11.8 provides to its distance
// function. MySQL (any version) has no distance function outside HeatWave, so
// its dialect lists no metric and a distance never reaches this table.
var mariaDBFunctions = map[dialect.Metric]string{
	dialect.MetricL2:     "VEC_DISTANCE_EUCLIDEAN",
	dialect.MetricCosine: "VEC_DISTANCE_COSINE",
}

// mysqlVectorLiteralFunc is the function that turns the bound text form of a
// vector into the server's VECTOR value: VEC_FromText on MariaDB and
// STRING_TO_VECTOR on MySQL.
func mysqlVectorLiteralFunc(f dialect.Flavor) string {
	if f == dialect.FlavorMariaDB {
		return "VEC_FromText"
	}
	return "STRING_TO_VECTOR"
}

// wrapMySQLVectorArg binds arg as the text form of a vector inside the
// server's text-to-vector function. The caller has already set the
// sensitivity of the bind.
func wrapMySQLVectorArg(b *Builder, flavor dialect.Flavor, arg any) {
	b.WriteString(mysqlVectorLiteralFunc(flavor) + "(")
	b.Arg(arg)
	b.WriteString(")")
}

// isMySQLVectorArg reports whether a typed bind of v on d needs the wrapper.
// Only the dense float32 vector has a MySQL or MariaDB type; the other vector
// types fail in DialectValue.
func isMySQLVectorArg(d *dialect.Dialect, v any) bool {
	if d.Name != dialect.MySQL || !d.Vector.Storage {
		return false
	}
	_, ok := v.(vector.Vector)
	return ok
}

// Minimum server versions with a VECTOR type.
const (
	mysqlVectorMajor   = 9
	mysqlVectorMinor   = 0
	mariaDBVectorMajor = 11
	mariaDBVectorMinor = 7
)

// MySQLVectorType returns the column type for a vector of dim elements on a
// MySQL or MariaDB server: "VECTOR(N)". serverVersion is the result of
// SELECT VERSION(). MySQL has the type from 9.0 and MariaDB from 11.7; an
// older server, a dimensionless column and an unreadable version fail with an
// error that wraps ErrUnsupported and, for a version problem, names the
// version. Migration code calls this when it renders a vector column.
func MySQLVectorType(flavor dialect.Flavor, serverVersion string, dim int) (string, error) {
	unsupported := func(format string, args ...any) error {
		return &Error{Kind: ErrUnsupported, Op: "migrate", Err: fmt.Errorf(format, args...)}
	}
	var minMajor, minMinor int
	switch flavor {
	case dialect.FlavorMySQL:
		minMajor, minMinor = mysqlVectorMajor, mysqlVectorMinor
	case dialect.FlavorMariaDB:
		minMajor, minMinor = mariaDBVectorMajor, mariaDBVectorMinor
	default:
		return "", unsupported("VECTOR requires MySQL 9.0 or MariaDB 11.7: unknown flavor %q", flavor)
	}
	major, minor, ok := parseServerVersion(serverVersion)
	if !ok {
		return "", unsupported("VECTOR requires MySQL 9.0 or MariaDB 11.7: cannot read the server version %q", serverVersion)
	}
	if major < minMajor || (major == minMajor && minor < minMinor) {
		return "", unsupported("VECTOR requires MySQL 9.0 or MariaDB 11.7: %s server version %s has no VECTOR type", flavor, serverVersion)
	}
	if dim <= 0 {
		return "", unsupported("VECTOR requires a dimension, got %d", dim)
	}
	return "VECTOR(" + strconv.Itoa(dim) + ")", nil
}

// parseServerVersion reads major.minor from a VERSION() string such as
// "9.7.4", "9.7.4-log", "11.8.9-MariaDB" or "5.5.5-11.8.9-MariaDB-ubu2404".
// The "5.5.5-" prefix is the replication-compatibility version that MariaDB
// puts in front of its own number in the handshake.
func parseServerVersion(v string) (major, minor int, ok bool) {
	v = strings.TrimPrefix(v, "5.5.5-")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	end := 0
	for end < len(parts[1]) && parts[1][end] >= '0' && parts[1][end] <= '9' {
		end++
	}
	minor, err = strconv.Atoi(parts[1][:end])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}
