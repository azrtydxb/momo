// SPDX-License-Identifier: Apache-2.0

// Command codesize prints the generated-code size report as JSON.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/azrtydxb/momo/bench/codesize"
)

func main() {
	dir := "models/momo"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	abs, err := filepath.Abs(dir)
	if err == nil {
		var rep codesize.Report
		rep, err = codesize.Measure(context.Background(), abs)
		if err == nil {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			err = enc.Encode(rep)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "codesize:", err)
		os.Exit(1)
	}
}
