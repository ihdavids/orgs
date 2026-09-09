#!/usr/bin/env python3
"""Generate the fallback race portraits shipped with the dnd module.

A character sheet with no DND_IMAGE of its own still wants a face in its
medallion, so the engine falls back to one of these by race. They are drawn
rather than photographed on purpose: a flat, stylised bust in the sheet's own
parchment and ink, so a generated portrait never pretends to be somebody's
actual character art.

Every portrait is built from the same pieces - a lit background, a cloaked
pair of shoulders, a head, a face, and whatever the race carries on top of it
- so the set reads as one hand. What differs is what the race's own entry
says about it: a tabaxi gets a cat's muzzle, ears and slit pupils, a
dragonborn a scaled snout and swept horns, a warforged plates and a lit eye
slit, a triton fins and gills. A portrait that could be swapped for another
race's and not be noticed is a portrait that is not finished.

The face matters as much as the silhouette. A bare oval with hair around it
reads as a person the viewer did not mean to draw, so every portrait here has
brows, eyes and a mouth.

    python3 tools/dndportraits/gen_portraits.py

writes internal/common/dnd/data/portraits/*.svg. They are embedded into the
binary from there, so rerun this and rebuild after changing anything here.

The medallion on the html sheet crops the square to its inscribed circle, so
nothing that matters may stray into the corners.
"""

import os
import sys

OUT = os.path.join("internal", "common", "dnd", "data", "portraits")

MID = 200                       # the vertical axis everything is mirrored about

# The head everything else is hung off. A race may widen, raise or replace it.
HEAD = dict(cx=200, cy=170, rx=58, ry=72)


# ------------------------------------------------------------------ drawing

def ellipse(cx, cy, rx, ry, fill, extra=""):
    return ('<ellipse cx="%g" cy="%g" rx="%g" ry="%g" fill="%s"%s/>'
            % (cx, cy, rx, ry, fill, extra))


def circle(cx, cy, r, fill, extra=""):
    return '<circle cx="%g" cy="%g" r="%g" fill="%s"%s/>' % (cx, cy, r, fill, extra)


def fill_path(d, fill, extra=""):
    return '<path d="%s" fill="%s"%s/>' % (d, fill, extra)


def line_path(d, stroke, width, extra=""):
    return ('<path d="%s" fill="none" stroke="%s" stroke-width="%g" '
            'stroke-linecap="round" stroke-linejoin="round"%s/>'
            % (d, stroke, width, extra))


def mirror(s):
    """Whatever was drawn on the left, flipped onto the right."""
    return '<g transform="translate(%d,0) scale(-1,1)">%s</g>' % (MID * 2, s)


def both(s):
    return s + mirror(s)


def shade(hex_color, factor=0.62):
    """A darker version of a colour, for the lines drawn onto a face."""
    n = hex_color.lstrip("#")
    rgb = [int(n[i:i + 2], 16) for i in (0, 2, 4)]
    return "#%02x%02x%02x" % tuple(max(0, min(255, int(v * factor))) for v in rgb)


def tint(hex_color, factor=1.22):
    return shade(hex_color, factor)


# ------------------------------------------------------------------- a face
#
# Brows, eyes, a nose and a mouth. Every race gets all four, because a head
# with none of them is not a portrait of anybody. What varies is the shape:
# a dwarf's brow is heavy and level, an elf's is thin and swept, a tabaxi's
# pupils are slits and a warforged's eyes are lit rather than looked out of.

def brows(ink, y=146, dx=25, length=42, thick=8, tilt=5, arc=7, opacity=1.0):
    x0, x1 = MID - dx - length / 2.0, MID - dx + length / 2.0
    d = "M%g %g Q %g %g %g %g" % (x0, y + tilt, (x0 + x1) / 2.0, y - arc, x1, y - tilt)
    o = "" if opacity >= 1 else ' opacity="%g"' % opacity
    return both(line_path(d, ink, thick, o))


def eyes(sclera, iris, ink, cy=176, dx=25, rx=13, ry=9.5,
         pupil="round", pr=4.6, lid=True, shine=True):
    """A pair of eyes. pupil is "round", "slit" (a cat or a dragon), "glow"
    (a solid lit eye with a halo, for a tiefling or a construct) or "dot"."""
    cx = MID - dx
    out = []
    if pupil == "glow":
        out.append(ellipse(cx, cy, rx + 5, ry + 5, iris, ' opacity=".28"'))
        out.append(ellipse(cx, cy, rx, ry, iris))
        out.append(ellipse(cx, cy, rx * 0.5, ry * 0.5, "#fffbe8", ' opacity=".8"'))
        return both("".join(out))

    out.append(ellipse(cx, cy, rx, ry, sclera))
    ir = min(ry, 7.6)
    out.append(circle(cx, cy, ir, iris))
    if pupil == "slit":
        out.append(ellipse(cx, cy, 2.3, ir * 0.98, ink))
    elif pupil == "dot":
        out.append(circle(cx, cy, pr * 0.7, ink))
    else:
        out.append(circle(cx, cy, pr, ink))
    if lid:
        out.append(line_path("M%g %g Q %g %g %g %g"
                             % (cx - rx - 1, cy - 1, cx, cy - ry - 3.5, cx + rx + 1, cy - 1),
                             ink, 2.8))
    if shine:
        out.append(circle(cx - 2.6, cy - 3.2, 2.0, "#ffffff", ' opacity=".85"'))
    return both("".join(out))


def nose(ink, y=200, w=8, h=20, kind="line"):
    """kind: "line" a plain bridge, "button" small and turned up, "broad" a
    flat wide one, "bulb" the great round nose a gnome or a firbolg wears."""
    if kind == "bulb":
        return (ellipse(MID, y + 4, w * 1.9, h * 0.66, ink, ' opacity=".35"') +
                both(circle(MID - w * 0.9, y + 8, 2.4, ink, ' opacity=".5"')))
    if kind == "broad":
        return (line_path("M%g %g L %g %g Q %g %g %g %g"
                          % (MID - 2, y - h * 0.6, MID - w, y + h * 0.4,
                             MID, y + h * 0.72, MID + w, y + h * 0.4), ink, 3.4,
                          ' opacity=".6"') +
                both(circle(MID - w * 0.75, y + h * 0.35, 2.6, ink, ' opacity=".55"')))
    if kind == "button":
        return both(circle(MID - w * 0.55, y + h * 0.25, 2.2, ink, ' opacity=".55"')) + \
            line_path("M%g %g Q %g %g %g %g"
                      % (MID - w * 0.8, y + h * 0.3, MID, y + h * 0.6,
                         MID + w * 0.8, y + h * 0.3), ink, 2.6, ' opacity=".5"')
    return (line_path("M%g %g L %g %g Q %g %g %g %g"
                      % (MID - 1.5, y - h * 0.5, MID - w * 0.6, y + h * 0.35,
                         MID, y + h * 0.6, MID + w * 0.7, y + h * 0.25),
                      ink, 3.0, ' opacity=".55"'))


def mouth(ink, y=228, w=24, kind="line", thick=3.6):
    """kind: "line" level, "smile", "grim" a downturned set, "open" a dark
    gap with a lip over it."""
    if kind == "open":
        return (fill_path("M%g %g Q %g %g %g %g Q %g %g %g %g Z"
                          % (MID - w, y, MID, y - 6, MID + w, y,
                             MID, y + 12, MID - w, y), shade(ink, 0.45)) +
                line_path("M%g %g Q %g %g %g %g" % (MID - w, y, MID, y - 6, MID + w, y),
                          ink, 2.6))
    if kind == "smile":
        d = "M%g %g Q %g %g %g %g" % (MID - w, y - 2, MID, y + 8, MID + w, y - 2)
    elif kind == "grim":
        d = "M%g %g Q %g %g %g %g" % (MID - w, y + 3, MID, y - 4, MID + w, y + 3)
    else:
        d = "M%g %g Q %g %g %g %g" % (MID - w, y, MID, y + 3, MID + w, y)
    return line_path(d, ink, thick, ' opacity=".7"')


def cheeks(color, y=206, dx=40, r=13, opacity=".3"):
    return both(circle(MID - dx, y, r, color, ' opacity="%s"' % opacity))


# --------------------------------------------------------------------- ears

def round_ear(face, ink, cy=182, dx=59, rx=11, ry=16):
    p = (ellipse(MID - dx, cy, rx, ry, face) +
         line_path("M%g %g q -4 6 0 12" % (MID - dx + 2, cy - 6), ink, 2.2, ' opacity=".4"'))
    return both(p)


def pointed_ear(face, ink, reach=42, up=48, cy=170, dx=52):
    """The long swept ear of an elf, and the shorter one of a half-elf."""
    x, y = MID - dx, cy
    p = (fill_path("M%g %g L %g %g L %g %g Z"
                   % (x + 4, y - 12, x - reach, y - up, x + 2, y + 20), face) +
         line_path("M%g %g L %g %g" % (x - 2, y + 4, x - reach + 9, y - up + 12),
                   ink, 2.2, ' opacity=".35"'))
    return both(p)


def cat_ear(fur, inner, ink, dx=46, base=118, tipx=44, tipy=76):
    """A tall triangular ear standing off the top of the skull, with the pink
    of the inside showing. Two of these are most of what makes a tabaxi."""
    x = MID - dx
    p = (fill_path("M%g %g L %g %g L %g %g Z"
                   % (x + 26, base - 4, x - tipx + 26, base - tipy, x - 8, base + 16), fur) +
         fill_path("M%g %g L %g %g L %g %g Z"
                   % (x + 19, base - 2, x - tipx + 30, base - tipy + 16, x - 1, base + 8), inner) +
         line_path("M%g %g L %g %g" % (x + 26, base - 4, x - tipx + 26, base - tipy),
                   ink, 2.0, ' opacity=".3"'))
    return both(p)


def fin_ear(fin, ink, dx=54, cy=176):
    """A triton's ear: a webbed fin swept back off the side of the head."""
    x = MID - dx
    p = (fill_path("M%g %g C %g %g %g %g %g %g C %g %g %g %g %g %g Z"
                   % (x + 6, cy - 16, x - 20, cy - 34, x - 40, cy - 26, x - 44, cy - 4,
                      x - 30, cy + 6, x - 14, cy + 18, x + 6, cy + 16), fin))
    for i in range(3):
        p += line_path("M%g %g L %g %g"
                       % (x + 2, cy - 8 + i * 9, x - 34 + i * 5, cy - 18 + i * 13),
                       ink, 2.0, ' opacity=".35"')
    return both(p)


def long_ear(face, ink, dx=54, cy=172, reach=34, drop=40):
    """The long, low, faintly bovine ear a firbolg is described with."""
    x = MID - dx
    p = (fill_path("M%g %g C %g %g %g %g %g %g C %g %g %g %g %g %g Z"
                   % (x + 6, cy - 14, x - 18, cy - 22, x - reach, cy + 4,
                      x - reach + 4, cy + drop,
                      x - 14, cy + drop - 6, x + 2, cy + 16, x + 6, cy - 14), face) +
         line_path("M%g %g C %g %g %g %g %g %g"
                   % (x + 2, cy - 6, x - 14, cy - 6, x - reach + 10, cy + 10,
                      x - reach + 9, cy + drop - 12), ink, 2.2, ' opacity=".35"'))
    return both(p)


# --------------------------------------------------------------------- hair

def hair_cap(hair, top=94, drop=178, inner=120, wide=60):
    """The crown of the head, a crescent sitting over the face."""
    l, r = MID - wide, MID + wide
    d = (f"M{l} {drop} C {l} {top + 16} {l + 26} {top} {MID} {top} "
         f"C {r - 26} {top} {r} {top + 16} {r} {drop} "
         f"C {r - 6} {inner + 12} {r - 28} {inner} {MID} {inner} "
         f"C {l + 28} {inner} {l + 6} {inner + 12} {l} {drop} Z")
    return fill_path(d, hair)


def long_hair(hair, fall=316, wide=60, top=88):
    """The crown again, with the length falling past the shoulders. The fall
    is a broad flaring mass rather than a strand each side: two thin strands
    read as a hairstyle nobody asked for."""
    l, r = MID - wide, MID + wide
    d = (f"M{l} 176 "
         f"C {l} 104 {l + 26} {top} {MID} {top} "
         f"C {r - 26} {top} {r} 104 {r} 176 "
         f"C {r + 14} {fall - 90} {r + 16} {fall - 30} {r + 8} {fall} "
         f"L {r - 44} {fall} "
         f"C {r - 32} {fall - 44} {r - 26} 202 {r - 26} 150 "
         f"C {r - 26} 122 {r - 46} 112 {MID} 112 "
         f"C {l + 46} 112 {l + 26} 122 {l + 26} 150 "
         f"C {l + 26} 202 {l + 32} {fall - 44} {l + 44} {fall} "
         f"L {l - 8} {fall} "
         f"C {l - 16} {fall - 30} {l - 14} {fall - 90} {l} 176 Z")
    return fill_path(d, hair)


def beard(hair, jaw=188, chin=326, half=54):
    """A beard covering the lower face and hanging below the jaw. A dwarf's is
    wider than their head, which is most of what makes it a dwarf's."""
    l, r = MID - half, MID + half
    d = (f"M{l} {jaw} C {l} 250 {l + 16} {chin} {MID} {chin} "
         f"C {r - 16} {chin} {r} 250 {r} {jaw} "
         f"C {r} 214 {r - 16} 226 {MID} 226 "
         f"C {l + 16} 226 {l} 214 {l} {jaw} Z")
    return fill_path(d, hair)


def moustache(hair, y=214, half=34):
    d = (f"M{MID} {y} C {MID - 12} {y - 6} {MID - half} {y - 2} {MID - half - 4} {y + 14} "
         f"C {MID - half + 8} {y + 12} {MID - 12} {y + 10} {MID} {y + 8} Z")
    return both(fill_path(d, hair))


def braid(hair, x=152, y=252, n=3):
    p = ""
    for i in range(n):
        p += ellipse(x - i * 3, y + i * 29, 13 - i * 2, 17 - i * 2, hair)
    return both('<g>%s</g>' % p)


def curls(hair, wide=62):
    """A halfling's hair: a cap with a scalloped edge."""
    out = [hair_cap(hair, top=98, drop=184, inner=126, wide=wide)]
    for cx, cy, r in ((148, 134, 23), (176, 110, 25), (224, 110, 25),
                      (252, 134, 23), (200, 98, 23)):
        out.append(circle(cx, cy, r, hair))
    return "".join(out)


def topknot(hair):
    return fill_path("M186 98 C 180 62 190 36 208 22 C 206 54 216 78 228 94 Z", hair)


def hood(cloak, lining, edge):
    """A hood pulled up over the head. It has to read as cloth rather than as
    hair, so it is wide at the brow and deep, the inside of it is a lighter
    lining, and it stops at the shoulders rather than running to the foot of
    the picture - a cowl that never opens is a habit, not a traveller."""
    return (fill_path("M74 344 C 68 186 126 58 200 58 "
                      "C 274 58 332 186 326 344 Z", cloak) +
            fill_path("M132 308 C 128 200 158 112 200 112 "
                      "C 242 112 272 200 268 308 "
                      "C 244 292 156 292 132 308 Z", lining) +
            line_path("M132 304 C 128 200 158 112 200 112 "
                      "C 242 112 272 200 268 304", edge, 3.5, ' opacity=".55"') +
            line_path("M98 320 C 92 188 134 82 200 82 "
                      "C 266 82 308 188 302 320", edge, 2.2, ' opacity=".28"'))


def hood_shadow(ink, y=150):
    """The shadow the brow of a hood throws across the top of the face."""
    return fill_path("M142 %g C 160 %g 240 %g 258 %g "
                     "C 250 %g 150 %g 142 %g Z"
                     % (y, y - 22, y - 22, y, y + 12, y + 12, y),
                     ink, ' opacity=".28"')


def hat(hair, brim):
    """A gnome's hat: a floppy cone with a brim across the brow."""
    return (fill_path("M124 120 C 144 56 178 18 216 6 C 222 56 246 96 282 120 Z", hair) +
            fill_path("M114 124 C 158 100 246 100 290 124 C 246 144 158 144 114 124 Z", brim))


# ------------------------------------------------------- horns, scales, kit

def horn(color, d, edge=None):
    p = fill_path(d, color)
    if edge:
        p += line_path(d.replace("Z", ""), edge, 1.6, ' opacity=".35"')
    return both(p)


SWEPT_HORN = ("M158 132 C 128 126 96 108 80 70 C 104 72 136 92 152 110 "
              "C 160 118 164 126 168 130 Z")
CURLED_HORN = ("M160 112 C 126 96 110 58 128 20 C 142 28 138 62 152 84 "
               "C 160 96 170 104 176 108 Z")
RAM_HORN = ("M158 122 C 118 116 92 82 100 40 C 118 46 116 82 134 96 "
            "C 144 104 156 112 164 118 Z")


def scales(color, rows=((174, 124), (200, 114), (226, 124), (158, 152), (242, 152))):
    out = []
    for cx, cy in rows:
        out.append(line_path("M%g %g q 11 -13 22 0" % (cx - 11, cy), color, 4,
                             ' opacity=".5"'))
    return "".join(out)


def spots(color, marks, r=5, opacity=".45"):
    return "".join(circle(x, y, r, color, ' opacity="%s"' % opacity)
                   for x, y in marks)


def stripes(color, marks, width=5, opacity=".4"):
    """Short arcs of darker fur, for a tabaxi's markings."""
    out = []
    for x, y, w, h in marks:
        out.append(line_path("M%g %g q %g %g %g %g" % (x, y, w / 2.0, h, w, 0),
                             color, width, ' opacity="%s"' % opacity))
    return "".join(out)


def whiskers(color, y=214, dx=30, n=3):
    out = []
    for i in range(n):
        out.append(line_path("M%g %g C %g %g %g %g %g %g"
                             % (MID - dx, y + i * 7 - 6,
                                MID - dx - 22, y + i * 9 - 12,
                                MID - dx - 44, y + i * 11 - 14,
                                MID - dx - 62, y + i * 12 - 12),
                             color, 2.0, ' opacity=".55"'))
    return both("".join(out))


def muzzle(face, ink, nose_color, cx=200, cy=214, rx=42, ry=32, nose_w=13):
    """A short furred or scaled muzzle, seen head on: the pad of the nose, the
    line down from it and the mouth split under it."""
    out = [ellipse(cx, cy, rx, ry, face, ' opacity=".55"'),
           fill_path("M%g %g Q %g %g %g %g Q %g %g %g %g Z"
                     % (cx - nose_w, cy - 12, cx, cy - 22, cx + nose_w, cy - 12,
                        cx, cy + 4, cx - nose_w, cy - 12), nose_color),
           line_path("M%g %g L %g %g" % (cx, cy + 3, cx, cy + 14), ink, 2.8,
                     ' opacity=".6"'),
           both(line_path("M%g %g Q %g %g %g %g"
                          % (cx, cy + 14, cx - 12, cy + 20, cx - 22, cy + 12),
                          ink, 3.0, ' opacity=".6"'))]
    return "".join(out)


def snout(face, ink, shade_color, cx=200, cy=222, rx=46, ry=36):
    """A dragonborn's muzzle: longer, ridged, and with the nostrils high."""
    return (ellipse(cx, cy, rx, ry, face) +
            line_path("M%g %g Q %g %g %g %g" % (cx - rx + 8, cy + 8, cx, cy + 22,
                                                cx + rx - 8, cy + 8), ink, 3.4,
                      ' opacity=".6"') +
            both(circle(cx - 13, cy - 12, 5, shade_color, ' opacity=".75"')) +
            both(line_path("M%g %g q 10 -8 20 -2" % (cx - rx + 6, cy - 2),
                           shade_color, 3, ' opacity=".4"')))


def tusks(color, y=232, dx=17, h=30):
    p = fill_path("M%g %g L %g %g L %g %g Z"
                  % (MID - dx - 7, y, MID - dx, y - h, MID - dx + 7, y), color)
    return both(p)


def gills(ink, y=272, dx=40):
    p = ""
    for i in range(3):
        p += line_path("M%g %g q 8 5 0 10" % (MID - dx + i * 9, y + i * 2), ink, 2.6,
                       ' opacity=".5"')
    return both(p)


def halo(color, cy=78, r=54):
    return (line_path("M%g %g a %g %g 0 1 1 .1 0" % (MID - r, cy, r, r * 0.34),
                      color, 5, ' opacity=".75"') +
            line_path("M%g %g a %g %g 0 1 1 .1 0" % (MID - r + 8, cy, r - 8, (r - 8) * 0.34),
                      color, 2, ' opacity=".45"'))


def plating(seam, joints):
    """The plates and seams of a warforged face."""
    out = []
    for d in joints:
        out.append(line_path(d, seam, 3.0, ' opacity=".65"'))
    return "".join(out)


def rune_marks(color, marks):
    return "".join(line_path(d, color, 3.2, ' opacity=".55"') for d in marks)


# ---------------------------------------------------------------- the races
#
# bg is the light behind them, cloak the shoulders, face the head, hair
# everything worn on it, and light a highlight used for tusks and rim light.
# under is drawn behind the head, over in front of it, and face_of draws the
# brows, eyes, nose and mouth - which every race has.

def human_face(p):
    return (brows(p["ink"], y=146, length=44) +
            eyes(p["sclera"], "#5d4526", p["ink"]) +
            nose(p["ink"], y=198, kind="line") +
            mouth(p["ink"], y=230, w=23))


RACES = [
    # ---------------------------------------------------------------- human
    dict(id="human", label="a human",
         bg=("#f2e5c9", "#c8ac80"), cloak=("#4c4033", "#2a231b"),
         face="#a8825c", hair="#3a2e23", light="#efe3c8",
         over=lambda p: (hair_cap(p["hair"], top=94, drop=170, inner=118) +
                         round_ear(p["face"], p["ink"])),
         face_of=human_face),

    # ------------------------------------------------------------------ elf
    # Slender and angular, with the long swept ears and the long hair the
    # race entry leads with.
    dict(id="elf", label="an elf",
         bg=("#e6efdd", "#a9bf9a"), cloak=("#3e4a3c", "#232b22"),
         face="#c6a884", hair="#6b552f", light="#f0f3e6",
         head=dict(rx=52, ry=76, cy=168),
         under=lambda p: long_hair(p["hair"], fall=318, wide=58),
         over=lambda p: (pointed_ear(p["face"], p["ink"], reach=46, up=54, dx=48) +
                         hair_cap(p["hair"], top=90, drop=150, inner=112, wide=56)),
         face_of=lambda p: (brows(p["ink"], y=142, length=44, thick=6, tilt=8, arc=4) +
                            eyes(p["sclera"], "#3f6f5e", p["ink"], cy=174, rx=14, ry=8.5) +
                            nose(p["ink"], y=198, w=7, kind="line") +
                            mouth(p["ink"], y=230, w=20, thick=3.0))),

    # ------------------------------------------------------------- half-elf
    dict(id="half-elf", label="a half-elf",
         bg=("#e2ebe6", "#a3b5ae"), cloak=("#3d4744", "#232a28"),
         face="#b18f68", hair="#4a3826", light="#eef3f0",
         head=dict(rx=55, ry=74),
         under=lambda p: long_hair(p["hair"], fall=288, wide=59),
         over=lambda p: (pointed_ear(p["face"], p["ink"], reach=26, up=30, dx=52) +
                         hair_cap(p["hair"], top=92, drop=158, inner=114, wide=58)),
         face_of=lambda p: (brows(p["ink"], y=144, length=43, thick=7) +
                            eyes(p["sclera"], "#4c6a4a", p["ink"], cy=175, rx=13.5) +
                            nose(p["ink"], y=199) +
                            mouth(p["ink"], y=231, w=22))),

    # ---------------------------------------------------------------- dwarf
    # Broad, heavy browed, and mostly beard - braided, because a dwarf's is.
    dict(id="dwarf", label="a dwarf",
         bg=("#f0e0c6", "#bf9f75"), cloak=("#4a3a2c", "#291f16"),
         face="#c0906a", hair="#8a4f21", light="#f2e6cf",
         head=dict(rx=64, ry=70, cy=166),
         over=lambda p: (hair_cap(p["hair"], top=86, drop=164, inner=110, wide=66) +
                         round_ear(p["face"], p["ink"], cy=180, dx=64) +
                         beard(p["hair"], jaw=182, chin=334, half=76) +
                         braid(p["hair"], x=140, y=254) +
                         moustache(shade(p["hair"], 0.86), y=212, half=38)),
         face_of=lambda p: (brows(p["hair"], y=142, dx=27, length=50, thick=13,
                                  tilt=2, arc=3) +
                            eyes(p["sclera"], "#4a6b8a", p["ink"], cy=176, dx=27,
                                 rx=13, ry=8.5) +
                            nose(p["ink"], y=196, w=10, h=22, kind="broad"))),

    # ------------------------------------------------------------- halfling
    # A round, cheerful face under a mop of curls - the entry's "cheerful and
    # rosy" rather than a small human.
    dict(id="halfling", label="a halfling",
         bg=("#f5ecd2", "#cbb583"), cloak=("#4a4230", "#2a2419"),
         face="#c69a6d", hair="#7b5427", light="#f3ecd8",
         head=dict(rx=63, ry=66, cy=178),
         over=lambda p: (curls(p["hair"]) + round_ear(p["face"], p["ink"], cy=188, dx=63)),
         face_of=lambda p: (brows(p["ink"], y=158, dx=26, length=40, thick=7, arc=9) +
                            eyes(p["sclera"], "#6b4a24", p["ink"], cy=188, dx=26,
                                 rx=14, ry=10.5, pr=5.2) +
                            nose(p["ink"], y=210, w=8, h=16, kind="button") +
                            mouth(p["ink"], y=236, w=22, kind="smile") +
                            cheeks("#c96a52", y=214, dx=42, r=15))),

    # ---------------------------------------------------------------- gnome
    dict(id="gnome", label="a gnome",
         bg=("#f6e7c4", "#cfa860"), cloak=("#4a3f2a", "#2a2317"),
         face="#c19a70", hair="#b9b0a2", light="#f5ead0",
         head=dict(rx=61, ry=68, cy=180),
         over=lambda p: (pointed_ear(p["face"], p["ink"], reach=28, up=26, dx=56) +
                         hat(p["cloak"][0], p["cloak"][1]) +
                         beard(p["hair"], jaw=210, chin=312, half=58) +
                         moustache(p["hair"], y=224, half=34)),
         face_of=lambda p: (brows(p["hair"], y=160, dx=25, length=42, thick=11, arc=8) +
                            eyes(p["sclera"], "#4e7a4a", p["ink"], cy=182, dx=25,
                                 rx=12, ry=9) +
                            nose(p["ink"], y=200, w=9, h=20, kind="bulb"))),

    # -------------------------------------------------------------- half-orc
    dict(id="half-orc", label="a half-orc",
         bg=("#e5e8cf", "#9fae7e"), cloak=("#3f4530", "#232719"),
         face="#7f9560", hair="#2f3320", light="#eaf0d8",
         head=dict(rx=66, ry=72, cy=172),
         over=lambda p: (hair_cap(p["hair"], top=92, drop=156, inner=118, wide=68) +
                         topknot(p["hair"]) +
                         round_ear(p["face"], p["ink"], cy=182, dx=66) +
                         tusks(p["light"], y=234, dx=17, h=30)),
         face_of=lambda p: (brows(p["hair"], y=146, dx=27, length=52, thick=13,
                                  tilt=-4, arc=1) +
                            eyes(p["sclera"], "#8a5a1c", p["ink"], cy=180, dx=28,
                                 rx=13, ry=8) +
                            nose(p["ink"], y=202, w=11, h=22, kind="broad") +
                            mouth(p["ink"], y=226, w=27, kind="grim", thick=4))),

    # ----------------------------------------------------------- dragonborn
    # A draconic head: no hair, a ridged snout, swept horns and scales.
    dict(id="dragonborn", label="a dragonborn",
         bg=("#f0dfc0", "#c08a4c"), cloak=("#4a3524", "#2a1d13"),
         face="#b8762f", hair="#5e3a1c", light="#f3e2c4",
         head=dict(rx=57, ry=72, cy=166),
         under=lambda p: horn(p["hair"], SWEPT_HORN, edge=p["light"]),
         over=lambda p: (scales(p["shade"]) +
                         snout(p["face"], p["ink"], p["shade"]) +
                         both(fill_path("M148 200 L 128 214 L 148 224 Z", p["face"]))),
         face_of=lambda p: (brows(p["shade"], y=152, dx=26, length=44, thick=9,
                                  tilt=-5, arc=2, opacity=0.6) +
                            eyes("#f2d79a", "#b8541c", p["ink"], cy=180, dx=27,
                                 rx=13, ry=8.5, pupil="slit"))),

    # -------------------------------------------------------------- tiefling
    # Horns, a tail's worth of red, and the solid, pupil-less eyes the entry
    # describes.
    dict(id="tiefling", label="a tiefling",
         bg=("#f0d9d3", "#b8746a"), cloak=("#43302c", "#261916"),
         face="#b0503f", hair="#2e1a17", light="#f4e0da",
         head=dict(rx=55, ry=74, cy=170),
         under=lambda p: (horn(p["hair"], CURLED_HORN, edge=p["light"]) +
                          long_hair(p["hair"], fall=302, wide=58)),
         over=lambda p: (pointed_ear(p["face"], p["ink"], reach=30, up=34, dx=51) +
                         hair_cap(p["hair"], top=92, drop=152, inner=114, wide=56)),
         face_of=lambda p: (brows(p["hair"], y=144, dx=26, length=44, thick=8,
                                  tilt=-6, arc=3) +
                            eyes("", "#e8a53c", p["ink"], cy=176, dx=26, rx=12.5,
                                 ry=8.5, pupil="glow") +
                            nose(p["ink"], y=200, w=7) +
                            mouth(p["ink"], y=232, w=21, kind="grim"))),

    # ---------------------------------------------------------------- tabaxi
    # A cat person, and it has to read as one at medallion size: ears on top
    # of the skull, a short furred muzzle, whiskers, slit pupils and the fur
    # markings the entry's wandering, catlike humanoids wear.
    dict(id="tabaxi", label="a tabaxi",
         bg=("#f3e6cc", "#c69a5e"), cloak=("#463a2a", "#271f16"),
         face="#c8a05c", hair="#7d5122", light="#f6ecd6",
         head=dict(rx=60, ry=64, cy=178),
         under=lambda p: cat_ear(p["hair"], "#d99a90", p["ink"], dx=46, base=140),
         over=lambda p: (
             # the cheek ruff, so the head is a cat's wedge rather than an oval
             both(fill_path("M142 176 C 128 196 128 226 146 244 "
                            "C 136 232 134 200 142 176 Z", p["hair"])) +
             stripes(p["hair"], [(156, 134, 24, -14), (188, 126, 24, -14),
                                 (220, 134, 24, -14), (142, 168, 18, -8),
                                 (240, 168, 18, -8)], width=6) +
             muzzle(p["light"], p["ink"], "#a8524a", cy=216, rx=40, ry=30, nose_w=12) +
             whiskers(p["shade"], y=216, dx=26) +
             spots(p["hair"], [(176, 206), (224, 206), (170, 220), (230, 220)],
                   r=3, opacity=".5")),
         face_of=lambda p: (brows(p["hair"], y=152, dx=27, length=42, thick=8,
                                  tilt=-5, arc=4) +
                            eyes("#f6e2a8", "#4f8f4a", p["ink"], cy=180, dx=28,
                                 rx=14, ry=10, pupil="slit"))),

    # --------------------------------------------------------------- aasimar
    # "Look like well-made humans until the light in them wakes": a halo, lit
    # eyes and the markings of the light under the skin.
    dict(id="aasimar", label="an aasimar",
         bg=("#f7eeda", "#d8bd84"), cloak=("#3f3c4c", "#22212c"),
         face="#dcbb92", hair="#d3c193", light="#fff6dd",
         head=dict(rx=56, ry=73, cy=172),
         under=lambda p: (long_hair(p["hair"], fall=300, wide=58) +
                          halo("#e8c96a", cy=76, r=56)),
         over=lambda p: (round_ear(p["face"], p["ink"], cy=184, dx=57) +
                         hair_cap(p["hair"], top=92, drop=156, inner=116, wide=57) +
                         rune_marks("#e8c96a",
                                    ["M156 208 q 8 10 0 20", "M244 208 q -8 10 0 20",
                                     "M200 142 l 0 -14"])),
         face_of=lambda p: (brows(shade(p["hair"], 0.78), y=146, dx=26, length=42,
                                  thick=7) +
                            eyes("", "#f6dd94", p["ink"], cy=178, dx=26,
                                 rx=13, ry=9, pupil="glow") +
                            nose(p["ink"], y=200, w=8) +
                            mouth(p["ink"], y=232, w=22))),

    # --------------------------------------------------------------- firbolg
    # Very large, long nosed, with the low bovine ears and the mossy colouring
    # of a people who live in the forest and would rather not fight you.
    dict(id="firbolg", label="a firbolg",
         bg=("#e4ead6", "#9cae85"), cloak=("#3b4634", "#212a1d"),
         face="#b09a6e", hair="#8a6a3c", light="#eef2e2",
         head=dict(rx=60, ry=78, cy=170),
         under=lambda p: long_ear(p["face"], p["ink"], dx=56, cy=168, reach=36, drop=44),
         over=lambda p: (hair_cap(p["hair"], top=88, drop=150, inner=112, wide=62) +
                         beard(p["hair"], jaw=214, chin=326, half=58) +
                         spots("#6f7f4e", [(166, 152), (234, 154), (200, 138)],
                               r=4, opacity=".3")),
         face_of=lambda p: (brows(p["hair"], y=146, dx=26, length=46, thick=11, arc=4) +
                            eyes(p["sclera"], "#4a6b3a", p["ink"], cy=176, dx=27,
                                 rx=12, ry=8.5) +
                            nose(p["ink"], y=202, w=10, h=26, kind="bulb"))),

    # ---------------------------------------------------------------- triton
    # Sea-coloured, finned rather than eared, gilled at the neck, and carrying
    # the confidence of a people who have never lost a war.
    dict(id="triton", label="a triton",
         bg=("#dceaea", "#7fa8ab"), cloak=("#274347", "#152528"),
         face="#6fa5a2", hair="#2f6d73", light="#e6f4f2",
         head=dict(rx=55, ry=73, cy=170),
         under=lambda p: (fin_ear("#3f8489", p["ink"], dx=52, cy=176) +
                          fill_path("M200 88 C 214 106 218 130 214 150 "
                                    "L 186 150 C 182 130 186 106 200 88 Z", "#3f8489")),
         over=lambda p: (gills(p["ink"], y=268, dx=36) +
                         line_path("M170 132 q 30 -14 60 0", "#2f6d73", 4, ' opacity=".5"')),
         face_of=lambda p: (brows(shade(p["face"], 0.6), y=148, dx=26, length=42,
                                  thick=7, tilt=-4) +
                            eyes("#e6f4f2", "#1f4f7a", p["ink"], cy=178, dx=26,
                                 rx=13.5, ry=9) +
                            nose(p["ink"], y=200, w=7, kind="line") +
                            mouth(p["ink"], y=232, w=22, kind="line"))),

    # --------------------------------------------------------------- goliath
    # Grey as mountain stone, bald, and marked with the dark lithoderms the
    # entry gives them.
    dict(id="goliath", label="a goliath",
         bg=("#e8e6e0", "#9a9790"), cloak=("#3d3d38", "#212120"),
         face="#9d9d95", hair="#4b4b46", light="#efeee9",
         head=dict(rx=64, ry=74, cy=170),
         over=lambda p: (round_ear(p["face"], p["ink"], cy=182, dx=64) +
                         spots("#54544d", [(168, 122), (200, 112), (232, 122),
                                           (150, 152), (250, 152), (200, 258)],
                               r=8, opacity=".45") +
                         rune_marks("#54544d", ["M162 196 q 10 12 0 24",
                                                "M238 196 q -10 12 0 24"])),
         face_of=lambda p: (brows("#54544d", y=148, dx=27, length=52, thick=13,
                                  tilt=-3, arc=2) +
                            eyes("#eceae4", "#5a6b7a", p["ink"], cy=180, dx=28,
                                 rx=13, ry=8) +
                            nose(p["ink"], y=204, w=11, h=22, kind="broad") +
                            mouth(p["ink"], y=238, w=26, kind="grim", thick=4))),

    # ------------------------------------------------------------- warforged
    # Built rather than born: plates, seams, a faceplate and a lit slit where
    # the eyes would be.
    dict(id="warforged", label="a warforged",
         bg=("#e7e2d4", "#9a9077"), cloak=("#413b31", "#23201a"),
         face="#8e8879", hair="#5d5749", light="#efe9da",
         head=dict(rx=57, ry=72, cy=170),
         under=lambda p: fill_path("M200 92 L 214 112 L 210 152 L 190 152 L 186 112 Z",
                                   "#6d6757"),
         over=lambda p: (
             plating("#4a453a",
                     ["M143 148 C 160 138 240 138 257 148",
                      "M148 214 C 170 224 230 224 252 214",
                      "M200 236 L 200 262",
                      "M160 250 C 180 262 220 262 240 250"]) +
             both(fill_path("M142 168 L 128 178 L 142 200 Z", "#6d6757")) +
             fill_path("M176 240 L 224 240 L 220 258 L 180 258 Z", "#6d6757")),
         face_of=lambda p: (fill_path("M150 168 L 250 168 L 246 190 L 154 190 Z",
                                      "#3a352c") +
                            eyes("", "#5fc2c8", p["ink"], cy=179, dx=25, rx=11,
                                 ry=6.5, pupil="glow") +
                            line_path("M172 216 L 228 216", "#4a453a", 3.4,
                                      ' opacity=".7"'))),

    # ------------------------------------------------------------- cairnborn
    # Barrow-pale, deep-set and very still, with the lichen of the old burial
    # hills on them. Original to orgs, so it is drawn to its own entry.
    dict(id="cairnborn", label="a cairnborn",
         bg=("#e3e2dd", "#8f9089"), cloak=("#38393a", "#1e1f20"),
         face="#a3a89c", hair="#4a4b45", light="#e9eae3",
         head=dict(rx=55, ry=68, cy=182),
         behind=lambda p: hood("#33342f", "#42443c", "#6d6f62"),
         over=lambda p: (hood_shadow(p["ink"], y=156) +
                         spots("#6d7a58", [(160, 214), (240, 218)],
                               r=6, opacity=".3")),
         face_of=lambda p: (brows(p["hair"], y=158, dx=26, length=46, thick=9,
                                  tilt=-2, arc=2) +
                            both(ellipse(174, 190, 18, 14, "#6a6f63", ' opacity=".4"')) +
                            eyes("#d9dbd2", "#7f8a6a", p["ink"], cy=190, dx=26,
                                 rx=12.0, ry=8) +
                            nose(p["ink"], y=212, w=8) +
                            mouth(p["ink"], y=242, w=22, kind="line"))),

    # ----------------------------------------------------------- adventurer
    # The one a race the ruleset has never heard of falls back to: a hooded
    # traveller, with a face in the hood rather than an empty oval.
    dict(id="adventurer", label="an adventurer",
         bg=("#ece3d2", "#a3968a"), cloak=("#443c33", "#26211b"),
         face="#9a7f5f", hair="#2f2820", light="#efe8db",
         head=dict(rx=54, ry=68, cy=182),
         behind=lambda p: hood(p["hair"], "#453a2d", "#7a6547"),
         over=lambda p: hood_shadow(p["ink"], y=158),
         face_of=lambda p: (brows(p["ink"], y=160, dx=25, length=40, thick=7) +
                            eyes(p["sclera"], "#5a4a33", p["ink"], cy=190, dx=25,
                                 rx=12.5, ry=9) +
                            nose(p["ink"], y=212, w=8) +
                            mouth(p["ink"], y=240, w=21))),
]


# ------------------------------------------------------------------ assembly

def portrait(race):
    p = dict(race)
    p["shade"] = shade(p["face"])
    p["ink"] = shade(p["face"], 0.34)
    p.setdefault("sclera", "#f6efdd")
    head = dict(HEAD, **race.get("head", {}))
    i = race["id"]

    body = []
    body.append('<rect width="400" height="400" fill="url(#bg-%s)"/>' % i)
    # The light the bust stands against, which keeps the silhouette readable
    # whatever colour the background is.
    body.append(circle(200, 176, 128, race["light"], ' opacity=".22"'))
    # Shoulders, then the neck, then everything above it.
    # Anything the shoulders should be in front of - a hood, most of all -
    # goes down before them.
    if "behind" in race:
        body.append(race["behind"](p))
    body.append('<path d="M36 400 C 44 318 104 268 200 264 C 296 268 356 318 364 400 Z" '
                'fill="url(#cloak-%s)"/>' % i)
    body.append(fill_path("M%g %g h%g v%g h-%g Z"
                          % (176, head["cy"] + head["ry"] - 12, 48, 60, 48),
                          p["shade"]))
    body.append(fill_path("M200 264 C 168 264 146 286 140 320 L 260 320 "
                          "C 254 286 232 264 200 264 Z", race["hair"], ' opacity=".55"'))
    body.append(circle(200, 300, 13, race["light"], ' opacity=".85"'))
    if "under" in race:
        body.append(race["under"](p))
    body.append(ellipse(head["cx"], head["cy"], head["rx"], head["ry"], p["face"]))
    # A little shading down one side, so the head is not a flat cut-out.
    body.append(ellipse(head["cx"] + head["rx"] * 0.42, head["cy"] + 6,
                        head["rx"] * 0.58, head["ry"] * 0.86, p["shade"],
                        ' opacity=".16"'))
    if "face_of" in race:
        body.append(race["face_of"](p))
    if "over" in race:
        body.append(race["over"](p))
    # A vignette, so the bust sits back into the medallion.
    body.append('<rect width="400" height="400" fill="url(#vig-%s)"/>' % i)

    return """<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 400" \
width="400" height="400" role="img" aria-label="Portrait of %s">
<title>%s</title>
<defs>
<radialGradient id="bg-%s" cx="50%%" cy="34%%" r="78%%">
<stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/>
</radialGradient>
<linearGradient id="cloak-%s" x1="0" y1="0" x2="0" y2="1">
<stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/>
</linearGradient>
<radialGradient id="vig-%s" cx="50%%" cy="46%%" r="72%%">
<stop offset=".55" stop-color="#000" stop-opacity="0"/>
<stop offset="1" stop-color="#1c1a17" stop-opacity=".42"/>
</radialGradient>
</defs>
%s
</svg>
""" % (race["label"], race["label"], i, race["bg"][0], race["bg"][1],
       i, race["cloak"][0], race["cloak"][1], i, "\n".join(body))


def main():
    out = os.path.join(os.getcwd(), OUT)
    if not os.path.isdir(os.path.dirname(out)):
        sys.exit("run this from the root of the orgs repo (no %s)" % OUT)
    os.makedirs(out, exist_ok=True)
    for race in RACES:
        path = os.path.join(out, race["id"] + ".svg")
        with open(path, "w") as fh:
            fh.write(portrait(race))
        print("wrote", path)


if __name__ == "__main__":
    main()
