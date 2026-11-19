// SPDX-License-Identifier: Apache-2.0

//lint:file-ignore SA1019 these tests exist to exercise the deprecated name

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

// Catches: Annotate that drops the detail or mutates its argument, a deprecated
// WithDetail that stops forwarding to it, and an error that is not a *Error
// being wrapped instead of returned unchanged.
func TestAnnotateAndDeprecatedWithDetail(t *testing.T) {
	for name, fn := range map[string]func(error, string) error{
		"Annotate":   momo.Annotate,
		"WithDetail": momo.WithDetail,
	} {
		t.Run(name, func(t *testing.T) {
			orig := &momo.Error{Kind: momo.ErrNotFound, Err: errors.New("boom")}
			got := fn(orig, "raw driver detail")
			var me *momo.Error
			if !errors.As(got, &me) {
				t.Fatalf("result %v is not a *momo.Error", got)
			}
			if me.Detail() != "raw driver detail" {
				t.Errorf("Detail() = %q, want the detail", me.Detail())
			}
			if orig.Detail() == "raw driver detail" {
				t.Error("the argument was modified")
			}
			plain := errors.New("plain")
			if out := fn(plain, "x"); !errors.Is(out, plain) || out.Error() != "plain" {
				t.Errorf("a non-*Error was changed: %v", out)
			}
		})
	}
}
