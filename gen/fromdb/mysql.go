// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/internal/dbdefault"
	"github.com/azrtydxb/momo/schema"
)

// mapMySQL maps a normalized MySQL or MariaDB column type to a Go type. enum
// and set are Go strings: the exact column type is written in the type tag, so
// the values stay in the database and the round trip is exact. A type it does
// not know is not mapped; there is no fallback.
func mapMySQL(s string) (GoType, bool) {
	base, args, rest := splitType(s)
	fields := append(strings.Fields(base), strings.Fields(rest)...)
	if len(fields) == 0 {
		return GoType{}, false
	}
	name, unsigned := fields[0], false
	for _, w := range fields[1:] {
		switch w {
		case "unsigned":
			unsigned = true
		case "zerofill":
		default:
			return GoType{}, false
		}
	}
	ints := map[string]string{"tinyint": "int8", "smallint": "int16", "mediumint": "int32", "int": "int32", "bigint": "int64"}
	if g, ok := ints[name]; ok {
		if name == "tinyint" && args == "1" && !unsigned {
			return GoType{Expr: "bool", Note: "tinyint(1) is a Go bool: a stored value other than 0 or 1 fails to scan the whole row; add an introspect.types rule for tinyint(1) with int8 if the column holds other values"}, true
		}
		if unsigned {
			g = "u" + g
		}
		return GoType{Expr: g}, true
	}
	switch name {
	case "float":
		return GoType{Expr: "float32"}, true
	case "double":
		return GoType{Expr: "float64"}, true
	case "decimal":
		return GoType{Expr: "types.Numeric", Import: importTypes}, true
	case "varchar":
		g := GoType{Expr: "string"}
		if args != "" {
			n, err := strconv.Atoi(args)
			if err != nil || n <= 0 {
				return GoType{}, false
			}
			g.Size = n
		}
		return g, true
	case "char", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
		return GoType{Expr: "string"}, true
	case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob", "bit":
		return GoType{Expr: "[]byte", Nilable: true}, true
	case "date":
		return GoType{Expr: "types.Date", Import: importTypes}, true
	case "datetime", "timestamp":
		return GoType{Expr: "time.Time", Import: "time"}, true
	case "time":
		return GoType{Expr: "string", Note: "time column is a Go string: the MySQL driver returns TIME as text"}, true
	case "year":
		return GoType{Expr: "int16"}, true
	case "json":
		return GoType{Expr: "types.JSON[json.RawMessage]", Import: "encoding/json"}, true
	}
	return GoType{}, false
}

// typeTagSafe reports whether v can be the value of a type tag: no quote,
// backslash or control character, and the tag grammar reads it as one option
// (commas only inside parentheses).
func typeTagSafe(v string) bool {
	if v == "" || !utf8.ValidString(v) || v != strings.TrimSpace(v) {
		return false
	}
	for _, r := range v {
		if r == '"' || r == '`' || r == '\\' || r < 0x20 || r == 0x7f {
			return false
		}
	}
	return len(tags.SplitOptions("type:"+v)) == 1
}

// mysqlTypeTags spells the column type of a MySQL or MariaDB column. The
// generator has no MySQL type table, so the exact column type goes in a type
// tag; a varchar uses size, which the generator turns into varchar(N).
func (g *generator) mysqlTypeTags(col *schema.Column, gt GoType) ([]string, error) {
	if gt.Size > 0 && gt.Expr == "string" {
		return []string{"size:" + strconv.Itoa(gt.Size)}, nil
	}
	if !typeTagSafe(col.Type) {
		return nil, fmt.Errorf("the column type %q cannot be written in a tag; add an introspect.types rule", col.Type)
	}
	return []string{"type:" + col.Type}, nil
}

// dbDefaultFree reports whether no other tag already decides how the field of
// col is written, so dbdefault may be added.
func dbDefaultFree(col *schema.Column, opts []string) bool {
	if col.Nullable || col.Identity != "" || strings.TrimSpace(col.Default) == "" {
		return false
	}
	for _, o := range opts {
		switch k, _, _ := strings.Cut(o, ":"); k {
		case "identity", "serial", "read_only", "version", "soft_delete", "created_at", "updated_at", "idgen", "dbdefault":
			return false
		}
	}
	return true
}

// mysqlNonZeroDefault reports whether a MySQL or MariaDB field has a NOT NULL
// bool or number column with a non-zero constant default, which a Go zero
// value would overwrite.
func mysqlNonZeroDefault(d dialect.Name, col *schema.Column, gt GoType, opts []string) bool {
	return d == dialect.MySQL && dbdefault.NonZeroConst(gt.Expr, col.Default) && dbDefaultFree(col, opts)
}

// mysqlDBDefault reports whether the field gets dbdefault and a pointer type:
// only on MariaDB, which has INSERT ... RETURNING to read the stored value
// back. MySQL 8.4 and 9.7 refuse an insert that leaves a dbdefault field
// unset, so there the field stays plain and the report says so.
func mysqlDBDefault(d dialect.Name, fl dialect.Flavor, col *schema.Column, gt GoType, opts []string) bool {
	return fl == dialect.FlavorMariaDB && mysqlNonZeroDefault(d, col, gt, opts)
}
