// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"reflect"
	"strings"
)

// metaFor returns the metadata Query and QueryRows scan through: the
// generated metadata when T has it (a model, a projection or Row1..Row10).
func metaFor[T any]() (*Meta[T], error) {
	if m, ok := any((*T)(nil)).(interface{ MomoMeta() *Meta[T] }); ok {
		return m.MomoMeta(), nil
	}
	return nil, &Error{Kind: ErrValidation, Op: "raw", Err: fmt.Errorf("%s has no momo metadata", reflect.TypeFor[T]())}
}

func snakeCase(s string) string { return strings.ToLower(s) }
