// SPDX-License-Identifier: Apache-2.0

package naming

import "testing"

func TestNamingMomoPreset(t *testing.T) {
	var n Momo
	cases := map[string]string{
		"User":        "users",
		"Category":    "categories",
		"Box":         "boxes",
		"Person":      "people",
		"Status":      "statuses",
		"UserProfile": "user_profiles",
		"Day":         "days",
	}
	for in, want := range cases {
		if got := n.TableName(in); got != want {
			t.Errorf("TableName(%q)=%q want %q", in, got, want)
		}
	}
	cols := map[string]string{"UserID": "user_id", "HTTPStatus": "http_status", "CreatedAt": "created_at", "ID": "id"}
	for in, want := range cols {
		if got := n.ColumnName(in); got != want {
			t.Errorf("ColumnName(%q)=%q want %q", in, got, want)
		}
	}
	if got := n.JoinTableName("Document", "Tag"); got != "documents_tags" {
		t.Errorf("JoinTableName=%q", got)
	}
	if _, err := ByName("nope"); err == nil {
		t.Error("unknown preset accepted")
	}
}

// The plan's table omits acronyms inside names, digits, single letters,
// snake input, unicode, the empty string and the other irregular plurals.
func TestNamingToSnakeEdgeCases(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"A":             "a",
		"a":             "a",
		"URL":           "url",
		"HTTPServer":    "http_server",
		"ServerHTTP":    "server_http",
		"UserURLPath":   "user_url_path",
		"V2Name":        "v2_name",
		"Address2":      "address2",
		"Address2Line":  "address2_line",
		"already_snake": "already_snake",
		"user_id":       "user_id",
		"Ünïcode":       "ünïcode",
		"ÜberUser":      "über_user",
		"ÉcoleID":       "école_id",
	}
	for in, want := range cases {
		if got := ToSnake(in); got != want {
			t.Errorf("ToSnake(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNamingPluralEdgeCases(t *testing.T) {
	cases := map[string]string{
		"":            "",
		"child":       "children",
		"man":         "men",
		"woman":       "women",
		"mouse":       "mice",
		"human":       "humans",
		"key":         "keys",
		"boy":         "boys",
		"city":        "cities",
		"bus":         "buses",
		"quiz":        "quizes",
		"church":      "churches",
		"dish":        "dishes",
		"user_person": "user_people",
		"person_user": "person_users",
		"user_status": "user_statuses",
	}
	for in, want := range cases {
		if got := Plural(in); got != want {
			t.Errorf("Plural(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNamingByName(t *testing.T) {
	n, err := ByName("momo")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := n.(Momo); !ok {
		t.Errorf("ByName(momo)=%T", n)
	}
	_, err = ByName("nope")
	if err == nil || err.Error() != `unknown naming preset "nope"` {
		t.Errorf("err=%v", err)
	}
	if _, err := ByName(""); err == nil {
		t.Error("empty preset name accepted")
	}
}
