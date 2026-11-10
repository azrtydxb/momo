// SPDX-License-Identifier: Apache-2.0

package advisory_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestCompileFailAdvisory checks that the advisory lock methods left DB and
// Tx for free functions in advisory, that a *momo.DB is refused at compile
// time, and that the key types stay separate.
// The programs live in testdata/compilefail and are built, not run.
func TestCompileFailAdvisory(t *testing.T) {
	for name, wants := range map[string][]string{
		"moved": {
			"tx.AdvisoryXactLock undefined",
			"tx.AdvisoryXactLockString undefined",
			"tx.TryAdvisoryXactLock undefined",
			"tx.TryAdvisoryXactLockString undefined",
			"db.AdvisoryXactLock undefined",
			"db.AdvisoryXactLockString undefined",
			"db.TryAdvisoryXactLock undefined",
			"db.TryAdvisoryXactLockString undefined",
		},
		"dbarg": {
			"as *momo.Tx value in argument to advisory.XactLock",
			"as *momo.Tx value in argument to advisory.XactLockString",
			"as *momo.Tx value in argument to advisory.TryXactLock",
			"as *momo.Tx value in argument to advisory.TryXactLockString",
		},
		"wrongtype": {
			"as int64 value in argument to advisory.XactLock",
			"as string value in argument to advisory.XactLockString",
		},
	} {
		cmd := exec.Command("go", "build", "-gcflags=-e", "-o", os.DevNull, "./testdata/compilefail/"+name)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s compiled; want a compile error", name)
			continue
		}
		for _, want := range wants {
			if !strings.Contains(string(out), want) {
				t.Errorf("%s: want %q in:\n%s", name, want, out)
			}
		}
	}
}
