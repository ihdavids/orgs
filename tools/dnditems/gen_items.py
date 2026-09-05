#!/usr/bin/env python3
"""Generate the orgs dnd item catalog org files from the reForged SRD markdown.

Usage:
    python3 tools/dnditems/gen_items.py [path-to-dndsrd] [output-dir]

Defaults to ../dndsrd and docs/dnd.

Emits one org file per *source*:

    dnd-items-srd.org        every item in SRD 5.1 (mundane + magic)
    dnd-items-homebrew.org   hand maintained, seeded once and never overwritten
    dnd-items-index.org      the master searchable table, built from all sources

Every item is an org heading carrying a PROPERTIES drawer of normalised
columns, a flavor block, and a Capabilities definition list. The index table is
generated from those drawers, so anything you add to a source file - homebrew
included - shows up in the table on the next run.

The SRD material is the System Reference Document 5.1 by Wizards of the Coast,
licensed under CC-BY-4.0. The generated org carries that attribution.
"""

import os
import re
import sys

SRD = sys.argv[1] if len(sys.argv) > 1 else os.path.join("..", "dndsrd")
OUT = sys.argv[2] if len(sys.argv) > 2 else os.path.join("docs", "dnd")


# ---------------------------------------------------------------------------
# text helpers (same conventions as tools/dndsrd/gen_srd.py)
# ---------------------------------------------------------------------------

def slug(name):
    name = name.lower().strip()
    name = name.replace("'", "").replace("’", "")
    name = re.sub(r"[^a-z0-9]+", "-", name)
    return name.strip("-")


def clean(text):
    """Normalise markdown inline formatting into plain text."""
    text = text.replace("’", "'").replace("‘", "'")
    text = text.replace("“", '"').replace("”", '"')
    text = text.replace("—", " - ").replace("–", "-")
    text = text.replace("­", "").replace("‑", "-")
    text = text.replace("½", ".5").replace("¼", ".25").replace("¾", ".75")
    text = re.sub(r"\*\*\*(.+?)\*\*\*", r"\1", text)
    text = re.sub(r"\*\*(.+?)\*\*", r"\1", text)
    text = re.sub(r"\*(.+?)\*", r"\1", text)
    text = re.sub(r"\[\[(.+?)\]\]", r"\1", text)
    text = re.sub(r"\[(.+?)\]\(.+?\)", r"\1", text)
    text = re.sub(r"[ \t]+", " ", text)
    return text.strip()


def paragraphs(text):
    return [b.strip() for b in re.split(r"\n\s*\n", text) if b.strip()]


def read(*parts):
    path = os.path.join(SRD, *parts)
    with open(path, encoding="utf-8") as fh:
        return fh.read()


def parse_weight(s):
    s = s.replace("lb.", "").replace("lb", "").strip()
    s = s.replace("½", ".5").replace("¼", ".25").replace("¾", ".75")
    s = s.replace("1/2", ".5").replace("1/4", ".25").replace(",", "")
    if not s or s == "-":
        return ""
    try:
        v = float(s)
        return str(int(v)) if v == int(v) else str(v)
    except ValueError:
        return ""


COIN = {"cp": 0.01, "sp": 0.1, "ep": 0.5, "gp": 1.0, "pp": 10.0}


def parse_cost(s):
    """Return the cost normalised to gold pieces, as a string, for sorting."""
    s = clean(s).replace(",", "").strip()
    m = re.match(r"^([\d.]+)\s*(cp|sp|ep|gp|pp)$", s, re.I)
    if not m:
        return ""
    v = float(m.group(1)) * COIN[m.group(2).lower()]
    return str(int(v)) if v == int(v) else str(v)


def tables(text):
    """Return a list of (caption, headers, rows) for every markdown table."""
    out = []
    lines = text.split("\n")
    caption = ""
    i = 0
    while i < len(lines):
        line = lines[i].strip()
        cap = re.match(r"^\*\*Table-\s*(.+?)\*\*$", line)
        if cap:
            caption = cap.group(1).strip()
            i += 1
            continue
        if line.startswith("|") and i + 1 < len(lines) and re.match(r"^\|[-: |+]+\|$", lines[i + 1].strip()):
            headers = [clean(c.strip()) for c in line.strip("|").split("|")]
            rows = []
            j = i + 2
            while j < len(lines) and lines[j].strip().startswith("|"):
                cells = [clean(c.strip()) for c in lines[j].strip().strip("|").split("|")]
                if any(c for c in cells):
                    rows.append(cells)
                j += 1
            out.append((caption, headers, rows))
            caption = ""
            i = j
            continue
        i += 1
    return out


def find_table(text, name_part):
    for cap, headers, rows in tables(text):
        if name_part.lower() in cap.lower():
            return headers, rows
    return None, None


def trait_map(text):
    """Pull ***Name***. body paragraphs into a dict keyed by lowered name."""
    out = {}
    for para in paragraphs(text):
        m = re.match(r"^\*\*\*(.+?)\*\*\*\.?\s*(.*)$", para, re.S)
        if m:
            out[m.group(1).strip().lower().rstrip(".")] = clean(m.group(2))
    return out


# ---------------------------------------------------------------------------
# the item model
# ---------------------------------------------------------------------------

# Every column the index table can carry, in table order. The PROPERTIES drawer
# uses the same keys, so a hand written homebrew entry only has to fill in the
# drawer to appear correctly in the table.
COLUMNS = [
    ("name",       "Item"),
    ("source",     "Source"),
    ("category",   "Category"),
    ("kind",       "Kind"),
    ("subtype",    "Subtype"),
    ("group",      "Group"),
    ("rarity",     "Rarity"),
    ("tier",       "Tier"),
    ("level",      "Level"),
    ("attunement", "Attune"),
    ("bonus",      "Bonus"),
    ("damage",     "Damage"),
    ("damagetype", "Dmg Type"),
    ("ac",         "AC"),
    ("charges",    "Charges"),
    ("range",      "Range"),
    ("traits",     "Traits"),
    ("cost",       "Cost"),
    ("costgp",     "Cost (gp)"),
    ("weight",     "Weight"),
]

# Rarity -> the character levels the item is usually handed out at. This is the
# long standing table convention for pacing magic item rewards, recorded here so
# the Level column can be searched; it is a derived planning aid, not SRD text.
TIERS = {
    "common":    ("1-4",   "1"),
    "uncommon":  ("1-4",   "1"),
    "rare":      ("5-10",  "5"),
    "very rare": ("11-16", "11"),
    "legendary": ("17-20", "17"),
    "artifact":  ("17-20", "17"),
}


class Item(dict):
    def __init__(self, **kw):
        super().__init__()
        for key, _ in COLUMNS:
            self[key] = ""
        self["flavor"] = ""
        self["capabilities"] = []   # list of (label, value)
        self["rules"] = []          # list of paragraphs
        self.update(kw)

    def set_rarity(self, rarity):
        self["rarity"] = rarity
        tier, level = TIERS.get(rarity, ("", ""))
        self["tier"] = tier
        self["level"] = level


def cost_caps(item):
    """Cost and weight close out the capability block on every mundane item."""
    out = []
    if item["cost"]:
        out.append(("Cost", item["cost"]))
    if item["weight"]:
        out.append(("Weight", "%s lb." % item["weight"]))
    return out


# mundane prices for items that also appear in the magic item list
PRICED_MAGIC = {}

ITEMS = []


def add(item):
    if not item.get("id"):
        item["id"] = slug(item["name"])
    ITEMS.append(item)
    return item


# ---------------------------------------------------------------------------
# authored flavor for the mundane gear the SRD lists as a bare table row
#
# The SRD prices and weights a longsword but never describes one, so these one
# line descriptions are written for this catalog rather than taken from the SRD.
# ---------------------------------------------------------------------------

FLAVOR = {
    # simple melee
    "club": "A shaped length of hardwood, heavy at one end. The weapon of tavern brawls, town militias and anyone who could not afford steel.",
    "dagger": "A hand's length of sharpened steel. Quick, quiet, easily hidden, and just as happy leaving your hand as staying in it.",
    "greatclub": "A tree limb barely worked into a weapon, swung in wide arcs that break bone through armor.",
    "handaxe": "A short hafted axe balanced for the throw as much as the swing. Woodcutter's tool and skirmisher's favorite alike.",
    "javelin": "A light spear made to be thrown, its slim iron head punching through shields at a run.",
    "light-hammer": "A one handed smith's hammer pressed into martial service, compact enough to hurl at a fleeing target.",
    "mace": "A flanged head on a short steel haft, designed to ruin armor and the person inside it without needing an edge.",
    "quarterstaff": "A stout pole of ash or oak, shod at both ends. Humble, legal in every city, and lethal in trained hands.",
    "sickle": "A curved harvesting blade. In desperate hands it reaps something other than grain.",
    "spear": "The oldest weapon there is: a point on a shaft, keeping trouble at arm's length or flying to meet it.",
    # simple ranged
    "crossbow-light": "A shoulder stocked bow drawn by a lever, slow to load but deadly in the hands of someone who has never trained with a bow.",
    "dart": "A weighted needle of a weapon, flicked from the fingers faster than the eye follows.",
    "shortbow": "A compact recurve favored by scouts and riders, easy to carry through brush and easy to draw from the saddle.",
    "sling": "A leather cradle on two cords. Ammunition is any stone at your feet, which is why shepherds and beggars fight well.",
    # martial melee
    "battleaxe": "A broad crescent head on a hand and a half haft, capable of splitting a shield or the arm holding it.",
    "flail": "A spiked head chained to a handle, swinging around a raised guard to strike what a sword cannot reach.",
    "glaive": "A single edged blade mounted on a long pole, sweeping a wide arc that keeps an entire rank at bay.",
    "greataxe": "An enormous double bit axe swung with the whole body. Slow to recover, devastating on contact.",
    "greatsword": "Four feet of double edged steel meant for two hands and open ground, cleaving through formations.",
    "halberd": "An axe head, a spike and a hook on a long shaft, the polearm that made peasant levies a threat to knights.",
    "lance": "A long tapering shaft couched under the arm, which turns a charging horse into the weapon and the rider into the aim.",
    "longsword": "The knight's weapon: straight, double edged and versatile, wielded in one hand behind a shield or two for power.",
    "maul": "A sledgehammer built for war, transferring every ounce of its head through plate and into the body beneath.",
    "morningstar": "A spiked ball fixed to a haft, its points concentrating a blow into a single armor piercing spike.",
    "pike": "An eighteen foot spear held in ranks, presenting a hedge of points that cavalry will not ride into.",
    "rapier": "A slender thrusting blade with a caged hilt, built for duels of speed rather than battles of strength.",
    "scimitar": "A curved single edged blade that cuts on the draw, light enough to keep a second one in the off hand.",
    "shortsword": "A broad leaf bladed stabbing sword for close press, where a longer weapon has nowhere to go.",
    "trident": "A three tined spear out of the fishing boats, its barbs made to hold what they catch.",
    "war-pick": "A narrow beak of hardened steel on a short haft, punching a hole clean through plate.",
    "warhammer": "A hammer face on one side, a crow's beak on the other, made to answer the rise of heavy armor.",
    "whip": "A braided leather lash that cracks past a guard, better at disarming and controlling than at killing.",
    # martial ranged
    "blowgun": "A hollow tube of lacquered wood. It barely wounds, which is the point: the needle carries the real weapon.",
    "crossbow-hand": "A small crossbow spanned by hand, small enough for a cloak and popular with those who prefer not to be searched.",
    "crossbow-heavy": "A steel prod crossbow cranked by windlass, throwing a bolt hard enough to punch a knight off his horse.",
    "longbow": "A stave of yew as tall as its archer, requiring a lifetime's strength and reaching further than anything else on the field.",
    "net": "A weighted mesh of knotted cord, thrown to entangle rather than wound.",
    # armor
    "padded": "Quilted layers of cloth and batting. Cheap, warm, noisy, and better than nothing.",
    "leather": "A breastplate and shoulder guards of leather hardened in boiling oil, worn over softer, more flexible pieces.",
    "studded-leather": "Tough flexible leather reinforced with close set rivets, the working armor of scouts and duelists.",
    "hide": "Thick furs and crudely cured pelts lashed together, worn where smiths and tanneries are scarce.",
    "chain-shirt": "A shirt of interlocking rings worn between layers of clothing, quietly protecting the vitals under a traveler's coat.",
    "scale-mail": "Overlapping metal scales sewn to a leather coat and leggings, rattling like a fish out of water.",
    "breastplate": "A fitted metal chest piece over supple leather, guarding the organs while leaving the limbs free.",
    "half-plate": "Shaped plates over most of the body, stopping short of full leg harness. The armor of a knight who still has debts.",
    "ring-mail": "Leather with heavy rings sewn onto it. Inferior to true mail, and worn by those who cannot afford true mail.",
    "chain-mail": "A full suit of interlocking rings over quilted padding, the standard by which armor was judged for centuries.",
    "splint": "Vertical strips of metal riveted to a leather backing, rigid, heavy and very hard to get through.",
    "plate": "A full articulated harness of shaped steel, fitted to one body and worth more than most farms.",
    "shield": "A band of wood and metal on the arm. The cheapest way to survive a mistake.",
    # mounts
    "camel": "Ill tempered, foul breathed and utterly unbothered by a week without water.",
    "donkey-or-mule": "Stubborn, sure footed and able to carry more than it has any right to.",
    "elephant": "A wall of grey muscle that walks. Where one goes, a road follows.",
    "horse-draft": "Broad backed and patient, bred to pull rather than run.",
    "horse-riding": "A steady mount for the road, fast enough to outrun trouble it sees coming.",
    "mastiff": "A war dog the size of a small pony, loyal to one hand and hostile to every other.",
    "pony": "Small, hardy and content in places a horse refuses to go.",
    "warhorse": "Trained to charge into a line of spears and to fight with hooves and teeth when the rider falls.",
}


# Gear, tools, mounts and trade goods the SRD prices without describing. These
# lines are written for this catalog; keys are the bare row name, so a grouped
# row like "Saddle, riding" is looked up as "riding".
FLAVOR.update({
    # --- adventuring gear ---
    "abacus": "Beads on wire in a wooden frame. Slower than a wizard and far more trusted by a merchant.",
    "arrows-20": "Twenty shafts of straight-grained ash, fletched grey. Half of them will be worth picking up again.",
    "blowgun-needles-50": "Fifty slivers of steel in a wooden tube, each one waiting to be dipped in something.",
    "crossbow-bolts-20": "Short, heavy and unlovely. They are not made to fly beautifully, only to arrive.",
    "sling-bullets-20": "Cast lead ovals. More reliable than a river stone and considerably more expensive.",
    "crystal": "A flawed quartz prism on a thong, cloudy enough to hide how much of the light it is bending.",
    "orb": "A polished sphere of glass or stone, heavy in the palm, warm long after it should have cooled.",
    "rod": "A short baton of banded metal, too ornamental to be a weapon and too heavy to be jewellery.",
    "staff": "A worked length of wood carried openly through cities that would confiscate a sword.",
    "wand": "A finger-length of turned wood or bone, the grip worn pale by one hand's constant use.",
    "amulet": "A god's mark cast in cheap metal and worn under the shirt, where faith usually lives.",
    "emblem": "A holy sign inlaid on a shield or brooch, meant to be seen across a battlefield.",
    "reliquary": "A tiny hinged box holding a splinter, a tooth, or a scrap of cloth that someone swears is genuine.",
    "sprig-of-mistletoe": "Cut with a curved blade and never allowed to touch the ground, still green out of season.",
    "totem": "Feathers, fur, bone and teeth bound into a shape that means something to one grove and nothing to anyone else.",
    "wooden-staff": "Drawn whole out of a living tree, not cut from it, and still faintly damp at the heart.",
    "yew-wand": "Pale yew, unvarnished, taken from a tree that has stood in a graveyard longer than the graveyard.",
    "backpack": "Waxed canvas and leather straps. It will hold everything you own and remind you of it every mile.",
    "barrel": "Oak staves and iron hoops. Watertight, rollable, and the single most useful object in any cellar.",
    "basket": "Woven withies with a carrying handle. Light, cheap, and useless in the rain.",
    "bedroll": "Blanket, ground sheet and ties. The difference between sleeping outdoors and lying outdoors.",
    "bell": "A small brass hand bell. For summoning servants, marking watches, or discovering where the tripwire went.",
    "blanket": "Coarse wool, heavy with lanolin, warm even when it is wet.",
    "bottle-glass": "Blown glass with a cork stopper. Precious, fragile, and the only way to see what you are drinking.",
    "bucket": "Bound wood with a rope handle. Carries water, bails boats, and doubles as a very poor helmet.",
    "chalk-1-piece": "A stub of soft white stone. It marks the turn you already took, which is the whole point.",
    "chest": "A banded wooden coffer with a hasp. Heavy enough that stealing it requires a plan.",
    "clothes-common": "Undyed wool and linen, cut loose, mended often. Nobody looks twice.",
    "clothes-costume": "Bright, oversized and badly finished up close, but convincing from ten feet and beyond.",
    "clothes-fine": "Good cloth, careful tailoring and quiet colours. It opens doors that a title alone will not.",
    "clothes-travelers": "Layered, weatherproofed and built for a fortnight on the road without a laundry.",
    "flask-or-tankard": "Pewter, dented, and claimed by whoever is holding it.",
    "grappling-hook": "Three iron flukes on a ring. It catches on the first throw roughly never.",
    "hammer": "A carpenter's hammer. Drives pitons, breaks locks badly, and settles arguments about tent pegs.",
    "hammer-sledge": "Two-handed and unsubtle, for when the door is the problem and finesse is not the answer.",
    "healers-kit": "A leather roll of bandages, salves and splints. Ten uses of not dying, and no skill required.",
    "hourglass": "Fine sand in blown glass. The only honest clock in a dungeon.",
    "ink-1-ounce-bottle": "Lampblack and gum in a stoppered pot. Black going on, brown by the next generation.",
    "ink-pen": "A cut quill. Splits, blots, and needs sharpening about as often as a sword needs oiling.",
    "jug-or-pitcher": "Fired clay with a thumb handle. It will hold a gallon and survive exactly one drop.",
    "ladder-10-foot": "Ten feet of awkward. Too long for corridors, too short for the wall you are looking at.",
    "lantern-bullseye": "A mirrored lens throws the flame into a single cone, so you see the room and the room sees a light.",
    "lantern-hooded": "Shuttered on all four sides, so the light can be cut to a slit or killed without losing the flame.",
    "mirror-steel": "Polished steel in a hinged case. It shows a dim, honest version of you, and looks around corners.",
    "paper-one-sheet": "Smooth, thin and expensive, made from rag pulp. It takes ink better than parchment and burns faster.",
    "parchment-one-sheet": "Scraped and stretched hide. It will outlast paper, the ink, and probably the scribe.",
    "perfume-vial": "Oil of roses and something animal underneath. Covers a smell rather than removing it.",
    "pick-miners": "A steel point on a short haft for working stone, and a poor but available answer to a locked grate.",
    "piton": "An iron spike with an eye. Hammered into rock, it holds a rope, a door, or a hope.",
    "pole-10-foot": "The most useful object in any dungeon and the least respected. It has found more traps than any rogue.",
    "pot-iron": "Cast iron with a bail handle. Heavy, indestructible and the reason the party eats hot food.",
    "potion-of-healing": "A red liquid that glimmers when agitated, in a stoppered flask that never quite loses its warmth.",
    "quiver": "Stiffened leather holding twenty arrows point down, close enough to the hand to draw without looking.",
    "ram-portable": "An iron-shod beam with side handles, carried by two, and worth every pound the moment a door refuses.",
    "robes": "Loose, sleeved and unremarkable, the uniform of scholars, priests and anyone who wants to be mistaken for one.",
    "rope-hempen-50-feet": "Fifty feet of scratchy, reliable hemp. Heavy, and the second thing you reach for after the pole.",
    "rope-silk-50-feet": "Half the weight of hemp and twice the strength, at ten times the price. Climbers weep for it.",
    "sack": "Rough cloth with a drawstring. Holds loot, holds rations, holds evidence.",
    "sealing-wax": "A stick of red wax and the assumption that a broken seal will be noticed.",
    "shovel": "A spade with a shoulder-worn handle. Digs latrines, graves and, occasionally, the point of the expedition.",
    "signal-whistle": "A carved wooden whistle carrying further than a shout and giving away far less.",
    "signet-ring": "An engraved bezel that presses a family's name into wax. Worth more as proof than as gold.",
    "soap": "A hard grey cake of lye and tallow. Underrated by adventurers and prized by everyone who meets them.",
    "spikes-iron-10": "Ten heavy spikes for wedging doors shut behind you, which is a skill in itself.",
    "tent-two-person": "Oiled canvas over a ridgepole. Cramped, damp, and enormously better than the alternative.",
    "vial": "Four ounces of stoppered glass, the standard measure for anything you would rather not spill.",
    "waterskin": "Four pints of tarred hide. Full it is a burden and empty it is a countdown.",
    "whetstone": "A flat grey block. Ten minutes with it is the cheapest maintenance any weapon will ever get.",
    # --- tools ---
    "alchemists-supplies": "Retorts, a burner, glass tubing and a stink that follows you out of the room.",
    "brewers-supplies": "A mash paddle, a hydrometer and the patient conviction that water is not good enough.",
    "calligraphers-supplies": "Cut nibs, three inks and rulers. Makes a document look official, honestly or otherwise.",
    "carpenters-tools": "Saw, plane, chisels and a square. Repairs a wagon, bars a door, builds a bridge nobody trusts.",
    "cartographers-tools": "Dividers, a straightedge, quills and vellum. Turns a week of walking into a line worth selling.",
    "cobblers-tools": "An awl, a last and waxed thread. Boots are the one piece of gear that fails everyone eventually.",
    "cooks-utensils": "Knives, a ladle and a battered pan. The difference between rations and a meal, and morale follows.",
    "glassblowers-tools": "A blowpipe, jacks and shears. The hot end is always the end you just put down.",
    "jewelers-tools": "Loupe, files and tiny pliers. Sets a stone, and tells you whether the stone was worth setting.",
    "leatherworkers-tools": "Punches, edgers and stitching needles for repairing everything the party owns except the metal.",
    "masons-tools": "Trowel, chisels and a hammer. Builds a wall, or finds the one course of it that was laid badly.",
    "painters-supplies": "Brushes, pigments, a palette and canvas. Flattering nobles is a genuine trade.",
    "potters-tools": "Ribs, wires and a shaping knife. Cheap clay, cheap fuel, and everyone needs a pot.",
    "smiths-tools": "Hammers, tongs and a hardy. Given a forge, this is where broken weapons stop being broken.",
    "tinkers-tools": "A hoard of pins, wire, solder and odd springs. Fixes what has no business being fixed.",
    "weavers-tools": "A shuttle, heddles and combs. Turns fleece into cloth, and cloth into money.",
    "woodcarvers-tools": "Gouges and a striking knife. Makes arrows, tent pegs, splints and small ugly figurines.",
    "dice-set": "Bone pips on bone cubes, worn round at the corners by a great many bad decisions.",
    "playing-card-set": "A stiff, greasy deck. Two cards are marked and everyone at the table assumes it was them.",
    "bagpipes": "A bag, a chanter and three drones. Audible at a mile, welcome at rather less.",
    "drum": "Stretched hide over a hoop. Keeps a march together and a heartbeat honest.",
    "dulcimer": "Struck strings over a trapezoid box, bright and carrying, hard to hear a knife over.",
    "flute": "A simple bored tube. Fits in a sleeve, and every second traveller can manage three tunes on it.",
    "horn": "A curl of brass or a hollowed horn. Two notes, both of them meant to be heard across a valley.",
    "lute": "A pear-bellied, gut-strung, permanently out of tune declaration that you are a professional.",
    "lyre": "A yoke of strings played on the lap. Old-fashioned enough that temples still approve of it.",
    "pan-flute": "Bound reeds of falling length, breathy and strange, the sound of somewhere further out than here.",
    "shawm": "A double reed with a bell mouth, loud and nasal, built to be heard outdoors over a crowd.",
    "viol": "Bowed, fretted and held on the knee. Slower than a fiddle and considerably more respectable.",
    "vehicles-land-or-water": "Not a tool you carry - the reins, the tiller and knowing which way a heavy thing will slide.",
    # --- mounts, tack, vehicles ---
    "bit-and-bridle": "Leather and a steel bit. Ten shillings of control over half a ton of opinion.",
    "exotic": "A saddle built around a body that was never meant to be ridden, with straps in places that suggest why.",
    "military": "A high cantle and a deep seat that hold a rider in place through an impact.",
    "pack": "No seat at all, only frames and lashing points. The animal will not thank you.",
    "riding": "A plain everyday saddle, comfortable enough for a long day and useless in a charge.",
    "saddlebags": "Paired leather panniers. Everything you would have carried on your back, carried by something else.",
    "carriage": "Enclosed, sprung and lacquered. It says you arrived rather than merely turned up.",
    "cart": "Two wheels and a bed, pulled by one animal. The workhorse of every road in the world.",
    "chariot": "A light platform on two wheels, fast, unstable, and obsolete everywhere the ground is bad.",
    "sled": "Runners instead of wheels. On snow it is effortless, and on anything else it is a disaster.",
    "wagon": "Four wheels, a canvas tilt and a team. Slow, heavy and the reason caravans exist.",
    "feed-per-day": "Grain and hay. An animal that is not fed becomes a very expensive corpse.",
    "stabling-per-day": "A stall, straw, water and someone else getting up at dawn to see to it.",
    "galley": "Oars and a sail, a shallow draught and a ram. Fast, thirsty for crew, and murder in open sea.",
    "keelboat": "A flat river boat with a shallow keel, poled and rowed, at home where a ship would ground.",
    "longship": "Clinker-built, double-ended and beachable, rowed up rivers no proper ship can enter.",
    "rowboat": "Ten feet of tarred planking. A hundred pounds, portable, and the last thing between you and the water.",
    "sailing-ship": "A single deck and square rig. Slow, capacious, and the standard way cargo crosses water.",
    "warship": "Heavy timbers, high castles and room for soldiers. Built to take a ship, not to outrun one.",
    # --- trade goods ---
    "1-lb-of-wheat": "Grain by the pound, the price everything else in a market is quietly measured against.",
    "1-lb-of-flour-or-one-chicken": "Milled flour or a live bird - a copper either way, and both feed a family tonight.",
    "1-lb-of-salt": "Preservation itself, sold by weight. Cheap here, and worth a fight three hundred miles inland.",
    "1-lb-of-iron-or-1-sq-yd-of-canvas": "Pig iron or heavy cloth. Neither is glamorous and everything is built out of them.",
    "1-lb-of-copper-or-1-sq-yd-of-cotton-cloth": "Copper ingot or cotton bolt, the honest middle of any cargo manifest.",
    "1-lb-of-cinnamon-or-pepper-or-one-sheep": "A pound of spice weighs the same as a sheep costs, which tells you how far it travelled.",
    "1-lb-of-ginger-or-one-goat": "Dried root from a coast nobody in this market has seen, priced like livestock.",
    "1-lb-of-cloves-or-one-pig": "Cloves come by sea in sealed jars, and the jars are guarded better than the ship.",
    "1-lb-of-silver-or-1-sq-yd-of-linen": "Refined silver or fine linen. The point where trade goods start attracting escorts.",
    "1-sq-yd-of-silk-or-one-cow": "A single yard of silk against a whole cow. Both are wealth, only one is portable.",
    "1-lb-of-saffron-or-one-ox": "Handpicked stigmas, thousands of flowers to the pound, priced like a plough team.",
    "1-lb-of-gold": "Coinage, unstruck. Heavy, soft, and the reason most of the roads have bandits on them.",
    "1-lb-of-platinum": "Denser than gold and rarer, moved in sealed cases and never discussed at the table.",
})


# ---------------------------------------------------------------------------
# mundane equipment
# ---------------------------------------------------------------------------

SRD_SOURCE = "SRD 5.1"

WEAPON_GROUPS = {
    "simple melee weapons":  ("simple-melee",  "simple", "melee"),
    "simple ranged weapons": ("simple-ranged", "simple", "ranged"),
    "martial melee weapons": ("martial-melee", "martial", "melee"),
    "martial ranged weapons": ("martial-ranged", "martial", "ranged"),
}


def parse_weapons():
    text = read("04_Equipment", "Weapons.md")
    specials = trait_map(text)
    headers, rows = find_table(text, "Weapons")
    if rows is None:
        cap, headers, rows = tables(text)[0]
    group = ("", "", "")
    for row in rows:
        name = row[0].strip()
        if not name:
            continue
        key = name.lower().strip("*")
        if key in WEAPON_GROUPS:
            group = WEAPON_GROUPS[key]
            continue
        if not group[0] or len(row) < 5:
            continue
        cost, dmg, weight, props = row[1], row[2], row[3], row[4]
        item = Item(name=name, source=SRD_SOURCE, category="weapon",
                    kind="weapon", subtype=group[0])
        m = re.match(r"^([\d]*d?[\d]+)\s+(\w+)$", dmg.strip())
        if m:
            item["damage"], item["damagetype"] = m.group(1), m.group(2)
        item["cost"] = cost if cost != "-" else ""
        item["costgp"] = parse_cost(cost)
        item["weight"] = parse_weight(weight)
        plist = [] if props.strip() in ("", "-") else [p.strip() for p in props.split(",")]
        rng = ""
        keep = []
        for p in plist:
            r = re.search(r"range\s+([\d/]+)", p)
            if r:
                rng = r.group(1)
            keep.append(re.sub(r"\s*\(range [\d/]+\)", "", p).strip())
        item["range"] = rng
        item["traits"] = ", ".join(k.lower() for k in keep if k)
        item["flavor"] = FLAVOR.get(item_id_for(name), "")
        caps = [("Proficiency", "%s weapon" % group[1]),
                ("Attack", "%s" % group[2])]
        if item["damage"]:
            caps.append(("Damage", "%s %s" % (item["damage"], item["damagetype"])))
        if rng:
            caps.append(("Range", "%s ft. normal, %s ft. long" % tuple(rng.split("/"))))
        if item["traits"]:
            caps.append(("Properties", item["traits"]))
        item["capabilities"] = caps + cost_caps(item)
        special = specials.get(key)
        if special:
            item["rules"] = [special]
        add(item)


def item_id_for(name):
    """Weapon names are listed 'Crossbow, light'; id them as crossbow-light."""
    return slug(name)


ARMOR_GROUPS = {"light armor": "light", "medium armor": "medium",
                "heavy armor": "heavy", "shield": "shield"}


def parse_armor():
    text = read("04_Equipment", "Armor.md")
    descs = trait_map(text)
    headers, rows = find_table(text, "Armor")
    if rows is None:
        for cap, headers, rows in tables(text):
            if headers and headers[0].lower() == "armor":
                break
    group = ""
    seen = set()
    for row in rows:
        name = row[0].strip()
        if not name or len(row) < 6:
            continue
        key = name.lower().strip("*")
        if key in ARMOR_GROUPS and not row[1].strip():
            group = ARMOR_GROUPS[key]
            continue
        if key in seen:
            continue
        seen.add(key)
        cost, ac, strength, stealth, weight = row[1], row[2], row[3], row[4], row[5]
        sub = "shield" if key == "shield" else group
        item = Item(name=name, source=SRD_SOURCE, category="armor",
                    kind="armor", subtype=sub)
        item["ac"] = ac
        item["cost"] = cost if cost != "-" else ""
        item["costgp"] = parse_cost(cost)
        item["weight"] = parse_weight(weight)
        props = []
        if strength and strength != "-":
            props.append(strength.lower().replace("str ", "requires str "))
        if stealth and stealth != "-":
            props.append("stealth disadvantage")
        item["traits"] = ", ".join(props)
        item["flavor"] = FLAVOR.get(slug(name), "")
        caps = [("Proficiency", "%s armor" % sub if sub != "shield" else "shields"),
                ("Armor Class", ac)]
        if strength and strength != "-":
            caps.append(("Strength", "%s, or speed drops by 10 ft." % strength))
        if stealth and stealth != "-":
            caps.append(("Stealth", "disadvantage on Dexterity (Stealth) checks"))
        item["capabilities"] = caps + cost_caps(item)
        d = descs.get(key)
        if d:
            item["rules"] = [d]
        add(item)


def parse_simple_table(path, table_name, category, kind, desc_source=None,
                       speed_col=None, capacity_col=None):
    """Gear, tools, tack and vehicles all share an Item/Cost/Weight shape."""
    text = read(*path)
    descs = trait_map(text) if desc_source is None else desc_source
    headers, rows = find_table(text, table_name)
    if rows is None:
        return
    lower = [h.lower() for h in headers]
    ci = lower.index("cost") if "cost" in lower else 1
    wi = lower.index("weight") if "weight" in lower else None
    si = lower.index(speed_col) if speed_col and speed_col in lower else None
    yi = lower.index(capacity_col) if capacity_col and capacity_col in lower else None
    group = ""
    for row in rows:
        raw = row[0].strip()
        if not raw:
            continue
        # "*Group*" heads a set, "~ Member" is a member of it
        if raw.startswith("~"):
            base = raw.lstrip("~ ").strip()
            # "Riding" alone says nothing; "Saddle, riding" says everything
            name = "%s, %s" % (group, base[0].lower() + base[1:]) if group else base
            sub = group
        elif len(row) > ci and not row[ci].strip():
            group = raw.strip("*").strip()
            continue
        else:
            name = base = raw
            sub = ""
        if not name:
            continue
        # This SRD markdown drops the price of a healing potion into its name.
        # The potion is also a magic item, and that entry is the fuller one, so
        # hand it the price and drop the duplicate row here.
        if base.lower().startswith("potion of healing"):
            PRICED_MAGIC["potion-of-healing"] = ("50 gp", "0.5")
            continue
        item = Item(name=name, source=SRD_SOURCE, category=category,
                    kind=kind, subtype=slug(sub) if sub else "")
        item["group"] = sub
        cost = row[ci] if len(row) > ci else ""
        item["cost"] = cost if cost.strip() not in ("", "-") else ""
        item["costgp"] = parse_cost(cost)
        if wi is not None and len(row) > wi:
            item["weight"] = parse_weight(row[wi])
        caps = []
        if sub:
            caps.append(("Group", sub))
        if si is not None and len(row) > si and row[si].strip() not in ("", "-"):
            caps.append(("Speed", row[si].strip()))
            item["traits"] = "speed %s" % row[si].strip()
        if yi is not None and len(row) > yi and row[yi].strip() not in ("", "-"):
            caps.append(("Carrying capacity", row[yi].strip()))
        if kind == "tool":
            caps.append(("Proficiency", "tool proficiency, added to checks made with it"))
        item["capabilities"] = caps + cost_caps(item)
        key = base.lower().rstrip(".")
        d = descs.get(key) or descs.get(re.sub(r"\s*\(.*\)$", "", key))
        if d:
            item["rules"] = [d]
            item["flavor"] = FLAVOR.get(slug(base), "") or first_sentence(d)
        else:
            item["flavor"] = FLAVOR.get(slug(base), "")
        add(item)


def parse_trade_goods():
    text = read("04_Equipment", "Trade_Goods.md")
    headers, rows = find_table(text, "Trade Goods")
    if rows is None:
        return
    for row in rows:
        if len(row) < 2 or not row[1].strip():
            continue
        cost, goods = row[0].strip(), row[1].strip()
        item = Item(name=goods, source=SRD_SOURCE, category="trade good",
                    kind="trade-good")
        item["cost"] = cost
        item["costgp"] = parse_cost(cost)
        item["flavor"] = FLAVOR.get(slug(goods), "")
        item["capabilities"] = [("Market value", cost)]
        add(item)


# ---------------------------------------------------------------------------
# magic items
# ---------------------------------------------------------------------------

RARITY_WORDS = ["common", "uncommon", "rare", "very rare", "legendary", "artifact"]
RARITY_ORDER = {r: i for i, r in enumerate(RARITY_WORDS)}

ATTUNE_RE = re.compile(r"\(requires attunement(?:\s+by\s+([^)]+))?\)", re.I)
TYPE_RE = re.compile(
    r"^([A-Za-z][A-Za-z' ]*?)\s*(?:\(((?:[^()]|\([^()]*\))*)\))?\s*,\s*(.+)$")


def parse_type_line(line):
    """'Armor (medium or heavy), uncommon (requires attunement)' -> parts."""
    line = clean(line).strip().rstrip(".")
    attune, attune_by = "no", ""
    m = ATTUNE_RE.search(line)
    if m:
        attune = "yes"
        attune_by = (m.group(1) or "").strip()
        line = ATTUNE_RE.sub("", line).strip().rstrip(",").strip()
    m = TYPE_RE.match(line)
    if not m:
        return line, "", line, "", attune, attune_by
    kind = m.group(1).strip()
    subtype = (m.group(2) or "").strip()
    rarity_text = m.group(3).strip()
    found = []
    low = rarity_text.lower()
    for r in RARITY_WORDS:
        for mm in re.finditer(r"\b%s\b" % re.escape(r), low):
            # "very rare" also matches "rare"; keep the longest at each position
            if r == "rare" and low[max(0, mm.start() - 5):mm.start()].endswith("very "):
                continue
            found.append(r)
    if not found:
        rarity = "varies"
    else:
        rarity = min(set(found), key=lambda r: RARITY_ORDER[r])
    return kind, subtype, rarity_text, rarity, attune, attune_by


def md_body_to_org(body):
    """Item bodies are prose plus the odd markdown table; org shares the syntax."""
    out = []
    for block in re.split(r"\n\s*\n", body):
        block = block.strip()
        if not block:
            continue
        lines = block.split("\n")
        if lines[0].strip().startswith("|"):
            cells = []
            for ln in lines:
                ln = ln.strip()
                if not ln.startswith("|"):
                    continue
                if re.match(r"^\|[-:+ ]*-[-:+| ]*\|$", ln):
                    continue          # markdown rule, org draws its own
                row = [clean(c.strip()) for c in ln.strip("|").split("|")]
                if any(row):          # SRD tables end on a blank padding row
                    cells.append(row)
            if cells:
                out.append(("table", org_table(cells[0], cells[1:])))
        elif re.match(r"^\*\*Table-\s*(.+?)\*\*$", lines[0].strip()):
            cap = re.match(r"^\*\*Table-\s*(.+?)\*\*$", lines[0].strip()).group(1)
            out.append(("caption", clean(cap)))
        elif lines[0].strip().startswith("#"):
            title = clean(re.sub(r"^#+\s*", "", lines[0]))
            rest = "\n".join(lines[1:]).strip()
            out.append(("sub", title))
            if rest:
                out.append(("para", clean(rest)))
        else:
            out.append(("para", clean(block)))
    return out


def first_sentence(text):
    text = clean(text)
    m = re.match(r"^(.+?[.!?])(?:\s|$)", text)
    return m.group(1).strip() if m else text


def parse_magic_items():
    base = os.path.join(SRD, "09_Magic_Items", "Magic_Items_Each")
    for fname in sorted(os.listdir(base)):
        if not fname.endswith(".md"):
            continue
        with open(os.path.join(base, fname), encoding="utf-8") as fh:
            text = fh.read()
        lines = text.split("\n")
        name = ""
        for ln in lines:
            if ln.strip().startswith("###"):
                name = clean(re.sub(r"^#+\s*", "", ln))
                break
        if not name:
            continue
        rest = text.split(name, 1)[-1] if name in text else text
        blocks = paragraphs(rest)
        type_line = ""
        body_start = 0
        for i, b in enumerate(blocks):
            if b.strip().startswith("*") and not b.strip().startswith("**"):
                type_line = b.strip()
                body_start = i + 1
                break
        kind, subtype, rarity_text, rarity, attune, attune_by = parse_type_line(type_line)
        item = Item(name=name, source=SRD_SOURCE, category="magic item",
                    kind=kind.lower(), subtype=subtype)
        item.set_rarity(rarity)
        if rarity == "varies":
            item["tier"], item["level"] = "", ""
        item["attunement"] = attune
        body = "\n\n".join(blocks[body_start:])
        item["flavor"] = first_sentence(body) if body else ""
        item["rules"] = md_body_to_org(body)

        # mechanical facts worth their own searchable column
        # "Armor, +1, +2, or +3" style entries carry the bonus in the rarity line
        if re.search(r"\(\+1\)", rarity_text):
            item["bonus"] = "+1/+2/+3"
            bonus_of = "attack and damage rolls" if kind.lower() == "weapon" else "AC"
        else:
            bonus_of = ""
            b = re.search(r"\+(\d)\s+bonus to ([^.;]+)", body, re.I)
            if b:
                item["bonus"] = "+" + b.group(1)
                bonus_of = re.sub(r"\s+", " ", b.group(2)).strip()
                # keep the first clause only; the rest belongs in the rules text
                bonus_of = re.split(r",? and (?:you|it) |,", bonus_of)[0].strip()
                bonus_of = re.sub(r"\s+(?:while|if|made with) .*$", "", bonus_of)
                bonus_of = re.split(r"\s+and (?:can|has|have|gains?) ", bonus_of)[0]
        a = re.search(r"\+(\d)\s+bonus to (?:AC|Armor Class)", body, re.I)
        if a:
            item["ac"] = "+" + a.group(1)
        c = re.search(r"has (\d+|\w+) charges", body, re.I)
        if c:
            item["charges"] = c.group(1)
        d = re.search(r"(\d+d\d+)\s+(acid|bludgeoning|cold|fire|force|lightning|"
                      r"necrotic|piercing|poison|psychic|radiant|slashing|thunder)\s+damage",
                      body, re.I)
        if d:
            item["damage"], item["damagetype"] = d.group(1), d.group(2).lower()
        if not item["ac"]:
            a = re.search(r"\byour AC (?:is|becomes) (\d+)\b", body, re.I)
            if a:
                item["ac"] = a.group(1)

        caps = [("Type", kind + (" (%s)" % subtype if subtype else ""))]
        caps.append(("Rarity", rarity_text or rarity))
        if attune == "yes":
            caps.append(("Attunement", "required" + (" by %s" % attune_by if attune_by else "")))
        else:
            caps.append(("Attunement", "not required"))
        if item["tier"]:
            caps.append(("Usual tier", "character levels %s" % item["tier"]))
        if item["bonus"]:
            caps.append(("Bonus", "%s to %s" % (item["bonus"], bonus_of)
                         if bonus_of else item["bonus"]))
        if item["ac"]:
            caps.append(("Armor Class", item["ac"]))
        if item["damage"]:
            caps.append(("Damage", "%s %s" % (item["damage"], item["damagetype"])))
        if item["charges"]:
            caps.append(("Charges", item["charges"]))
        item["capabilities"] = caps
        priced = PRICED_MAGIC.get(slug(name))
        if priced:
            item["cost"], item["weight"] = priced
            item["costgp"] = parse_cost(priced[0])
            caps += cost_caps(item)
            item["capabilities"] = caps
        item["traits"] = ", ".join(
            p for p in ["attunement" if attune == "yes" else "",
                        "charges" if item["charges"] else "",
                        "consumable" if kind.lower() in ("potion", "scroll") else ""] if p)
        add(item)


# ---------------------------------------------------------------------------
# org emitters
# ---------------------------------------------------------------------------

ATTRIB = """#+BEGIN_COMMENT
GENERATED FILE - do not edit by hand.
Regenerate with: python3 tools/dnditems/gen_items.py [srd-path] [out-dir]

This work includes material taken from the System Reference Document 5.1
("SRD 5.1") by Wizards of the Coast LLC, available at
https://dnd.wizards.com/resources/systems-reference-document and licensed
under the Creative Commons Attribution 4.0 International License
(https://creativecommons.org/licenses/by/4.0/legalcode).

Flavor text for equipment the SRD lists without a description is original to
this catalog. The Tier and Level columns are a derived pacing aid, not SRD text.
#+END_COMMENT
"""

DRAWER_KEYS = [k for k, _ in COLUMNS if k != "name"]


def drawer(item, indent="   "):
    out = [indent + ":PROPERTIES:"]
    out.append("%s:ID:%s%s" % (indent, " " * 10, item["id"]))
    for key in DRAWER_KEYS:
        val = str(item.get(key, "")).strip()
        if not val:
            continue
        label = ":%s:" % key.upper()
        out.append("%s%s%s%s" % (indent, label, " " * max(1, 14 - len(label)), val))
    out.append(indent + ":END:")
    return out


def wrap(text, width=76, indent="   "):
    words, lines, cur = text.split(), [], ""
    for w in words:
        if cur and len(cur) + 1 + len(w) > width:
            lines.append(indent + cur)
            cur = w
        else:
            cur = (cur + " " + w).strip()
    if cur:
        lines.append(indent + cur)
    return lines


def render_item(item, level=2):
    stars = "*" * level
    ind = " " * (level + 1)
    out = ["%s %s" % (stars, item["name"])]
    out += drawer(item, ind)
    out.append("")
    if item["flavor"]:
        out.append(ind + "#+BEGIN_QUOTE")
        out += wrap(item["flavor"], 74, ind)
        out.append(ind + "#+END_QUOTE")
        out.append("")
    if item["capabilities"]:
        out.append(ind + "*Capabilities*")
        for label, value in item["capabilities"]:
            body = wrap("%s :: %s" % (label, value), 74, ind + "  ")
            body[0] = ind + "- " + body[0].lstrip()
            out += body
        out.append("")
    if item["rules"]:
        out.append(ind + "*Rules*")
        for block in item["rules"]:
            if isinstance(block, str):
                out += wrap(block, 74, ind)
                out.append("")
                continue
            kind, payload = block
            if kind == "para":
                out += wrap(payload, 74, ind)
                out.append("")
            elif kind == "sub":
                out.append(ind + "/%s/" % payload)
                out.append("")
            elif kind == "caption":
                out.append(ind + "#+CAPTION: %s" % payload)
            elif kind == "table":
                out += [ind + r for r in payload]
                out.append("")
    while out and not out[-1].strip():
        out.pop()
    out.append("")
    return out


GROUPS = [
    ("Weapons", "weapon"),
    ("Armor", "armor"),
    ("Adventuring Gear", "gear"),
    ("Tools", "tool"),
    ("Mounts and Vehicles", "transport"),
    ("Trade Goods", "trade good"),
    ("Magic Items", "magic item"),
]


def write_srd_file(path):
    out = ["#+TITLE: D&D Item Catalog - SRD 5.1",
           "#+CATEGORY: dnd-items",
           "#+SOURCE: SRD 5.1",
           "#+STARTUP: overview",
           "",
           ATTRIB]
    for title, category in GROUPS:
        group = [i for i in ITEMS if i["category"] == category]
        if not group:
            continue
        out.append("* %s" % title)
        out.append("  :PROPERTIES:")
        out.append("  :COUNT:       %d" % len(group))
        out.append("  :END:")
        out.append("")
        for item in sorted(group, key=lambda i: i["name"].lower()):
            out += render_item(item, 2)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(out).rstrip() + "\n")
    return sum(1 for i in ITEMS)


# ---------------------------------------------------------------------------
# homebrew seed - written once, then left alone so your edits survive
# ---------------------------------------------------------------------------

HOMEBREW_SEED = """#+TITLE: D&D Item Catalog - Homebrew
#+CATEGORY: dnd-items
#+SOURCE: Homebrew
#+STARTUP: overview

#+BEGIN_COMMENT
This file is YOURS. The generator seeds it once and never overwrites it.

Add items as second level headings anywhere in this file. Everything in the
PROPERTIES drawer becomes a column in dnd-items-index.org the next time you run

    python3 tools/dnditems/gen_items.py

Recognised drawer keys, all optional except ID:

  ID CATEGORY KIND SUBTYPE RARITY TIER LEVEL ATTUNEMENT BONUS DAMAGE
  DAMAGETYPE AC CHARGES RANGE TRAITS COST COSTGP WEIGHT

SOURCE defaults to the #+SOURCE line above, so you only set it per item if one
file holds several sources. COSTGP is the cost normalised to gold pieces, which
is what the index sorts on. RARITY drives TIER and LEVEL if you leave those
blank: common/uncommon -> 1-4, rare -> 5-10, very rare -> 11-16,
legendary -> 17-20.

Drop other third party or converted sources into their own file named
dnd-items-<source>.org next to this one and they will be picked up too.
#+END_COMMENT

* Magic Items
** Ledger of Debts Unpaid
   :PROPERTIES:
   :ID:          ledger-of-debts-unpaid
   :CATEGORY:    magic item
   :KIND:        wondrous item
   :RARITY:      rare
   :ATTUNEMENT:  yes
   :CHARGES:     3
   :TRAITS:      attunement, charges
   :END:

   #+BEGIN_QUOTE
   A merchant's account book bound in grey hide, its columns already filled in
   a hand nobody recognises, always one entry ahead of the person holding it.
   #+END_QUOTE

   *Capabilities*
   - Type :: Wondrous item
   - Rarity :: rare
   - Attunement :: required
   - Usual tier :: character levels 5-10
   - Charges :: 3

   *Rules*
   While attuned to this ledger you know, on sight, whether a creature owes you
   a favour, a debt or a grudge, and roughly how large it is.

   The ledger has 3 charges and regains all of them at dawn. As an action you
   can expend a charge and name a creature you can see that owes you a debt.
   Until the end of your next turn that creature has disadvantage on attack
   rolls against you. If the debt was a life you saved, the creature must
   instead succeed on a DC 15 Wisdom saving throw or be charmed by you for
   1 minute.

** Quiet Coat
   :PROPERTIES:
   :ID:          quiet-coat
   :CATEGORY:    magic item
   :KIND:        wondrous item
   :RARITY:      uncommon
   :ATTUNEMENT:  no
   :END:

   #+BEGIN_QUOTE
   A long travelling coat of undyed wool that never rustles, never flaps, and
   never quite catches the light the way the rest of you does.
   #+END_QUOTE

   *Capabilities*
   - Type :: Wondrous item
   - Rarity :: uncommon
   - Attunement :: not required
   - Usual tier :: character levels 1-4

   *Rules*
   While wearing this coat you have advantage on Dexterity (Stealth) checks
   made to move quietly, and armor you wear under it never imposes
   disadvantage on Stealth checks from noise alone.

* Weapons
** Tollkeeper's Bill
   :PROPERTIES:
   :ID:          tollkeepers-bill
   :CATEGORY:    weapon
   :KIND:        weapon
   :SUBTYPE:     martial-melee
   :RARITY:      uncommon
   :ATTUNEMENT:  no
   :BONUS:       +1
   :DAMAGE:      1d10
   :DAMAGETYPE:  slashing
   :TRAITS:      heavy, reach, two-handed
   :COST:        350 gp
   :COSTGP:      350
   :WEIGHT:      6
   :END:

   #+BEGIN_QUOTE
   A bridge guard's halberd, the haft worn smooth by generations of bored
   hands, the hook still bright from pulling people back from the rail.
   #+END_QUOTE

   *Capabilities*
   - Type :: Weapon (halberd)
   - Rarity :: uncommon
   - Attunement :: not required
   - Bonus :: +1
   - Damage :: 1d10 slashing
   - Properties :: heavy, reach, two-handed

   *Rules*
   You have a +1 bonus to attack and damage rolls made with this magic weapon.

   When you hit a creature with an opportunity attack using this weapon, you
   can use your reaction to move the target 5 feet toward you.
"""


# ---------------------------------------------------------------------------
# harvest every source file back out of org, so the index covers all sources
# ---------------------------------------------------------------------------

HEAD_RE = re.compile(r"^(\*+)\s+(.*)$")
PROP_RE = re.compile(r"^\s*:([A-Z_]+):\s*(.*)$")


def harvest(path, default_source):
    """Read item headings and their PROPERTIES drawers back out of an org file."""
    found = []
    with open(path, encoding="utf-8") as fh:
        lines = fh.read().split("\n")
    i = 0
    while i < len(lines):
        m = HEAD_RE.match(lines[i])
        if not m or len(m.group(1)) < 2:
            i += 1
            continue
        name = m.group(2).strip()
        i += 1
        if i >= len(lines) or lines[i].strip().upper() != ":PROPERTIES:":
            continue
        i += 1
        props = {}
        while i < len(lines) and lines[i].strip().upper() != ":END:":
            pm = PROP_RE.match(lines[i])
            if pm:
                props[pm.group(1).lower()] = pm.group(2).strip()
            i += 1
        item = Item(name=name, source=props.get("source", default_source))
        item["id"] = props.get("id", slug(name))
        for key, _ in COLUMNS:
            if key in ("name", "source"):
                continue
            if props.get(key):
                item[key] = props[key]
        if item["rarity"] and not item["tier"]:
            tier, level = TIERS.get(item["rarity"], ("", ""))
            item["tier"], item["level"] = tier, level
        found.append(item)
    return found


def file_source(path, fallback):
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            m = re.match(r"^#\+SOURCE:\s*(.+)$", line.strip(), re.I)
            if m:
                return m.group(1).strip()
            if line.startswith("*"):
                break
    return fallback


# ---------------------------------------------------------------------------
# the master index table
# ---------------------------------------------------------------------------

def org_table(headers, rows):
    widths = [len(h) for h in headers]
    for row in rows:
        for i, cell in enumerate(row):
            widths[i] = max(widths[i], len(cell))
    def line(cells):
        return "| " + " | ".join(c.ljust(widths[i]) for i, c in enumerate(cells)) + " |"
    out = [line(headers)]
    out.append("|" + "|".join("-" * (w + 2) for w in widths) + "|")
    for row in rows:
        out.append(line(row))
    return out


def table_rows(items, keys):
    rows = []
    for item in items:
        rows.append([str(item.get(k, "") or "").replace("|", "/") for k in keys])
    return rows


def sort_key(item):
    try:
        cost = float(item["costgp"]) if item["costgp"] else 0.0
    except ValueError:
        cost = 0.0
    return (item["category"], item["name"].lower(), cost)


SEARCH_HELP = """* How to search this catalog

  Every item carries the same PROPERTIES drawer in its source file, and every
  drawer key is a column in the tables above. That gives you three ways in.

** From the table

   Put the cursor in a column and press =C-c ^= to sort the table by it. Sort
   by Level to see everything a party of that level should be finding, by
   Cost (gp) to price out a shop, or by Rarity to lay out a hoard.

** With a sparse tree over the drawers

   In a source file, =C-c / m= matches a property expression against the
   headings and folds everything else away:

   : LEVEL="5"                          items paced for a level 5 party
   : RARITY="rare"&ATTUNEMENT="yes"     rare items that need attunement
   : KIND="potion"                      every potion
   : SUBTYPE="martial-melee"            the martial melee weapons
   : CHARGES<>""&RARITY="very rare"     very rare items with charges
   : COSTGP<"10"                        everything a starting party can afford

** From orgs itself

   The same drawers are visible to the server, so the CLI can query them with
   MatchProperty(NAME, REGEX):

   : orgs grep -query 'MatchProperty("RARITY","^rare$")'
   : orgs grep -query 'MatchProperty("KIND","wand") && MatchProperty("ATTUNEMENT","yes")'
   : orgs grep -query 'MatchProperty("LEVEL","^5$") && MatchProperty("CATEGORY","magic item")'

** Rolling something random

   The tier tables above are already grouped by the levels their rarity suits,
   so a random treasure roll is a random row out of one table:

   : sed -n '/^\\*\\* Tier 2/,/^\\*/p' dnd-items-index.org \\
   :   | awk -F'|' '/^   \\|/ && $2 !~ /Item|---/ {print $2}' | sort -R | head -1

   Swap Tier 2 for the band you want: Tier 1 is levels 1-4, Tier 2 is 5-10,
   Tier 3 is 11-16, Tier 4 is 17-20.

* Columns

  | Column     | Meaning                                                          |
  |------------|------------------------------------------------------------------|
  | Source     | Which book or file the item came from.                            |
  | Category   | weapon, armor, gear, tool, transport, trade good, magic item.     |
  | Kind       | The item type line: wondrous item, ring, potion, weapon, armor.   |
  | Subtype    | The parenthetical: any sword, medium armor, martial-melee.        |
  | Group      | The set a table row belongs to, such as Saddle or Arcane focus.   |
  | Rarity     | common, uncommon, rare, very rare, legendary, or varies.          |
  | Tier       | Character levels the item is usually handed out at. Derived.      |
  | Level      | The low end of Tier, so the column sorts numerically. Derived.    |
  | Attune     | yes if the item requires attunement.                              |
  | Bonus      | Flat magic bonus the item grants, where it grants one.            |
  | Damage     | Damage dice, for weapons and for items that deal damage.          |
  | Dmg Type   | acid, fire, slashing and so on.                                   |
  | AC         | Armor Class the item sets or adds.                                |
  | Charges    | Number of charges, for items that carry them.                     |
  | Range      | Normal/long range in feet.                                        |
  | Traits     | Weapon and armor properties, plus attunement/charges/consumable.  |
  | Cost       | The price as the source writes it.                                |
  | Cost (gp)  | Cost normalised to gold, so the column sorts. Blank if priceless. |
  | Weight     | Weight in pounds.                                                 |
"""


def write_index(path, items):
    keys = [k for k, _ in COLUMNS]
    headers = [h for _, h in COLUMNS]
    magic = [i for i in items if i["category"] == "magic item"]
    out = ["#+TITLE: D&D Item Catalog - Master Index",
           "#+CATEGORY: dnd-items",
           "#+STARTUP: overview",
           "",
           ATTRIB,
           "* Summary",
           ""]
    by_source = {}
    for i in items:
        by_source[i["source"]] = by_source.get(i["source"], 0) + 1
    rows = [[s, str(n)] for s, n in sorted(by_source.items())]
    rows.append(["Total", str(len(items))])
    out += ["  " + r for r in org_table(["Source", "Items"], rows)]
    out.append("")
    by_cat = {}
    for i in items:
        by_cat[i["category"]] = by_cat.get(i["category"], 0) + 1
    rows = [[c or "(none)", str(n)] for c, n in sorted(by_cat.items())]
    out += ["  " + r for r in org_table(["Category", "Items"], rows)]
    out.append("")

    out.append("* Magic items by tier")
    out.append("")
    out.append("  Sorted into the character levels the rarity is usually paced for,")
    out.append("  so a random roll for treasure is a random row in one table.")
    out.append("")
    tier_keys = ["name", "source", "kind", "subtype", "rarity", "attunement",
                 "bonus", "damage", "charges", "traits"]
    tier_heads = ["Item", "Source", "Kind", "Subtype", "Rarity", "Attune",
                  "Bonus", "Damage", "Charges", "Traits"]
    bands = [("1-4", "Tier 1 - character levels 1-4 (common and uncommon)"),
             ("5-10", "Tier 2 - character levels 5-10 (rare)"),
             ("11-16", "Tier 3 - character levels 11-16 (very rare)"),
             ("17-20", "Tier 4 - character levels 17-20 (legendary and artifacts)"),
             ("", "Rarity varies - read the item before placing it")]
    for band, title in bands:
        group = sorted([i for i in magic if i["tier"] == band],
                       key=lambda i: (i["rarity"], i["name"].lower()))
        if not group:
            continue
        out.append("** %s" % title)
        out.append("   :PROPERTIES:")
        out.append("   :COUNT:       %d" % len(group))
        out.append("   :END:")
        out.append("")
        out += ["   " + r for r in org_table(tier_heads, table_rows(group, tier_keys))]
        out.append("")

    out.append("* All items")
    out.append("   :PROPERTIES:")
    out.append("   :COUNT:       %d" % len(items))
    out.append("   :END:")
    out.append("")
    out += ["  " + r for r in org_table(headers, table_rows(sorted(items, key=sort_key), keys))]
    out.append("")
    out.append(SEARCH_HELP)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write("\n".join(out).rstrip() + "\n")


# ---------------------------------------------------------------------------

def main():
    if not os.path.isdir(SRD):
        sys.exit("srd path not found: %s" % SRD)
    os.makedirs(OUT, exist_ok=True)

    parse_weapons()
    parse_armor()
    parse_simple_table(("04_Equipment", "Adventuring_Gear.md"),
                       "Adventuring Gear", "gear", "gear")
    parse_simple_table(("04_Equipment", "Tools.md"), "Tools", "tool", "tool")
    parse_simple_table(("04_Equipment", "Transportation.md"),
                       "Mounts and Other Animals", "transport", "mount",
                       speed_col="speed", capacity_col="carrying capacity")
    parse_simple_table(("04_Equipment", "Transportation.md"),
                       "Tack, Harness, and Drawn Vehicles", "transport", "tack")
    parse_simple_table(("04_Equipment", "Transportation.md"),
                       "Waterborne Vehicles", "transport", "vehicle",
                       speed_col="speed")
    parse_trade_goods()
    parse_magic_items()

    srd_path = os.path.join(OUT, "dnd-items-srd.org")
    write_srd_file(srd_path)
    print("wrote %s (%d items)" % (srd_path, len(ITEMS)))

    hb_path = os.path.join(OUT, "dnd-items-homebrew.org")
    if not os.path.exists(hb_path):
        with open(hb_path, "w", encoding="utf-8") as fh:
            fh.write(HOMEBREW_SEED)
        print("wrote %s (seeded)" % hb_path)
    else:
        print("kept  %s (yours)" % hb_path)

    # the index is built by reading every source file back, homebrew included
    all_items = []
    for fname in sorted(os.listdir(OUT)):
        if not (fname.startswith("dnd-items-") and fname.endswith(".org")):
            continue
        if fname == "dnd-items-index.org":
            continue
        path = os.path.join(OUT, fname)
        src = file_source(path, fname[len("dnd-items-"):-len(".org")].title())
        got = harvest(path, src)
        all_items += got
        print("      %-28s %4d items (%s)" % (fname, len(got), src))

    idx_path = os.path.join(OUT, "dnd-items-index.org")
    write_index(idx_path, all_items)
    print("wrote %s (%d items across %d sources)"
          % (idx_path, len(all_items), len({i["source"] for i in all_items})))


if __name__ == "__main__":
    main()
