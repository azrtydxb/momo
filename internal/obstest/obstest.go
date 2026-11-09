// SPDX-License-Identifier: Apache-2.0

// Package obstest is test support for the observability tests. The recording
// hook now lives in package obs; the names here stay so existing tests keep
// working.
package obstest

import "github.com/azrtydxb/momo/obs"

// Event is a copy of an AfterQuery event plus the value the same hook put
// into the context in BeforeQuery.
type Event = obs.Event

// Recorder is a momo.Hook that records every finished statement.
type Recorder = obs.Recorder
