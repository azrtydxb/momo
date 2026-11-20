// SPDX-License-Identifier: Apache-2.0

package pluralize

import "testing"

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
