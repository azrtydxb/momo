// SPDX-License-Identifier: Apache-2.0
//lint:file-ignore SA1019 these tests exercise the deprecated names

package migrate_test

import (
	"testing"
	"time"

	"github.com/azrtydxb/momo/migrate"
)

// Catches: migrate.FormatVersion drifting from migrate.HistoryFormat, so code
// still using the deprecated name would read a different history format than
// the engine writes.
func TestDeprecatedFormatVersionIsHistoryFormat(t *testing.T) {
	if migrate.FormatVersion != migrate.HistoryFormat {
		t.Fatalf("FormatVersion = %d, HistoryFormat = %d", migrate.FormatVersion, migrate.HistoryFormat)
	}
}

func lockTimeoutOf(s migrate.Settings) time.Duration { return s.LockTimeout }

// Catches: migrate.Config becoming a distinct type from migrate.Settings, so a
// Config value could no longer be assigned to a Settings variable or passed to
// WithSettings, and WithConfig and WithSettings could diverge.
func TestDeprecatedConfigIsSettings(t *testing.T) {
	c := migrate.Config{LockTimeout: time.Second}
	// The call assigns a Config to a Settings parameter: it compiles only
	// while the two are the same type.
	if got := lockTimeoutOf(c); got != time.Second {
		t.Fatalf("LockTimeout = %v, want 1s", got)
	}
	if migrate.WithConfig(c) == nil {
		t.Error("WithConfig returned a nil Option")
	}
	if migrate.WithSettings(c) == nil {
		t.Error("WithSettings returned a nil Option")
	}
}
