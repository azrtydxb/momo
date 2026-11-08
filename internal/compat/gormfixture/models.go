// SPDX-License-Identifier: Apache-2.0

// Package gormfixture is a small project written with GORM. The importer
// reads its source, and its test compares the schema GORM creates with the
// schema Momo derives from the imported models.
package gormfixture

import "gorm.io/gorm"

// Org is a tenant.
type Org struct {
	gorm.Model
	Name string `gorm:"size:120;not null;uniqueIndex"`
	Slug string `gorm:"size:60;unique"`
}

// User belongs to an Org.
type User struct {
	gorm.Model
	OrgID  uint   `gorm:"not null;index"`
	Org    Org    `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	Email  string `gorm:"size:255;not null;uniqueIndex:idx_user_email"`
	Active bool   `gorm:"default:true"`
	Score  int    `gorm:"type:bigint;default:0"`
	Note   *string
}

// TableName names the table GORM creates for User.
func (User) TableName() string { return "app_users" }

// Audit is embedded into Post under a prefix.
type Audit struct {
	CreatedBy string `gorm:"size:50"`
}

// Post has many Comments; the key lives on Comment.
type Post struct {
	gorm.Model
	Title    string `gorm:"size:200;not null"`
	Audit    Audit  `gorm:"embedded;embeddedPrefix:audit_"`
	Comments []Comment
}

// Comment holds the key of its Post and a composite index.
type Comment struct {
	gorm.Model
	PostID uint   `gorm:"not null"`
	Kind   string `gorm:"size:20;index:idx_comment_kind_body"`
	Body   string `gorm:"size:500;index:idx_comment_kind_body"`
}
