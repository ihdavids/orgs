package mapgen

import (
	"math/rand/v2"
	"strings"
)

// Place names, English-sounding: a first part and an ending ("Ashford",
// "Kingsbridge"), now and then an old word in front ("Old Harrowmere") or a
// river or a lord behind ("Ashford-on-Lune").
var (
	nameStarts = []string{"Ash", "Black", "Bram", "Brigh", "Cold", "Crow", "Dun", "Elm", "Fair", "Fen", "Glen", "Gold",
		"Green", "Grey", "Hal", "Har", "Hart", "Hawk", "Holl", "Iron", "King", "Lang", "Mar", "Mill", "Moor", "North",
		"Oak", "Red", "Ros", "Salt", "Shep", "Silver", "Stan", "Stone", "Swin", "Thorn", "Tod", "Wey", "White", "Wil", "Win", "Wolf"}
	nameEnds = map[string][]string{
		"town":    {"ford", "bridge", "bury", "ton", "ham", "wick", "field", "minster", "stead", "worth", "port", "mouth"},
		"village": {"ley", "by", "thorpe", "cott", "hollow", "hythe", "dale", "well", "stoke", "combe"},
		"city":    {"minster", "bury", "chester", "caster", "haven", "gate", "port"},
		"castle":  {"hold", "keep", "watch", "tower", "guard", "crag"},
		"ruin":    {"barrow", "fall", "grave", "moot", "henge", "low"},
	}
	nameRivers = []string{"Lune", "Wye", "Ouse", "Tarr", "Aven", "Esk", "Mere", "Dane"}
)

// PlaceName is a name for a place of a kind - on a map town, village, city,
// castle, ruin (anything else is a town); in a town tavern, inn, temple, shop,
// smithy, guild, manor, barracks, market, well, gate or tower - the same for
// the same seed.
func PlaceName(seed uint64, kind string) string {
	r := rand.New(rand.NewPCG(seed, seed^0xa0761d6478bd642f))
	if n, ok := townPlace(r, kind); ok {
		return n
	}
	ends, ok := nameEnds[kind]
	if !ok {
		ends = nameEnds["town"]
	}
	start := nameStarts[r.IntN(len(nameStarts))]
	end := ends[r.IntN(len(ends))]
	if strings.HasSuffix(strings.ToLower(start), end[:1]) {
		end = ends[(r.IntN(len(ends))+1)%len(ends)]
	}
	name := start + end
	switch {
	case kind == "ruin" && r.IntN(3) == 0:
		name = "Old " + name
	case kind == "castle" && r.IntN(3) == 0:
		name = name + " Castle"
	case kind == "village" && r.IntN(5) == 0:
		name = "Little " + name
	case (kind == "town" || kind == "") && r.IntN(6) == 0:
		name = name + "-on-" + nameRivers[r.IntN(len(nameRivers))]
	}
	return name
}

// Places in a town: an inn's sign, a temple's dedication, a shopkeeper's name.
var (
	signAdj   = []string{"Gilded", "Prancing", "Drowned", "Laughing", "Crooked", "Sleeping", "Rusty", "Silver", "Black", "Wandering", "Golden", "Jolly", "Broken", "Green", "Red", "Three"}
	signNoun  = []string{"Goose", "Pony", "Rat", "Stag", "Lantern", "Kettle", "Anchor", "Dragon", "Barrel", "Crown", "Boar", "Swan", "Fiddle", "Hound", "Owl", "Wheel"}
	dedicate  = []string{"the Dawn", "the Morning Lord", "the Sea Mother", "Saint Aldric", "the Harvest", "the Silent Watcher", "the Lady of Mercy", "the Forge", "the Twin Moons"}
	folk      = []string{"Hobb", "Marta", "Aldous", "Tilly", "Garrick", "Wenna", "Osric", "Bryn", "Cedric", "Elsbeth", "Dunstan", "Maud", "Piers", "Agnes", "Rowan", "Hale"}
	goods     = []string{"Candles", "Cloth", "Spices", "Curios", "Lamps", "Rope", "Herbs", "Maps", "Bread", "Boots", "Books", "Pots"}
	crafts    = []string{"Masons'", "Weavers'", "Merchants'", "Tanners'", "Brewers'", "Scribes'", "Goldsmiths'", "Carpenters'", "Fishmongers'"}
	families  = []string{"Ashgrove", "Blackwood", "Vance", "Harrow", "Thorne", "Wyndham", "Corwin", "Mallory", "Steele", "Dunmore"}
	marketDay = []string{"Monday", "Fish", "Corn", "Cloth", "Horse", "Old", "High", "Butter", "Sheep"}
	gateNames = []string{"North", "South", "East", "West", "River", "King's", "Old", "Sea", "Market", "Hangman's"}
)

func townPlace(r *rand.Rand, kind string) (string, bool) {
	pick := func(xs []string) string { return xs[r.IntN(len(xs))] }
	switch kind {
	case "tavern", "inn":
		return "The " + pick(signAdj) + " " + pick(signNoun), true
	case "temple":
		return "Temple of " + pick(dedicate), true
	case "shop":
		return pick(folk) + "'s " + pick(goods), true
	case "smithy":
		return pick(folk) + "'s Forge", true
	case "guild":
		return "The " + pick(crafts) + " Guild", true
	case "manor":
		return pick(families) + " House", true
	case "barracks":
		return "The " + pick([]string{"Old", "Red", "Iron", "Watch", "Lower"}) + " Barracks", true
	case "market":
		return pick(marketDay) + " Market", true
	case "well":
		return pick([]string{"Wishing", "Old", "Sweet", "Black", "Saint's", "Deep"}) + " Well", true
	case "gate":
		return pick(gateNames) + " Gate", true
	case "tower":
		return "The " + pick([]string{"Wizard's", "Bell", "Leaning", "Watch", "Grey", "Burnt"}) + " Tower", true
	}
	return "", false
}
