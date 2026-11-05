// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestClassifyUniqueRedactsDetail(t *testing.T) {
	secret := strings.Join([]string{"secret", "example.com"}, "@")
	pe := &pgconn.PgError{
		Code:           "23505",
		Message:        `duplicate key value violates unique constraint "users_email_key"`,
		Detail:         "Key (email)=(" + secret + ") already exists.",
		TableName:      "users",
		ColumnName:     "email",
		ConstraintName: "users_email_key",
	}
	got := ClassifyError(pe)
	if !errors.Is(got, momo.ErrUniqueViolation) {
		t.Fatalf("not classified as unique violation: %v", got)
	}
	for name, s := range map[string]string{
		"Error()": got.Error(), "%v": fmt.Sprintf("%v", got), "%+v": fmt.Sprintf("%+v", got),
	} {
		if strings.Contains(s, secret) {
			t.Fatalf("%s leaked the offending value: %q", name, s)
		}
	}
	var me *momo.Error
	if !errors.As(got, &me) || me.Table != "users" || me.Constraint != "users_email_key" || me.Column != "email" {
		t.Fatalf("structured fields missing: %+v", me)
	}
	if strings.Contains(me.Detail(), secret) {
		t.Fatalf("momo.Error.Detail leaked the value: %q", me.Detail())
	}
	var back *pgconn.PgError
	if !errors.As(got, &back) || back.Detail == "" {
		t.Fatalf("Detail not reachable through errors.As")
	}
}

func TestClassifySerialization(t *testing.T) {
	if got := ClassifyError(&pgconn.PgError{Code: "40P01"}); !errors.Is(got, momo.ErrSerialization) {
		t.Fatalf("deadlock not ErrSerialization: %v", got)
	}
}

func TestClassifyMapping(t *testing.T) {
	cases := map[string]error{
		"23505": momo.ErrUniqueViolation,
		"23503": momo.ErrForeignKeyViolation,
		"23514": momo.ErrCheckViolation,
		"23502": momo.ErrNotNullViolation,
		"40001": momo.ErrSerialization,
		"40P01": momo.ErrSerialization,
		"55P03": momo.ErrBusy,
	}
	for code, kind := range cases {
		wrapped := fmt.Errorf("exec: %w", &pgconn.PgError{Code: code, Message: "m"})
		if got := ClassifyError(wrapped); !errors.Is(got, kind) {
			t.Errorf("%s: want %v, got %v", code, kind, got)
		}
	}
}

func TestClassifyPassthrough(t *testing.T) {
	plain := errors.New("boom")
	if got := ClassifyError(plain); got != plain {
		t.Fatalf("plain error changed: %v", got)
	}
	other := &pgconn.PgError{Code: "42P01", Message: "no table"}
	if got := ClassifyError(other); got != error(other) {
		t.Fatalf("unmapped code changed: %v", got)
	}
	if got := ClassifyError(nil); got != nil {
		t.Fatalf("nil became %v", got)
	}
	already := &momo.Error{Kind: momo.ErrBusy}
	if got := ClassifyError(already); got != error(already) {
		t.Fatalf("momo.Error rewrapped: %v", got)
	}
}
