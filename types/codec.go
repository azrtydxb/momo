// SPDX-License-Identifier: Apache-2.0

// Package types holds the custom column types Momo understands: each is a
// Codec, so database/sql can scan it, drivers can bind it and the schema
// layer can name its SQL type per dialect.
package types

import (
	"database/sql"
	"database/sql/driver"
	"fmt"

	"github.com/azrtydxb/momo/dialect"
)

// Codec is the custom-type contract: database/sql scanning, driver value
// encoding and the SQL type per dialect.
type Codec interface {
	sql.Scanner
	driver.Valuer
	SQLType(d dialect.Name) string
}

// text returns the textual form of a driver source value. A nil source and
// any type other than string or []byte are errors: the codecs here model
// non-null columns, so a NULL must not silently become a zero value.
func text(codec string, src any) (string, error) {
	switch v := src.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case nil:
		return "", fmt.Errorf("types: cannot scan NULL into %s", codec)
	default:
		return "", fmt.Errorf("types: cannot scan %T into %s", src, codec)
	}
}
