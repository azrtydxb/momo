// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestColumnByName(t *testing.T) {
	al := momo.NewAllowlist(DocTitle, DocTenantID)
	got, err := al.Column("title")
	if err != nil || got.ColumnName() != "title" {
		t.Fatalf("known name: %v %v", got, err)
	}
	if _, err := al.Column("title; DROP TABLE documents"); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("unknown name err = %v, want ErrValidation", err)
	}
	if _, err := al.Column(""); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("empty name err = %v, want ErrValidation", err)
	}
}

func TestNewAllowlistSkipsNil(t *testing.T) {
	al := momo.NewAllowlist(nil, DocTitle, nil)
	if _, err := al.Column("title"); err != nil {
		t.Fatalf("known name after nil entries: %v", err)
	}
	if _, err := al.Column("tenant_id"); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("unknown name err = %v, want ErrValidation", err)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := momo.EscapeLike(`50%_off!`); got != `50!%!_off!!` {
		t.Fatalf("EscapeLike = %q", got)
	}
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := t.Context()
		for _, title := range []string{"50%_off", "50 off", "50!off"} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", title); err != nil {
				t.Fatal(err)
			}
		}
		for _, c := range []struct{ in, want string }{{"%_", "50%_off"}, {"!", "50!off"}} {
			rows, err := momo.Select[suite.Document](db).Where(momo.ContainsLiteral(suite.DocTitle, c.in)).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].Title != c.want {
				t.Fatalf("ContainsLiteral(%q) returned %d rows, want only %q", c.in, len(rows), c.want)
			}
		}
	})
}

func TestContainsLiteralSQLPerDialect(t *testing.T) {
	cases := []struct {
		d    dialect.Name
		want string
	}{
		{dialect.Postgres, `"documents"."title" LIKE $1 ESCAPE '!'`},
		{dialect.SQLite, `"documents"."title" LIKE ? ESCAPE '!'`},
		{dialect.MySQL, "`documents`.`title` LIKE ? ESCAPE '!'"},
	}
	for _, c := range cases {
		db := momo.New(fakedriver.New(c.d))
		got, err := momo.Select[unitDocument](db).Where(momo.ContainsLiteral(DocTitle, `a%b`)).SQL()
		if err != nil {
			t.Fatalf("%s: %v", c.d, err)
		}
		if !strings.Contains(got.SQL, c.want) {
			t.Errorf("%s: SQL %q lacks %q", c.d, got.SQL, c.want)
		}
		if len(got.Args) != 1 || got.Args[0] != `%a!%b%` {
			t.Errorf("%s: args = %v", c.d, got.Args)
		}
	}
}

func FuzzEscapeLike(f *testing.F) {
	for _, s := range []string{"", "%", "_", "!", `\`, "50%_off!", "!%", "ünï%"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out := momo.EscapeLike(s)
		// Every wildcard or escape char in the output is introduced by an
		// escape; unescaping must round-trip to the input.
		var back strings.Builder
		for i := 0; i < len(out); i++ {
			switch out[i] {
			case '!':
				i++
				if i >= len(out) || !strings.ContainsRune(`%_!`, rune(out[i])) {
					t.Fatalf("dangling or useless escape in %q (input %q)", out, s)
				}
			case '%', '_':
				t.Fatalf("unescaped wildcard at %d in %q (input %q)", i, out, s)
			}
			back.WriteByte(out[i])
		}
		if back.String() != s {
			t.Fatalf("round trip %q -> %q -> %q", s, out, back.String())
		}
	})
}
