// SPDX-License-Identifier: Apache-2.0

package env

import (
	"runtime"
	"testing"
)

func TestEnvCapture(t *testing.T) {
	e, err := Capture()
	if err != nil {
		t.Fatal(err)
	}
	if e.GoVersion == "" || e.GOOS != runtime.GOOS || e.GOARCH != runtime.GOARCH {
		t.Fatalf("toolchain fields wrong: %+v", e)
	}
	if e.NumCPU < 1 || e.GOMAXPROCS < 1 {
		t.Fatalf("cpu fields wrong: %+v", e)
	}
	if e.CapturedAtUTC == "" {
		t.Fatal("captured_at_utc empty")
	}
	if _, ok := e.ModuleVersions["github.com/jackc/pgx/v5"]; !ok {
		t.Fatalf("pinned module pgx/v5 missing from module_versions: %v", e.ModuleVersions)
	}
}
