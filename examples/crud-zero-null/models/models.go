// SPDX-License-Identifier: Apache-2.0

// Package models is the schema of the crud-zero-null example: one table of
// tasks that shows how a write tells zero from NULL from "not mentioned".
package models

import (
	"time"

	"github.com/azrtydxb/momo"
)

// Task is one to-do item.
//
// A column that can hold NULL needs a Go type that can too, so Note is a
// *string: nil is NULL. That also makes the generated column a
// momo.NullColumn, the only kind with SetNull. Priority and Done are plain
// types, so their zero values (0 and false) are real values, never NULL.
type Task struct {
	momo.Model `momo:"table:tasks"`
	ID         int64      `momo:"pk,identity"`
	Title      string     `momo:"size:200,notnull"`
	Priority   int64      `momo:"notnull,default:3"`
	Done       bool       `momo:"notnull,default:false"`
	Note       *string    `momo:"size:500"`
	Version    int64      `momo:"version,default:0"`
	DeletedAt  *time.Time `momo:"column:deleted_at,soft_delete:timestamp"`
}
