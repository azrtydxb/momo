// SPDX-License-Identifier: Apache-2.0

// Package redact masks sensitive query arguments before they reach hooks. It
// imports nothing from the momo root package.
package redact

import (
	"encoding/json"
	"slices"
)

// Arg is the value that replaces a sensitive argument in the copy handed to
// hooks.
const Arg = "[redacted]"

// Mask returns a copy of args with the sensitive positions replaced, so a
// hook can neither read those values nor mutate the slice sent to the driver.
// Byte slices are copied too, so changing their contents in place does not reach
// the driver. It runs only when hooks are registered.
func Mask(args []any, sens []int) []any {
	if args == nil {
		return nil
	}
	out := slices.Clone(args)
	for i, a := range out {
		switch v := a.(type) {
		case []byte:
			out[i] = slices.Clone(v)
		case json.RawMessage:
			out[i] = slices.Clone(v)
		}
	}
	for _, i := range sens {
		if i >= 0 && i < len(out) {
			out[i] = Arg
		}
	}
	return out
}
