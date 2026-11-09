// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
)

var errBadTitle = errors.New("bad title")

func fieldDB(hooks ...momo.FieldHook[suite.Document]) (*momo.DB, *fakedriver.Driver) {
	drv := fakedriver.New(dialect.Postgres)
	return momo.New(drv, momo.WithFieldHooks(hooks)), drv
}

func titleValidator(seen *[]any) momo.FieldHook[suite.Document] {
	return momo.FieldHook[suite.Document]{Column: 2, Validate: func(v any) error {
		*seen = append(*seen, v)
		if s, _ := v.(string); s == "" {
			return errBadTitle
		}
		return nil
	}}
}

func TestFieldDefaultAppliesToOmittedValuesColumn(t *testing.T) {
	db, _ := fieldDB(momo.FieldHook[suite.Document]{Column: 4, Default: func(*suite.Document) any { return int64(7) }})
	c, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("a")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `"version"`) || len(c.Args) < 3 || c.Args[2] != int64(7) {
		t.Fatalf("default not applied: %q %v", c.SQL, c.Args)
	}
	// An explicit assignment (even zero) wins over the default.
	c, err = momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("a"), suite.DocVersion.Set(0)).SQL()
	if err != nil || c.Args[2] != int64(0) {
		t.Fatalf("zero was replaced: %q %v %v", c.SQL, c.Args, err)
	}
	c, err = momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("a"), suite.DocVersion.SetDefault()).SQL()
	if err != nil || !strings.Contains(c.SQL, "DEFAULT") {
		t.Fatalf("explicit DEFAULT was replaced: %q %v", c.SQL, err)
	}
}

func TestFieldDefaultPerRowInValuesRows(t *testing.T) {
	n := int64(0)
	db, _ := fieldDB(momo.FieldHook[suite.Document]{Column: 4, Default: func(*suite.Document) any { n++; return n * 10 }})
	c, err := momo.Insert[suite.Document](db).ValuesRows(
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("a")},
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("b"), suite.DocVersion.Set(0)},
		[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("c")},
	).SQL()
	if err != nil {
		t.Fatal(err)
	}
	var versions []any
	for i, a := range c.Args {
		if i%5 == 2 {
			versions = append(versions, a)
		}
	}
	if len(versions) != 3 || versions[0] != int64(10) || versions[1] != int64(0) || versions[2] != int64(20) {
		t.Fatalf("versions = %v (args %v)", versions, c.Args)
	}
}

func TestFieldDefaultOnOmittedModelColumnOnly(t *testing.T) {
	db, _ := fieldDB(momo.FieldHook[suite.Document]{Column: 4, Default: func(m *suite.Document) any { return int64(9) + m.TenantID }})
	m := suite.Document{TenantID: 1, Title: "a", Version: 0}
	c, err := momo.Insert[suite.Document](db).Model(&m).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range c.Args {
		if a == int64(10) {
			t.Fatalf("zero version replaced by default: %v", c.Args)
		}
	}
	c, err = momo.Insert[suite.Document](db).Model(&m).Omit(suite.DocVersion).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `"version"`) {
		t.Fatalf("omitted column with a default must be written: %q", c.SQL)
	}
	found := false
	for _, a := range c.Args {
		found = found || a == int64(10)
	}
	if !found {
		t.Fatalf("default (from the model) not bound: %v", c.Args)
	}
	// Without a default an omitted column stays out of the statement.
	plain, _ := fieldDB()
	c, err = momo.Insert[suite.Document](plain).Model(&m).Omit(suite.DocVersion).SQL()
	if err != nil || strings.Contains(c.SQL, `"version"`) {
		t.Fatalf("omitted column without a default was written: %q %v", c.SQL, err)
	}
}

func TestFieldValidatorStructuredErrorAndNoSQL(t *testing.T) {
	var seen []any
	db, drv := fieldDB(titleValidator(&seen))
	ctx := context.Background()
	cases := map[string]func() error{
		"values": func() error {
			_, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("")).Exec(ctx)
			return err
		},
		"omitted": func() error {
			_, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1)).Exec(ctx)
			return err
		},
		"model": func() error {
			_, err := momo.Insert[suite.Document](db).Model(&suite.Document{TenantID: 1}).Exec(ctx)
			return err
		},
		"rows": func() error {
			_, err := momo.Insert[suite.Document](db).ValuesRows(
				[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("ok")},
				[]momo.Assignment{suite.DocTenantID.Set(1), suite.DocTitle.Set("")}).Exec(ctx)
			return err
		},
		"update": func() error {
			_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("")).Where(suite.DocID.Eq(1)).Exec(ctx)
			return err
		},
		"update model": func() error {
			_, err := momo.Update[suite.Document](db).Model(&suite.Document{ID: 1, Version: 1}).Exec(ctx)
			return err
		},
		"upsert": func() error {
			_, err := momo.Upsert[suite.Document](db).Model(&suite.Document{TenantID: 1}).Exec(ctx)
			return err
		},
	}
	for name, run := range cases {
		err := run()
		var me *momo.Error
		if !errors.As(err, &me) || !errors.Is(err, momo.ErrValidation) || !errors.Is(err, errBadTitle) {
			t.Fatalf("%s: err = %v, want ErrValidation wrapping the validator error", name, err)
		}
		if me.Table != "documents" || me.Column != "title" || me.Op == "" {
			t.Fatalf("%s: error fields = %+v", name, me)
		}
	}
	if n := len(drv.Calls()); n != 0 {
		t.Fatalf("%d statements executed despite validation failures", n)
	}
}

func TestFieldValidatorSeesWrittenValuesAndNilForOmitted(t *testing.T) {
	var seen []any
	db, _ := fieldDB(titleValidator(&seen))
	if _, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("x")).SQL(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] != "x" {
		t.Fatalf("seen = %v", seen)
	}
	seen = nil
	_, err := momo.Insert[suite.Document](db).Values(suite.DocTenantID.Set(1)).SQL()
	if !errors.Is(err, momo.ErrValidation) || len(seen) != 1 || seen[0] != nil {
		t.Fatalf("omitted column: seen = %v err = %v", seen, err)
	}
	// An update that does not set the column does not run its validator.
	seen = nil
	if _, err := momo.Update[suite.Document](db).Set(suite.DocBody.SetNull()).Where(suite.DocID.Eq(1)).SQL(); err != nil || len(seen) != 0 {
		t.Fatalf("validator ran for an unset column: %v %v", seen, err)
	}
}

func TestFieldChunkedInsertValidatesAllRowsBeforeAnyStatement(t *testing.T) {
	var seen []any
	db, drv := fieldDB(titleValidator(&seen))
	// Eight columns are written per row, so 9000 rows exceed 65535 parameters and chunk.
	docs := make([]suite.Document, 9000)
	for i := range docs {
		docs[i] = suite.Document{TenantID: 1, Title: "t"}
	}
	docs[len(docs)-1].Title = ""
	_, err := momo.Insert[suite.Document](db).Models(docs).Exec(context.Background())
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if n := len(drv.Calls()); n != 0 || drv.Commits()+drv.Rollbacks() != 0 {
		t.Fatalf("chunked insert touched the database before validating: %d calls", n)
	}
}

func TestFieldOnUpdate(t *testing.T) {
	db, _ := fieldDB(momo.FieldHook[suite.Document]{Column: 3, OnUpdate: func(*suite.Document) any { s := "touched"; return &s }})
	c, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("n")).Where(suite.DocID.Eq(1)).SQL()
	if err != nil || !strings.Contains(c.SQL, `"body" = $`) {
		t.Fatalf("OnUpdate not applied: %q %v", c.SQL, err)
	}
	// An explicit Set of the column wins.
	c, err = momo.Update[suite.Document](db).Set(suite.DocBody.SetNull()).Where(suite.DocID.Eq(1)).SQL()
	if err != nil || !strings.Contains(c.SQL, `"body" = NULL`) {
		t.Fatalf("explicit Set lost to OnUpdate: %q %v", c.SQL, err)
	}
	// An update with no assignments still fails: OnUpdate does not make it valid.
	if _, err := momo.Update[suite.Document](db).Where(suite.DocID.Eq(1)).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("empty update err = %v", err)
	}
	// OnUpdate on a reserved column is refused.
	bad, _ := fieldDB(momo.FieldHook[suite.Document]{Column: 4, OnUpdate: func(*suite.Document) any { return int64(1) }})
	if _, err := momo.Update[suite.Document](bad).Set(suite.DocTitle.Set("n")).Where(suite.DocID.Eq(1)).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("OnUpdate on version err = %v", err)
	}
}

func TestFieldHookBadColumnIndex(t *testing.T) {
	db, _ := fieldDB(momo.FieldHook[suite.Document]{Column: 99, Default: func(*suite.Document) any { return 1 }})
	_, err := momo.Insert[suite.Document](db).Values(suite.DocTitle.Set("a")).SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v", err)
	}
}

func TestFieldHooksFromMeta(t *testing.T) {
	saved := suite.DocumentMeta.Fields
	t.Cleanup(func() { suite.DocumentMeta.Fields = saved })
	var seen []any
	suite.DocumentMeta.Fields = []momo.FieldHook[suite.Document]{titleValidator(&seen)}
	db := momo.New(fakedriver.New(dialect.Postgres))
	_, err := momo.Insert[suite.Document](db).Values(suite.DocTitle.Set("")).SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Meta.Fields not applied: %v", err)
	}
}

func TestFieldValidatorCoversUpsertDoUpdate(t *testing.T) {
	var seen []any
	db, drv := fieldDB(titleValidator(&seen))
	_, err := momo.Upsert[suite.Document](db).
		Values(suite.DocTenantID.Set(1), suite.DocTitle.Set("fine")).
		OnConflict(suite.DocID).DoUpdate(suite.DocTitle.Set("")).Exec(context.Background())
	if !errors.Is(err, errBadTitle) || len(drv.Calls()) != 0 {
		t.Fatalf("DoUpdate value not validated: err = %v, calls = %d", err, len(drv.Calls()))
	}
}

func TestFieldHooksSharedByTxAndNotByOtherHandles(t *testing.T) {
	var seen []any
	db, _ := fieldDB(titleValidator(&seen))
	other := momo.New(fakedriver.New(dialect.Postgres))
	if _, err := momo.Insert[suite.Document](other).Values(suite.DocTitle.Set("")).SQL(); err != nil {
		t.Fatalf("hooks leaked to another DB: %v", err)
	}
	err := db.Tx(context.Background(), momo.TxOptions{}, func(tx *momo.Tx) error {
		_, err := momo.Insert[suite.Document](tx).Values(suite.DocTitle.Set("")).Exec(context.Background())
		return err
	})
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("tx err = %v", err)
	}
}
