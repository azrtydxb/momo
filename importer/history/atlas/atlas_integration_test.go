// SPDX-License-Identifier: Apache-2.0

//go:build integration

package atlas

import (
	"os"
	"testing"

	"github.com/azrtydxb/momo/internal/importtest"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestHistoryImporters(t *testing.T) {
	t.Run("atlas", func(t *testing.T) {
		src, err := Read(os.DirFS("../../../internal/compat/atlasfixture/migrations"), []Revision{
			{Version: "20260101000000", Applied: 1, Total: 1},
			{Version: "20260101000001", Applied: 1, Total: 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(src.Applied) != 2 || src.Dirty {
			t.Fatalf("revisions not read: applied=%v dirty=%v", src.Applied, src.Dirty)
		}
		live := testopen.Postgres(t)
		importtest.CheckImport(t, live, importtest.Input{Tool: "atlas", Source: src})
	})
}
