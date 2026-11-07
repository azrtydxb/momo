// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
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
