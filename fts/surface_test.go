// SPDX-License-Identifier: Apache-2.0

package fts_test

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/fts"
)

// Catches: a portable function growing a numeric score threshold.
func TestFullTextScoreNotNormalised(t *testing.T) {
	for name, fn := range map[string]any{"Search": fts.Search, "Score": fts.Score, "Snippet": fts.Snippet} {
		typ := reflect.TypeOf(fn)
		for i := 0; i < typ.NumIn(); i++ {
			switch typ.In(i).Kind() {
			case reflect.Float32, reflect.Float64:
				t.Errorf("fts.%s takes a numeric parameter %v: scores are not comparable across dialects", name, typ.In(i))
			}
		}
	}
}
