// SPDX-License-Identifier: Apache-2.0

//go:build integration

package gormfixture

import (
	"database/sql"

	"gorm.io/gorm"
)

// Sized has the integer sizes gorm.io/driver/postgres maps by bit width: int32
// is an integer, an unsigned type is one bit wider, so uint16 is an integer
// too, and sql.NullInt32 takes int32 from its first field.
type Sized struct {
	gorm.Model
	Rank  int32
	Level uint16
	Flag  int8
	Alt   sql.NullInt32
}

// Folder has many Files and sets their key to NULL when it is deleted.
type Folder struct {
	gorm.Model
	Files []File `gorm:"constraint:OnDelete:SET NULL"`
}

// File holds the key of its Folder.
type File struct {
	gorm.Model
	FolderID uint
}
