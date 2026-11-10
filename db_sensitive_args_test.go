// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/sqlfn"
)

const sensitiveValue = "hunter2-sensitive"

// assertMasked checks that no hook event shows the sensitive value, at least
// one shows the placeholder, and the driver received the real value.
func assertMasked(t *testing.T, rec *obstest.Recorder, drv *fakedriver.Driver) {
	t.Helper()
	masked := false
	for _, e := range rec.Events() {
		if e.NumArgs != len(e.Args) {
			t.Errorf("NumArgs %d != len(Args) %d", e.NumArgs, len(e.Args))
		}
		for _, a := range e.Args {
			if a == sensitiveValue {
				t.Fatalf("hook saw the sensitive value in %q", e.SQL)
			}
			if a == momo.RedactedArg {
				masked = true
			}
		}
	}
	if !masked {
		t.Fatal("no hook event carried the redaction placeholder")
	}
	sent := false
	for _, c := range drv.Calls() {
		if slices.Contains(c.Args, any(sensitiveValue)) {
			sent = true
		}
	}
	if !sent {
		t.Fatal("the driver did not receive the real value")
	}
}

func TestHooksSeeMaskedSensitiveInsertArgs(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	_, err := momo.Insert[suite.Document](db).Models([]suite.Document{{TenantID: 1, Title: "t", Secret: sensitiveValue}}).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertMasked(t, rec, drv)
}

func TestHooksSeeMaskedSensitiveUpdateArgs(t *testing.T) {
	rec := &obstest.Recorder{}
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
	_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("visible"), suite.DocSecret.Set(sensitiveValue)).
		Where(suite.DocID.Eq(1)).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assertMasked(t, rec, drv)
	for _, e := range rec.Events() {
		if !slices.Contains(e.Args, any("visible")) {
			t.Errorf("non-sensitive argument was masked: %v", e.Args)
		}
	}
}

// mutatingHook overwrites its arguments; that must not change what the driver gets.
type mutatingHook struct{}

func (mutatingHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	for i := range e.Args {
		e.Args[i] = "tampered"
	}
	return ctx
}
func (mutatingHook) AfterQuery(context.Context, *momo.QueryEvent) {}

func TestHookCannotMutateBoundArgs(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(mutatingHook{}), momo.WithArgLogging())
	_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("keep")).Where(suite.DocID.Eq(1)).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range drv.Calls() {
		if slices.Contains(c.Args, any("tampered")) {
			t.Fatalf("a hook changed the arguments sent to the driver: %v", c.Args)
		}
	}
}

// sensitiveVersion is a version number that must never reach a hook.
const sensitiveVersion int64 = 99887766

func isSecret(a any) bool {
	switch v := a.(type) {
	case string:
		return strings.Contains(v, sensitiveValue)
	case int64:
		return v == sensitiveVersion
	}
	return false
}

// vault is a model whose primary key and version columns are sensitive.
type vault struct {
	Token   string
	Version int64
	Note    string
}

var (
	vaultToken   = momo.NewColumn[string]("vaults", "token", momo.FlagPK|momo.FlagSensitive)
	vaultVersion = momo.NewColumn[int64]("vaults", "version", momo.FlagVersion|momo.FlagSensitive)
	vaultNote    = momo.NewColumn[string]("vaults", "note", 0)
	vaultMeta    = &momo.Meta[vault]{
		Table: "vaults",
		Columns: []momo.ColumnMeta{
			{Name: "token", Flags: vaultToken.Flags()},
			{Name: "version", Flags: vaultVersion.Flags()},
			{Name: "note", Flags: vaultNote.Flags()},
		},
		Ptr: func(m *vault, i int) any {
			switch i {
			case 0:
				return &m.Token
			case 1:
				return &m.Version
			}
			return &m.Note
		},
		Value: func(m *vault, i int) any {
			switch i {
			case 0:
				return m.Token
			case 1:
				return m.Version
			}
			return m.Note
		},
		Identity: -1, Version: 1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		PK: []int{0},
	}
)

func (*vault) MomoMeta() *momo.Meta[vault] { return vaultMeta }

// predicateCaseOn runs one statement on dialect d and checks hook arguments
// position by position: every driver argument holding a secret, and every
// value in also (the other values bound to the same sensitive column), shows
// as RedactedArg in the last hook event; every other argument is unchanged.
// wantSecret is the exact number of secret arguments, or -1 for at least one.
func predicateCaseOn(t *testing.T, d dialect.Name, name string, wantSecret int, also []any, run func(ctx context.Context, db *momo.DB) error) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		rec := &obstest.Recorder{}
		drv := fakedriver.New(d)
		db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
		if err := run(t.Context(), db); err != nil {
			t.Fatal(err)
		}
		calls := drv.Calls()
		evs := rec.Events()
		if len(calls) == 0 || len(evs) == 0 {
			t.Fatalf("calls %d, events %d", len(calls), len(evs))
		}
		// The statement with the most arguments that reached both the driver
		// and a hook (a follow-up query may carry fewer).
		var sent, seen []any
		for i := len(calls) - 1; i >= 0; i-- {
			for j := len(evs) - 1; j >= 0; j-- {
				if evs[j].SQL == calls[i].SQL && (seen == nil || len(calls[i].Args) > len(sent)) {
					sent, seen = calls[i].Args, evs[j].Args
					break
				}
			}
		}
		if seen == nil && len(sent) == 0 {
			t.Fatal("no statement was seen by both the driver and a hook")
		}
		if len(sent) != len(seen) {
			t.Fatalf("driver got %d args, hook saw %d", len(sent), len(seen))
		}
		secrets, extra := 0, 0
		for i := range sent {
			switch {
			case isSecret(sent[i]):
				secrets++
			case slices.Contains(also, sent[i]):
				extra++
			default:
				if seen[i] != sent[i] {
					t.Errorf("arg %d: hook saw %v, driver got %v", i, seen[i], sent[i])
				}
				continue
			}
			if seen[i] != momo.RedactedArg {
				t.Errorf("arg %d: hook saw %v, want the placeholder (driver args %v)", i, seen[i], sent)
			}
		}
		if (wantSecret >= 0 && secrets != wantSecret) || (wantSecret < 0 && secrets == 0) || extra != len(also) {
			t.Errorf("masked %d secrets and %d sibling values, want %d and %d (driver args %v)", secrets, extra, wantSecret, len(also), sent)
		}
	})
}

func predicateCase(t *testing.T, name string, wantSecret int, also []any, run func(ctx context.Context, db *momo.DB) error) {
	t.Helper()
	predicateCaseOn(t, dialect.Postgres, name, wantSecret, also, run)
}

func TestHooksSeeMaskedSensitivePredicateArgs(t *testing.T) {
	sel := func(p momo.Predicate) func(context.Context, *momo.DB) error {
		return func(ctx context.Context, db *momo.DB) error {
			_, err := momo.Select[suite.Document](db).Where(p).All(ctx)
			return err
		}
	}
	predicateCase(t, "eq", 1, nil, sel(momo.And(suite.DocTitle.Eq("visible"), suite.DocSecret.Eq(sensitiveValue))))
	predicateCase(t, "ne", 1, nil, sel(suite.DocSecret.Ne(sensitiveValue)))
	predicateCase(t, "distinct", 2, nil, sel(momo.Or(suite.DocSecret.IsDistinctFrom(sensitiveValue), suite.DocSecret.IsNotDistinctFrom(sensitiveValue))))
	predicateCase(t, "in", 1, []any{"other"}, sel(momo.And(suite.DocSecret.In(sensitiveValue, "other"), suite.DocTitle.In("a", "b"))))
	predicateCase(t, "not in", 1, nil, sel(suite.DocSecret.NotIn(sensitiveValue)))
	predicateCase(t, "between", 1, []any{"lo"}, sel(suite.DocSecret.Between("lo", sensitiveValue)))
	predicateCase(t, "like", 1, nil, sel(momo.Like(suite.DocSecret, "%"+sensitiveValue+"%")))
	predicateCase(t, "ilike", 1, nil, sel(momo.ILike(suite.DocSecret, "%"+sensitiveValue+"%")))
	predicateCase(t, "contains literal", 1, nil, sel(momo.ContainsLiteral(suite.DocSecret, sensitiveValue)))
	predicateCase(t, "eq expr with value", 1, nil, sel(suite.DocSecret.EqExpr(momo.Value(sensitiveValue))))
	predicateCase(t, "ne expr with value", 1, nil, sel(suite.DocSecret.NeExpr(momo.Value(sensitiveValue))))
	predicateCase(t, "gt expr with value", 1, nil, sel(suite.DocSecret.GtExpr(momo.Value(sensitiveValue))))
	predicateCase(t, "expr ops eq", 1, nil, sel(sqlfn.Lower(suite.DocSecret).Eq(sensitiveValue)))
	predicateCase(t, "expr ops in", 1, []any{"x"}, sel(sqlfn.Lower(suite.DocSecret).In(sensitiveValue, "x")))
	predicateCase(t, "expr ops not in", 1, []any{"x"}, sel(sqlfn.Lower(suite.DocSecret).NotIn(sensitiveValue, "x")))
	predicateCase(t, "expr ops between", 1, []any{"lo"}, sel(sqlfn.Lower(suite.DocSecret).Between("lo", sensitiveValue)))
	predicateCase(t, "expr ops on a visible column", 0, nil, sel(sqlfn.Lower(suite.DocTitle).In("a", "b")))
	predicateCase(t, "non-sensitive stays visible", 0, nil, sel(suite.DocTitle.Eq("visible")))
	predicateCase(t, "update where", 1, nil, func(ctx context.Context, db *momo.DB) error {
		_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("visible")).Where(suite.DocSecret.Eq(sensitiveValue)).Exec(ctx)
		return err
	})
	predicateCase(t, "delete where", 1, nil, func(ctx context.Context, db *momo.DB) error {
		_, err := momo.ForceDelete[suite.Document](db).Where(suite.DocSecret.Eq(sensitiveValue)).Exec(ctx)
		return err
	})
	predicateCase(t, "set expr", 1, nil, func(ctx context.Context, db *momo.DB) error {
		_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("visible"), suite.DocSecret.SetExpr(momo.Value(sensitiveValue))).
			Where(suite.DocID.Eq(1)).Exec(ctx)
		return err
	})
	predicateCase(t, "insert set expr", 1, nil, func(ctx context.Context, db *momo.DB) error {
		_, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("visible"), suite.DocSecret.SetExpr(momo.Value(sensitiveValue))).Exec(ctx)
		return err
	})
}

func TestHooksSeeMaskedSensitiveModelKeys(t *testing.T) {
	predicateCase(t, "model update primary key and version", 2, nil, func(ctx context.Context, db *momo.DB) error {
		// The fake driver reports no affected row, which a versioned update
		// treats as a conflict; the statement was still sent.
		_, _ = momo.Update[vault](db).Model(&vault{Token: sensitiveValue, Version: sensitiveVersion, Note: "n"}).Exec(ctx)
		return nil
	})
	predicateCase(t, "model delete primary key", 1, nil, func(ctx context.Context, db *momo.DB) error {
		_, _ = momo.Delete[vault](db).Model(&vault{Token: sensitiveValue, Version: sensitiveVersion}).Exec(ctx)
		return nil
	})
	predicateCase(t, "keyset on a sensitive order key", 2, nil, func(ctx context.Context, db *momo.DB) error {
		_, err := momo.Select[suite.Document](db).OrderBy(suite.DocSecret.Asc()).After(sensitiveValue, int64(5)).All(ctx)
		return err
	})
	predicateCase(t, "keyset on a sensitive primary key tie-breaker", 1, nil, func(ctx context.Context, db *momo.DB) error {
		_, err := momo.Select[vault](db).OrderBy(vaultNote.Asc()).After("n", sensitiveValue).All(ctx)
		return err
	})
	predicateCase(t, "find or create key", -1, nil, func(ctx context.Context, db *momo.DB) error {
		_, _ = momo.FindOrCreate[vault](ctx, db, []momo.ColumnRef{vaultToken}, &vault{Token: sensitiveValue, Version: sensitiveVersion})
		return nil
	})
}

// TestMaskedArgsOnEveryDialect builds the same predicates for SQLite and
// MySQL, which need no database.
func TestMaskedArgsOnEveryDialect(t *testing.T) {
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		predicateCaseOn(t, d, string(d)+" predicates", 4, []any{"other", "lo"}, func(ctx context.Context, db *momo.DB) error {
			_, err := momo.Select[suite.Document](db).Where(momo.And(
				suite.DocSecret.Eq(sensitiveValue),
				suite.DocSecret.In(sensitiveValue, "other"),
				suite.DocSecret.Between("lo", sensitiveValue),
				momo.Like(suite.DocSecret, "%"+sensitiveValue+"%"),
			)).All(ctx)
			return err
		})
		predicateCaseOn(t, d, string(d)+" update set expr", 1, nil, func(ctx context.Context, db *momo.DB) error {
			_, err := momo.Update[suite.Document](db).Set(suite.DocSecret.SetExpr(momo.Value(sensitiveValue))).Where(suite.DocID.Eq(1)).Exec(ctx)
			return err
		})
	}
}

// byteHook overwrites the bytes of any []byte argument in place.
type byteHook struct{}

func (byteHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	for _, a := range e.Args {
		if b, ok := a.([]byte); ok {
			for i := range b {
				b[i] = 'X'
			}
		}
	}
	return ctx
}
func (byteHook) AfterQuery(context.Context, *momo.QueryEvent) {}

func TestHookCannotMutateByteArgsInPlace(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(byteHook{}), momo.WithArgLogging())
	_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("t")).Where(momo.SQL("title = ?", []byte("keep"))).Exec(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range drv.Calls() {
		for _, a := range c.Args {
			if b, ok := a.([]byte); ok && string(b) != "keep" {
				t.Fatalf("a hook changed a byte argument in place: %q", b)
			}
		}
	}
}

// rawHook overwrites any json.RawMessage argument in place.
type rawHook struct{}

func (rawHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	for _, a := range e.Args {
		if b, ok := a.(json.RawMessage); ok {
			for i := range b {
				b[i] = 'X'
			}
		}
	}
	return ctx
}
func (rawHook) AfterQuery(context.Context, *momo.QueryEvent) {}

func TestHookCannotMutateRawMessageInPlace(t *testing.T) {
	drv := fakedriver.New(dialect.Postgres)
	db := momo.New(drv, momo.WithHook(rawHook{}), momo.WithArgLogging())
	_, err := momo.Select[suite.Document](db).Where(momo.Func[bool]("jsonb_test", json.RawMessage(`{"a":1}`))).All(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range drv.Calls() {
		for _, a := range c.Args {
			if b, ok := a.(json.RawMessage); ok {
				found = true
				if string(b) != `{"a":1}` {
					t.Fatalf("a hook changed a json.RawMessage argument in place: %q", b)
				}
			}
		}
	}
	if !found {
		t.Fatal("the driver did not receive the json.RawMessage argument")
	}
}
