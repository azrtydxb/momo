// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/internal/suite"

	"github.com/azrtydxb/momo"
)

// OnUpdate on the updated_at column is refused with a hint that the column is
// maintained automatically; other reserved columns get no such hint.
func TestFieldOnUpdateRefusalHintsAtAutomaticUpdatedAt(t *testing.T) {
	hook := func(col int) momo.FieldHook[suite.Document] {
		return momo.FieldHook[suite.Document]{Column: col, OnUpdate: func(*suite.Document) any { return int64(1) }}
	}
	db, _ := fieldDB(hook(7))
	_, err := momo.Update[suite.Document](db).Set(suite.DocTitle.Set("n")).Where(suite.DocID.Eq(1)).SQL()
	if err == nil || !strings.Contains(err.Error(), "maintained automatically") || !strings.Contains(err.Error(), "WithClock") {
		t.Fatalf("updated_at refusal lacks the hint: %v", err)
	}
	db, _ = fieldDB(hook(4))
	_, err = momo.Update[suite.Document](db).Set(suite.DocTitle.Set("n")).Where(suite.DocID.Eq(1)).SQL()
	if err == nil || strings.Contains(err.Error(), "maintained automatically") {
		t.Fatalf("version refusal has a misleading hint: %v", err)
	}
}
