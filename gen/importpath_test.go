// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestImportPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	const mod = "example.com/app"
	tests := []struct {
		name    string
		pkgDir  string
		want    string
		wantErr string
	}{
		{"nested", filepath.Join(root, "internal", "models", "user"), "example.com/app/internal/models/user", ""},
		{"same dir", root, "example.com/app", ""},
		{"outside", filepath.Join(filepath.Dir(root), "other"), "", "is outside module"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ImportPath(mod, root, tc.pkgDir)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ImportPath err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("ImportPath = %q, want %q", got, tc.want)
			}
		})
	}
}
