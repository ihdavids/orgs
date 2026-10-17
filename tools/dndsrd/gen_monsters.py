#!/usr/bin/env python3
"""Generate the monster list behind the random encounter generator.

Usage:
    python3 tools/dndsrd/gen_monsters.py [path-to-dndsrd] [output-file]

Defaults to ../dndsrd and internal/common/dnd/randgen/data/srd-monsters.yaml.

Kept: what an encounter needs (name, size, type, alignment, armour class, hit
points, challenge) and the stat block the initiative tracker shows (ability
scores, speed, the detail lines, traits, actions, reactions and legendary
actions). The SRD says nothing about where a monster
lives, so the environments come from ENVIRONMENTS below, which is ours: a
pattern over the name, then a fallback by creature type.

The source material is the System Reference Document 5.1 by Wizards of the
Coast, licensed under CC-BY-4.0. The generated yaml carries that attribution.
"""

import os
import re
import sys

SRC = sys.argv[1] if len(sys.argv) > 1 else os.path.join("..", "dndsrd")
OUT = sys.argv[2] if len(sys.argv) > 2 else os.path.join(
    "internal", "common", "dnd", "randgen", "data", "srd-monsters.yaml")

ALL = ["arctic", "coastal", "desert", "forest", "grassland", "hill",
       "mountain", "swamp", "underdark", "underwater", "urban", "planar"]

# First match wins, so the specific patterns go before the general ones.
ENVIRONMENTS = [
    (r"black dragon", ["swamp"]),
    (r"blue dragon", ["desert", "coastal"]),
    (r"green dragon", ["forest"]),
    (r"red dragon", ["mountain", "hill"]),
    (r"white dragon", ["arctic"]),
    (r"brass dragon", ["desert"]),
    (r"bronze dragon", ["coastal"]),
    (r"copper dragon", ["hill"]),
    (r"gold dragon", ["forest", "grassland"]),
    (r"silver dragon", ["mountain", "urban"]),
    (r"dragon turtle", ["underwater", "coastal"]),
    (r"pseudodragon", ["forest", "hill", "urban"]),
    (r"wyvern", ["mountain", "hill"]),
    (r"kraken|sahuagin|merfolk|merrow|sea hag|shark|quippers?|octopus|plesiosaurus|killer whale|sea horse|giant crab", ["underwater", "coastal"]),
    (r"^crab$", ["coastal"]),
    (r"aboleth|chuul", ["underdark", "underwater"]),
    (r"drow|duergar|svirfneblin|deep gnome|drider|grimlock|hook horror|cloaker|darkmantle|roper|piercer|purple worm|gibbering|otyugh|ooze|pudding|jelly|gelatinous|rust monster|grick|carrion|shrieker|violet fungus|spider|xorn|umber|mind flayer|beholder|minotaur skeleton", ["underdark"]),
    (r"frost giant|winter wolf|ice mephit|polar bear|mammoth|remorhaz|saber-toothed|yeti|ice devil", ["arctic"]),
    (r"fire giant|magmin|magma mephit|salamander|azer|efreeti|fire elemental", ["mountain", "planar"]),
    (r"cloud giant|storm giant|roc|giant eagle|air elemental|djinni", ["mountain", "coastal"]),
    (r"stone giant|stone golem|earth elemental|gargoyle|galeb", ["mountain", "hill"]),
    (r"hill giant|ettin|ogre|troll", ["hill", "forest", "mountain"]),
    (r"water elemental|steam mephit", ["coastal", "underwater", "planar"]),
    (r"dust mephit|mummy|androsphinx|gynosphinx|lamia|giant scorpion|scorpion|camel|jackal|vulture|giant lizard|behir|bulette|ankheg|gnoll|hyena|death dog", ["desert", "grassland"]),
    (r"lizardfolk|crocodile|green hag|shambling mound|giant frog|frog|toad|will-o|bullywug|swarm of insects|giant poisonous snake|stirge", ["swamp"]),
    (r"treant|dryad|satyr|sprite|pixie|unicorn|owlbear|ettercap|blink dog|awakened|panther|tiger|elk|giant owl|centaur|pegasus|wolf|worg|boar|bear|deer|giant badger|badger|giant boar|weasel|giant wasp|giant centipede|flying snake|constrictor|giant spider|night hag|wererat|werewolf|wereboar|weretiger|werebear", ["forest"]),
    (r"lion|elephant|rhinoceros|triceratops|tyrannosaurus|axe beak|hippogriff|griffon|horse|warhorse|pony|mule|giant goat|goat|hawk|blood hawk|eagle|cockatrice|basilisk|gorgon|chimera|manticore", ["grassland", "hill", "mountain"]),
    (r"harpy|giant ape|ape|baboon|wyrmling", ["hill", "forest", "mountain"]),
    (r"rat|raven|cat|mastiff|bat|swarm|mimic|doppelganger|rakshasa|vampire|wight|ghoul|ghast|ghost|specter|shadow|wraith|zombie|skeleton|lich|cult\w*|thug|spy|noble|guard|knight|priest|acolyte|mage|archmage|assassin|bandit|commoner|gladiator|veteran|imp|quasit|homunculus|succubus|incubus|oni|animated|flying sword|rug of smothering|golem|shield guardian|invisible stalker", ["urban"]),
    (r"tribal warrior|berserker|scout|druid|hobgoblin|goblin|bugbear|orc|kobold", ["forest", "hill", "mountain", "underdark", "grassland"]),
]

FALLBACK = {
    "fiend": ["planar"],
    "celestial": ["planar"],
    "elemental": ["planar"],
    "aberration": ["underdark"],
    "undead": ["urban", "swamp", "underdark"],
    "construct": ["urban", "underdark"],
    "beast": ["forest", "grassland", "hill"],
    "humanoid": ["urban", "forest", "hill"],
    "fey": ["forest"],
    "plant": ["forest", "swamp"],
    "ooze": ["underdark"],
    "giant": ["hill", "mountain"],
    "dragon": ["mountain"],
    "monstrosity": ["hill", "forest", "mountain"],
}

# Planar creatures that also turn up somewhere on this one.
ALSO = {
    "fiend": ["urban", "underdark"],
}


FAMILIES = ["Chromatic", "Metallic", "Ooze", "Demon", "Devil", "Creature", "NPC",
            "Giant", "Angel", "Genie", "Sphinx", "Animated Object", "Dinosaur",
            "Fungi", "Lycanthrope", "Ghoul"]


def quote(s):
    s = s.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")
    return '"' + s + '"'


def slug(s):
    return re.sub(r"[^a-z0-9]+", "-", s.lower()).strip("-")


def parse(path):
    text = open(path, encoding="utf-8").read()
    name = None
    for line in text.splitlines():
        m = re.match(r"^#+\s+(.*\S)\s*$", line)
        if m:
            name = m.group(1)
            break
    ch = re.search(r"^\*\*Challenge\*\*\s+([0-9/]+)\s+\(([0-9,]+) XP\)", text, re.M)
    meta = re.search(r"^\*([A-Z][^*]*)\*\s*$", text, re.M)
    if not name or not ch or not meta:
        return None
    # The wiki files a monster under a family it adds to the title -
    # "Adult Red Dragon (Chromatic)", "Balor (Demon)" - which is not its name.
    name = re.sub(r"\s*\((?:%s)\)$" % "|".join(FAMILIES), "", name)
    line = meta.group(1)
    m = re.match(r"(Tiny|Small|Medium|Large|Huge|Gargantuan)\s+(.*?),\s*(.*)$", line)
    if not m:
        return None
    size, kind, align = m.group(1), m.group(2), m.group(3)
    swarm = kind.startswith("swarm")
    sub = ""
    sm = re.match(r"(.*?)\s*\((.*)\)$", kind)
    if sm:
        kind, sub = sm.group(1), sm.group(2)
    if swarm:
        kind = "swarm"
    ac = re.search(r"^\*\*Armor Class\*\*\s+(\d+)(?:\s*\(([^)]*)\))?", text, re.M)
    hp = re.search(r"^\*\*Hit Points\*\*\s+(\d+)(?:\s+\(([^)]*)\))?", text, re.M)
    return {
        "id": slug(name),
        "name": name,
        "size": size.lower(),
        "type": kind.strip().lower(),
        "subtype": sub.strip().lower(),
        "alignment": align.strip(),
        "ac": int(ac.group(1)) if ac else 10,
        "hp": int(hp.group(1)) if hp else 1,
        "hitDice": (hp.group(2) or "") if hp else "",
        "cr": ch.group(1),
        "xp": int(ch.group(2).replace(",", "")),
        **statblock(text),
        "acNote": (ac.group(2) or "") if ac else "",
    }


def plain(s):
    """Markdown emphasis off: the tracker draws its own."""
    s = re.sub(r"\*\*\*([^*]+)\*\*\*", r"\1", s)
    s = re.sub(r"\*\*([^*]+)\*\*", r"\1", s)
    s = re.sub(r"\*([^*]+)\*", r"\1", s)
    s = s.replace("_", "")
    return s.strip()


DETAILS = ["Saving Throws", "Skills", "Damage Vulnerabilities", "Damage Resistances",
           "Damage Immunities", "Condition Immunities", "Senses", "Languages"]


def statblock(text):
    out = {"speed": "", "abilities": [], "details": [], "traits": [], "actions": [],
           "reactions": [], "legendary": [], "legendaryIntro": ""}
    sp = re.search(r"^\*\*Speed\*\*\s+(.*)$", text, re.M)
    if sp:
        out["speed"] = plain(sp.group(1))
    row = re.search(r"^\|\s*(\d+)\s*\([^)]*\)\s*\|\s*(\d+)\s*\([^)]*\)\s*\|\s*(\d+)\s*\([^)]*\)\s*\|\s*(\d+)\s*\([^)]*\)\s*\|\s*(\d+)\s*\([^)]*\)\s*\|\s*(\d+)\s*\([^)]*\)\s*\|", text, re.M)
    if row:
        out["abilities"] = [int(row.group(i)) for i in range(1, 7)]
    for label in DETAILS:
        m = re.search(r"^\*\*" + re.escape(label) + r"\*\*\s+(.*)$", text, re.M)
        if m:
            out["details"].append([label, plain(m.group(1))])
    # Traits run from after the challenge line to the first ###### section.
    body = text.split("\n")
    section = None
    after_challenge = False
    for line in body:
        t = line.strip()
        if t.startswith("**Challenge**"):
            after_challenge = True
            section = "traits"
            continue
        if not after_challenge or not t:
            continue
        m = re.match(r"^#{2,6}\s+(.*)$", t)
        if m:
            name = m.group(1).strip().lower()
            section = {"actions": "actions", "reactions": "reactions", "legendary actions": "legendary"}.get(name)
            continue
        if section is None:
            continue
        e = re.match(r"^\*{2,3}([^*]+?)\*{2,3}\.?\s*(.*)$", t)
        if e:
            out[section].append({"name": plain(e.group(1)).rstrip("."), "text": plain(e.group(2))})
        elif out[section]:
            out[section][-1]["text"] += "\n" + plain(t)
        elif section == "legendary":
            out["legendaryIntro"] = plain(t)
    return out


def environments(mon):
    name = mon["name"].lower()
    for pat, envs in ENVIRONMENTS:
        if re.search(r"\b(?:%s)\b" % pat, name):
            return envs
    base = mon["type"]
    if base == "swarm":
        base = "beast"
    envs = list(FALLBACK.get(base, ALL))
    return envs + [e for e in ALSO.get(base, []) if e not in envs]


def main():
    each = os.path.join(SRC, "10_Monsters", "Monsters_Each")
    mons = []
    for f in sorted(os.listdir(each)):
        if not f.endswith(".md"):
            continue
        mon = parse(os.path.join(each, f))
        if mon is None:
            continue
        mon["environments"] = environments(mon)
        mons.append(mon)
    os.makedirs(os.path.dirname(OUT), exist_ok=True)
    with open(OUT, "w", encoding="utf-8") as out:
        out.write("# GENERATED by tools/dndsrd/gen_monsters.py - do not edit by hand.\n")
        out.write("#\n")
        out.write("# This work includes material taken from the System Reference Document 5.1\n")
        out.write("# (\"SRD 5.1\") by Wizards of the Coast LLC, available at\n")
        out.write("# https://dnd.wizards.com/resources/systems-reference-document and licensed\n")
        out.write("# under the Creative Commons Attribution 4.0 International License\n")
        out.write("# (https://creativecommons.org/licenses/by/4.0/legalcode).\n")
        out.write("# The environments are not from the SRD.\n")
        out.write("monsters:\n")
        for m in mons:
            out.write("  - id: %s\n" % quote(m["id"]))
            out.write("    name: %s\n" % quote(m["name"]))
            out.write("    size: %s\n" % quote(m["size"]))
            out.write("    type: %s\n" % quote(m["type"]))
            if m["subtype"]:
                out.write("    subtype: %s\n" % quote(m["subtype"]))
            out.write("    alignment: %s\n" % quote(m["alignment"]))
            out.write("    ac: %d\n" % m["ac"])
            out.write("    hp: %d\n" % m["hp"])
            if m["hitDice"]:
                out.write("    hitDice: %s\n" % quote(m["hitDice"]))
            out.write("    cr: %s\n" % quote(m["cr"]))
            out.write("    xp: %d\n" % m["xp"])
            out.write("    environments: [%s]\n" % ", ".join(quote(e) for e in m["environments"]))
            if m["acNote"]:
                out.write("    acNote: %s\n" % quote(m["acNote"]))
            if m["speed"]:
                out.write("    speed: %s\n" % quote(m["speed"]))
            if m["abilities"]:
                out.write("    abilities: [%s]\n" % ", ".join(str(a) for a in m["abilities"]))
            if m["details"]:
                out.write("    details:\n")
                for k, v in m["details"]:
                    out.write("      - [%s, %s]\n" % (quote(k), quote(v)))
            for key in ["traits", "actions", "reactions", "legendary"]:
                if m[key]:
                    out.write("    %s:\n" % key)
                    for e in m[key]:
                        out.write("      - name: %s\n        text: %s\n" % (quote(e["name"]), quote(e["text"])))
            if m["legendaryIntro"]:
                out.write("    legendaryIntro: %s\n" % quote(m["legendaryIntro"]))
    print("wrote %d monsters to %s" % (len(mons), OUT), file=sys.stderr)


if __name__ == "__main__":
    main()
