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
)

func TestUnnestRendersOneArrayParameterPerColumn(t *testing.T) {
	ms := []suite.Document{{TenantID: 1, Title: "a"}, {TenantID: 2, Title: "b"}, {TenantID: 3, Title: "c"}}
	c, err := momo.Insert[suite.Document](unitDB()).
		Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt, suite.DocVersion, suite.DocCreatedAt, suite.DocUpdatedAt).
		Models(ms).Unnest().SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.SQL, `INSERT INTO "documents" ("tenant_id", "title") SELECT * FROM unnest($1::`) || !strings.HasSuffix(c.SQL, `[])`) {
		t.Fatalf("SQL = %s", c.SQL)
	}
	if len(c.Args) != 2 {
		t.Fatalf("%d args, want one array per written column (2): %v", len(c.Args), c.Args)
	}
	for i, a := range c.Args {
		col, ok := a.([]any)
		if !ok || len(col) != len(ms) {
			t.Fatalf("arg %d = %#v, want a []any with one value per row", i, a)
		}
	}
}

func TestUnnestBindCountDoesNotGrowWithRows(t *testing.T) {
	// 40000 rows of 2 columns would be 80000 binds as VALUES, over the 65535 limit.
	ms := make([]suite.Document, 40000)
	c, err := momo.Insert[suite.Document](unitDB()).
		Omit(suite.DocBody, suite.DocSecret, suite.DocDeletedAt, suite.DocVersion, suite.DocCreatedAt, suite.DocUpdatedAt).
		Models(ms).Unnest().SQL()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Args) != 2 {
		t.Fatalf("%d args, want 2", len(c.Args))
	}
}

func TestUnnestIsPostgresOnlyAndModelsOnly(t *testing.T) {
	ms := []suite.Document{{TenantID: 1, Title: "a"}}
	mysql := momo.New(fakedriver.New(dialect.MySQL))
	if _, err := momo.Insert[suite.Document](mysql).Models(ms).Unnest().SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("MySQL error = %v, want ErrUnsupported", err)
	}
	_, err := momo.Insert[suite.Document](unitDB()).Values(suite.DocTitle.Set("x")).Unnest().SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("Values with Unnest error = %v, want ErrValidation", err)
	}
}
