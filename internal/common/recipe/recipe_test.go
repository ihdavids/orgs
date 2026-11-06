package recipe

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func load(t *testing.T, name, url string) *Recipe {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse(b, url)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return r
}

func TestParseGraphWithSections(t *testing.T) {
	r := load(t, "wordpress.html", "https://spiceandsimmer.example/chicken-curry/")
	if r.Name != "Weeknight Chicken Curry" {
		t.Errorf("name %q", r.Name)
	}
	if r.Servings != 4 || r.Yield != "4 servings" {
		t.Errorf("servings %v yield %q", r.Servings, r.Yield)
	}
	if r.PrepTime != "15 min" || r.CookTime != "25 min" || r.TotalTime != "40 min" {
		t.Errorf("times %q %q %q", r.PrepTime, r.CookTime, r.TotalTime)
	}
	if r.Image != "https://spiceandsimmer.example/wp-content/uploads/curry-1200x1200.jpg" {
		t.Errorf("image %q", r.Image)
	}
	if r.Description != "A creamy, mild curry that's on the table in 40 minutes." {
		t.Errorf("description %q", r.Description)
	}
	if r.Author != "Priya Natarajan" || r.Cuisine != "Indian" || r.Category != "Main Course" {
		t.Errorf("author %q cuisine %q category %q", r.Author, r.Cuisine, r.Category)
	}
	if len(r.Ingredients) != 8 || r.Ingredients[5] != "½ cup plain yogurt" {
		t.Errorf("ingredients %q", r.Ingredients)
	}
	want := []Step{
		{"For the base", "Heat the oil in a large pan over medium heat."},
		{"For the base", "Add the onion and cook until soft, about 8 minutes. Stir in the garlic."},
		{"For the curry", "Add the chicken and garam masala; cook 5 minutes."},
		{"For the curry", "Pour in the coconut milk and simmer 15 minutes. Stir in the yogurt & season."},
	}
	if !reflect.DeepEqual(r.Directions, want) {
		t.Errorf("directions %q", r.Directions)
	}
	if !reflect.DeepEqual(r.Keywords, []string{"curry", "chicken", "weeknight"}) {
		t.Errorf("keywords %q", r.Keywords)
	}
}

func TestParseArrayTypeAndImageObject(t *testing.T) {
	r := load(t, "bigsite.html", "https://www.bigsite.example/recipe/85452/")
	if r.Name != "Homemade Black Bean Veggie Burgers" || r.Servings != 4 {
		t.Errorf("%q %v", r.Name, r.Servings)
	}
	if r.Url != "https://www.bigsite.example/recipe/85452/homemade-black-bean-veggie-burgers/" {
		t.Errorf("url %q", r.Url)
	}
	if r.Image != "https://www.bigsite.example/thmb/burgers.jpg" || r.Author != "Kate" {
		t.Errorf("image %q author %q", r.Image, r.Author)
	}
	if len(r.Directions) != 3 || r.Directions[0].Section != "" || strings.HasSuffix(r.Directions[0].Text, "\n") {
		t.Errorf("directions %q", r.Directions)
	}
}

func TestParseStringInstructionsRelativeImage(t *testing.T) {
	r := load(t, "plain.html", "https://bread.example/recipes/soda/")
	if r.Name != "Granny’s Soda Bread" {
		t.Errorf("name %q", r.Name)
	}
	if r.Servings != 1 || r.TotalTime != "1 h 5 min" {
		t.Errorf("servings %v total %q", r.Servings, r.TotalTime)
	}
	if r.Image != "https://bread.example/images/soda-bread.jpg" {
		t.Errorf("image %q", r.Image)
	}
	if r.Url != "https://bread.example/recipes/soda/" {
		t.Errorf("url %q", r.Url)
	}
	want := []string{"Heat the oven to 200C.", "Mix the dry ingredients, then stir in the buttermilk.", "Shape into a round, cut a cross on top and bake for 45 minutes."}
	if len(r.Directions) != 3 {
		t.Fatalf("directions %q", r.Directions)
	}
	for i, w := range want {
		if r.Directions[i].Text != w {
			t.Errorf("step %d %q", i, r.Directions[i].Text)
		}
	}
}

func TestParseMicrodata(t *testing.T) {
	r := load(t, "microdata.html", "https://old.example/lemon-bars")
	if r.Name != "Lemon Bars" || r.Yield != "16 bars" || r.Servings != 16 {
		t.Errorf("%q %q %v", r.Name, r.Yield, r.Servings)
	}
	if r.PrepTime != "20 min" || r.CookTime != "40 min" || r.Image != "https://old.example/lemon-bars.jpg" {
		t.Errorf("%q %q %q", r.PrepTime, r.CookTime, r.Image)
	}
	if r.Description != "Tart, sweet and easy." {
		t.Errorf("description %q", r.Description)
	}
	if len(r.Ingredients) != 4 || r.Ingredients[1] != "½ cup white sugar" {
		t.Errorf("ingredients %q", r.Ingredients)
	}
	if len(r.Directions) != 3 || r.Directions[2].Text != "Bake 15 to 20 minutes." {
		t.Errorf("directions %q", r.Directions)
	}
}

func TestParseNoRecipe(t *testing.T) {
	b, _ := os.ReadFile("testdata/norecipe.html")
	if _, err := Parse(b, "https://x.example"); err != ErrNoRecipe {
		t.Fatalf("got %v", err)
	}
}

func TestDurations(t *testing.T) {
	cases := map[string]int{
		"PT15M": 15, "PT1H20M": 80, "P0DT0H15M": 15, "PT90M": 90, "P1DT2H": 1560, "PT0.5H": 30,
		"PT1H30M0S": 90, "15  m": 15, "1 h 20 min": 80, "1 hour 20 minutes": 80, "1:30": 90,
		"45": 45, "2 hrs": 120, "": 0, "soon": 0,
	}
	for in, want := range cases {
		if got := ParseDuration(in); got != want {
			t.Errorf("ParseDuration(%q) = %d, want %d", in, got, want)
		}
	}
	fm := map[int]string{0: "", 15: "15 min", 60: "1 h", 80: "1 h 20 min", 1560: "1 d 2 h"}
	for in, want := range fm {
		if got := FormatMinutes(in); got != want {
			t.Errorf("FormatMinutes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestScaleIngredient(t *testing.T) {
	cases := []struct {
		in     string
		factor float64
		want   string
	}{
		{"2 cups flour", 2, "4 cups flour"},
		{"1 cup sugar", 2, "2 cups sugar"},
		{"2 cups sugar", 0.5, "1 cup sugar"},
		{"1/2 cup milk", 2, "1 cup milk"},
		{"1 1/2 pounds chicken", 2, "3 pounds chicken"},
		{"1 1/2 pounds chicken", 0.5, "3/4 pound chicken"},
		{"½ cup yogurt", 3, "1½ cups yogurt"},
		{"1½ cups stock", 2, "3 cups stock"},
		{"0.5 l water", 3, "1.5 l water"},
		{"2-3 teaspoons garam masala", 2, "4-6 teaspoons garam masala"},
		{"1 to 2 tbsp honey", 1.5, "1 1/2-3 tbsp honey"},
		{"1 (16 ounce) can black beans", 2, "2 (16 ounce) cans black beans"},
		{"400g spaghetti", 1.5, "600g spaghetti"},
		{"450 g plain flour", 1.0 / 3, "150 g plain flour"},
		{"1 egg", 3, "3 eggs"},
		{"salt to taste", 2, "salt to taste"},
		{"3 eggs", 0.5, "1 1/2 eggs"},
		{"about 2 cups rice", 2, "about 4 cups rice"},
		{"1 tsp salt", 0.05, "0.05 tsp salt"}, {"1 tsp salt", 0.1, "1/8 tsp salt"}, {"3/4 cup oil", 2, "1 1/2 cups oil"},
		{"2 cups flour", 1, "2 cups flour"},
	}
	for _, c := range cases {
		if got := ScaleIngredient(c.in, c.factor); got != c.want {
			t.Errorf("ScaleIngredient(%q, %v) = %q, want %q", c.in, c.factor, got, c.want)
		}
	}
}

func TestScaleRecipe(t *testing.T) {
	r := &Recipe{Name: "x", Servings: 4, Yield: "4 servings", Ingredients: []string{"2 cups flour", "1 egg"}}
	s, f := Scale(r, 6, 0)
	if f != 1.5 || s.Servings != 6 || s.Ingredients[0] != "3 cups flour" || s.Ingredients[1] != "1 1/2 eggs" {
		t.Errorf("%v %+v", f, s)
	}
	if r.Ingredients[0] != "2 cups flour" {
		t.Error("scaled the original")
	}
	n := &Recipe{Ingredients: []string{"2 cups flour"}}
	if s, f := Scale(n, 6, 0); f != 1 || s.Ingredients[0] != "2 cups flour" {
		t.Errorf("no servings: %v %q", f, s.Ingredients)
	}
	if s, _ := Scale(n, 0, 2); s.Ingredients[0] != "4 cups flour" {
		t.Errorf("factor: %q", s.Ingredients)
	}
}

func TestShop(t *testing.T) {
	lists := map[string][]string{
		"Curry":  {"1 large onion, finely chopped", "2 cups rice", "3 cloves garlic, minced", "salt to taste", "1 tbsp oil"},
		"Burger": {"½ onion, cut into wedges", "1 cup rice", "2 cloves garlic", "salt", "200 g cheese", "2 tablespoons oil"},
	}
	got := Shop(lists, []string{"Curry", "Burger"})
	texts := []string{}
	for _, g := range got {
		texts = append(texts, g.Text)
	}
	want := []string{"1 large onion, finely chopped", "3 cups rice", "5 cloves garlic, minced", "salt to taste", "3 tbsp oil", "½ onion, cut into wedges", "200 g cheese"}
	if !reflect.DeepEqual(texts, want) {
		t.Errorf("got  %q\nwant %q", texts, want)
	}
	if !reflect.DeepEqual(got[1].From, []string{"Curry", "Burger"}) {
		t.Errorf("from %q", got[1].From)
	}
}

func TestOrgRoundTrip(t *testing.T) {
	r := load(t, "wordpress.html", "https://spiceandsimmer.example/chicken-curry/")
	lines := ToOrg(r, Options{Level: 2, Record: true, ImageLink: "[[file:images/recipes/weeknight-chicken-curry.jpg]]"})
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"** Weeknight Chicken Curry\n:PROPERTIES:\n:source-url: https://spiceandsimmer.example/chicken-curry/\n:servings:   4\n:prep-time:  15 min",
		":ready-in:   40 min", ":yield:      4 servings", ":IMAGE:      [[file:images/recipes/weeknight-chicken-curry.jpg]]", ":RECORD:     recipe\n:END:",
		"\n*** Ingredients\n- 2 tablespoons vegetable oil\n",
		"\n*** Directions\n**** For the base\n1. Heat the oil", "\n**** For the curry\n1. Add the chicken",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	props := map[string]string{"source-url": r.Url, "servings": "4", "prep-time": "15 min", "IMAGE": "[[file:x.jpg]]", "yield": "4 servings"}
	back := FromOrg(r.Name, props, lines)
	if !reflect.DeepEqual(back.Ingredients, r.Ingredients) {
		t.Errorf("ingredients %q", back.Ingredients)
	}
	if !reflect.DeepEqual(back.Directions, r.Directions) {
		t.Errorf("directions %q", back.Directions)
	}
	if back.Servings != 4 || back.PrepTime != "15 min" || back.Description != r.Description {
		t.Errorf("%+v", back)
	}
}

// What org-chef itself writes, indented as org-adapt-indentation leaves it,
// and a heading typed from its manual capture template with checkboxes.
func TestFromOrgChefFile(t *testing.T) {
	text := `* Homemade Black Bean Veggie Burgers
  :PROPERTIES:
  :source-url: https://www.allrecipes.com/recipe/85452/homemade-black-bean-veggie-burgers/
  :servings: 4
  :prep-time: 15  m
  :cook-time: 20  m
  :ready-in: 35  m
  :END:
** Ingredients
   - 1 (16 ounce) can black beans, drained and rinsed
   - 1/2 green bell pepper, cut into 2 inch pieces
** Directions
   1. If grilling, preheat an outdoor grill for high heat, and lightly oil a
      sheet of aluminum foil.
   2. In a medium bowl, mash black beans with a fork until thick and pasty.
** My notes
   - not an ingredient
* Next recipe`
	r := FromOrg("Homemade Black Bean Veggie Burgers", map[string]string{"source-url": "u", "servings": "4", "prep-time": "15  m"}, strings.Split(text, "\n"))
	if len(r.Ingredients) != 2 || r.Ingredients[1] != "1/2 green bell pepper, cut into 2 inch pieces" {
		t.Errorf("ingredients %q", r.Ingredients)
	}
	if len(r.Directions) != 2 || r.Directions[0].Text != "If grilling, preheat an outdoor grill for high heat, and lightly oil a sheet of aluminum foil." {
		t.Errorf("directions %q", r.Directions)
	}
	if r.PrepTime != "15 min" || r.TotalTime != "" {
		t.Errorf("prep %q", r.PrepTime)
	}
	manual := "* Pancakes\n** Ingredients\n- [X] 2 eggs\n- [ ] 1 cup milk\n** Directions\n- Whisk.\n- Fry.\n"
	m := FromOrg("Pancakes", nil, strings.Split(manual, "\n"))
	if !reflect.DeepEqual(m.Ingredients, []string{"2 eggs", "1 cup milk"}) || len(m.Directions) != 2 {
		t.Errorf("%q %q", m.Ingredients, m.Directions)
	}
}
