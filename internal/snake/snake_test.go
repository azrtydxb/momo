// SPDX-License-Identifier: Apache-2.0

package snake

import (
	"slices"
	"strings"
	"testing"
	"unicode"
)

// Catches: the shared word list answering differently for a word and its
// plural, or for another letter case.
func TestInitialism(t *testing.T) {
	for in, want := range map[string]string{
		"id": "ID", "ID": "ID", "ids": "IDs", "Urls": "URLs", "uuids": "UUIDs",
		"api": "API", "http": "HTTP", "https": "HTTPS",
	} {
		if got, ok := Initialism(in); !ok || got != want {
			t.Errorf("Initialism(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "s", "user", "users", "key", "keys", "uses", "ss"} {
		if got, ok := Initialism(in); ok {
			t.Errorf("Initialism(%q) = %q, want none", in, got)
		}
	}
}

// oldToSnake is ToSnake as it was before 0.8.0, kept here to pin the blast
// radius of the plural-initialism rule: the two must agree on every name that
// has no plural initialism.
func oldToSnake(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if unicode.IsUpper(r) && i > 0 {
			prev := rs[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) ||
				(unicode.IsUpper(prev) && i+1 < len(rs) && unicode.IsLower(rs[i+1])) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// Catches: a word of the list that breaks in one position (alone, plural,
// leading, embedded, before another word) while the rest work, and a plural
// that no longer differs from the old output (the blast radius moving). For
// every word W of the list, the new output is pinned for W, Ws, WsList,
// UserW, UserWs and UserWsList, and the old output is compared: the singular
// forms are unchanged, the plural forms are not (IPs was i_ps).
func TestToSnakeEveryInitialism(t *testing.T) {
	for w := range goInitialisms {
		lw := strings.ToLower(w)
		cases := []struct {
			in, want string
			plural   bool // the old output differs
		}{
			{w, lw, false},
			{w + "s", lw + "s", true},
			{w + "sList", lw + "s_list", true},
			{"User" + w, "user_" + lw, false},
			{"User" + w + "s", "user_" + lw + "s", true},
			{"User" + w + "sList", "user_" + lw + "s_list", true},
			{"User" + w + "Key", "user_" + lw + "_key", false},
		}
		for _, c := range cases {
			got, old := ToSnake(c.in), oldToSnake(c.in)
			if got != c.want {
				t.Errorf("ToSnake(%q) = %q, want %q (old: %q)", c.in, got, c.want, old)
			}
			if (got != old) != c.plural {
				t.Errorf("ToSnake(%q) = %q, old %q: changed = %v, want %v", c.in, got, old, got != old, c.plural)
			}
		}
	}
}

// Catches: the list words the integrator added, and the old split they gave.
func TestToSnakePidOidSsidGui(t *testing.T) {
	for in, c := range map[string][2]string{ // new, old
		"PIDs":     {"pids", "pi_ds"},
		"OIDs":     {"oids", "oi_ds"},
		"SSIDs":    {"ssids", "ssi_ds"},
		"GUIs":     {"guis", "gu_is"},
		"UserPIDs": {"user_pids", "user_pi_ds"},
		"PID":      {"pid", "pid"},
		"GUI":      {"gui", "gui"},
	} {
		if got := ToSnake(in); got != c[0] {
			t.Errorf("ToSnake(%q) = %q, want %q", in, got, c[0])
		}
		if got := oldToSnake(in); got != c[1] {
			t.Errorf("oldToSnake(%q) = %q, want %q", in, got, c[1])
		}
	}
}

// Catches: the plural rule changing a name that has no plural initialism. The
// rule is an upper-case run that ends in a word of the list followed by a
// lone s that ends the name or is not followed by a lower-case letter.
// Every name built from these parts, up to four of them, is compared with the
// old algorithm, except the names the rule applies to, which must differ.
func TestToSnakeAgreesWithOldOutsideThePluralRule(t *testing.T) {
	parts := []string{"User", "ID", "Ids", "Http", "HTTP", "HTTPS", "Server", "A", "B", "Line", "2", "s", "URL", "OAuth", "x", "é", "_", "API", "Key", "S", "Ds", "PIDs"}
	var rec func(prefix string, depth int)
	same, differ := 0, 0
	rec = func(prefix string, depth int) {
		if prefix != "" {
			noSplit, _ := pluralInitialisms([]rune(prefix))
			plural := slices.Contains(noSplit, true)
			got, old := ToSnake(prefix), oldToSnake(prefix)
			switch {
			case !plural && got != old:
				t.Errorf("ToSnake(%q) = %q, old %q: a name without a plural initialism changed", prefix, got, old)
			case plural && got == old:
				t.Errorf("ToSnake(%q) = %q equals the old output although the plural rule applies", prefix, got)
			case plural:
				differ++
			default:
				same++
			}
		}
		if depth == 4 {
			return
		}
		for _, p := range parts {
			rec(prefix+p, depth+1)
		}
	}
	rec("", 0)
	if same == 0 || differ == 0 {
		t.Fatalf("the sweep covered %d unchanged and %d changed names, want both", same, differ)
	}
}

// Catches: a run in front of a plural s cut at the wrong place. The run is
// segmented into list words, the s goes with the last one: HTTPIDs is HTTP and
// IDs, not HTT and PIDs (the longest-suffix reading PID would give). Each case
// names the rule that decides it.
func TestToSnakePluralSegmentation(t *testing.T) {
	for _, c := range []struct{ in, want, rule string }{
		{"HTTPIDs", "http_ids", "full segmentation HTTP+ID beats the suffix PID"},
		{"XMLHTTPIDs", "xml_http_ids", "full segmentation XML+HTTP+ID"},
		{"PIDs", "pids", "the whole run is one word"},
		{"OIDs", "oids", "the whole run is one word"},
		{"SSIDs", "ssids", "the whole run is one word, not S+SID"},
		{"GUIs", "guis", "the whole run is one word"},
		{"GUIDs", "guids", "GUID, not GUI+D"},
		{"UserPIDs", "user_pids", "embedded: the run is PID"},
		{"ABCIDs", "abc_ids", "no full segmentation: the longest list-word suffix ID"},
		{"XMLABCIDs", "xmlabc_ids", "no full segmentation: the suffix ID, the rest stays together"},
		{"APIIDs", "api_ids", "full segmentation API+ID"},
		{"UIIDs", "ui_ids", "full segmentation UI+ID"},
		{"IPIDs", "ip_ids", "last word longest among full segmentations: IP+ID, I is no word"},
		{"HTTPSIDs", "https_ids", "full segmentation HTTPS+ID"},
		{"XMLIDs", "xml_ids", "full segmentation XML+ID"},
		{"UserAPIIDsList", "user_api_ids_list", "embedded, followed by another word"},
	} {
		if got := ToSnake(c.in); got != c.want {
			t.Errorf("ToSnake(%q) = %q, want %q (%s)", c.in, got, c.want, c.rule)
		}
	}
}
