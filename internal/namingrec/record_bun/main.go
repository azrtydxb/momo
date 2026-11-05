// SPDX-License-Identifier: Apache-2.0

// Command record_bun prints Bun's naming of the fixture models as JSON.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/jinzhu/inflection"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/schema"
)

// UserProfile is a fixture model.
type UserProfile struct {
	ID        int64 `bun:",pk"`
	UserID    int64
	HTTPURL   string
	CreatedAt time.Time
	Tags      []Tag `bun:"m2m:user_profile_tags,join:UserProfile=Tag"`
}

// Tag is a fixture model.
type Tag struct {
	ID   int64 `bun:",pk"`
	Name string
}

// UserProfileTag is the join model Bun requires for an m2m relation.
type UserProfileTag struct {
	UserProfileID int64        `bun:",pk"`
	UserProfile   *UserProfile `bun:"rel:belongs-to,join:user_profile_id=id"`
	TagID         int64        `bun:",pk"`
	Tag           *Tag         `bun:"rel:belongs-to,join:tag_id=id"`
}

// Status is a fixture model.
type Status struct {
	ID    int64 `bun:",pk"`
	Label string
}

// Person is a fixture model.
type Person struct {
	ID   int64 `bun:",pk"`
	Nick string
}

// words is the fixed Go-identifier list whose table and column names are
// recorded under the "words" key.
var words = []string{"User", "UserIDs", "IDs", "ID", "UserID", "HTTPURL", "APIKey", "JSONData", "UUID", "SQLQuery", "IPAddress", "Address", "Class", "Analysis", "Status", "Bus", "Category", "Box", "Person", "Child", "Mouse", "Wife", "Knife", "Quiz", "Sheep", "Fish", "Series", "Species", "Equipment", "Information", "Money", "Index", "Matrix", "Vertex", "Datum", "OrderItem", "CreatedAt"}

// wordColumn returns bun's column name for a struct field called name, by
// letting bun build a table from a one-field struct. Bun's underscore
// function is internal, so this is the only route to it.
func wordColumn(tables *schema.Tables, name string) string {
	typ := reflect.StructOf([]reflect.StructField{{Name: name, Type: reflect.TypeOf("")}})
	return tables.Get(typ).Fields[0].Name
}

func main() {
	tables := sqlitedialect.New().Tables()
	tables.Register((*UserProfileTag)(nil))
	out := map[string]any{"orm": "bun", "version": "v1.3.0"}
	tbl := map[string]string{}
	cols := map[string]string{}
	joins := map[string]string{}
	for _, m := range []struct {
		name string
		typ  reflect.Type
	}{
		{"UserProfile", reflect.TypeOf(UserProfile{})},
		{"Tag", reflect.TypeOf(Tag{})},
		{"Status", reflect.TypeOf(Status{})},
		{"Person", reflect.TypeOf(Person{})},
	} {
		t := tables.Get(m.typ)
		tbl[m.name] = t.Name
		if m.name != "UserProfile" {
			continue
		}
		for _, f := range t.Fields {
			if f.GoName == "UserID" || f.GoName == "HTTPURL" || f.GoName == "CreatedAt" {
				cols[m.name+"."+f.GoName] = f.Name
			}
		}
		if r, ok := t.Relations["Tags"]; ok && r.M2MTable != nil {
			joins["UserProfile.Tags"] = r.M2MTable.Name
		}
	}
	// Bun names a table inflection.Plural(underscore(type name)); the
	// underscore step is the same one it applies to field names.
	wt, wc := map[string]string{}, map[string]string{}
	for _, w := range words {
		c := wordColumn(tables, w)
		wc[w] = c
		wt[w] = inflection.Plural(c)
	}
	out["words"] = map[string]any{"tables": wt, "columns": wc}
	out["tables"], out["columns"], out["join_tables"] = tbl, cols, joins
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}
