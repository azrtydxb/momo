// SPDX-License-Identifier: Apache-2.0

// Command record_gorm prints GORM's naming of the fixture models as JSON.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"gorm.io/gorm/schema"
)

// UserProfile is a fixture model.
type UserProfile struct {
	ID        int64
	UserID    int64
	HTTPURL   string
	CreatedAt time.Time
	Tags      []Tag `gorm:"many2many:user_profile_tags"`
}

// Tag is a fixture model.
type Tag struct {
	ID   int64
	Name string
}

// Status is a fixture model.
type Status struct {
	ID    int64
	Label string
}

// Person is a fixture model.
type Person struct {
	ID   int64
	Nick string
}

// words is the fixed Go-identifier list whose table and column names are
// recorded under the "words" key.
var words = []string{"User", "UserIDs", "IDs", "ID", "UserID", "HTTPURL", "APIKey", "JSONData", "UUID", "SQLQuery", "IPAddress", "Address", "Class", "Analysis", "Status", "Bus", "Category", "Box", "Person", "Child", "Mouse", "Wife", "Knife", "Quiz", "Sheep", "Fish", "Series", "Species", "Equipment", "Information", "Money", "Index", "Matrix", "Vertex", "Datum", "OrderItem", "CreatedAt"}

func main() {
	out := map[string]any{"orm": "gorm", "version": "v1.31.2"}
	tables := map[string]string{}
	columns := map[string]string{}
	joins := map[string]string{}
	cache := &sync.Map{}
	for _, m := range []struct {
		name  string
		model any
	}{{"UserProfile", &UserProfile{}}, {"Tag", &Tag{}}, {"Status", &Status{}}, {"Person", &Person{}}} {
		s, err := schema.Parse(m.model, cache, schema.NamingStrategy{})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		tables[m.name] = s.Table
		if m.name == "UserProfile" {
			for _, f := range s.Fields {
				if f.Name == "UserID" || f.Name == "HTTPURL" || f.Name == "CreatedAt" {
					columns[m.name+"."+f.Name] = f.DBName
				}
			}
			if r, ok := s.Relationships.Relations["Tags"]; ok && r.JoinTable != nil {
				joins["UserProfile.Tags"] = r.JoinTable.Table
			}
		}
	}
	wt, wc := map[string]string{}, map[string]string{}
	ns := schema.NamingStrategy{}
	for _, w := range words {
		wt[w] = ns.TableName(w)
		wc[w] = ns.ColumnName("", w)
	}
	out["words"] = map[string]any{"tables": wt, "columns": wc}
	out["tables"], out["columns"], out["join_tables"] = tables, columns, joins
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}
