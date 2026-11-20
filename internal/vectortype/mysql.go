// SPDX-License-Identifier: Apache-2.0

// Package vectortype renders the column type a vector column gets on a
// dialect that has no declared dimension in its native type: the VECTOR(N) of
// MySQL and MariaDB, and the length-checked BLOB of SQLite. Migration code and
// the integration tests use it; it is not part of the public API.
package vectortype

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Minimum server versions with a VECTOR type.
const (
	mysqlVectorMajor   = 9
	mysqlVectorMinor   = 0
	mariaDBVectorMajor = 11
	mariaDBVectorMinor = 7
)

// MySQL returns the column type for a vector of dim elements on a MySQL or
// MariaDB server: "VECTOR(N)". serverVersion is the result of SELECT
// VERSION(). MySQL has the type from 9.0 and MariaDB from 11.7; an older
// server, a dimensionless column and an unreadable version fail with an error
// that wraps momo.ErrUnsupported and, for a version problem, names the
// version. Migration code calls this when it renders a vector column.
func MySQL(flavor dialect.Flavor, serverVersion string, dim int) (string, error) {
	unsupported := func(format string, args ...any) error {
		return &momo.Error{Kind: momo.ErrUnsupported, Op: "migrate", Err: fmt.Errorf(format, args...)}
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
