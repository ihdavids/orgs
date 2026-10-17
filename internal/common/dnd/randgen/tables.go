package randgen

// The tables behind the generators. All of these are our own; nothing here
// is copied from donjon or a published book. Rule numbers (thresholds, trap
// DCs) live next to the code that uses them.

// ---- names -----------------------------------------------------------------

var sounds = map[string]nameSounds{
	"human": {
		start:    []string{"al", "ber", "cor", "da", "ed", "fen", "gar", "hal", "is", "jor", "ka", "lor", "mar", "ned", "os", "per", "ros", "sa", "tam", "ul", "ver", "wil", "bri", "cat", "el", "mae", "ro", "tes"},
		mid:      []string{"a", "e", "i", "o", "an", "el", "ar", "in", "or"},
		end:      []string{"ric", "a", "an", "en", "wyn", "ton", "ard", "ella", "ia", "is", "mund", "ry", "son", "da", "ene", "ett", "ley", "ius"},
		surStart: []string{"ash", "black", "bright", "cold", "cross", "fair", "green", "hart", "iron", "long", "mill", "oak", "red", "stone", "thorn", "under", "west", "wood", "barrow", "kettle"},
		surEnd:   []string{"well", "wood", "field", "ford", "ley", "smith", "brook", "wright", "man", "worth", "hill", "mere", "gate", "ward", "stead"},
	},
	"elf": {
		start:    []string{"ae", "cal", "ela", "fae", "gal", "ili", "lae", "mir", "nae", "quel", "rhi", "sil", "tha", "vae", "yl", "aer", "ith", "sha"},
		mid:      []string{"la", "ri", "the", "wy", "ne", "sa", "lo", "ae", "ia"},
		end:      []string{"nor", "riel", "wen", "dil", "thas", "lis", "ndra", "ion", "vyre", "ra", "las", "nae", "seth", "lian"},
		surStart: []string{"amber", "dawn", "dusk", "moon", "silver", "star", "wind", "leaf", "mist", "sun", "glimmer", "brook"},
		surEnd:   []string{"whisper", "song", "bough", "shadow", "petal", "blossom", "glade", "light", "runner", "weaver", "fall"},
	},
	"dwarf": {
		start:    []string{"bar", "bom", "dur", "dag", "far", "grim", "har", "kil", "mor", "nor", "orm", "rur", "thor", "ul", "bryn", "eld", "gun", "hel"},
		mid:      []string{"a", "o", "u", "in", "ar", "um"},
		end:      []string{"din", "dra", "gar", "grim", "hild", "in", "li", "na", "rak", "rik", "unn", "vild", "dis", "bek"},
		surStart: []string{"anvil", "battle", "copper", "deep", "flint", "forge", "gold", "granite", "hammer", "iron", "rock", "stout", "coal", "ember"},
		surEnd:   []string{"beard", "fist", "helm", "delver", "breaker", "shield", "mantle", "hand", "brow", "heart", "vein"},
	},
	"halfling": {
		start:    []string{"bil", "cora", "dro", "eli", "fin", "jas", "kel", "lav", "mer", "nel", "os", "pip", "ros", "sam", "tad", "wel", "ver", "lid"},
		mid:      []string{"a", "o", "i", "e"},
		end:      []string{"bo", "da", "do", "ie", "ly", "na", "ric", "wise", "y", "lo", "mie", "by"},
		surStart: []string{"brush", "good", "green", "hill", "honey", "tea", "tumble", "under", "high", "apple", "thistle", "warm"},
		surEnd:   []string{"gather", "barrel", "bottle", "foot", "leaf", "toe", "hearth", "bough", "hollow", "pot", "button"},
	},
	"gnome": {
		start:    []string{"al", "bim", "bod", "dim", "fen", "gim", "jeb", "kel", "nim", "orr", "pock", "quil", "ree", "san", "tink", "wren", "zan", "fiz"},
		mid:      []string{"ble", "li", "ni", "ra", "wi", "zo", "pi"},
		end:      []string{"ble", "do", "kin", "na", "nob", "ock", "sy", "ter", "wick", "zle", "bee", "pip"},
		surStart: []string{"beren", "cog", "fizz", "garrick", "nack", "sprocket", "tinker", "turen", "whistle", "spindle", "copper"},
		surEnd:   []string{"wheel", "spark", "gear", "spring", "button", "tock", "nozzle", "bell", "kettle"},
	},
	"orc": {
		start:    []string{"bru", "dur", "gak", "gor", "hol", "kar", "mog", "nar", "ruk", "shag", "thok", "ug", "vol", "yur", "bag", "em", "ov"},
		mid:      []string{"a", "u", "o", "ra"},
		end:      []string{"ash", "dak", "gar", "ka", "mok", "nar", "rash", "tha", "uk", "zag", "gra", "ra"},
		surStart: []string{"skull", "bone", "blood", "iron", "red", "black", "bitter", "storm", "tusk"},
		surEnd:   []string{"splitter", "crusher", "hide", "jaw", "eye", "fang", "howl", "biter"},
	},
	"dragon": {
		start:    []string{"ar", "bal", "dor", "ghe", "hes", "kri", "med", "nal", "pan", "rho", "sor", "tor", "ur", "vyr", "zor", "bir", "ka"},
		mid:      []string{"ja", "ra", "va", "ix", "ul", "es"},
		end:      []string{"ash", "hun", "jhan", "kas", "nar", "rax", "rinn", "thar", "vix", "xos", "dra", "ys"},
		surStart: []string{"clethtinth", "daardend", "delmirev", "drachedan", "fenkenkab", "kepeshkm", "kerrhylon", "myastan", "nemmonis", "turnuroth", "verthisa", "yarjerit"},
		surEnd:   []string{""},
	},
	"infernal": {
		start:    []string{"ak", "am", "bar", "dam", "ek", "ia", "kal", "le", "mor", "nem", "ori", "ska", "va", "zar", "cri"},
		mid:      []string{"ma", "ri", "ke", "lo", "a"},
		end:      []string{"akos", "ai", "emon", "ios", "kas", "mos", "rith", "sha", "zel", "dra", "thos"},
	},
}

var virtueNames = []string{"art", "carrion", "chant", "creed", "despair", "ember", "glory", "hope", "ideal", "mercy", "music", "nowhere", "open", "poetry", "quest", "random", "reverence", "sorrow", "temerity", "torment", "weary", "wisp", "lantern", "patience"}

// ---- NPCs ------------------------------------------------------------------

var npcAges = []string{"young", "young", "adult", "adult", "adult", "middle-aged", "middle-aged", "old", "very old"}

var occupationsBy = map[string][]string{
	"commoner":  {"farmer", "shepherd", "miller", "fisher", "laborer", "beggar", "servant", "stablehand", "woodcutter", "washer", "rat-catcher", "gravedigger"},
	"craft":     {"blacksmith", "carpenter", "cooper", "weaver", "tanner", "potter", "jeweler", "cobbler", "glassblower", "mason", "tailor", "bookbinder"},
	"merchant":  {"innkeeper", "peddler", "spice trader", "moneylender", "fishmonger", "wine merchant", "fence", "caravan master", "pawnbroker", "apothecary"},
	"martial":   {"town guard", "mercenary", "sellsword captain", "gladiator", "retired soldier", "bounty hunter", "bodyguard", "watch sergeant", "archer", "duelist"},
	"learned":   {"scribe", "sage", "physician", "astrologer", "cartographer", "tutor", "herbalist", "librarian", "alchemist", "lawyer"},
	"religious": {"acolyte", "priest", "monk", "pilgrim", "temple guard", "hermit", "relic seller", "oracle", "gravekeeper", "missionary"},
	"criminal":  {"pickpocket", "smuggler", "con artist", "burglar", "thug", "forger", "spy", "poacher", "highwayman", "cultist"},
	"noble":     {"minor noble", "heir", "courtier", "steward", "diplomat", "landholder", "knight errant", "magistrate", "herald"},
	"wanderer":  {"minstrel", "tinker", "hunter", "trapper", "guide", "sailor", "messenger", "explorer", "exile", "street performer"},
}

var occupations = func() []string {
	out := []string{}
	for _, k := range []string{"commoner", "commoner", "craft", "merchant", "martial", "learned", "religious", "criminal", "noble", "wanderer"} {
		out = append(out, occupationsBy[k]...)
	}
	return out
}()

var appearances = []string{
	"a nose that has been broken more than once", "ink-stained fingers", "a missing front tooth", "an elaborate braid with beads in it",
	"a burn scar along one arm", "unusually tall and stooped", "short and broad as a barrel", "eyes of two different colours",
	"a fine coat a size too big", "freckles everywhere", "a shaved head with a tattoo of a key", "a wooden leg carved with vines",
	"a constant squint", "very pale, almost grey", "sun-browned and weathered", "rings on every finger",
	"a patch over one eye", "a hat with a single long feather", "smells strongly of woodsmoke", "a nervous twitch of the left hand",
	"clothes patched in many colours", "a silver tooth", "hair gone white early", "a heavy limp",
	"a fresh bandage on one hand", "immaculately groomed", "a cloak hemmed in mud", "a necklace of small bones",
	"calloused, enormous hands", "an old soldier's tattoo", "a beard tied in a knot", "a faded noble crest on their sleeve",
}

var voices = []string{
	"speaks very quietly", "booming laugh", "whistles between words", "a thick rural accent", "clipped and precise",
	"never finishes a sentence", "rhymes without meaning to", "talks with their hands", "calls everyone 'friend'",
	"a rasping smoker's voice", "speaks of themselves in the third person", "pauses a long time before answering",
	"constantly hums", "fast and breathless", "formal to the point of comedy", "swears creatively",
}

var traits = []string{
	"generous to a fault", "deeply suspicious of magic", "cheerful no matter what", "easily flattered", "blunt",
	"collects gossip like coins", "terrified of dogs", "hates being interrupted", "quick to anger, quick to forgive",
	"always hungry", "superstitious", "a terrible liar", "loves a wager", "pious and judgemental", "lazy but clever",
	"fiercely protective of family", "melancholy", "boastful", "curious about everything", "polite but cold",
	"tells long stories", "absent-minded", "competitive about small things", "keeps meticulous lists",
}

var ideals = []string{
	"Fairness: everyone deserves the same chance.", "Freedom: no one should be owned.", "Tradition: the old ways keep us safe.",
	"Wealth: coin is the only honest measure.", "Charity: help those who cannot help themselves.", "Power: the strong decide.",
	"Knowledge: every secret wants to be known.", "Community: we stand or fall together.", "Honour: a promise is everything.",
	"Change: nothing should stay as it is.", "Faith: the gods have a plan.", "Survival: whatever it takes.",
}

var bonds = []string{
	"owes a debt to a local crime boss", "would die for their younger sibling", "searching for a parent who vanished",
	"keeps a promise made to a dying friend", "loyal to their guild above all", "in love with someone above their station",
	"protects a secret map", "devoted to a small shrine on the edge of town", "raising an orphan who isn't theirs",
	"sworn to avenge a burned village", "carries the ashes of a mentor", "tied to this land by an old oath",
}

var flaws = []string{
	"drinks too much", "can't resist a pretty trinket", "holds grudges forever", "a coward when it counts",
	"lies about their past", "trusts anyone in uniform", "gambles away every coin", "secretly hates their employer",
	"jealous of a rival", "talks when they should listen", "thinks they're always right", "steals small things without thinking",
}

var wants = []string{
	"to get out of this town", "a rare herb for a sick relative", "revenge on a former partner", "to be taken seriously",
	"a buyer for something stolen", "an escort through dangerous country", "to find a lost dog", "proof that a rival is cheating",
	"to join an adventuring party", "quiet, and to be left alone", "a letter delivered without questions", "to learn to read",
}

var secrets = []string{
	"where a smuggler's cache is buried", "that the mayor is being blackmailed", "the password to a thieves' den",
	"a back way into the old keep", "who really started the fire", "that the well water is poisoned", "a rumour about a dragon's lair",
	"the name of a hidden cult member", "nothing useful at all", "that a noble's heir is an impostor",
	"where the missing caravan went", "a song that opens a sealed door",
}

// ---- taverns ---------------------------------------------------------------

var tavernAdjectives = []string{"rusty", "golden", "drunken", "laughing", "prancing", "sleeping", "crooked", "silver", "jolly", "wandering", "howling", "black", "merry", "salty", "sunken", "lucky", "broken", "copper", "velvet", "grinning", "blind", "honest", "frozen", "hungry"}
var tavernNouns = []string{"dragon", "goat", "lantern", "kettle", "boar", "griffon", "mermaid", "anchor", "barrel", "crow", "stag", "fiddle", "tankard", "wyvern", "owl", "hound", "sword", "pony", "candle", "badger", "harp", "wheel", "maiden", "rooster"}
var tavernPlaces = []string{"rest", "den", "hall", "inn", "tap", "cellar", "hearth", "lodge", "haven", "house", "nest", "table"}

var tavernSpecials = []string{
	"a pepper stew that makes grown warriors weep", "honey cakes", "roast boar on feast days", "a fish pie of uncertain fish",
	"the cleanest beds in the province", "bare-knuckle fights on the back porch", "a bard who knows every song", "dice games that run all night",
	"an unbroken cellar of old wine", "mushroom soup from the deep caves", "goat's cheese with blue veins", "a pickled egg challenge",
}
var tavernDrinks = []string{
	"black ale thick enough to stand a spoon in", "sour cider", "spiced wine served hot", "a pale beer brewed with pine",
	"dwarven firewater, served by the thimble", "elven mead that tastes of summer", "watered-down everything", "a green liqueur nobody can name",
	"buttermilk, for the brave", "a smoky rye whisky", "barley wine, strong", "a sweet plum brandy",
}
var tavernFeatures = []string{
	"a stuffed owlbear by the fireplace", "a hearth big enough to roast an ox", "a mural of a famous battle, badly painted",
	"a notice board thick with jobs", "a private room upstairs that is always 'booked'", "a resident cat that picks favourites",
	"a cracked bell that customers ring for luck", "a wall of carved names", "low beams that everyone hits their head on",
	"a ship's figurehead over the bar", "a trapdoor behind the counter", "dozens of mismatched lanterns",
}
var tavernTonight = []string{
	"a wedding party has taken over", "a caravan just arrived and everyone is buying", "an arm-wrestling tournament",
	"a quiet night, two old regulars playing cards", "a group of off-duty guards drinking hard", "a travelling fortune teller in the corner",
	"a brawl is one insult away", "a merchant is hiring guards", "a bard is being booed", "a stranger in a hooded cloak watches the door",
	"the cook has quit; the food is worse than usual", "a noble is slumming it, badly disguised",
}
var rumours = []string{
	"Wolves have been seen walking on two legs near the old mill.", "The baron's tax collector hasn't come back from the hills.",
	"Someone pays good silver for unusual bones.", "The well in the square sings at midnight.", "A ship came in with no crew aboard.",
	"The temple's relic is a fake; the real one was sold.", "Lights move in the abandoned tower again.",
	"Mushrooms from the east wood give you true dreams.", "The miller's daughter can talk to crows.",
	"Bandits on the north road wear the duke's colours.", "A tomb opened by a rockslide is full of gold.",
	"The innkeeper waters the ale with river water.", "Three children went missing at the last full moon.",
	"A dragon was seen over the mountains; nobody believes it.", "There's a door in the sewers that wasn't there last year.",
	"The old witch in the marsh is offering a reward.",
}

// ---- treasure --------------------------------------------------------------

var gems = map[int][]string{
	10:   {"azurite", "banded agate", "blue quartz", "eye agate", "hematite", "lapis lazuli", "malachite", "moss agate", "obsidian", "rhodochrosite", "tiger eye", "turquoise"},
	50:   {"bloodstone", "carnelian", "chalcedony", "chrysoprase", "citrine", "jasper", "moonstone", "onyx", "quartz", "sardonyx", "star rose quartz", "zircon"},
	100:  {"amber", "amethyst", "chrysoberyl", "coral", "garnet", "jade", "jet", "pearl", "spinel", "tourmaline"},
	500:  {"alexandrite", "aquamarine", "black pearl", "blue spinel", "peridot", "topaz"},
	1000: {"black opal", "blue sapphire", "emerald", "fire opal", "opal", "star ruby", "star sapphire", "yellow sapphire"},
	5000: {"black sapphire", "diamond", "jacinth", "ruby"},
}
var gemLooks = []string{"cut and polished", "rough, still in its matrix", "set in a broken ring", "drilled for a necklace", "flawed but bright", "unusually large", "cloudy, with a bubble inside", "carved into a tiny skull"}

var artMaterials = [][]string{
	{"carved bone", "painted wooden", "copper", "pewter", "embroidered silk", "brass", "clay"},
	{"silver", "ivory", "gold-leafed", "bronze", "jade", "silk"},
	{"silver and gold", "obsidian", "enamelled", "gilded ivory", "coral", "electrum"},
	{"gold", "platinum-inlaid", "jewelled silver", "mithral-trimmed", "crystal"},
	{"jewelled gold", "platinum", "star-metal", "adamantine-inlaid gold"},
}
var artThings = []string{"statuette", "goblet", "comb", "mirror", "mask", "music box", "chalice", "bracelet", "brooch", "crown", "dagger", "censer", "tapestry", "idol", "pendant", "ewer", "chess set", "reliquary", "hand fan", "drinking horn"}
var artDetails = [][]string{
	{"with a crude hunting scene", "worn smooth by handling", "shaped like a fish", "with a maker's mark nobody knows"},
	{"engraved with vines", "showing a pair of dancers", "with a hidden compartment", "decorated with tiny bells"},
	{"set with small pearls", "carved with a coronation", "showing the night sky", "inlaid with mother-of-pearl"},
	{"set with emeralds", "bearing a royal crest", "depicting a dragon in flight", "with rubies for eyes"},
	{"set with a single great diamond", "once belonging to an emperor", "that chimes softly when moved", "encrusted with sapphires"},
}
var artConditions = []string{"perfect", "tarnished", "chipped", "dented", "missing a piece", "recently cleaned", "caked in grime", "repaired once, skilfully"}

var hoardPlaces = []string{
	"in an iron-bound chest", "scattered in a nest of bones", "in sacks under a loose flagstone", "piled on a stone altar",
	"in a rotting wardrobe", "inside a hollow statue", "in a sunken pool", "in clay jars sealed with wax",
	"in a locked strongbox (DC 15 to pick)", "heaped carelessly in a corner",
}
var purseKinds = []string{"a leather pouch", "a sock", "a belt purse", "a hidden boot pocket", "a tin box", "a twist of cloth", "a fine silk purse"}
var pocketThings = []string{
	"a love letter, unsent", "a crude map with an X", "a set of loaded dice", "a key with a tag reading '4'", "a vial of cheap perfume",
	"a lucky rabbit's foot", "a wanted poster with their own face", "a half-eaten sausage", "a tooth wrapped in paper", "a copper ring",
}

var bookBindings = []string{
	"black leather with silver corners", "oak boards and brass clasps", "red-dyed hide, much scuffed", "sharkskin, water-stained",
	"plain canvas, labelled 'Recipes'", "blue velvet gone threadbare", "bone covers lashed with sinew", "thin copper plates on rings",
}
var bookQuirks = []string{
	"the margins are full of furious notes", "every page smells faintly of lavender", "a pressed flower marks one spell",
	"the last pages are torn out", "written in a cipher (DC 15 Investigation to read)", "the ink glows faintly in the dark",
	"a previous owner's name is crossed out", "has a small lock with no key", "illustrations move when no one is looking",
	"a shopping list is scrawled on the inside cover",
}

// ---- encounters ------------------------------------------------------------

var encounterActivities = []string{
	"hunting", "resting", "guarding something", "arguing among themselves", "lying in ambush", "fleeing something worse",
	"feeding on a kill", "on patrol", "lost and confused", "building or digging", "worshipping at a shrine", "fighting another creature",
	"carrying loot", "sleeping", "searching for someone", "setting a trap",
}

// ---- traps -----------------------------------------------------------------

type trapEffect struct {
	kind   string
	name   string
	text   string
	attack bool
	save   string
	damage string
	extra  string
	noHalf bool
}

var trapEffects = []trapEffect{
	{kind: "mechanical", name: "dart volley", text: "Darts shoot from holes in the wall.", attack: true, damage: "piercing", extra: "the darts may be poisoned (DC 12 Constitution or poisoned for an hour)"},
	{kind: "mechanical", name: "hidden pit", text: "The floor gives way into a pit lined with spikes.", save: "Dexterity", damage: "piercing", extra: "a creature that fails falls in", noHalf: true},
	{kind: "mechanical", name: "swinging blade", text: "A blade scythes out of a slot in the wall.", attack: true, damage: "slashing"},
	{kind: "mechanical", name: "falling block", text: "A stone block drops from the ceiling.", save: "Dexterity", damage: "bludgeoning", extra: "the block then blocks the passage"},
	{kind: "mechanical", name: "collapsing ceiling", text: "Supports give way and the ceiling comes down.", save: "Dexterity", damage: "bludgeoning", extra: "the area becomes difficult terrain"},
	{kind: "mechanical", name: "poison needle", text: "A needle springs from the lock.", attack: true, damage: "poison", extra: "DC 13 Constitution or poisoned for an hour"},
	{kind: "mechanical", name: "flooding room", text: "Doors slam shut and water pours in.", save: "Strength", damage: "bludgeoning", extra: "the room fills over ten rounds"},
	{kind: "mechanical", name: "rolling sphere", text: "A huge stone ball rolls down the corridor.", save: "Dexterity", damage: "bludgeoning", extra: "it keeps rolling each round until stopped"},
	{kind: "mechanical", name: "spiked wall", text: "A wall of spikes slides out toward the party.", save: "Dexterity", damage: "piercing"},
	{kind: "mechanical", name: "net drop", text: "A weighted net falls from above.", save: "Dexterity", damage: "bludgeoning", extra: "a creature that fails is restrained (DC 10 Strength to escape)"},
	{kind: "mechanical", name: "gas vent", text: "Choking gas hisses out of the floor.", save: "Constitution", damage: "poison"},
	{kind: "mechanical", name: "crossbow battery", text: "A rack of crossbows fires down the hall.", attack: true, damage: "piercing"},
	{kind: "magical", name: "fire glyph", text: "A glyph flares and engulfs the area in flame.", save: "Dexterity", damage: "fire"},
	{kind: "magical", name: "lightning arc", text: "Lightning leaps between two metal statues.", save: "Dexterity", damage: "lightning"},
	{kind: "magical", name: "frost rune", text: "A rune sheds a burst of killing cold.", save: "Constitution", damage: "cold", extra: "speed halved until the end of the next turn"},
	{kind: "magical", name: "necrotic mist", text: "A grey mist drains the warmth from living things.", save: "Constitution", damage: "necrotic"},
	{kind: "magical", name: "thunder door", text: "The door booms like a struck gong.", save: "Constitution", damage: "thunder", extra: "a creature that fails is pushed 10 feet and deafened for a minute"},
	{kind: "magical", name: "acid sigil", text: "The floor weeps acid.", save: "Dexterity", damage: "acid"},
	{kind: "magical", name: "psychic scream", text: "A carved face shrieks into the minds of those nearby.", save: "Wisdom", damage: "psychic", extra: "a creature that fails is frightened for a minute"},
	{kind: "magical", name: "radiant eye", text: "A painted eye opens and burns with holy light.", save: "Constitution", damage: "radiant", extra: "undead and fiends have disadvantage on the save"},
	{kind: "magical", name: "teleport snare", text: "A circle on the floor flashes and the creature on it vanishes.", save: "Charisma", damage: "force", extra: "a creature that fails is teleported to a cell elsewhere in the dungeon"},
	{kind: "magical", name: "animated guardians", text: "Suits of armour lurch to life.", attack: true, damage: "slashing", extra: "they attack each round for three rounds"},
}

var trapTriggers = map[string][]string{
	"mechanical": {"a pressure plate under a flagstone", "a trip wire at ankle height", "opening the door", "lifting the treasure from its plinth", "turning the wrong key in the lock", "pulling the lever", "stepping onto the third stair", "opening the chest lid"},
	"magical":    {"crossing the threshold", "touching the inscription", "speaking above a whisper", "carrying a light source past the arch", "a living creature entering the circle", "reading the scroll aloud", "opening the book", "anyone not wearing the temple's symbol passing"},
}
var trapPlaces = []string{"in a narrow corridor", "at a doorway", "on a treasure chest", "in a stairwell", "on a bridge over a chasm", "around an altar", "in a vault", "at a junction of four passages", "in a library alcove", "beside a fountain"}
var trapTells = map[string][]string{
	"mechanical": {"scratches on the floor", "tiny holes in the wall", "a flagstone that sits a little high", "a glint of wire", "old bloodstains", "a draught from a hidden slot"},
	"magical":    {"faint lettering in the dust", "a tingle on the skin", "a smell of ozone", "frost in the mortar", "burn marks around the arch", "a hum just at the edge of hearing"},
}
var trapBypasses = []string{
	"a hidden catch on the left side", "step only on the darker stones", "speak the builder's name", "wedge the plate with a spike",
	"the guards carry a token that suppresses it", "it can only fire once a day", "crawl beneath the line of fire", "douse all light",
}

// ---- oddities --------------------------------------------------------------

var trinkets = []string{
	"a glass eye that always looks north", "a tin soldier missing its head", "a key carved from a single tooth", "a tiny bell that makes no sound",
	"a love letter in a language you don't know", "a jar holding a single, never-wilting rose petal", "a coin with two tails",
	"a wooden whistle shaped like a frog", "a lock of silver hair tied with red thread", "a chess piece made of black glass",
	"a map of a city that doesn't exist", "a thimble that is always warm", "a small brass compass with no needle",
	"a pressed moth with gold wings", "a dried seahorse", "a ring too small for any finger", "a button from a nobleman's coat",
	"a deck of cards with one extra queen", "a candle stub that relights itself once", "a spoon bent into a spiral",
	"a feather that floats a little too slowly", "a snail shell with a tiny door painted on", "an hourglass filled with black sand",
	"a clay pipe in the shape of a dragon", "a page torn from a prayer book", "a fragment of stained glass", "a wax seal with an unknown crest",
	"a lens that makes everything look blue", "a doll's shoe", "a small stone with a hole through it", "a vial of sand that shifts on its own",
	"a ribbon that is always damp", "a carved wooden bird with one wing", "a pebble that hums when held", "a monocle with a cracked lens",
	"a tooth from something large", "a scroll case with a note: 'Not yet'", "a copper cricket that chirps at dawn", "a die whose faces are all sixes",
	"a broken arrowhead of black stone", "a lock with no keyhole", "a tiny painting of you, a little older", "a stoppered bottle of fog",
	"an acorn made of iron", "a small mirror that shows the room behind you a moment late", "a knotted cord with seven beads",
	"a sealed envelope addressed to 'whoever finds this'", "a cat's collar with a name tag: 'Lord Whiskers'", "a pocket watch that runs backwards",
	"a seashell that whispers your name", "half of a silver locket", "a bird skull painted gold", "a wooden token from a closed gambling hall",
}

var surges = []string{
	"You turn bright blue for a minute.", "A flock of harmless butterflies bursts from your sleeves.", "Your hair grows a foot and turns white.",
	"You cast fog cloud centred on yourself.", "For the next minute, you can only speak in rhymes.", "You teleport up to 30 feet to a space you can see.",
	"You regain 2d10 hit points.", "Each creature within 30 feet of you takes 1d10 thunder damage.", "You glow with bright light for a minute.",
	"A random creature within 60 feet is turned into a newt for a minute.", "You levitate 5 feet off the ground for a minute.",
	"Every flame within 30 feet goes out.", "You become invisible until you attack or cast a spell.", "Flowers sprout in every square around you.",
	"Your voice booms three times louder for a minute.", "You are frightened of the nearest creature until the end of your next turn.",
	"You can see invisible creatures for a minute.", "You grow a long tail for an hour.", "A spectral cat follows you for a day.",
	"The next spell you cast in the next minute is cast at one level higher.", "You shrink by half for a minute.",
	"All your coins turn into copper. They change back after an hour.", "Snow falls in a 20-foot radius around you for a minute.",
	"You can breathe underwater for an hour.", "You cast grease centred on yourself.", "You smell overwhelmingly of roses for a day.",
	"Gravity reverses for you alone for one round; you fall up 10 feet and then back down.", "You and a random creature within 30 feet swap places.",
	"Your skin turns to bark: +2 AC for a minute, speed reduced by 10 feet.", "Every creature within 20 feet must succeed on a DC 13 Wisdom save or laugh for a round.",
	"You regain one expended spell slot of the lowest level.", "You take 1d6 force damage as the magic backfires.",
	"Small rocks orbit your head for a minute; anyone hitting you in melee takes 1 bludgeoning damage.", "You become resistant to all damage for one round.",
	"Your shadow detaches and wanders off. It returns at dawn.", "A loud trumpet sounds from nowhere.", "You cannot lie for the next hour.",
	"You speak only Sylvan for the next minute.", "Your eyes glow red for a day.", "A friendly duck appears and follows you around for a day.",
	"You cast entangle centred on yourself.", "You gain the ability to walk on walls for a minute.", "The temperature around you drops sharply; your breath frosts for an hour.",
	"Each creature within 15 feet of you is pushed 10 feet away.", "You age 1d10 years. It wears off after a long rest.", "Nothing happens. Or does it?",
}

var carousing = []string{
	"You wake up in jail, accused of insulting a magistrate.", "You win a bet with {who} and are owed a favour.",
	"You lose your boots in a game of cards.", "You make a new friend: {who}, who swears eternal loyalty.",
	"You are now engaged to {who}. You don't remember proposing.", "You wake up with a tattoo of a fish.",
	"You win a small house in a dice game; it may be haunted.", "You insult a member of the thieves' guild.",
	"You start a brawl and are banned from the tavern.", "You join a secret society. They expect you at the next meeting.",
	"You hear a rumour about a treasure from {who}.", "You lose 20 gp to a pickpocket.", "You are challenged to a duel at dawn by {who}.",
	"You end up on a ship that is now leaving the harbour.", "A local bard writes a song mocking you; it is popular.",
	"You wake in a temple, having pledged yourself to its god.", "You win a drinking contest and earn a nickname.",
	"You buy a horse. It is probably stolen.", "{who} now owes you 50 gp and has left town.", "You learn the location of a hidden shrine.",
}
var carousingChecks = []string{"DC 10 Constitution to remember any of it", "DC 12 Charisma to keep your reputation", "DC 15 Wisdom to avoid a hangover", "no check, it simply happened"}

var hooks = []string{
	"{who} offers good coin to recover {thing} from {place}.", "A child begs for help: their family has been taken to {place}.",
	"{who} has been murdered, and the only clue points to {place}.", "Strange lights have been seen over {place} every night this week.",
	"{who} needs an escort to {place} and won't say why.", "A map found on a dead courier leads to {place}.",
	"{who} wants {thing} destroyed before the next new moon.", "Someone is selling forged deeds to {place}; {who} wants it stopped.",
	"The sickness spreading in town started at {place}.", "{who} has stolen {thing} and fled toward {place}.",
	"An old friend's letter asks you to meet at {place} - but they died a year ago.", "A rival party set out for {place} a week ago and never came back.",
}
var hookPlaces = []string{"the drowned abbey", "an abandoned mine", "the old keep on the hill", "the goblin warrens", "a sunken temple", "the haunted manor", "a tower in the marsh", "the barrow mounds", "a smuggler's cove", "the ruins of an elven city", "a cave behind the waterfall", "the sewers under the city"}
var hookThings = []string{"a stolen holy relic", "a family heirloom", "a sealed letter", "a cursed sword", "a dragon's egg", "a missing ledger", "a crown of black iron", "the deed to the mill", "a jar of preserved hearts", "a talking skull"}
var hookRewards = []string{"100 gp", "a minor magic item", "a plot of land", "a noble's favour", "free lodging for life", "a map to something better", "membership in a guild", "250 gp and a horse", "a share of what's found", "a knighthood (they claim)"}
var hookTwists = []string{
	"the patron is the villain", "the thing they want back was never theirs", "a rival party is after the same prize", "the place is not abandoned",
	"the victim isn't dead", "it is all a test by a secret order", "the monster is protecting something innocent", "the reward is counterfeit",
	"someone in the party has been there before", "there is no twist, just a lot of goblins",
}
