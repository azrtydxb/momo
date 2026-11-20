// SPDX-License-Identifier: Apache-2.0
//lint:file-ignore SA1019 these tests exercise the deprecated names

package gen_test

import (
	"reflect"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

// Catches: gen.Config becoming a distinct type from gen.LoadConfig, so a value
// built with the deprecated name could no longer be passed to Load or
// BuildSchema, or the two zero values could differ.
func TestDeprecatedConfigIsLoadConfig(t *testing.T) {
	var _ gen.LoadConfig = gen.Config{} //nolint:staticcheck // compile-time assertion that the alias is assignable
	var old gen.Config
	var current gen.LoadConfig
	if !reflect.DeepEqual(old, current) {
		t.Fatalf("zero gen.Config = %+v, zero gen.LoadConfig = %+v", old, current)
	}
	if reflect.TypeOf(old) != reflect.TypeOf(current) {
		t.Fatalf("gen.Config is %v, want the same type as gen.LoadConfig", reflect.TypeOf(old))
	}
}
