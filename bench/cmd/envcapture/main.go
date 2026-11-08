// SPDX-License-Identifier: Apache-2.0

// Command envcapture prints the benchmark environment as JSON.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/azrtydxb/momo/bench/env"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "envcapture:", err)
		os.Exit(1)
	}
}

func run() error {
	e, err := env.Capture()
	if err != nil {
		return err
	}
	if dsn := os.Getenv("BENCH_PG_URL"); dsn != "" {
		if err := env.CapturePostgres(context.Background(), dsn, &e); err != nil {
			return err
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}
