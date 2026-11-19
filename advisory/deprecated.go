// SPDX-License-Identifier: Apache-2.0

package advisory

import (
	"context"
	"errors"
	"strings"

	"github.com/azrtydxb/momo"
)

// The functions below are the one-per-variant API that Acquire and Release
// replace. Each forwards to them with the options its name implies and runs the
// same statement it always ran. They stay for one release.
//
// The String forms still take ...Option, but only for WithHash64 (and Hash64),
// as they always did. Xact, Shared and Try would change what the function does,
// so they are refused with momo.ErrValidation instead of being applied.

// with puts first before the options the caller passed.
func with(opts []Option, first ...Option) []Option { return append(first, opts...) }

// hashOnly refuses any option other than WithHash64.
func hashOnly(opts []Option) error {
	s := resolve(opts)
	var bad []string
	if s.xact {
		bad = append(bad, "Xact")
	}
	if s.shared {
		bad = append(bad, "Shared")
	}
	if s.try {
		bad = append(bad, "Try")
	}
	if len(bad) == 0 {
		return nil
	}
	return &momo.Error{Kind: momo.ErrValidation, Op: "advisory lock", Err: errors.New("a deprecated advisory function takes only WithHash64, not " + strings.Join(bad, ", ") + ": use Acquire or Release with that option")}
}

// XactLock takes the transaction-scoped advisory lock for key and waits until it
// is granted.
//
// Deprecated: use Acquire with Xact.
func XactLock(ctx context.Context, tx *momo.Tx, key int64) error {
	_, err := Acquire(ctx, tx, key, Xact())
	return err
}

// XactLockString is XactLock with a text key. Only WithHash64 is accepted as an
// option; any other is momo.ErrValidation.
//
// Deprecated: use Acquire with Xact and a string key.
func XactLockString(ctx context.Context, tx *momo.Tx, key string, opts ...Option) error {
	if err := hashOnly(opts); err != nil {
		return err
	}
	_, err := Acquire(ctx, tx, key, with(opts, Xact())...)
	return err
}

// TryXactLock is the non-blocking form of XactLock.
//
// Deprecated: use Acquire with Xact and Try.
func TryXactLock(ctx context.Context, tx *momo.Tx, key int64) (bool, error) {
	return Acquire(ctx, tx, key, Xact(), Try())
}

// TryXactLockString is TryXactLock with a text key. Only WithHash64 is accepted
// as an option; any other is momo.ErrValidation.
//
// Deprecated: use Acquire with Xact, Try and a string key.
func TryXactLockString(ctx context.Context, tx *momo.Tx, key string, opts ...Option) (bool, error) {
	if err := hashOnly(opts); err != nil {
		return false, err
	}
	return Acquire(ctx, tx, key, with(opts, Xact(), Try())...)
}

// Lock takes the session-scoped advisory lock for key on the pinned connection
// behind db and waits until it is granted.
//
// Deprecated: use Acquire.
func Lock(ctx context.Context, db *momo.DB, key int64) error {
	_, err := Acquire(ctx, db, key)
	return err
}

// LockString is Lock with a text key. Only WithHash64 is accepted as an option;
// any other is momo.ErrValidation.
//
// Deprecated: use Acquire with a string key.
func LockString(ctx context.Context, db *momo.DB, key string, opts ...Option) error {
	if err := hashOnly(opts); err != nil {
		return err
	}
	_, err := Acquire(ctx, db, key, opts...)
	return err
}

// TryLock is the non-blocking form of Lock.
//
// Deprecated: use Acquire with Try.
func TryLock(ctx context.Context, db *momo.DB, key int64) (bool, error) {
	return Acquire(ctx, db, key, Try())
}

// TryLockString is TryLock with a text key. Only WithHash64 is accepted as an
// option; any other is momo.ErrValidation.
//
// Deprecated: use Acquire with Try and a string key.
func TryLockString(ctx context.Context, db *momo.DB, key string, opts ...Option) (bool, error) {
	if err := hashOnly(opts); err != nil {
		return false, err
	}
	return Acquire(ctx, db, key, with(opts, Try())...)
}

// Unlock releases one hold of the session-scoped lock for key.
//
// Deprecated: use Release.
func Unlock(ctx context.Context, db *momo.DB, key int64) (bool, error) {
	return Release(ctx, db, key)
}

// UnlockString is Unlock for a text key. Only WithHash64 is accepted as an
// option; any other is momo.ErrValidation.
//
// Deprecated: use Release with a string key.
func UnlockString(ctx context.Context, db *momo.DB, key string, opts ...Option) (bool, error) {
	if err := hashOnly(opts); err != nil {
		return false, err
	}
	return Release(ctx, db, key, opts...)
}

// XactLockShared takes the transaction-scoped shared advisory lock for key and
// waits until it is granted.
//
// Deprecated: use Acquire with Xact and Shared.
func XactLockShared(ctx context.Context, tx *momo.Tx, key int64) error {
	_, err := Acquire(ctx, tx, key, Xact(), Shared())
	return err
}

// XactLockSharedString is XactLockShared with a text key. Only WithHash64 is
// accepted as an option; any other is momo.ErrValidation.
//
// Deprecated: use Acquire with Xact, Shared and a string key.
func XactLockSharedString(ctx context.Context, tx *momo.Tx, key string, opts ...Option) error {
	if err := hashOnly(opts); err != nil {
		return err
	}
	_, err := Acquire(ctx, tx, key, with(opts, Xact(), Shared())...)
	return err
}

// TryXactLockShared is the non-blocking form of XactLockShared.
//
// Deprecated: use Acquire with Xact, Shared and Try.
func TryXactLockShared(ctx context.Context, tx *momo.Tx, key int64) (bool, error) {
	return Acquire(ctx, tx, key, Xact(), Shared(), Try())
}

// TryXactLockSharedString is TryXactLockShared with a text key. Only WithHash64
// is accepted as an option; any other is momo.ErrValidation.
//
// Deprecated: use Acquire with Xact, Shared, Try and a string key.
func TryXactLockSharedString(ctx context.Context, tx *momo.Tx, key string, opts ...Option) (bool, error) {
	if err := hashOnly(opts); err != nil {
		return false, err
	}
	return Acquire(ctx, tx, key, with(opts, Xact(), Shared(), Try())...)
}

// LockShared takes the session-scoped shared advisory lock for key on the pinned
// connection behind db and waits until it is granted.
//
// Deprecated: use Acquire with Shared.
func LockShared(ctx context.Context, db *momo.DB, key int64) error {
	_, err := Acquire(ctx, db, key, Shared())
	return err
}

// LockSharedString is LockShared with a text key. Only WithHash64 is accepted as
// an option; any other is momo.ErrValidation.
//
// Deprecated: use Acquire with Shared and a string key.
func LockSharedString(ctx context.Context, db *momo.DB, key string, opts ...Option) error {
	if err := hashOnly(opts); err != nil {
		return err
	}
	_, err := Acquire(ctx, db, key, with(opts, Shared())...)
	return err
}

// TryLockShared is the non-blocking form of LockShared.
//
// Deprecated: use Acquire with Shared and Try.
func TryLockShared(ctx context.Context, db *momo.DB, key int64) (bool, error) {
	return Acquire(ctx, db, key, Shared(), Try())
}

// TryLockSharedString is TryLockShared with a text key. Only WithHash64 is
// accepted as an option; any other is momo.ErrValidation.
//
// Deprecated: use Acquire with Shared, Try and a string key.
func TryLockSharedString(ctx context.Context, db *momo.DB, key string, opts ...Option) (bool, error) {
	if err := hashOnly(opts); err != nil {
		return false, err
	}
	return Acquire(ctx, db, key, with(opts, Shared(), Try())...)
}

// UnlockShared releases one hold of the session-scoped shared lock for key.
//
// Deprecated: use Release with Shared.
func UnlockShared(ctx context.Context, db *momo.DB, key int64) (bool, error) {
	return Release(ctx, db, key, Shared())
}

// UnlockSharedString is UnlockShared for a text key. Only WithHash64 is accepted
// as an option; any other is momo.ErrValidation.
//
// Deprecated: use Release with Shared and a string key.
func UnlockSharedString(ctx context.Context, db *momo.DB, key string, opts ...Option) (bool, error) {
	if err := hashOnly(opts); err != nil {
		return false, err
	}
	return Release(ctx, db, key, with(opts, Shared())...)
}
