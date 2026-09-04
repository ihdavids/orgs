#!/usr/bin/env python3
"""Generate the orgs dnd ruleset modules from the reForged SRD markdown.

Usage:
    python3 tools/dndsrd/gen_srd.py [path-to-dndsrd] [output-dir]

Defaults to ../dndsrd and internal/common/dnd/data.

The source material is the System Reference Document 5.1 by Wizards of the
Coast, licensed under CC-BY-4.0. The generated yaml carries that attribution.
"""

import os
import re
import sys

# ---------------------------------------------------------------------------
# tiny yaml emitter (no third party dependencies)
# ---------------------------------------------------------------------------

def is_scalar(v):
    return isinstance(v, (str, int, float, bool)) or v is None


def quote(s):
    s = s.replace("\\", "\\\\").replace('"', '\\"')
    return '"' + s + '"'


def scalar(v):
    if v is None:
        return '""'
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return str(v)
    return quote(v)


def emit(value, indent=0, out=None):
    """Emit a python structure as yaml lines."""
    pad = "  " * indent
    if out is None:
        out = []
    if isinstance(value, dict):
        for k, v in value.items():
            if v is None or v == "" or v == [] or v == {}:
                continue
            if is_scalar(v):
                if isinstance(v, str) and "\n" in v:
                    out.append(f"{pad}{k}: |-")
                    for line in v.split("\n"):
                        out.append(f"{pad}  {line}".rstrip())
                else:
                    out.append(f"{pad}{k}: {scalar(v)}")
            elif isinstance(v, list) and all(is_scalar(x) and not (isinstance(x, str) and "\n" in x) for x in v) and len(v) <= 12 and sum(len(str(x)) for x in v) < 90:
                out.append(f"{pad}{k}: [{', '.join(scalar(x) for x in v)}]")
            else:
                out.append(f"{pad}{k}:")
                emit(v, indent + 1, out)
    elif isinstance(value, list):
        for item in value:
            if is_scalar(item):
                out.append(f"{pad}- {scalar(item)}")
            else:
                sub = []
                emit(item, indent + 1, sub)
                if not sub:
                    continue
                first = sub[0]
                out.append(pad + "- " + first[len(pad) + 2:])
                out.extend(sub[1:])
    return out


def dump(obj):
    return "\n".join(emit(obj)) + "\n"


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

def slug(name):
    name = name.lower().strip()
    name = name.replace("'", "").replace("\u2019", "")
    name = re.sub(r"[^a-z0-9]+", "-", name)
    return name.strip("-")


def clean(text):
    """Normalise markdown inline formatting into plain text."""
    text = text.replace("\u2019", "'").replace("\u2018", "'")
    text = text.replace("\u201c", '"').replace("\u201d", '"')
    text = text.replace("\u2014", " - ").replace("\u2013", "-")
    text = text.replace("\u00bd", ".5").replace("\u00bc", ".25").replace("\u00be", ".75")
    text = re.sub(r"\*\*\*(.+?)\*\*\*", r"\1", text)
    text = re.sub(r"\*\*(.+?)\*\*", r"\1", text)
    text = re.sub(r"\*(.+?)\*", r"\1", text)
    text = re.sub(r"\[\[(.+?)\]\]", r"\1", text)
    text = re.sub(r"\[(.+?)\]\(.+?\)", r"\1", text)
    text = re.sub(r"[ \t]+", " ", text)
    return text.strip()


def paragraphs(text):
    out = []
    for block in re.split(r"\n\s*\n", text):
        block = block.strip()
        if block:
            out.append(block)
    return out


def sections(text, level):
    """Split markdown into (title, body) pairs for headings of exactly level."""
    pattern = re.compile(r"^%s (.+)$" % ("#" * level), re.MULTILINE)
    out = []
    matches = list(pattern.finditer(text))
    for i, m in enumerate(matches):
        start = m.end()
        end = matches[i + 1].start() if i + 1 < len(matches) else len(text)
        out.append((m.group(1).strip(), text[start:end].strip()))
    return out


def before_first(text, level):
    pattern = re.compile(r"^%s .+$" % ("#" * level), re.MULTILINE)
    m = pattern.search(text)
    return text[: m.start()] if m else text


TRAIT_RE = re.compile(r"^\*\*\*(.+?)\*\*\*\.?\s*(.*)$", re.S)


def parse_traits(text):
    """Parse '***Name***. text' blocks, folding continuation paragraphs in."""
    traits = []
    for para in paragraphs(text):
        if para.startswith("#") or para.startswith("|") or para.startswith(">"):
            continue
        m = TRAIT_RE.match(para)
        if m:
            traits.append({"name": clean(m.group(1)), "text": clean(m.group(2))})
        elif traits and not para.startswith("**"):
            traits[-1]["text"] = (traits[-1]["text"] + "\n" + clean(para)).strip()
    return traits


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
            headers = [c.strip() for c in line.strip("|").split("|")]
            rows = []
            j = i + 2
            while j < len(lines) and lines[j].strip().startswith("|"):
                cells = [clean(c.strip()) for c in lines[j].strip().strip("|").split("|")]
                if any(c for c in cells):
                    rows.append(cells)
                j += 1
            out.append((caption, [clean(h) for h in headers], rows))
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


NUMBER_WORDS = {
    "a": 1, "an": 1, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5,
    "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10, "twelve": 12,
    "twenty": 20,
}

ABILITIES = {
    "strength": "str", "dexterity": "dex", "constitution": "con",
    "intelligence": "int", "wisdom": "wis", "charisma": "cha",
}

SKILLS = [
    ("acrobatics", "Acrobatics", "dex", "Keep your balance, tumble, flip and stay on your feet."),
    ("animal-handling", "Animal Handling", "wis", "Calm, control or read the intentions of an animal."),
    ("arcana", "Arcana", "int", "Recall lore about spells, magic items and planes of existence."),
    ("athletics", "Athletics", "str", "Climb, jump, swim and grapple."),
    ("deception", "Deception", "cha", "Convincingly hide the truth."),
    ("history", "History", "int", "Recall lore about events, people, wars and civilisations."),
    ("insight", "Insight", "wis", "Determine the true intentions of a creature."),
    ("intimidation", "Intimidation", "cha", "Influence someone through overt threats."),
    ("investigation", "Investigation", "int", "Look for clues and make deductions from them."),
    ("medicine", "Medicine", "wis", "Stabilise a dying companion or diagnose an illness."),
    ("nature", "Nature", "int", "Recall lore about terrain, plants, animals and weather."),
    ("perception", "Perception", "wis", "Spot, hear or otherwise detect something."),
    ("performance", "Performance", "cha", "Delight an audience with music, dance, acting or storytelling."),
    ("persuasion", "Persuasion", "cha", "Influence someone with tact, social grace or good nature."),
    ("religion", "Religion", "int", "Recall lore about deities, rites, prayers and holy symbols."),
    ("sleight-of-hand", "Sleight of Hand", "dex", "Plant something, conceal an object or pick a pocket."),
    ("stealth", "Stealth", "dex", "Conceal yourself from enemies and move silently."),
    ("survival", "Survival", "wis", "Follow tracks, hunt, navigate and avoid natural hazards."),
]
SKILL_IDS = {name.lower(): sid for sid, name, _, _ in SKILLS}

LEVEL_RE = re.compile(r"\b(\d+)(?:st|nd|rd|th) level\b", re.I)


def level_from_text(text, default=1):
    m = LEVEL_RE.search(text)
    if m:
        return int(m.group(1))
    return default


def ordinal_to_int(s):
    m = re.match(r"(\d+)", s.strip())
    return int(m.group(1)) if m else 0


def parse_weight(s):
    s = s.replace("lb.", "").replace("lb", "").strip()
    s = s.replace("\u00bd", ".5").replace("\u00bc", ".25").replace("\u00be", ".75")
    s = s.replace("1/2", ".5").replace("1/4", ".25")
    s = s.replace(",", "")
    if not s or s == "-":
        return 0
    try:
        v = float(s)
        return int(v) if v == int(v) else v
    except ValueError:
        return 0


# ---------------------------------------------------------------------------
# items (weapons, armor, gear, tools, packs)
# ---------------------------------------------------------------------------

# Names the rules text uses that are not exactly the table entry.
EXTRA_ALIASES = {
    "arrows": "arrows-20", "quiver of 20 arrows": "arrows-20", "arrow": "arrows-20",
    "bolts": "crossbow-bolts-20", "crossbow bolts": "crossbow-bolts-20",
    "sling bullets": "sling-bullets-20", "blowgun needles": "blowgun-needles-50",
    "wooden shield": "shield", "shields": "shield",
    "set of common clothes": "clothes-common",
    "hempen rope": "rope-hempen-50-feet", "silk rope": "rope-silk-50-feet",
    "rations": "rations-1-day", "days rations": "rations-1-day",
    "prayer wheel": "holy-symbol", "prayer book": "book",
    "prayer book or prayer wheel": "book",
    "sticks of incense": "incense", "stick of incense": "incense",
    "any other musical instrument": "musical-instrument",
    "musical instrument": "musical-instrument",
    "artisan's tools": "artisans-tools", "gaming set": "gaming-set",
}

# Category level items the SRD lists as a group rather than a single row.
SYNTHETIC_ITEMS = [
    {"id": "arcane-focus", "name": "Arcane focus", "kind": "gear", "category": "focus",
     "cost": "10 gp", "weight": 2, "text": "A crystal, orb, rod, staff or wand used to channel arcane magic."},
    {"id": "druidic-focus", "name": "Druidic focus", "kind": "gear", "category": "focus",
     "cost": "10 gp", "weight": 2, "text": "A sprig of mistletoe, totem, wooden staff or yew wand."},
    {"id": "holy-symbol", "name": "Holy symbol", "kind": "gear", "category": "focus",
     "cost": "5 gp", "weight": 1, "text": "An amulet, emblem or reliquary used as a divine spellcasting focus."},
    {"id": "musical-instrument", "name": "Musical instrument", "kind": "tool", "category": "tool",
     "cost": "20 gp", "weight": 3, "text": "Any one musical instrument of your choice."},
    {"id": "artisans-tools", "name": "Artisan's tools", "kind": "tool", "category": "tool",
     "cost": "15 gp", "weight": 5, "text": "Any one set of artisan's tools of your choice."},
    {"id": "gaming-set", "name": "Gaming set", "kind": "tool", "category": "tool",
     "cost": "1 gp", "weight": 0, "text": "Any one gaming set of your choice."},
    {"id": "vestments", "name": "Vestments", "kind": "gear", "category": "gear",
     "cost": "-", "weight": 4, "text": "The garments of a priest of your faith."},
    {"id": "incense", "name": "Stick of incense", "kind": "gear", "category": "gear",
     "cost": "-", "weight": 0, "text": "Burned during rites and devotions."},
    {"id": "monk-weapon", "name": "Monk weapon", "kind": "gear", "category": "gear",
     "cost": "-", "weight": 0, "text": "Any shortsword or simple melee weapon without the two handed or heavy property."},
]


class Items:
    def __init__(self):
        self.items = []
        self.alias = {}

    def add(self, item):
        self.items.append(item)
        self._alias(item["id"], item["id"])
        name = item["name"]
        self._alias(slug(name), item["id"])
        low = name.lower()
        if "," in low:
            parts = [p.strip() for p in low.split(",")]
            self._alias(slug(" ".join(reversed(parts))), item["id"])
        if "(" in low:
            self._alias(slug(re.sub(r"\(.*?\)", "", low)), item["id"])
        if item.get("kind") == "armor":
            self._alias(slug(low + " armor"), item["id"])
        for extra, target in EXTRA_ALIASES.items():
            if target == item["id"]:
                self._alias(slug(extra), item["id"])
        return item

    def _alias(self, key, target):
        if key and key not in self.alias:
            self.alias[key] = target

    def resolve(self, phrase):
        """Resolve a natural language item phrase onto an item id."""
        p = phrase.lower().strip().strip(".,")
        p = re.sub(r"\(.*?\)", "", p)
        p = re.sub(r"^(a|an|the) ", "", p).strip()
        cands = [p, re.sub(r"s$", "", p), re.sub(r"es$", "", p),
                 re.sub(r"ies$", "y", p), p.replace("'s", "s")]
        # "light crossbow" -> "crossbow, light"
        words = p.split()
        if len(words) > 1:
            cands.append(" ".join(reversed(words)))
        for c in cands:
            key = slug(c)
            if key in self.alias:
                return self.alias[key]
        for c in cands:
            key = slug(c)
            for a, target in self.alias.items():
                if a.startswith(key) and len(key) > 3:
                    return target
        return None

    def of_category(self, category):
        return [i for i in self.items if i.get("category") == category]


def parse_weapons(text, items):
    headers, rows = find_table(text, "Weapons")
    category = ""
    for row in rows:
        name = row[0]
        if not name:
            continue
        if not row[1].strip() and not row[2].strip():
            category = slug(name).replace("-weapons", "")
            continue
        damage, dtype = "", ""
        m = re.match(r"([0-9]+d[0-9]+)\s+(\w+)", row[2])
        if m:
            damage, dtype = m.group(1), m.group(2)
        props_raw = row[4] if len(row) > 4 else ""
        props, versatile, rng = [], "", ""
        if props_raw and props_raw != "-":
            for part in re.split(r",\s*(?![^()]*\))", props_raw):
                part = part.strip()
                if not part:
                    continue
                v = re.match(r"versatile \((.+?)\)", part, re.I)
                if v:
                    versatile = v.group(1)
                    props.append("versatile")
                    continue
                r = re.match(r"(ammunition|thrown)\s*\(range (.+?)\)", part, re.I)
                if r:
                    props.append(r.group(1).lower())
                    rng = r.group(2)
                    continue
                props.append(slug(part))
        items.add({
            "id": slug(name), "name": name, "kind": "weapon", "category": category,
            "cost": row[1], "damage": damage, "damageType": dtype,
            "weight": parse_weight(row[3]), "properties": props,
            "versatile": versatile, "range": rng,
        })


def parse_armor(text, items):
    headers, rows = find_table(text, "Armor")
    category = ""
    for row in rows:
        name = row[0]
        if not name:
            continue
        if not row[1].strip():
            category = slug(name).replace("-armor", "")
            continue
        ac_raw = row[2]
        ac, dexmax, nodex = 0, 0, False
        m = re.match(r"(\d+)", ac_raw)
        if m:
            ac = int(m.group(1))
        if "max 2" in ac_raw:
            dexmax = 2
        if "Dex" not in ac_raw:
            nodex = True
        if ac_raw.strip().startswith("+"):
            ac = int(ac_raw.strip().lstrip("+"))
        armor_type = category if category in ("light", "medium", "heavy") else "shield"
        strength = 0
        sm = re.search(r"Str (\d+)", row[3])
        if sm:
            strength = int(sm.group(1))
        items.add({
            "id": slug(name), "name": name,
            "kind": "shield" if armor_type == "shield" else "armor",
            "category": armor_type + "-armor", "armorType": armor_type,
            "cost": row[1], "ac": ac, "dexMax": dexmax, "noDex": nodex,
            "strengthReq": strength,
            "stealthDisadvantage": "disadvantage" in row[4].lower(),
            "weight": parse_weight(row[5]),
        })


def parse_gear(text, items):
    headers, rows = find_table(text, "Adventuring Gear")
    for row in rows:
        name = row[0]
        if not name or name.startswith("*"):
            continue
        name = name.lstrip("~ ").strip()
        if not name:
            continue
        if not row[1].strip() and not row[2].strip():
            continue
        items.add({
            "id": slug(name), "name": name, "kind": "gear", "category": "gear",
            "cost": row[1], "weight": parse_weight(row[2]),
        })
    # equipment packs
    for para in paragraphs(text):
        m = re.match(r"^\*\*\*(.+?) \((\d+) gp\)\*\*\*\.\s*Includes (.+)$", para, re.S)
        if not m:
            continue
        name = m.group(1).strip()
        contents = []
        body = clean(m.group(3))
        body = body.replace("The pack also has", ", and").replace("strapped to the side of it", "")
        for part in re.split(r",| and ", body):
            part = part.strip(" .")
            if not part:
                continue
            # "50 feet of hempen rope" is one item, not fifty of them
            part = re.sub(r"^\d+ (feet|foot) of ", "", part)
            qty = 1
            q = re.match(r"^(\d+|[a-z]+)\s+(.*)$", part)
            if q:
                word = q.group(1)
                if word.isdigit():
                    qty = int(word)
                    part = q.group(2)
                elif word in NUMBER_WORDS and NUMBER_WORDS[word] > 1:
                    qty = NUMBER_WORDS[word]
                    part = q.group(2)
            part = re.sub(r"^(a|an|the) ", "", part).strip()
            part = re.sub(r"^\d+ (feet|days) of ", "", part)
            part = re.sub(r" of rations$", "", part)
            iid = items.resolve(part)
            if iid:
                contents.append({"id": iid, "qty": qty})
        items.add({
            "id": slug(name), "name": name, "kind": "pack", "category": "pack",
            "cost": m.group(2) + " gp", "weight": 0, "contents": contents,
        })


def parse_tools(text, items):
    for cap, headers, rows in tables(text):
        if not headers or headers[0].lower() not in ("item", "tool"):
            continue
        for row in rows:
            name = row[0]
            if not name or name.startswith("*"):
                continue
            name = name.lstrip("~ ").strip()
            if not name or len(row) < 3:
                continue
            if not row[1].strip():
                continue
            items.add({
                "id": slug(name), "name": name, "kind": "tool", "category": "tool",
                "cost": row[1], "weight": parse_weight(row[2]),
            })


# ---------------------------------------------------------------------------
# races
# ---------------------------------------------------------------------------

RACE_SPELLS = {
    "Drow Magic": [("dancing-lights", 1), ("faerie-fire", 3), ("darkness", 5)],
    "Infernal Legacy": [("thaumaturgy", 1), ("hellish-rebuke", 3), ("darkness", 5)],
    "Natural Illusionist": [("minor-illusion", 1)],
}

# Flavour name suggestions for the random generator. Not SRD content.
RACE_NAMES = {
    "dwarf": ["Adrik", "Baern", "Dain", "Gardain", "Harbek", "Kildrak", "Rurik", "Thoradin",
              "Bardryn", "Diesa", "Eldeth", "Gunnloda", "Kathra", "Riswynn", "Torbera", "Vistra"],
    "elf": ["Adran", "Aelar", "Beiro", "Carric", "Erdan", "Heian", "Lucan", "Peren",
            "Adrie", "Althaea", "Caelynn", "Drusilia", "Jelenneth", "Keyleth", "Leshanna", "Silaqui"],
    "halfling": ["Alton", "Cade", "Corrin", "Eldon", "Garret", "Lyle", "Milo", "Roscoe",
                 "Andry", "Bree", "Callie", "Cora", "Jillian", "Lavinia", "Portia", "Seraphina"],
    "human": ["Anton", "Diero", "Marcon", "Pieron", "Rimardo", "Bor", "Fodel", "Darvin",
              "Iuliana", "Kara", "Katernin", "Seipora", "Shandri", "Rowan", "Morgan", "Elena"],
    "dragonborn": ["Arjhan", "Balasar", "Donaar", "Ghesh", "Kriv", "Medrash", "Rhogar", "Torinn",
                   "Akra", "Biri", "Harann", "Kava", "Korinn", "Mishann", "Sora", "Thava"],
    "gnome": ["Boddynock", "Dimble", "Fonkin", "Gerbo", "Nackle", "Seebo", "Warryn", "Zook",
              "Bimpnottin", "Caramip", "Duvamil", "Ella", "Nissa", "Oda", "Roywyn", "Shamil"],
    "half-elf": ["Aramil", "Berrian", "Dayereth", "Hadarai", "Immeral", "Rolen", "Theren", "Varis",
                 "Bethrynna", "Caelynn", "Felosial", "Meriele", "Shanairra", "Thia", "Valna", "Ielenia"],
    "half-orc": ["Dench", "Feng", "Gell", "Henk", "Holg", "Imsh", "Keth", "Mhurren",
                 "Baggi", "Emen", "Myev", "Neega", "Ovak", "Shautha", "Sutha", "Volen"],
    "tiefling": ["Akmenos", "Amnon", "Barakas", "Damakos", "Ekemon", "Kairon", "Melech", "Skamos",
                 "Akta", "Bryseis", "Damaia", "Kallista", "Lerissa", "Nemeia", "Orianna", "Rieta"],
}


def race_details(traits, race):
    """Pull structured fields out of the trait text."""
    keep = []
    for t in traits:
        name, text = t["name"], t["text"]
        low = text.lower()
        if name == "Ability Score Increase":
            if "each increase by 1" in low:
                race["abilityBonuses"] = {a: 1 for a in ABILITIES.values()}
            for ability, aid in ABILITIES.items():
                m = re.search(r"your %s score increases by (\d+)" % ability, low)
                if m:
                    race.setdefault("abilityBonuses", {})[aid] = int(m.group(1))
            m = re.search(r"(two|three) different ability scores of your choice increase by (\d+)", low)
            if m:
                race["abilityChoice"] = {
                    "count": NUMBER_WORDS[m.group(1)], "amount": int(m.group(2)),
                }
            m = re.search(r"two other ability scores of your choice increase by (\d+)", low)
            if m:
                race["abilityChoice"] = {"count": 2, "amount": int(m.group(1))}
            continue
        if name == "Age":
            race["age"] = text
            continue
        if name == "Alignment":
            race["alignment"] = text
            continue
        if name == "Size":
            m = re.search(r"your size is (\w+)", low)
            if m:
                race["size"] = m.group(1).capitalize()
            continue
        if name == "Speed":
            m = re.search(r"base walking speed is (\d+) feet", low)
            if m:
                race["speed"] = int(m.group(1))
            continue
        if name in ("Languages", "Extra Language"):
            langs = []
            m = re.search(r"speak, read, and write ([^.]+)", text)
            if m:
                for part in re.split(r",| and ", m.group(1)):
                    part = part.strip()
                    if not part or "extra language" in part or "choice" in part:
                        continue
                    if part[:1].isupper():
                        langs.append(part)
            if langs:
                race["languages"] = langs
            if "extra language" in low or "additional language" in low:
                race["extraLanguages"] = race.get("extraLanguages", 0) + 1
            if name == "Languages":
                continue
        if "darkvision" in name.lower():
            m = re.search(r"within (\d+) feet", low)
            if m:
                race["darkvision"] = int(m.group(1))
        m = re.search(r"proficiency in the (.+?) skill", low)
        if m:
            for part in re.split(r",| and ", m.group(1)):
                sid = SKILL_IDS.get(part.strip())
                if sid:
                    race.setdefault("proficiencies", {}).setdefault("skills", []).append(sid)
        m = re.search(r"proficiency with the ([^.]+?)\.", text)
        if m and "tools" not in m.group(1) and "supplies" not in m.group(1):
            weapons = []
            for part in re.split(r",| and ", m.group(1)):
                part = part.strip()
                if part:
                    weapons.append(slug(part))
            if weapons:
                race.setdefault("proficiencies", {}).setdefault("weapons", []).extend(weapons)
        if "hit point maximum increases by 1" in low:
            race["hpPerLevel"] = 1
        if name in RACE_SPELLS:
            race.setdefault("spells", []).extend(
                {"id": sid, "level": lvl, "ability": "cha" if name != "Natural Illusionist" else "int"}
                for sid, lvl in RACE_SPELLS[name])
        if name == "Cantrip" and "wizard spell list" in low:
            race.setdefault("choices", []).append({
                "id": "elf-cantrip", "name": "Elven Cantrip", "kind": "spells", "count": 1,
                "from": ["wizard"], "spellLevels": [0],
                "prompt": "Choose a wizard cantrip, Intelligence is your spellcasting ability for it.",
            })
        if name == "Tool Proficiency" and "artisan's tools of your choice" in low:
            opts = re.search(r"choice: ([^.]+)", text)
            tools = [t.strip() for t in re.split(r",| or ", opts.group(1))] if opts else []
            race.setdefault("choices", []).append({
                "id": slug(race["id"] + "-tools"), "name": "Tool Proficiency", "kind": "tools",
                "count": 1, "from": [t for t in tools if t],
                "prompt": "Choose one set of artisan's tools.",
            })
        if name == "Skill Versatility":
            race.setdefault("choices", []).append({
                "id": "skill-versatility", "name": "Skill Versatility", "kind": "skills",
                "count": 2, "prompt": "Choose two skill proficiencies of your choice.",
            })
        if name == "Menacing":
            race.setdefault("proficiencies", {}).setdefault("skills", []).append("intimidation")
        keep.append(t)
    return keep


def parse_races(src, items):
    races = []
    dirname = os.path.join(src, "01_Races", "Races_Each")
    for fname in sorted(os.listdir(dirname)):
        if not fname.endswith(".md"):
            continue
        text = open(os.path.join(dirname, fname), encoding="utf-8").read()
        name = re.search(r"^# (.+)$", text, re.MULTILINE).group(1).strip()
        rid = slug(name)
        body = before_first(text, 2)
        race = {"id": rid, "name": name, "source": "SRD", "size": "Medium", "speed": 30,
                "languages": [], "traits": []}
        traits = parse_traits(body)
        race["traits"] = race_details(traits, race)
        # description: the first non trait paragraph of the race intro
        intro = [p for p in paragraphs(before_first(body, 3)) if not p.startswith("#")]
        if intro:
            race["text"] = clean(intro[0])
        # dragonborn ancestry table
        headers, rows = find_table(text, "Draconic Ancestry")
        if rows:
            options = []
            for row in rows:
                if not row[0]:
                    continue
                options.append({"name": "%s (%s)" % (row[0], row[1].lower()),
                                "text": "%s damage, %s." % (row[1], row[2])})
            race.setdefault("choices", []).insert(0, {
                "id": "draconic-ancestry", "name": "Draconic Ancestry", "kind": "options",
                "count": 1, "options": options,
                "prompt": "Choose your draconic ancestry, it sets your breath weapon and resistance.",
            })
        race["names"] = RACE_NAMES.get(rid, [])
        subraces = []
        for sub_name, sub_body in sections(text, 2):
            sub = {"id": slug(sub_name), "name": sub_name, "source": "SRD", "traits": []}
            sub_traits = parse_traits(sub_body)
            sub["traits"] = race_details(sub_traits, sub)
            intro = [p for p in paragraphs(sub_body) if not TRAIT_RE.match(p) and not p.startswith("#")]
            if intro:
                sub["summary"] = summarize(sub)
                sub["text"] = clean(intro[0])
            else:
                sub["summary"] = summarize(sub)
            subraces.append(sub)
        if subraces:
            race["subraces"] = subraces
        race["summary"] = summarize(race)
        races.append(race)
    return races


def summarize(race):
    bits = []
    for aid in ("str", "dex", "con", "int", "wis", "cha"):
        v = race.get("abilityBonuses", {}).get(aid)
        if v:
            bits.append("%s +%d" % (aid.upper(), v))
    ac = race.get("abilityChoice")
    if ac:
        bits.append("+%d to %d abilities of your choice" % (ac["amount"], ac["count"]))
    if race.get("darkvision"):
        bits.append("darkvision %d ft" % race["darkvision"])
    if race.get("speed") and race.get("speed") != 30:
        bits.append("speed %d ft" % race["speed"])
    return ", ".join(bits)


# ---------------------------------------------------------------------------
# backgrounds
# ---------------------------------------------------------------------------

def parse_backgrounds(src, items):
    text = open(os.path.join(src, "03_Characterization", "Backgrounds.md"), encoding="utf-8").read()
    out = []
    for name, body in sections(text, 2):
        if "Skill Proficiencies:" not in body:
            continue
        bg = {"id": slug(name), "name": name, "source": "SRD", "proficiencies": {}}
        intro = [p for p in paragraphs(body) if not p.startswith("**") and not p.startswith("#")]
        if intro:
            bg["text"] = clean(intro[0])
        m = re.search(r"\*\*Skill Proficiencies:\*\* (.+)", body)
        if m:
            skills = []
            for part in re.split(r",| and ", m.group(1)):
                sid = SKILL_IDS.get(clean(part).strip().lower())
                if sid:
                    skills.append(sid)
            bg["proficiencies"]["skills"] = skills
        m = re.search(r"\*\*Tool Proficiencies:\*\* (.+)", body)
        if m:
            tools = [clean(p).strip() for p in re.split(r",| and ", m.group(1)) if clean(p).strip()]
            bg["proficiencies"]["tools"] = tools
        m = re.search(r"\*\*Languages:\*\* (.+)", body)
        if m:
            word = clean(m.group(1)).split()[0].lower()
            bg["languages"] = NUMBER_WORDS.get(word, 1)
        m = re.search(r"\*\*Equipment:\*\* (.+)", body)
        if m:
            equip, gold = parse_equipment_text(clean(m.group(1)), items)
            bg["equipment"] = equip
            bg["gold"] = gold
        for fname, fbody in sections(body, 3):
            if fname.lower().startswith("feature:"):
                bg["feature"] = {
                    "name": fname.split(":", 1)[1].strip(),
                    "text": "\n\n".join(clean(p) for p in paragraphs(fbody) if not p.startswith("#")),
                }
                break
        chars = parse_characteristics(body)
        bg.update(chars)
        skills = bg["proficiencies"].get("skills", [])
        pretty = [s.replace("-", " ").title() for s in skills]
        bg["summary"] = ", ".join(pretty)
        if bg.get("feature"):
            bg["summary"] += " - " + bg["feature"]["name"]
        out.append(bg)
    return out


def parse_characteristics(body):
    """Pull the suggested personality trait / ideal / bond / flaw tables apart."""
    out = {"traits": [], "ideals": [], "bonds": [], "flaws": []}
    key_map = {"personality trait": "traits", "ideal": "ideals", "bond": "bonds", "flaw": "flaws"}
    for cap, headers, rows in tables(body):
        current = None
        if len(headers) > 1:
            current = key_map.get(headers[1].strip().lower())
        for row in rows:
            if len(row) < 2:
                continue
            label = row[1].strip()
            if not label:
                continue
            key = key_map.get(label.lower())
            if key and (row[0].strip().lower().startswith("d") or not row[0].strip()):
                current = key
                continue
            if current:
                out[current].append(label)
    return out


def parse_equipment_text(text, items):
    """Turn a background equipment sentence into item refs plus gold."""
    gold = 0
    m = re.search(r"pouch containing (\d+) gp", text)
    if m:
        gold = int(m.group(1))
        text = text[: m.start()] + "a pouch"
    equip = []
    for part in re.split(r",| and ", text):
        part = part.strip(" .")
        if not part:
            continue
        qty = 1
        q = re.match(r"^(\d+|[a-z]+)\s+(.+)$", part)
        if q and (q.group(1).isdigit() or q.group(1) in NUMBER_WORDS):
            word = q.group(1)
            n = int(word) if word.isdigit() else NUMBER_WORDS[word]
            if n > 1:
                qty = n
                part = q.group(2)
        part = re.sub(r"^(a|an|the) ", "", part).strip()
        part = re.sub(r"^set of ", "", part)
        iid = items.resolve(part)
        if iid:
            equip.append({"id": iid, "qty": qty})
        else:
            equip.append({"name": part[:60], "qty": qty})
    return equip, gold


# ---------------------------------------------------------------------------
# classes
# ---------------------------------------------------------------------------

CLASS_META = {
    "barbarian": {"unarmoredAc": "10+dex+con", "multiclassReq": {"str": 13}},
    "bard": {"spellcasting": {"progression": "full", "ability": "cha", "ritual": True,
                              "focus": "musical instrument"}, "multiclassReq": {"cha": 13}},
    "cleric": {"spellcasting": {"progression": "full", "ability": "wis", "prepares": True,
                                "preparedFrom": "list", "ritual": True, "focus": "holy symbol",
                                "preparedFormula": "mod+level"}, "multiclassReq": {"wis": 13}},
    "druid": {"spellcasting": {"progression": "full", "ability": "wis", "prepares": True,
                               "preparedFrom": "list", "ritual": True, "focus": "druidic focus",
                               "preparedFormula": "mod+level"}, "multiclassReq": {"wis": 13}},
    "fighter": {"multiclassReq": {"str": 13}},
    "monk": {"unarmoredAc": "10+dex+wis", "multiclassReq": {"dex": 13, "wis": 13}},
    "paladin": {"spellcasting": {"progression": "half", "ability": "cha", "prepares": True,
                                 "preparedFrom": "list", "startLevel": 2, "focus": "holy symbol",
                                 "preparedFormula": "mod+level/2"},
                "multiclassReq": {"str": 13, "cha": 13}},
    "ranger": {"spellcasting": {"progression": "half", "ability": "wis", "startLevel": 2},
               "multiclassReq": {"dex": 13, "wis": 13}},
    "rogue": {"multiclassReq": {"dex": 13}},
    "sorcerer": {"spellcasting": {"progression": "full", "ability": "cha",
                                  "focus": "arcane focus"}, "multiclassReq": {"cha": 13}},
    "warlock": {"spellcasting": {"progression": "pact", "ability": "cha",
                                 "focus": "arcane focus",
                                 "notes": "pact magic slots recharge on a short rest"},
                "multiclassReq": {"cha": 13}},
    "wizard": {"spellcasting": {"progression": "full", "ability": "int", "prepares": True,
                                "preparedFrom": "spellbook", "ritual": True, "focus": "arcane focus",
                                "preparedFormula": "mod+level"}, "multiclassReq": {"int": 13}},
}

# Feature sections that are really "choose N of these" blocks. The options
# themselves are either "***Name***." paragraphs or "#### Name" subsections.
# count_by_level, when present, is indexed by class level.
CHOICE_FEATURES = {
    "Fighting Style": {"count": 1, "kind": "options"},
    "Metamagic": {"count": 2, "kind": "options",
                  "count_by_level": [0, 0, 0, 2, 2, 2, 2, 2, 2, 2, 3, 3, 3, 3, 3, 3, 3, 4, 4, 4, 4]},
    "Eldritch Invocations": {"count": 2, "kind": "options", "column": "invocations known"},
    "Pact Boon": {"count": 1, "kind": "options"},
}


def parse_choice_options(bodies):
    """Collect the option list of a choice feature from every section that
    shares its name (a class may describe the feature once and list the
    options again at the end of the chapter)."""
    best = []
    for body in bodies:
        opts = [{"name": t["name"], "text": t["text"]} for t in parse_traits(body)]
        if not opts:
            opts = [{"name": name, "text": clean_feature_text(sub)}
                    for name, sub in sections(body, 4)]
        if len(opts) > len(best):
            best = opts
    return best

SUBCLASS_MARKERS = [
    "Primal Path", "Bard College", "Divine Domain", "Druid Circle", "Martial Archetype",
    "Monastic Tradition", "Sacred Oath", "Ranger Archetype", "Roguish Archetype",
    "Sorcerous Origin", "Otherworldly Patron", "Arcane Tradition",
]

ARMOR_WORDS = {
    "light armor": "light", "medium armor": "medium", "heavy armor": "heavy",
    "shields": "shields", "all armor": "all",
}


def parse_proficiency_line(line, kind, items):
    parts = [p.strip() for p in re.split(r",| and ", line) if p.strip()]
    out = []
    for p in parts:
        low = clean(p).lower().strip(" .")
        if kind == "armor":
            if low in ARMOR_WORDS:
                val = ARMOR_WORDS[low]
                if val == "all":
                    out.extend(["light", "medium", "heavy"])
                else:
                    out.append(val)
            continue
        if kind == "weapons":
            if low in ("simple weapons", "martial weapons"):
                out.append(low.split()[0])
                continue
            if low == "none":
                continue
            iid = items.resolve(low)
            out.append(iid if iid else slug(low))
            continue
        if kind == "tools":
            if low in ("none", ""):
                continue
            out.append(clean(p).strip(" ."))
    return out


def parse_class_equipment(body, items):
    """Parse the starting equipment bullet list into choices + fixed gear."""
    choices, fixed = [], []
    equip_section = ""
    for name, sub in sections(body, 4):
        if name.strip().lower() == "equipment":
            # the section body runs on to the level table, keep the bullet list
            equip_section = sub.split("**Table-")[0]
            break
    if not equip_section:
        return choices, fixed
    idx = 0
    for line in equip_section.split("\n"):
        line = line.strip()
        if not line.startswith("- "):
            continue
        line = line[2:].strip()
        markers = list(re.finditer(r"\((?:\*)?([a-z])(?:\*)?\)", line))
        if not markers:
            # a fixed line can still say "any simple weapon", which is a choice
            expanded = expand_option(line, items)
            if len(expanded) > 1:
                idx += 1
                choices.append({"id": "start-%d" % idx, "prompt": "", "options": expanded})
            else:
                items_, _ = parse_option_items(line, items)
                fixed.extend(items_)
            continue
        options = []
        for i, m in enumerate(markers):
            start = m.end()
            end = markers[i + 1].start() if i + 1 < len(markers) else len(line)
            chunk = line[start:end].strip(" ,")
            chunk = re.sub(r"\s+or$", "", chunk).strip(" ,")
            options.extend(expand_option(chunk, items))
        if options:
            idx += 1
            choices.append({
                "id": "start-%d" % idx,
                "prompt": "Choose your starting equipment (%d of %d)." % (idx, 0),
                "options": options,
            })
    for i, ch in enumerate(choices):
        ch["prompt"] = "Choose your starting equipment (%d of %d)." % (i + 1, len(choices))
    return choices, fixed


ANY_RE = re.compile(
    r"^(?:(two|a|an|one)\s+)?(?:any\s+)?(simple|martial)(?: (melee|ranged))? weapons?$")


def expand_option(chunk, items):
    """Expand 'any martial melee weapon' into concrete weapon options."""
    chunk = re.sub(r"\(if proficient\)", "", chunk).strip(" ,.")
    pieces = [p.strip() for p in re.split(r",| and ", chunk) if p.strip()]
    any_piece, rest = None, []
    for p in pieces:
        m = ANY_RE.match(p.lower())
        if m:
            any_piece = m
        else:
            rest.append(p)
    rest_items, rest_labels = [], []
    for p in rest:
        got, label = parse_option_items(p, items)
        rest_items.extend(got)
        rest_labels.append(label)
    if not any_piece:
        return [{"label": chunk, "items": rest_items}]
    qty = NUMBER_WORDS.get(any_piece.group(1) or "a", 1)
    cat_kind, cat_range = any_piece.group(2), any_piece.group(3)
    out = []
    for it in items.items:
        if it["kind"] != "weapon":
            continue
        cat = it.get("category", "")
        if not cat.startswith(cat_kind):
            continue
        if cat_range and not cat.endswith(cat_range):
            continue
        label = it["name"] if qty == 1 else "%d %ss" % (qty, it["name"])
        parts = [label] + [l for l in rest_labels if l]
        out.append({
            "label": ", ".join(parts),
            "items": [{"id": it["id"], "qty": qty}] + rest_items,
        })
    return out


def parse_option_items(text, items):
    """Parse 'two handaxes' / '20 arrows' / 'a light crossbow' into item refs."""
    out = []
    label_parts = []
    for part in re.split(r",| and ", text):
        part = part.strip(" .")
        part = re.sub(r"\(if proficient\)", "", part).strip()
        if not part:
            continue
        qty = 1
        m = re.match(r"^(\d+)\s+(.+)$", part)
        if m:
            qty, part = int(m.group(1)), m.group(2)
        else:
            m = re.match(r"^([a-z]+)\s+(.+)$", part.lower())
            if m and m.group(1) in NUMBER_WORDS and NUMBER_WORDS[m.group(1)] > 1:
                qty = NUMBER_WORDS[m.group(1)]
                part = part.split(None, 1)[1]
        part = re.sub(r"^(A|An|The|a|an|the) ", "", part).strip()
        label_parts.append(part)
        iid = items.resolve(part)
        if iid:
            out.append({"id": iid, "qty": qty})
        else:
            out.append({"name": part[:50], "qty": qty})
    return out, ", ".join(label_parts)


def parse_level_table(text, class_name):
    headers, rows = find_table(text, "The " + class_name)
    if not rows:
        return {}, {}
    cols = {h.strip().lower(): i for i, h in enumerate(headers)}
    per_level = {}
    for row in rows:
        if not row or not row[0].strip():
            continue
        lvl = ordinal_to_int(row[0])
        if not lvl:
            continue
        entry = {}
        for key, i in cols.items():
            if i < len(row):
                entry[key] = row[i].strip()
        per_level[lvl] = entry
    return cols, per_level


def clean_feature_text(body, stop_at_options=False):
    """Flatten a feature section into plain paragraphs.

    Features that are really "choose one of these" blocks (fighting styles,
    invocations, metamagic) keep only their introduction, the options
    themselves become a Choice so the builder can prompt for them.
    """
    out = []
    for para in paragraphs(body):
        if stop_at_options and TRAIT_RE.match(para):
            break
        if para.startswith("|") or para.startswith(">") or para.startswith("**Table-"):
            continue
        if re.match(r"^#+ ", para):
            para = re.sub(r"^#+ ", "", para)
            out.append(clean(para) + ":")
            continue
        out.append(clean(para))
    return "\n\n".join(out).strip()


def parse_classes(src, items, spell_ids):
    classes = []
    dirname = os.path.join(src, "02_Classes")
    for fname in sorted(os.listdir(dirname)):
        if not fname.endswith(".md"):
            continue
        text = open(os.path.join(dirname, fname), encoding="utf-8").read()
        name = re.search(r"^# (.+)$", text, re.MULTILINE).group(1).strip()
        cid = slug(name)
        meta = CLASS_META.get(cid, {})
        body = before_first(text, 2)
        cls = {"id": cid, "name": name, "source": "SRD"}
        m = re.search(r"\*\*Hit Dice:\*\* 1d(\d+)", text)
        cls["hitDie"] = int(m.group(1)) if m else 8
        prof = {}
        m = re.search(r"\*\*Armor:\*\* (.+)", text)
        if m:
            prof["armor"] = parse_proficiency_line(m.group(1), "armor", items)
        m = re.search(r"\*\*Weapons:\*\* (.+)", text)
        if m:
            prof["weapons"] = parse_proficiency_line(m.group(1), "weapons", items)
        m = re.search(r"\*\*Tools:\*\* (.+)", text)
        if m:
            prof["tools"] = parse_proficiency_line(m.group(1), "tools", items)
        cls["proficiencies"] = prof
        m = re.search(r"\*\*Saving Throws:\*\* (.+)", text)
        if m:
            saves = []
            for part in re.split(r",| and ", clean(m.group(1))):
                aid = ABILITIES.get(part.strip().lower())
                if aid:
                    saves.append(aid)
            cls["savingThrows"] = saves
        m = re.search(r"\*\*Skills:\*\* (.+)", text)
        if m:
            line = clean(m.group(1))
            cm = re.match(r"Choose (\w+)", line)
            count = NUMBER_WORDS.get(cm.group(1).lower(), 2) if cm else 2
            cls["skillCount"] = count
            skills = []
            body_line = re.sub(r"^Choose \w+ from ", "", line)
            for part in re.split(r",| and ", body_line):
                sid = SKILL_IDS.get(part.strip().strip(".").lower())
                if sid:
                    skills.append(sid)
            if not skills or "any" in line.lower():
                skills = [s[0] for s in SKILLS]
            cls["skillsFrom"] = skills
        equip_choices, fixed = parse_class_equipment(body, items)
        cls["equipment"] = equip_choices
        cls["fixedEquipment"] = fixed
        cols, per_level = parse_level_table(text, name)

        # features, from the level table plus any left over sections
        feature_bodies = {}
        for n, b in sections(text, 3):
            feature_bodies.setdefault(n, []).append(b)
        # the class features themselves only come from before the first h2
        feature_sections = {}
        for n, b in sections(body, 3):
            feature_sections.setdefault(n, b)
        features, asi_levels, seen = [], [], set()
        subclass_level, subclass_label = 0, ""
        for lvl in sorted(per_level):
            raw = per_level[lvl].get("features", "")
            if not raw or raw == "-":
                continue
            for feat in [f.strip() for f in raw.split(",") if f.strip()]:
                base = re.sub(r"\s*\(.*?\)$", "", feat).strip()
                if base.lower().endswith("feature") or base in ("-",):
                    continue
                if base == "Ability Score Improvement":
                    asi_levels.append(lvl)
                    if base in seen:
                        continue
                if base in SUBCLASS_MARKERS:
                    subclass_level, subclass_label = lvl, base
                seen.add(base)
                body_text = feature_sections.get(base, "")
                features.append({
                    "name": feat, "level": lvl,
                    "text": clean_feature_text(body_text, base in CHOICE_FEATURES) if body_text else "",
                })
        for sec_name, sec_body in feature_sections.items():
            if sec_name in seen or sec_name == "Class Features":
                continue
            lvl = level_from_text(sec_body, 1)
            features.append({"name": sec_name, "level": lvl,
                             "text": clean_feature_text(sec_body, sec_name in CHOICE_FEATURES)})
            seen.add(sec_name)
        features.sort(key=lambda f: (f["level"], f["name"]))
        cls["features"] = features
        if asi_levels:
            cls["asiLevels"] = sorted(set(asi_levels))
        if subclass_level:
            cls["subclassLevel"] = subclass_level
            cls["subclassLabel"] = subclass_label

        # generic choices out of feature sections
        choices = []
        for feat_name, spec in CHOICE_FEATURES.items():
            if feat_name not in feature_sections:
                continue
            options = parse_choice_options(feature_bodies.get(feat_name, []))
            if not options:
                continue
            lvl = next((f["level"] for f in features if f["name"] == feat_name), 1)
            choice = {
                "id": slug(cid + "-" + feat_name), "name": feat_name, "kind": spec["kind"],
                "count": spec["count"], "level": lvl, "options": options,
                "prompt": "Choose your %s." % feat_name.lower(),
            }
            if spec.get("count_by_level"):
                choice["countByLevel"] = spec["count_by_level"]
            elif spec.get("column"):
                by_level = [0] * 21
                for l, row in per_level.items():
                    v = row.get(spec["column"], "")
                    if v and v not in ("-", ""):
                        by_level[l] = int(re.sub(r"\D", "", v) or 0)
                if any(by_level):
                    choice["countByLevel"] = by_level
            choices.append(choice)
        if "Expertise" in feature_sections:
            lvl = next((f["level"] for f in features if f["name"] == "Expertise"), 1)
            choices.append({
                "id": cid + "-expertise", "name": "Expertise", "kind": "expertise", "count": 2,
                "level": lvl, "prompt": "Choose two proficiencies to double (expertise).",
            })
        if choices:
            cls["choices"] = choices

        # spellcasting
        sc = dict(meta.get("spellcasting", {})) if meta.get("spellcasting") else None
        if sc is not None:
            cantrips = [0] * 21
            known = [0] * 21
            for lvl, row in per_level.items():
                if lvl > 20:
                    continue
                c = row.get("cantrips known", "")
                if c and c not in ("-", ""):
                    cantrips[lvl] = int(re.sub(r"\D", "", c) or 0)
                k = row.get("spells known", "")
                if k and k not in ("-", ""):
                    known[lvl] = int(re.sub(r"\D", "", k) or 0)
            if any(cantrips):
                sc["cantripsKnown"] = cantrips
            if any(known):
                sc["spellsKnown"] = known
            if cid == "wizard":
                sc["spellsKnown"] = [0] + [6 + 2 * (l - 1) for l in range(1, 21)]
            sc["spellList"] = cid
            cls["spellcasting"] = sc
        for key in ("unarmoredAc", "multiclassReq"):
            if meta.get(key):
                cls[key] = meta[key]

        # subclasses live under the trailing h2 section
        subclasses = []
        for h2_name, h2_body in sections(text, 2):
            subs = sections(h2_body, 3)
            if not subs:
                continue
            for sub_name, sub_body in subs:
                sub = {"id": slug(sub_name), "name": sub_name, "source": "SRD"}
                intro = [p for p in paragraphs(before_first(sub_body, 4)) if not p.startswith("#")
                         and not p.startswith("|") and not p.startswith("**Table")]
                if intro:
                    sub["summary"] = first_sentence(clean(intro[0]))
                    sub["text"] = clean(intro[0])
                feats = []
                for f_name, f_body in sections(sub_body, 4):
                    feats.append({
                        "name": f_name, "level": level_from_text(f_body, subclass_level or 1),
                        "text": clean_feature_text(f_body),
                    })
                sub["features"] = feats
                spells = []
                for cap, headers, rows in tables(sub_body):
                    if not headers or "spell" not in " ".join(headers).lower():
                        continue
                    if not headers[0].lower().endswith("level"):
                        continue
                    for row in rows:
                        lvl = ordinal_to_int(row[0])
                        if not lvl or len(row) < 2:
                            continue
                        for sp in row[1].split(","):
                            sid = slug(sp.strip())
                            if sid in spell_ids:
                                spells.append({"id": sid, "level": lvl})
                if spells:
                    sub["spells"] = spells
                subclasses.append(sub)
        if subclasses:
            cls["subclasses"] = subclasses
        cls["summary"] = class_summary(cls)
        prim = PRIMARY_ABILITY.get(cid)
        if prim:
            cls["primaryAbility"] = prim
        cls["text"] = CLASS_BLURB.get(cid, "")
        classes.append(cls)
    return classes


PRIMARY_ABILITY = {
    "barbarian": ["str"], "bard": ["cha"], "cleric": ["wis"], "druid": ["wis"],
    "fighter": ["str", "dex"], "monk": ["dex", "wis"], "paladin": ["str", "cha"],
    "ranger": ["dex", "wis"], "rogue": ["dex"], "sorcerer": ["cha"],
    "warlock": ["cha"], "wizard": ["int"],
}

CLASS_BLURB = {
    "barbarian": "A fierce warrior who channels a primal rage.",
    "bard": "An inspiring magician whose power echoes the music of creation.",
    "cleric": "A priestly champion who wields divine magic in service of a higher power.",
    "druid": "A priest of the Old Faith, wielding the powers of nature and adopting animal forms.",
    "fighter": "A master of martial combat, skilled with a variety of weapons and armor.",
    "monk": "A master of martial arts, harnessing the power of the body in pursuit of perfection.",
    "paladin": "A holy warrior bound to a sacred oath.",
    "ranger": "A warrior who uses martial prowess and nature magic to combat threats on the edges of civilisation.",
    "rogue": "A scoundrel who uses stealth and trickery to overcome obstacles and enemies.",
    "sorcerer": "A spellcaster who draws on inherent magic from a gift or bloodline.",
    "warlock": "A wielder of magic derived from a bargain with an extraplanar entity.",
    "wizard": "A scholarly magic user capable of manipulating the structures of reality.",
}


def class_summary(cls):
    blurb = CLASS_BLURB.get(cls["id"], "")
    prim = PRIMARY_ABILITY.get(cls["id"], [])
    bits = ["d%d hit die" % cls["hitDie"]]
    if prim:
        bits.append("%s based" % "/".join(a.upper() for a in prim))
    sc = cls.get("spellcasting")
    if sc:
        bits.append("%s caster" % sc["progression"])
    return "%s (%s)" % (blurb, ", ".join(bits)) if blurb else ", ".join(bits)


def first_sentence(text):
    text = text.replace("\n", " ").strip()
    idx = text.find(". ")
    if idx > 0:
        return text[: idx + 1]
    return text[:160]


# ---------------------------------------------------------------------------
# spells
# ---------------------------------------------------------------------------

SPELL_HEAD_RE = re.compile(r"^\*(.+?)\*$", re.MULTILINE)


def parse_spell_lists(src):
    text = open(os.path.join(src, "07_Spells", "Spell_Lists.md"), encoding="utf-8").read()
    out = {}
    for title, body in sections(text, 2):
        m = re.match(r"(\w[\w-]*) Spells", title.strip())
        if not m:
            continue
        cid = slug(m.group(1))
        for line in body.split("\n"):
            line = line.strip()
            if line.startswith("- "):
                sid = slug(clean(line[2:]))
                out.setdefault(sid, []).append(cid)
    return out


def parse_spells(src, lists):
    dirname = os.path.join(src, "07_Spells", "Spells_Each")
    out = []
    for fname in sorted(os.listdir(dirname)):
        if not fname.endswith(".md"):
            continue
        text = open(os.path.join(dirname, fname), encoding="utf-8").read()
        m = re.search(r"^#+ (.+)$", text, re.MULTILINE)
        if not m:
            continue
        name = clean(m.group(1))
        sid = slug(name)
        spell = {"id": sid, "name": name, "source": "SRD"}
        head = SPELL_HEAD_RE.search(text)
        if head:
            h = head.group(1).strip()
            ritual = "(ritual)" in h
            h = h.replace("(ritual)", "").strip()
            cm = re.match(r"(\d+)(?:st|nd|rd|th)-level (\w+)", h, re.I)
            if cm:
                spell["level"] = int(cm.group(1))
                spell["school"] = cm.group(2).capitalize()
            else:
                cm = re.match(r"(\w+) cantrip", h, re.I)
                spell["level"] = 0
                spell["school"] = cm.group(1).capitalize() if cm else ""
            if ritual:
                spell["ritual"] = True
        for key, field in (("Casting Time", "castingTime"), ("Range", "range"),
                           ("Components", "components"), ("Duration", "duration")):
            fm = re.search(r"\*\*%s:\*\* (.+)" % key, text)
            if fm:
                spell[field] = clean(fm.group(1))
        comps = spell.get("components", "")
        mm = re.search(r"\((.+)\)", comps)
        if mm:
            spell["materials"] = mm.group(1)
            spell["components"] = clean(re.sub(r"\s*\(.+\)", "", comps))
        if spell.get("duration", "").lower().startswith("concentration"):
            spell["concentration"] = True
        # body text
        body_start = text
        for marker in ("**Duration:**",):
            idx = text.find(marker)
            if idx >= 0:
                body_start = text[idx:]
                nl = body_start.find("\n")
                body_start = body_start[nl:]
        paras = [p for p in paragraphs(body_start) if not p.startswith("#")]
        higher = ""
        keep = []
        for p in paras:
            hm = TRAIT_RE.match(p)
            if hm and hm.group(1).lower().startswith("at higher levels"):
                higher = clean(hm.group(2))
                continue
            keep.append(clean(p))
        spell["text"] = "\n\n".join(keep).strip()
        if higher:
            spell["higherLevel"] = higher
        low = spell["text"].lower()
        if "spell attack" in low or "ranged spell attack" in low or "melee spell attack" in low:
            spell["attack"] = True
        sm = re.search(r"make an? (\w+) saving throw", low)
        if sm and sm.group(1) in ABILITIES:
            spell["save"] = ABILITIES[sm.group(1)]
        dm = re.search(r"(\d+d\d+)(?: \+ \d+)? (\w+) damage", low)
        if dm:
            spell["damage"] = "%s %s" % (dm.group(1), dm.group(2))
        spell["classes"] = sorted(set(lists.get(sid, [])))
        out.append(spell)
    return out


# ---------------------------------------------------------------------------
# feats, languages, alignments
# ---------------------------------------------------------------------------

def parse_feats(src):
    path = os.path.join(src, "05_Feats", "Feats.md")
    if not os.path.exists(path):
        return []
    text = open(path, encoding="utf-8").read()
    out = []
    for name, body in sections(text, 2):
        feat = {"id": slug(name), "name": name, "source": "SRD"}
        pre = re.search(r"^\*Prerequisite: (.+?)\*$", body, re.MULTILINE)
        if pre:
            feat["prerequisite"] = clean(pre.group(1))
        paras = [clean(p) for p in paragraphs(body) if not p.startswith("*Prerequisite")]
        feat["text"] = "\n\n".join(p for p in paras if p)
        out.append(feat)
    return out


def parse_languages(src):
    text = open(os.path.join(src, "03_Characterization", "Languages.md"), encoding="utf-8").read()
    langs = []
    for cap, headers, rows in tables(text):
        if not headers or headers[0].lower() != "language":
            continue
        for row in rows:
            if row[0].strip():
                langs.append(row[0].strip())
    return langs


def parse_alignments(src):
    text = open(os.path.join(src, "03_Characterization", "Alignment.md"), encoding="utf-8").read()
    found = []
    for m in re.finditer(r"\*\*(Lawful good|Neutral good|Chaotic good|Lawful neutral|Neutral|Chaotic neutral|Lawful evil|Neutral evil|Chaotic evil)\*\*", text):
        val = m.group(1)
        val = "True Neutral" if val == "Neutral" else val.title()
        if val not in found:
            found.append(val)
    order = ["Lawful Good", "Neutral Good", "Chaotic Good", "Lawful Neutral", "True Neutral",
             "Chaotic Neutral", "Lawful Evil", "Neutral Evil", "Chaotic Evil"]
    return [a for a in order if a in found] or order


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

HEADER = """# ---------------------------------------------------------------------------
# %s
#
# GENERATED FILE - do not edit by hand.
# Regenerate with: python3 tools/dndsrd/gen_srd.py [srd-path] [out-dir]
#
# This work includes material taken from the System Reference Document 5.1
# ("SRD 5.1") by Wizards of the Coast LLC, available at
# https://dnd.wizards.com/resources/systems-reference-document and licensed
# under the Creative Commons Attribution 4.0 International License
# (https://creativecommons.org/licenses/by/4.0/legalcode).
#
# To add or override content, drop your own yaml module with "id: srd" into
# one of your configured dndPaths. See the dnd module documentation.
# ---------------------------------------------------------------------------
"""


def write(path, title, data):
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(HEADER % title)
        fh.write(dump(data))
    print("wrote %-46s %6d bytes" % (path, os.path.getsize(path)))


def main():
    src = sys.argv[1] if len(sys.argv) > 1 else os.path.join("..", "dndsrd")
    out = sys.argv[2] if len(sys.argv) > 2 else os.path.join("internal", "common", "dnd", "data")
    if not os.path.isdir(src):
        sys.exit("SRD source not found: %s" % src)
    os.makedirs(out, exist_ok=True)

    items = Items()
    eq = os.path.join(src, "04_Equipment")
    parse_weapons(open(os.path.join(eq, "Weapons.md"), encoding="utf-8").read(), items)
    parse_armor(open(os.path.join(eq, "Armor.md"), encoding="utf-8").read(), items)
    parse_gear(open(os.path.join(eq, "Adventuring_Gear.md"), encoding="utf-8").read(), items)
    parse_tools(open(os.path.join(eq, "Tools.md"), encoding="utf-8").read(), items)
    for syn in SYNTHETIC_ITEMS:
        items.add(dict(syn))
    for alias, target in EXTRA_ALIASES.items():
        items._alias(slug(alias), target)

    lists = parse_spell_lists(src)
    spells = parse_spells(src, lists)
    spell_ids = {s["id"] for s in spells}
    races = parse_races(src, items)
    backgrounds = parse_backgrounds(src, items)
    classes = parse_classes(src, items, spell_ids)
    feats = parse_feats(src)

    base = {
        "id": "srd",
        "name": "SRD 5.1 Core Rules",
        "version": "5.1",
        "description": "The core races, classes, backgrounds, equipment and spells of "
                       "fifth edition, generated from the SRD 5.1.",
    }

    core = dict(base)
    core["languages"] = parse_languages(src)
    core["alignments"] = parse_alignments(src)
    core["skills"] = [{"id": sid, "name": n, "ability": a, "text": t} for sid, n, a, t in SKILLS]
    core["races"] = races
    core["backgrounds"] = backgrounds
    core["feats"] = feats
    write(os.path.join(out, "srd-core.yaml"), "SRD 5.1: skills, races, backgrounds, feats", core)

    cl = dict(base)
    cl["classes"] = classes
    write(os.path.join(out, "srd-classes.yaml"), "SRD 5.1: classes and subclasses", cl)

    it = dict(base)
    it["items"] = items.items
    write(os.path.join(out, "srd-items.yaml"), "SRD 5.1: weapons, armor, gear and packs", it)

    sp = dict(base)
    sp["spells"] = spells
    write(os.path.join(out, "srd-spells.yaml"), "SRD 5.1: spells", sp)

    print("\nsummary: %d races, %d classes, %d backgrounds, %d items, %d spells, %d feats" % (
        len(races), len(classes), len(backgrounds), len(items.items), len(spells), len(feats)))
    unlisted = [s["id"] for s in spells if not s["classes"]]
    if unlisted:
        print("note: %d spells are not on any class list (%s...)" % (
            len(unlisted), ", ".join(unlisted[:5])))
    unresolved = set()
    for cls in classes:
        for grp in cls.get("equipment", []):
            for opt in grp["options"]:
                unresolved.update(i["name"] for i in opt["items"] if "name" in i)
        unresolved.update(i["name"] for i in cls.get("fixedEquipment", []) if "name" in i)
    for bg in backgrounds:
        unresolved.update(i["name"] for i in bg.get("equipment", []) if "name" in i)
    if unresolved:
        print("note: %d equipment names did not resolve to an item: %s" % (
            len(unresolved), ", ".join(sorted(unresolved))))


if __name__ == "__main__":
    main()
