// SPDX-License-Identifier: Apache-2.0

package mysql

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/schema"
)

// Catches: a parser declared for MariaDB passing validation and failing only
// at apply time, and the MySQL flavor refusing a parser it supports.
func TestFullTextMariaDBFlavorRefusesParser(t *testing.T) {
	s := ftTable("text", "", false, "ngram")
	d, ok := Dialect{}.WithFlavor(dialect.FlavorMariaDB).(migrate.SchemaValidator)
	if !ok {
		t.Fatal("the flavored dialect lost ValidateSchema")
	}
	if err := d.ValidateSchema(s); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("MariaDB: err = %v, want ErrUnsupported", err)
	}
	my := Dialect{}.WithFlavor(dialect.FlavorMySQL).(migrate.SchemaValidator)
	if err := my.ValidateSchema(s); err != nil {
		t.Fatalf("MySQL: %v", err)
	}
	if err := d.ValidateSchema(ftTable("text", "", false, "")); err != nil {
		t.Fatalf("MariaDB without a parser: %v", err)
	}
}

// Catches: the unflavored dialect (offline engine without a driver) silently
// dropping the parser instead of rendering it as declared.
func TestFullTextZeroFlavorRendersParser(t *testing.T) {
	if err := (Dialect{}).ValidateSchema(ftTable("text", "", false, "ngram")); err != nil {
		t.Fatalf("zero flavor refused a parser: %v", err)
	}
	ix := ftTable("text", "", false, "ngram").Tables[0].Indexes[0]
	st, err := createIndex("", "articles", ix)
	if err != nil || !strings.HasSuffix(st.SQL, "WITH PARSER ngram") {
		t.Fatalf("SQL = %q, err = %v", st.SQL, err)
	}
}

// Catches: more than 16 columns reaching the server, which rejects it with a
// raw driver error.
func TestFullTextMySQLTooManyColumns(t *testing.T) {
	s := ftTable("text", "", false, "")
	tb := &s.Tables[0]
	var names []string
	for i := range 17 {
		n := fmt.Sprintf("c%d", i)
		names = append(names, n)
		tb.Columns = append(tb.Columns, schema.Column{Name: n, Type: "text"})
	}
	tb.Indexes[0].FullText = &schema.FullText{Columns: names}
	err := (Dialect{}).ValidateSchema(s)
	if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "17 columns") {
		t.Fatalf("err = %v, want ErrValidation naming 17 columns", err)
	}
}

// Catches: a parser name outside the allowlist written into DDL by createIndex
// when the schema never went through ValidateSchema.
func TestCreateIndexRejectsUnlistedParser(t *testing.T) {
	ix := schema.Index{Name: "articles_search", Method: "fulltext",
		Parts:    []schema.IndexPart{{Column: "title"}},
		FullText: &schema.FullText{Columns: []string{"title"}, Parser: "ngram; DROP TABLE x"}}
	st, err := createIndex("", "articles", ix)
	if !errors.Is(err, momo.ErrValidation) || st.SQL != "" {
		t.Fatalf("SQL = %q, err = %v, want ErrValidation and no SQL", st.SQL, err)
	}
}

// Catches: a full-text declaration accepted on a table that the database holds
// as MyISAM, and an InnoDB table or a table without a declaration refused.
func TestValidateAgainstLive(t *testing.T) {
	live := func(engine string) *schema.Schema {
		return &schema.Schema{Dialect: "mysql", Tables: []schema.Table{{Name: "articles", Engine: engine}}}
	}
	desired := ftTable("text", "", false, "")
	err := (Dialect{}).ValidateAgainstLive(desired, live("MyISAM"))
	if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "articles") {
		t.Fatalf("err = %v, want ErrValidation naming articles", err)
	}
	if err := (Dialect{}).ValidateAgainstLive(desired, live("")); err != nil {
		t.Fatalf("InnoDB table refused: %v", err)
	}
	plain := ftTable("text", "", false, "")
	plain.Tables[0].Indexes = nil
	if err := (Dialect{}).ValidateAgainstLive(plain, live("MyISAM")); err != nil {
		t.Fatalf("a table without a declaration refused: %v", err)
	}
}
