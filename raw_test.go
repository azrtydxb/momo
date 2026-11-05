// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
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
