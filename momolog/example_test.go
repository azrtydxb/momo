// SPDX-License-Identifier: Apache-2.0

package momolog_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/momolog"
)

// Example logs an insert with argument values turned on. A value bound from a
// sensitive column is logged as momo.RedactedArg, never as the value itself.
// Values(true) and momo.WithArgLogging are both needed to log arguments.
func Example() {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	db := momo.New(fakedriver.New(dialect.Postgres),
		momo.WithHook(momolog.New(l, momolog.Values(true))), momo.WithArgLogging())

	doc := suite.Document{TenantID: 1, Title: "t", Secret: "hunter2-sensitive"}
	if _, err := momo.Insert[suite.Document](db).Models([]suite.Document{doc}).Exec(context.Background()); err != nil {
		fmt.Println(err)
	}
	out := buf.String()
	fmt.Println("redacted:", strings.Contains(out, momo.RedactedArg))
	fmt.Println("leaked:", strings.Contains(out, "hunter2-sensitive"))
	// Output:
	// redacted: true
	// leaked: false
}
