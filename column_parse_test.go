// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

type parseID int64

func renderParsed(t *testing.T, p momo.Predicate) (string, []any) {
	t.Helper()
	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	p.AppendSQL(b)
	if err := b.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String(), b.Args()
}

// Catches: a parsed filter value bound with the wrong Go type, or an operator
// mapped to the wrong comparison.
func TestParseCmpBindsTypedValue(t *testing.T) {
	id := momo.NewColumn[parseID]("t", "id", 0)
	p, err := id.ParseCmp("ge", "42")
	if err != nil {
		t.Fatal(err)
	}
	sql, args := renderParsed(t, p)
	if !strings.Contains(sql, ">=") || len(args) != 1 || args[0] != parseID(42) {
		t.Fatalf("sql=%s args=%#v", sql, args)
	}

	at := momo.NewColumn[time.Time]("t", "at", 0)
	p, err = at.ParseEq("2026-01-02T03:04:05Z")
	if err != nil {
		t.Fatal(err)
	}
	if _, args = renderParsed(t, p); len(args) != 1 {
		t.Fatalf("args=%#v", args)
	}
	if got, ok := args[0].(time.Time); !ok || !got.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("time arg = %#v", args[0])
	}

	ok := momo.NewColumn[bool]("t", "ok", 0)
	if p, err = ok.ParseCmp("ne", "true"); err != nil {
		t.Fatal(err)
	}
	if _, args = renderParsed(t, p); args[0] != true {
		t.Fatalf("bool arg = %#v", args[0])
	}
}

// Catches: a value that does not parse, an unknown operator or a vector
// column being accepted instead of failing with ErrValidation.
func TestParseCmpRejects(t *testing.T) {
	n := momo.NewColumn[int32]("t", "n", 0)
	for _, raw := range []string{"abc", "", "99999999999"} {
		if _, err := n.ParseEq(raw); !errors.Is(err, momo.ErrValidation) {
			t.Errorf("ParseEq(%q) = %v, want ErrValidation", raw, err)
		}
	}
	if _, err := n.ParseCmp("like", "1"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("unknown op: %v", err)
	}
	if _, err := n.ParseEq("secret-value"); err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Errorf("error must not echo the raw value: %v", err)
	}
	emb := momo.NewVectorColumn[vector.Vector]("t", "e", 3, 0)
	if _, err := emb.ParseEq("[1,2,3]"); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("vector: %v", err)
	}
	var _ momo.ColumnRef = momo.NewReadOnlyColumn[int64]("t", "id", 0)
}
