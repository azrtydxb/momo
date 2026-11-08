// SPDX-License-Identifier: Apache-2.0

// Package gormfixture is a small project written with GORM. The importer
// reads its source, and its test compares the schema GORM creates with the
// schema the imported models describe.
package gormfixture

import "gorm.io/gorm"

// Org is a tenant.
type Org struct {
	gorm.Model
	Name string `gorm:"size:120;not null;uniqueIndex"`
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
