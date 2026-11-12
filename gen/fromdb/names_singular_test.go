// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"testing"

	"github.com/azrtydxb/momo/gen/naming"
)

// Reasons a preset cannot round-trip an expected name. The generator keeps the
// first candidate whose plural gives the table name back, so when the expected
// name does not round-trip, a later wrong candidate that does wins. Those
// outputs are pinned below so that any drift shows.
const (
	whyEntCase    = "ent matches its rules case-sensitively on the Go name, so the capitalized word is not pluralized back"
	whyInflection = "gorm and bun (jinzhu/inflection) leave a word ending in s unchanged, so the singular does not round-trip"
	whyNoORule    = "the preset has no rule for a plural in -oes, so the singular plus s does not round-trip"
	whyMomoQuiz   = "momo pluralizes quiz to quizes, so quiz does not round-trip and quizz does"
)

// TestStructNameSingularWords pins the model name --from-db derives from a
// table name, under every naming preset naming.ByName knows. want is the
// expected name; pinned maps a preset that cannot round-trip it to the name
// that preset actually gives.
func TestStructNameSingularWords(t *testing.T) {
	presets := []string{"momo", "gorm", "bun", "ent", "prisma"}
	type pin struct{ got, why string }
	cases := []struct {
		table, want string
		pinned      map[string]pin
	}{
		// Words ending in -se: only the s is stripped.
		{"releases", "Release", nil},
		{"cases", "Case", nil},
		{"courses", "Course", nil},
		{"responses", "Response", nil},
		{"licenses", "License", nil},
		{"bases", "Base", nil},
		{"houses", "House", nil},
		{"purposes", "Purpose", nil},
		{"uses", "Use", nil},
		{"causes", "Cause", nil},
		{"clauses", "Clause", nil},
		{"sizes", "Size", nil},
		{"prizes", "Prize", nil},
		{"mazes", "Maze", nil},
		{"caches", "Cache", nil},
		{"niches", "Niche", nil},
		// Words that take -es.
		{"addresses", "Address", nil},
		{"classes", "Class", nil},
		{"boxes", "Box", nil},
		{"matches", "Match", nil},
		{"wishes", "Wish", nil},
		{"statuses", "Status", map[string]pin{"ent": {"Statuse", whyEntCase}}},
		{"buses", "Bus", map[string]pin{"ent": {"Buse", whyEntCase}}},
		{"quizzes", "Quiz", map[string]pin{
			"momo": {"Quizz", whyMomoQuiz},
			"ent":  {"Quizze", whyEntCase},
		}},
		// A table whose name is already singular and ends in -ss (or is a
		// known -us word) is kept, not stripped to Clas or Statu: no plural
		// ends in a lone s after another s. The plural forms above still work.
		{"class", "Class", nil},
		{"address", "Address", nil},
		{"status", "Status", nil},
		{"glass", "Glass", nil},
		{"boss", "Boss", nil},
		{"process", "Process", nil},
		{"access", "Access", nil},
		{"assessment_class", "AssessmentClass", nil},
		{"user_status", "UserStatus", nil},
		// Singular words in -us are kept, except the plurals of -u words.
		{"virus", "Virus", nil},
		{"bonus", "Bonus", nil},
		{"census", "Census", nil},
		{"corpus", "Corpus", nil},
		{"nucleus", "Nucleus", nil},
		{"stimulus", "Stimulus", nil},
		{"campus", "Campus", nil},
		{"bus", "Bus", nil},
		{"menus", "Menu", nil},
		{"emus", "Emu", nil},
		{"gnus", "Gnu", nil},
		{"haikus", "Haiku", nil},
		{"tutus", "Tutu", nil},
		{"kudus", "Kudu", nil},
		// Listed singular words in -is, -as and -ns keep their s.
		{"analysis", "Analysis", nil},
		{"axis", "Axis", nil},
		{"crisis", "Crisis", nil},
		{"basis", "Basis", nil},
		{"thesis", "Thesis", nil},
		{"iris", "Iris", nil},
		{"synopsis", "Synopsis", nil},
		{"diagnosis", "Diagnosis", nil},
		{"alias", "Alias", nil},
		{"canvas", "Canvas", nil},
		{"atlas", "Atlas", nil},
		{"bias", "Bias", nil},
		{"gas", "Gas", nil},
		{"news", "News", nil},
		{"lens", "Lens", nil},
		// -as and -is are not blanket rules: other plurals lose their s.
		{"areas", "Area", nil},
		{"ideas", "Idea", nil},
		// A -zes word that is not a doubled z loses only its s (Waltze, not
		// Waltz): the rule cannot tell it from sizes.
		{"waltzes", "Waltze", nil},
		// Words in -as, -is, -ens: the stem keeps its s.
		{"aliases", "Alias", map[string]pin{"ent": {"Aliase", whyEntCase}}},
		{"biases", "Bias", map[string]pin{
			"gorm": {"Biase", whyInflection}, "bun": {"Biase", whyInflection}, "ent": {"Biase", whyEntCase},
		}},
		{"canvases", "Canvas", map[string]pin{
			"gorm": {"Canvase", whyInflection}, "bun": {"Canvase", whyInflection}, "ent": {"Canvase", whyEntCase},
		}},
		{"gases", "Gas", map[string]pin{
			"gorm": {"Gase", whyInflection}, "bun": {"Gase", whyInflection}, "ent": {"Gase", whyEntCase},
		}},
		{"atlases", "Atlas", map[string]pin{
			"gorm": {"Atlase", whyInflection}, "bun": {"Atlase", whyInflection}, "ent": {"Atlase", whyEntCase},
		}},
		{"lenses", "Lens", map[string]pin{
			"gorm": {"Lense", whyInflection}, "bun": {"Lense", whyInflection}, "ent": {"Lense", whyEntCase},
		}},
		{"irises", "Iris", map[string]pin{
			"gorm": {"Irise", whyInflection}, "bun": {"Irise", whyInflection}, "ent": {"Irise", whyEntCase},
		}},
		// Words in -oes.
		{"tomatoes", "Tomato", map[string]pin{
			"momo": {"Tomatoe", whyNoORule}, "ent": {"Tomatoe", whyNoORule},
		}},
		{"buffaloes", "Buffalo", map[string]pin{
			"momo": {"Buffaloe", whyNoORule}, "ent": {"Buffaloe", whyNoORule},
		}},
		{"potatoes", "Potato", map[string]pin{
			"momo": {"Potatoe", whyNoORule}, "gorm": {"Potatoe", whyNoORule},
			"bun": {"Potatoe", whyNoORule}, "ent": {"Potatoe", whyNoORule},
		}},
		{"heroes", "Hero", map[string]pin{
			"momo": {"Heroe", whyNoORule}, "gorm": {"Heroe", whyNoORule},
			"bun": {"Heroe", whyNoORule}, "ent": {"Heroe", whyNoORule},
		}},
		{"echoes", "Echo", map[string]pin{
			"momo": {"Echoe", whyNoORule}, "gorm": {"Echoe", whyNoORule},
			"bun": {"Echoe", whyNoORule}, "ent": {"Echoe", whyNoORule},
		}},
		{"vetoes", "Veto", map[string]pin{
			"momo": {"Vetoe", whyNoORule}, "gorm": {"Vetoe", whyNoORule},
			"bun": {"Vetoe", whyNoORule}, "ent": {"Vetoe", whyNoORule},
		}},
		// -ies and irregular plurals.
		{"categories", "Category", nil},
		{"companies", "Company", nil},
		{"users", "User", nil},
		{"order_items", "OrderItem", nil},
		{"people", "Person", nil},
	}
	for _, name := range presets {
		namer, err := naming.ByName(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cases {
			want := c.want
			if p, ok := c.pinned[name]; ok {
				want = p.got
			}
			g := &generator{o: Options{Namer: namer}}
			if got := g.structName(c.table); got != want {
				t.Errorf("%s: structName(%q) = %q, want %q", name, c.table, got, want)
			}
		}
	}
}
