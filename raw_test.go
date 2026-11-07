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

func TestRawPlaceholders(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Select[unitDocument](pg).Where(momo.SQL("tenant_id = ? AND title ?? 'k'", int64(3))).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `tenant_id = $1 AND title ? 'k'`) {
		t.Fatalf("raw rendering wrong: %s", c.SQL)
	}
	if len(c.Args) != 1 || c.Args[0] != int64(3) {
		t.Fatalf("raw args wrong: %#v", c.Args)
	}

	if _, err := momo.Select[unitDocument](pg).Where(momo.SQL("a = ? AND b = ?", 1)).SQL(); err == nil {
		t.Fatal("placeholder/argument mismatch must fail to build")
	}
	if _, err := momo.Select[unitDocument](pg).Where(momo.SQL("a = ?", 1, 2)).SQL(); err == nil {
		t.Fatal("more arguments than placeholders must fail to build")
	}
}

func TestRawDollarPlaceholdersGolden(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	sq := momo.New(fakedriver.New(dialect.SQLite))

	c, err := momo.Select[unitDocument](pg).
		Where(DocTitle.Eq("t")).
		Where(momo.SQL("tenant_id = $2 OR tenant_id = $1 OR parent_id = $2", int64(1), int64(2))).
		SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `tenant_id = $2 OR tenant_id = $3 OR parent_id = $2`) {
		t.Fatalf("renumbering wrong: %s", c.SQL)
	}
	if len(c.Args) != 3 || c.Args[0] != "t" || c.Args[1] != int64(2) || c.Args[2] != int64(1) {
		t.Fatalf("args wrong: %#v", c.Args)
	}

	// A TypedSQL fragment holding a RawSubquery, after WHERE binds.
	sub := momo.RawSubquery("SELECT max(v) FROM x WHERE a = $1 AND b = $1", int64(5))
	c, err = momo.Select[unitDocument](pg).
		Where(DocTitle.Eq("t")).
		Where(momo.TypedSQL[bool]("tenant_id > ? AND tenant_id < $1", sub)).
		SQL()
	if err == nil {
		t.Fatalf("mixed ? and $n in one fragment must fail, got %s", c.SQL)
	}
	c, err = momo.Select[unitDocument](pg).
		Where(DocTitle.Eq("t")).
		Where(momo.TypedSQL[bool]("tenant_id > $2 AND tenant_id < $1", int64(9), sub)).
		SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `tenant_id > (SELECT max(v) FROM x WHERE a = $2 AND b = $2) AND tenant_id < $3`) {
		t.Fatalf("subquery renumbering wrong: %s", c.SQL)
	}

	if _, err := momo.Select[unitDocument](sq).Where(momo.SQL("tenant_id = $1", int64(1))).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("$n on SQLite: err = %v, want ErrUnsupported", err)
	}
}
