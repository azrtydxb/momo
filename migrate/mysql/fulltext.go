// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"errors"
	"fmt"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// maxFullTextColumns is the most columns one FULLTEXT index can hold.
const maxFullTextColumns = 16

// ExpandSchema returns a copy of s in which every full-text declaration keeps
// only the options MySQL and MariaDB honour. A FULLTEXT index derives no other
// object, so nothing is added. It is pure and idempotent.
func (Dialect) ExpandSchema(s *schema.Schema) (*schema.Schema, error) {
	out := *s
	out.Tables = make([]schema.Table, len(s.Tables))
	for i, t := range s.Tables {
		t.Indexes = append([]schema.Index(nil), t.Indexes...)
		for j := range t.Indexes {
			ix := &t.Indexes[j]
			if ft := ix.FullText; ft != nil {
				c := ft.ForDialect(dialect.MySQL)
				ix.FullText = &c
			} else if strings.EqualFold(ix.Method, "fulltext") {
				// A FULLTEXT index declared before the description existed (a
				// c.Index(...).Method("fulltext")): inspection reports its columns,
				// so the declaration carries them too.
				if cols := plainColumns(ix.Parts); len(cols) == len(ix.Parts) && len(cols) > 0 {
					ix.FullText = &schema.FullText{Columns: cols}
				}
			}
		}
		out.Tables[i] = t
	}
	return &out, nil
}

// plainColumns lists the column of every part that is a plain column.
func plainColumns(parts []schema.IndexPart) []string {
	var cols []string
	for _, p := range parts {
		if p.Column != "" && p.Expr == "" {
			cols = append(cols, p.Column)
		}
	}
	return cols
}

func ftErr(kind error, format string, args ...any) error {
	return &momo.Error{Kind: kind, Op: "migrate/mysql", Err: fmt.Errorf(format, args...)}
}

// isTextType reports whether a MySQL column type holds text a FULLTEXT index
// can cover.
func isTextType(typ string) bool {
	base := schema.NormalizeType(dialect.MySQL, typ)
	if i := strings.IndexAny(base, "( "); i >= 0 {
		base = base[:i]
	}
	switch base {
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext":
		return true
	}
	return false
}

// ValidateAgainstLive refuses a full-text declaration on a table that exists
// in the database with a storage engine other than InnoDB (ErrValidation naming
// the table). A desired schema never carries an engine, so this is the check
// that can fire on a normal path; AutoMigrate calls it with the inspected
// database.
func (Dialect) ValidateAgainstLive(desired, live *schema.Schema) error {
	var errs []error
	for _, t := range desired.Tables {
		if _, ok := t.FullTextIndex(); !ok {
			continue
		}
		for _, lt := range live.Tables {
			if lt.Schema == t.Schema && lt.Name == t.Name && lt.Engine != "" && !strings.EqualFold(lt.Engine, "InnoDB") {
				errs = append(errs, ftErr(momo.ErrValidation, "table %s: a full-text index needs an InnoDB table, the table in the database uses %s", t.Name, lt.Engine))
			}
		}
	}
	return errors.Join(errs...)
}

// validateFullText checks the full-text indexes of a table: the engine, the
// column types, uniqueness, the parser and the column count.
func (d Dialect) validateFullText(t schema.Table) []error {
	var errs []error
	for _, ix := range t.Indexes {
		if ix.FullText == nil {
			continue
		}
		ft := *ix.FullText
		if t.Engine != "" && !strings.EqualFold(t.Engine, "InnoDB") {
			errs = append(errs, ftErr(momo.ErrValidation, "table %s: the full-text index %q needs an InnoDB table, the table uses %s", t.Name, ix.Name, t.Engine))
		}
		if ix.Unique {
			errs = append(errs, ftErr(momo.ErrValidation, "table %s: the full-text index %q cannot be unique", t.Name, ix.Name))
		}
		if err := ft.Validate(); err != nil {
			errs = append(errs, ftErr(momo.ErrValidation, "table %s: the full-text index %q: %w", t.Name, ix.Name, err))
		}
		if len(ft.Columns) > maxFullTextColumns {
			errs = append(errs, ftErr(momo.ErrValidation, "table %s: the full-text index %q has %d columns, MySQL allows %d", t.Name, ix.Name, len(ft.Columns), maxFullTextColumns))
		}
		if ft.Parser != "" && d.flavor == dialect.FlavorMariaDB {
			errs = append(errs, ftErr(momo.ErrUnsupported, "table %s: the full-text index %q names the parser %q: MariaDB InnoDB has no full-text parser plugin", t.Name, ix.Name, ft.Parser))
		}
		for _, name := range ft.Columns {
			col, ok := t.Column(name)
			if !ok {
				errs = append(errs, ftErr(momo.ErrValidation, "table %s: the full-text index %q names the column %q, which the table does not have", t.Name, ix.Name, name))
				continue
			}
			if !isTextType(col.Type) {
				errs = append(errs, ftErr(momo.ErrValidation, "table %s: column %q of the full-text index %q is %s, a FULLTEXT index needs CHAR, VARCHAR or TEXT", t.Name, name, ix.Name, col.Type))
			}
		}
	}
	return errs
}
