package randgen

// The dungeon's tables. Our own, like tables.go.

var roomPurposes = []string{
	"Guard room", "Barracks", "Storeroom", "Shrine", "Kitchen", "Library", "Prison cells", "Armory",
	"Crypt", "Workshop", "Throne room", "Well room", "Dining hall", "Torture chamber", "Laboratory",
	"Chapel", "Larder", "Lair", "Trophy room", "Bedchamber", "Gallery", "Ossuary", "Vault", "Cistern",
	"Meditation chamber", "Map room", "Kennels", "Forge", "Scriptorium", "Audience hall",
}

var cavePurposes = []string{
	"Dripping grotto", "Fungus cave", "Bat roost", "Crystal hollow", "Echoing cavern", "Mud pit",
	"Collapsed passage", "Underground pool", "Bone-strewn den", "Narrow fissure", "Glowworm gallery",
	"Smoke-blackened cave", "Chasm edge", "Mineral terraces", "Root-choked hollow",
}

var entrancePurposes = []string{"Entrance hall", "Gatehouse", "Antechamber", "Collapsed entry", "Vestibule", "Landing"}

var purposeProps = map[string][]string{
	"Shrine": {"altar", "brazier"}, "Chapel": {"altar", "statue"}, "Crypt": {"sarcophagus", "sarcophagus"},
	"Ossuary": {"bones", "bones"}, "Library": {"table"}, "Scriptorium": {"table"}, "Kitchen": {"barrel", "table"},
	"Larder": {"barrel", "barrel"}, "Storeroom": {"barrel", "chest"}, "Barracks": {"bed", "bed"},
	"Bedchamber": {"bed"}, "Throne room": {"throne", "statue"}, "Audience hall": {"throne"},
	"Well room": {"well"}, "Cistern": {"well"}, "Dining hall": {"table"}, "Laboratory": {"table", "brazier"},
	"Forge": {"brazier"}, "Gallery": {"statue", "statue"}, "Trophy room": {"statue"}, "Vault": {"chest"},
	"Torture chamber": {"brazier"}, "Lair": {"bones"}, "Bone-strewn den": {"bones"}, "Underground pool": {"well"},
	"Entrance hall": {"statue"}, "Gatehouse": {"brazier"},
}

var roomDressing = []string{
	"dust lies thick on every surface", "the walls are scored with old claw marks", "a cold draught blows from somewhere",
	"water drips steadily from the ceiling", "the floor is littered with broken pottery", "faded murals cover the walls",
	"the smell of rot hangs in the air", "torch brackets line the walls, all empty", "cobwebs hang in heavy curtains",
	"the flagstones are cracked and uneven", "a pile of rubble blocks one corner", "the ceiling is lost in darkness",
	"scorch marks blacken the floor", "someone has scratched tally marks by the door", "moss grows thick between the stones",
	"the air is warm and close", "old bloodstains trail across the floor", "a broken table lies on its side",
	"bones are swept into one corner", "the walls glisten with damp", "a faint humming comes from the stone",
	"rusted chains hang from the walls", "fresh footprints cross the dust", "an overturned chest lies empty",
	"rotting tapestries hang in rags", "the room is bitterly cold", "a guttering candle burns in a niche",
	"ash and cinders cover the floor", "crude graffiti is daubed on the walls", "the floor slopes noticeably",
}

var motifDressing = map[string][]string{
	"undead":    {"coffins stand open along the walls", "the dead are stacked in alcoves", "grave dust drifts underfoot", "a smell of embalming oils lingers"},
	"aberrant":  {"the walls pulse faintly, like flesh", "strange slime coats the stone", "the geometry of the room feels wrong", "whispers come from nowhere"},
	"giant":     {"everything is built at twice human scale", "a huge stone seat dominates the room", "gnawed cattle bones lie about", "enormous footprints mark the floor"},
	"vermin":    {"droppings carpet the floor", "the walls rustle with movement", "nests of rags fill the corners", "something skitters away from the light"},
	"aquatic":   {"knee-deep water covers the floor", "barnacles crust the walls", "the room smells of brine", "seaweed hangs from the ceiling"},
	"desert":    {"sand has drifted in through cracks", "the air is dry as old bone", "hieroglyphs cover the walls", "a sun disc is carved into the ceiling"},
	"underdark": {"phosphorescent fungus lights the room", "stalactites hang low", "a chasm splits the floor", "strange spider-silk covers the doorway"},
	"arcane":    {"runes glow faintly on the floor", "floating motes of light drift about", "shelves of broken glassware line the walls", "a summoning circle is chalked on the floor"},
	"fire":      {"the floor is hot to the touch", "a vent breathes sulphurous smoke", "veins of magma glow in the walls", "everything is coated in soot"},
	"cold":      {"frost rimes every surface", "icicles hang like teeth", "the floor is slick with ice", "breath freezes in the air"},
	"abyssal":   {"the walls weep a black ichor", "screaming faces are carved into the stone", "the shadows move on their own", "a stench of sulphur and rot"},
	"infernal":  {"iron chains and hooks hang everywhere", "contracts are nailed to the walls", "a brazier of green flame burns", "the floor is tiled in red and black"},
	"abandoned": {"everything is thick with dust", "furniture has collapsed with age", "roots have broken through the ceiling", "a long-dead campfire sits in the middle"},
}

type roomFeature struct {
	text, note, prop string
}

var roomFeatures = []roomFeature{
	{"A fountain of clear water. Drinking from it heals 1d8 hit points once a day.", "A fountain still runs with clear water", "fountain"},
	{"A statue of a forgotten hero, one hand outstretched. Placing a coin in it opens a hidden niche.", "A statue reaches out an empty hand", "statue"},
	{"An altar to a dark god. Anyone touching it must make a DC 13 Wisdom save or be frightened for a minute.", "A black altar thrums with menace", "altar"},
	{"A well that drops into darkness. A rope descends 60 feet to an underground stream.", "A deep well drops into darkness", "well"},
	{"A brazier of cold blue flame that never goes out and casts no heat.", "A brazier burns with cold blue fire", "brazier"},
	{"A mosaic on the floor shows a map of the dungeon, with one room marked.", "A floor mosaic maps the halls", ""},
	{"A mirror that shows the room as it was centuries ago.", "A mirror shows the past", "statue"},
	{"A sealed sarcophagus. Its occupant is long gone, but a silver ring remains (25 gp).", "A sarcophagus lies sealed", "sarcophagus"},
	{"A pool of still water reflects a starry sky that is not there.", "A still pool reflects strange stars", "well"},
	{"A throne carved from a single block of obsidian. Whoever sits in it hears the last king's voice.", "An obsidian throne waits", "throne"},
	{"A collapsed section of floor opens onto the level below.", "The floor has fallen through", "rubble"},
	{"A talking skull on a shelf, which answers one question a day, truthfully but unhelpfully.", "A skull on a shelf is watching", "bones"},
}

var trapNotes = []string{
	"The flagstones here are suspiciously clean.", "Something about this room feels wrong.", "A skeleton lies just inside the door.",
	"The walls are pocked with small holes.", "Faint runes glow underfoot.", "A thin wire glints in the torchlight.",
}

var treasureNotes = []string{
	"Something glitters beneath the rubble.", "A loose stone in the wall hides a cache.", "Coins are scattered across the floor.",
	"A locked strongbox sits in plain sight.", "A false bottom hides something in the chest.",
}

var hiddenPlaces = []string{
	"hidden behind a loose stone (DC 15 to find)", "under a false floor", "in a locked iron box", "inside a hollow statue",
	"buried under rubble", "sewn into an old mattress", "in a niche high on the wall", "at the bottom of a cistern",
}

// ---- story -----------------------------------------------------------------

var storyPlaces = map[string][]string{
	"none":      {"halls", "vaults", "keep", "warrens", "catacombs", "sanctum", "dungeon", "cellars", "labyrinth", "stronghold"},
	"undead":    {"tomb", "crypt", "ossuary", "barrow", "necropolis", "mausoleum"},
	"aberrant":  {"hollow", "pit", "depths", "grotto", "nest"},
	"giant":     {"hall", "stronghold", "steading", "fortress"},
	"vermin":    {"burrows", "nest", "warrens", "sewers"},
	"aquatic":   {"cistern", "grotto", "drowned halls", "sunken temple"},
	"desert":    {"pyramid", "tomb", "buried temple", "sand vaults"},
	"underdark": {"depths", "chasm", "underways", "deep halls"},
	"arcane":    {"tower", "laboratory", "sanctum", "observatory"},
	"fire":      {"forge", "furnace", "caldera", "pits"},
	"cold":      {"ice caves", "frozen halls", "glacier vaults", "rime keep"},
	"abyssal":   {"pit", "temple", "rift", "maw"},
	"infernal":  {"citadel", "court", "chapel", "prison"},
	"abandoned": {"ruins", "mine", "outpost", "monastery"},
}
var storyAdjectives = []string{"lost", "forgotten", "sunken", "black", "silent", "shattered", "hollow", "drowned", "burning", "cursed", "nameless", "whispering", "crimson", "iron", "pale", "endless"}
var storyNouns = []string{"king", "serpent", "moon", "flame", "crown", "witch", "hound", "star", "bell", "saint", "wyrm", "veil"}

var storyPast = []string{
	"{who} built the {place} to hide a terrible secret.", "Long ago the {place} was a refuge for those fleeing a war.",
	"The {place} was sealed after {who} vanished inside.", "Monks once kept a holy relic in the {place}.",
	"Dwarves dug the {place} chasing a vein of silver.", "{who} was buried in the {place} with all their wealth.",
	"The {place} was a prison for those the crown wanted forgotten.", "A cult of the old gods met in the {place} in secret.",
	"The {place} was a wizard's retreat until the experiments went wrong.", "{who} died defending the {place} against an army.",
}
var storyNow = []string{
	"Now it has been taken over by something hungry.", "Now strange lights are seen at its entrance.",
	"Now its doors stand open for the first time in a century.", "Now the dead do not rest within it.",
	"Now bandits use it as their hideout.", "Now an earthquake has opened a new way in.",
	"Now the villagers nearby are going missing.", "Now a rival party has gone in and not come out.",
}
var motifNow = map[string][]string{
	"undead":   {"Now the dead walk its halls once more.", "Now a necromancer has woken its sleepers."},
	"aberrant": {"Now something from beyond the stars has made it its nest.", "Now its stones whisper to those who sleep nearby."},
	"vermin":   {"Now it crawls with vermin grown huge and bold.", "Now a swarm boils out of it every night."},
	"fire":     {"Now it burns from within, and the ground around it smokes.", "Now fire elementals guard its forges."},
	"cold":     {"Now it is locked in ice that never melts.", "Now a winter that will not end spreads from it."},
	"arcane":   {"Now its wards are failing, and wild magic leaks out.", "Now its guardians still follow orders no one remembers."},
	"aquatic":  {"Now it is half flooded, and something swims in the dark water.", "Now the tide reaches it and brings strange visitors."},
	"abyssal":  {"Now a rift to the Abyss has opened in its depths.", "Now demons pour from its lowest level."},
	"infernal": {"Now devils hold court in its halls.", "Now a contract signed there is coming due."},
}
var storyHooks = []string{
	"A reward is offered for whoever clears it.", "A map to it was found on a dead man.", "Someone the party loves is inside.",
	"It holds the only cure for a spreading plague.", "A sage will pay well for what is written on its walls.",
	"The local lord wants it sealed for good.", "Its treasure is said to be beyond counting.", "A prophecy names one of the party as its heir.",
}
