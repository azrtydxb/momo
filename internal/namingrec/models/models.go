// SPDX-License-Identifier: Apache-2.0

// Package models is the reference fixture model set. Each recorder declares
// its own tagged copy, because gorm, bun and ent need different tags; keep
// those copies in step with this file when a model changes.
package models

import "time"

// UserProfile is the many-to-many owner.
type UserProfile struct {
	ID        int64
	UserID    int64
	HTTPURL   string
	CreatedAt time.Time
	Tags      []Tag
}

// Tag is the many-to-many target.
type Tag struct {
	ID   int64
	Name string
}

// Status exercises words that look plural.
type Status struct {
	ID    int64
	Label string
}

// Person exercises irregular pluralisation.
type Person struct {
	ID   int64
	Nick string
}
