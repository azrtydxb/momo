// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"context"
	"regexp"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

var parserRE = regexp.MustCompile(`WITH PARSER ` + "`" + `([A-Za-z_][A-Za-z0-9_]*)` + "`")

// fullTextParsers fills FullText.Parser of every full-text index from the
// table's SHOW CREATE TABLE, the only place MySQL reports it (as the version
// comment /*!50100 WITH PARSER `ngram` */ after the key). MariaDB InnoDB has no
// parser plugin, so nothing is read there. Only tables that have a full-text
// index are asked.
func (in *inspector) fullTextParsers(ctx context.Context, out *schema.Schema) error {
	if in.flavor == dialect.FlavorMariaDB {
		return nil
	}
	for i := range out.Tables {
		t := &out.Tables[i]
		if _, ok := t.FullTextIndex(); !ok {
			continue
		}
		var ddl string
		err := forEach(ctx, in.c, "SHOW CREATE TABLE "+quoteIdent(t.Name), nil, func(r momo.Rows) error {
			var name string
			return r.Scan(&name, &ddl)
		})
		if err != nil {
			return err
		}
		for j := range t.Indexes {
			ix := &t.Indexes[j]
			if ix.FullText == nil {
				continue
			}
			ix.FullText.Parser = keyParser(ddl, ix.Name)
		}
	}
	return nil
}

// keyParser returns the parser named on the FULLTEXT KEY line of index name in
// a SHOW CREATE TABLE text, or "" for the default parser.
func keyParser(ddl, name string) string {
	prefix := "FULLTEXT KEY " + quoteIdent(name)
	for _, line := range strings.Split(ddl, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		if m := parserRE.FindStringSubmatch(line); m != nil {
			return m[1]
		}
		return ""
	}
	return ""
}
