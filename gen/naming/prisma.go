// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"sort"
	"unicode"
)

// Prisma reproduces the identifiers a Prisma schema creates on PostgreSQL:
// the table is the model name as written (a quoted PascalCase identifier such
// as "KbChunk"), and the column is the field name as written, which by
// convention is camelCase ("createdAt", "accountId").
//
// A Go field is turned into that camelCase name by lowering its leading
// upper-case run (CreatedAt becomes createdAt, ID becomes id, HTTPServer
// becomes httpServer) and by spelling a trailing or interior ID word as Id
// (AccountID becomes accountId). A column that does not follow the rule takes
// a momo column tag.
//
// Prisma's constraint and index names are <Table>_pkey, <Table>_<col>_fkey,
// <Table>_<col>_key (unique) and <Table>_<col>_idx. They are the names momo
// already derives from the table and column names, so this preset needs no
// extra hook: with the Prisma table and column names they come out right.
//
// JoinTableName follows Prisma's implicit many-to-many table, "_AToB" with
// the two model names in alphabetical order. The A and B columns and the
// foreign key names of that table are not reproduced; declare such a table
// with an explicit join tag and model.
type Prisma struct{}

// TableName returns the model name unchanged.
func (Prisma) TableName(goModel string) string { return goModel }

// ColumnName returns the camelCase column name.
func (Prisma) ColumnName(goField string) string { return prismaCamel(goField) }

// JoinTableName returns Prisma's implicit relation table name.
func (Prisma) JoinTableName(owner, target string) string {
	names := []string{owner, target}
	sort.Strings(names)
	return "_" + names[0] + "To" + names[1]
}

func prismaCamel(s string) string {
	rs := []rune(s)
	n := 0
	for n < len(rs) && unicode.IsUpper(rs[n]) {
		n++
	}
	if n > 1 && n < len(rs) && unicode.IsLower(rs[n]) {
		n-- // keep the first letter of the next word: HTTPServer
	}
	for i := 0; i < n; i++ {
		rs[i] = unicode.ToLower(rs[i])
	}
	// An ID word after a lower-case letter or digit is spelled Id.
	for i := 1; i+2 <= len(rs); i++ {
		if rs[i] != 'I' || rs[i+1] != 'D' {
			continue
		}
		prev := rs[i-1]
		if !(unicode.IsLower(prev) || unicode.IsDigit(prev)) {
			continue
		}
		if i+2 == len(rs) || unicode.IsUpper(rs[i+2]) {
			rs[i+1] = 'd'
		}
	}
	return string(rs)
}
