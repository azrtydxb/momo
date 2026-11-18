// SPDX-License-Identifier: Apache-2.0

// Package model declares the outbox table as a Momo model. The outbox tests
// load it with the generator and create the table from the schema it produces,
// so the table the relay reads is the one `momo migrate diff` writes.
package model

import (
	"time"

	"github.com/azrtydxb/momo"
)

// OutboxEvent is one event that waits to be published.
type OutboxEvent struct {
	momo.Model  `momo:"table:outbox_events"`
	ID          int64      `momo:"column:id,pk,identity"`
	Topic       string     `momo:"column:topic,size:255"`
	Key         *string    `momo:"column:key,size:255"`
	Payload     []byte     `momo:"column:payload"`
	Headers     *string    `momo:"column:headers,type:text"`
	CreatedAt   time.Time  `momo:"column:created_at"`
	AvailableAt time.Time  `momo:"column:available_at"`
	Attempts    int32      `momo:"column:attempts,default:0"`
	PublishedAt *time.Time `momo:"column:published_at"`
	DeadAt      *time.Time `momo:"column:dead_at"`
	LastError   *string    `momo:"column:last_error,type:text"`
}

// MomoConfig declares the two indexes the relay and the retention cleanup read
// through. They are plain indexes, not partial ones, because MySQL and MariaDB
// have no partial indexes; a NULL published_at or dead_at is searched as a key
// value.
func (e *OutboxEvent) MomoConfig(c *momo.Config) {
	c.Index("outbox_events_due_idx", &e.PublishedAt, &e.DeadAt, &e.AvailableAt)
	c.Index("outbox_events_key_idx", &e.Key, &e.PublishedAt, &e.ID)
}
