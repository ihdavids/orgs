import math
from common import uri, leaf, f

INK = '#6b4a35'      # soft brown line
STEM = '#7a6b3a'
SAGE = ['#9db88a', '#86a874', '#b0c79a', '#7f9f6c']
BERRY, BERRY_D = '#e04a5c', '#9c2f3c'
PETAL, PETAL_D = '#f8c9d1', '#c47a86'
NS = "vector-effect='non-scaling-stroke'"

def svg(w, h, body, extra=''):
    return (f"<svg xmlns='http://www.w3.org/2000/svg' width='{w}' height='{h}' "
            f"viewBox='0 0 {w} {h}' {extra}>{body}</svg>")

def blossom(x, y, r=4.2, fill=PETAL, ink=PETAL_D, rot=0):
    b = f"<g transform='translate({f(x)} {f(y)}) rotate({rot})'>"
    for i in range(5):
        a = math.radians(i * 72 - 90)
        b += f"<circle cx='{f(math.cos(a)*r*.95)}' cy='{f(math.sin(a)*r*.95)}' r='{f(r*.78)}' fill='{fill}' stroke='{ink}' stroke-width='.7'/>"
    b += f"<circle r='{f(r*.48)}' fill='#f4c552' stroke='#c08a2a' stroke-width='.6'/></g>"
    return b

def berry(x, y, s=1, rot=0):
    """a strawberry hanging from (x,y), tip pointing along +y after rotation"""
    b = f"<g transform='translate({f(x)} {f(y)}) rotate({rot}) scale({s})'>"
    b += (f"<path d='M0 1 C7 0 10 5 8.5 10 C7 15 3 18.5 0 20 C-3 18.5 -7 15 -8.5 10 C-10 5 -7 0 0 1Z' "
          f"fill='{BERRY}' stroke='{BERRY_D}' stroke-width='1'/>")
    b += "<path d='M-4 5 C-3 4 -1 4 0 4.5' fill='none' stroke='#fff' stroke-width='1.2' opacity='.5' stroke-linecap='round'/>"
    for (sx, sy) in [(-4, 8), (0, 7.5), (4, 8), (-5.5, 11.5), (-1.8, 11.5), (1.8, 12), (5.3, 11.4), (-3, 15), (1, 15.5), (3.4, 14.8)]:
        b += f"<ellipse cx='{sx}' cy='{sy}' rx='.6' ry='.95' fill='#ffe8a0'/>"
    for a in (-150, -110, -70, -30, -90):
        b += leaf(0, 1.2, a + 180 if False else a, 6.5 if a != -90 else 4, 3, SAGE[1], '#5d7a4a', .6, rib=False)
    b += f"<path d='M0 1 L0 -4' stroke='#5d7a4a' stroke-width='1.2' stroke-linecap='round'/></g>"
    return b

# ---- the vine frame: 9-slice, slice 40 of 256 ----
def frame():
    W, S = 256, 40
    # one edge: two stems twining between x=40 and x=216, centred on y=20
    def wave(sign):
        pts = []
        for i in range(0, 177, 4):
            x = 40 + i
            y = 20 + sign * 4.2 * math.sin(2 * math.pi * i / 88)
            pts.append(f"{f(x)} {f(y)}")
        return 'M' + ' L'.join(pts)
    e = (f"<path d='{wave(1)}' fill='none' stroke='{STEM}' stroke-width='2.2' stroke-linecap='round'/>"
         f"<path d='{wave(-1)}' fill='none' stroke='#94824a' stroke-width='1.6' stroke-linecap='round'/>")
    e += leaf(62, 15.8, -140, 11, 5.4, SAGE[0], '#5d7a4a', .7)
    e += leaf(62, 15.8, -40, 10, 5, SAGE[2], '#5d7a4a', .7)
    e += leaf(106, 24.2, 140, 11, 5.4, SAGE[1], '#5d7a4a', .7)
    e += leaf(150, 15.8, -50, 11, 5.4, SAGE[3], '#5d7a4a', .7)
    e += leaf(194, 24.2, 40, 10, 5, SAGE[0], '#5d7a4a', .7)
    e += blossom(128, 20, 3.4)
    e += f"<path d='M172 20 q2 -6 6 -9' fill='none' stroke='{STEM}' stroke-width='.9'/>"
    e += f"<path d='M178 11 c3 -2 5 1 2 3 c-2 1 -3 -1 -1 -2' fill='none' stroke='{STEM}' stroke-width='.8'/>"
    # one corner: stems meet under a blossom, a berry hangs off it
    c = (f"<path d='M40 20 C30 20 24 22 20 20 M20 40 C20 30 22 24 20 20' fill='none' stroke='{STEM}' stroke-width='2.2' stroke-linecap='round'/>"
         f"<path d='M20 20 C14 14 8 12 5 8 C3 5 6 3 8 5' fill='none' stroke='{STEM}' stroke-width='1.1'/>")
    c += leaf(20, 20, 200, 12, 6, SAGE[1], '#5d7a4a', .7)
    c += leaf(20, 20, 250, 12, 6, SAGE[0], '#5d7a4a', .7)
    c += berry(29, 25, .78, -35)
    c += blossom(20, 20, 5.6)
    body = f"<defs><g id='e'>{e}</g><g id='c'>{c}</g></defs>"
    for r in (0, 90, 180, 270):
        t = f" transform='rotate({r} 128 128)'" if r else ''
        body += f"<use href='#e'{t}/>"
    for r in (0, 90, 180, 270):
        t = f" transform='rotate({r} 128 128)'" if r else ''
        body += f"<use href='#c'{t}/>"
    return svg(W, W, body)

# ---- a strawberry badge for an ability score ----
def strawberry():
    body = 'M50 20 C80 15 97 33 95 58 C93 86 70 107 50 117 C30 107 7 86 5 58 C3 33 20 15 50 20Z'
    b = (f"<defs><radialGradient id='g' cx='.4' cy='.35' r='.75'><stop offset='0' stop-color='#f0707e'/>"
         f"<stop offset='.6' stop-color='{BERRY}'/><stop offset='1' stop-color='#c23446'/></radialGradient></defs>"
         f"<path d='{body}' fill='url(#g)' stroke='{BERRY_D}' stroke-width='2.2'/>")
    seeds = [(17, 40), (83, 40), (12, 58), (88, 58), (16, 78), (84, 78), (28, 96), (72, 96), (50, 110), (38, 106), (62, 106), (26, 30), (74, 30)]
    for (x, y) in seeds:
        b += f"<path d='M{x} {y-2.2} C{x+1.6} {y} {x+1} {y+2} {x} {y+2.2} C{x-1} {y+2} {x-1.6} {y} {x} {y-2.2}Z' fill='#ffe6a0' stroke='#b9862e' stroke-width='.5'/>"
    b += "<ellipse cx='50' cy='70' rx='31' ry='28' fill='#fff8ee' stroke='#a5303f' stroke-width='1.6'/>"
    b += "<ellipse cx='50' cy='70' rx='27.5' ry='24.5' fill='none' stroke='#f2b6bf' stroke-width='1' stroke-dasharray='2 2'/>"
    b += "<path d='M24 34 C28 27 36 24 44 24' fill='none' stroke='#fff' stroke-width='2.4' opacity='.45' stroke-linecap='round'/>"
    for a, l in [(-180, 22), (-145, 20), (-110, 16), (-70, 16), (-35, 20), (0, 22)]:
        b += leaf(50, 19, a, l, 9, SAGE[1], '#55703f', 1.1)
    b += f"<path d='M50 19 C50 12 53 7 58 3' fill='none' stroke='#55703f' stroke-width='2.6' stroke-linecap='round'/>"
    return svg(100, 120, b)

# ---- the armour class: a shield with a little sprig ----
def shield():
    b = (f"<path d='M50 4 C64 10 80 10 95 7 L95 45 C95 74 76 95 50 106 C24 95 5 74 5 45 L5 7 C20 10 36 10 50 4Z' "
         f"fill='#fff8ee' stroke='#d77a88' stroke-width='2' {NS}/>"
         f"<path d='M50 10 C63 15 77 15 89 13 L89 45 C89 70 72 88 50 98 C28 88 11 70 11 45 L11 13 C23 15 37 15 50 10Z' "
         f"fill='none' stroke='#9db88a' stroke-width='1.2' stroke-dasharray='3 2' {NS}/>")
    return svg(100, 110, b, "preserveAspectRatio='none'")

def underline():
    """a twine of stem with tiny blossoms, tiles along x"""
    b = f"<path d='M0 7 C12 3 20 3 32 7 S52 11 64 7' fill='none' stroke='{STEM}' stroke-width='1.1'/>"
    b += leaf(14, 4.6, -30, 8, 3.8, SAGE[0], '#5d7a4a', .5, rib=False)
    b += leaf(46, 9.3, 25, 8, 3.8, SAGE[1], '#5d7a4a', .5, rib=False)
    b += blossom(32, 7, 2.4)
    return svg(64, 14, b)

def icon():
    return svg(18, 20, berry(9, 2.5, .78, 0))

def petals():
    """falling petals, a tile"""
    b = ''
    import random
    rnd = random.Random(5)
    for i in range(11):
        x, y = rnd.uniform(10, 290), rnd.uniform(10, 290)
        r = rnd.uniform(0, 360); s = rnd.uniform(.7, 1.3)
        col = rnd.choice(['#f7c3cc', '#fbdde2', '#f2aab6', '#fff1f2'])
        b += (f"<path transform='translate({f(x)} {f(y)}) rotate({f(r)}) scale({f(s)})' "
              f"d='M0 0 C4 -5 9 -4 10 0 C9 4 4 5 0 0Z M10 0 L8.6 -1.2 L8.6 1.2Z' fill='{col}' stroke='#d998a4' stroke-width='.5' opacity='.85'/>")
    b += blossom(150, 220, 4, '#fff6f6', '#e3a5b0', 20)
    b += blossom(40, 120, 3.2, '#fbd3da', '#d98d9b', 50)
    return svg(300, 300, b)

def wreath():
    C, R = 120, 103
    stems, deco = '', ''
    k = 0
    for side in (-1, 1):
        a0, a1 = (98, 264) if side < 0 else (82, -84)
        step = 12 if side < 0 else -12
        pts = []
        a = a0
        while (a <= a1) if side < 0 else (a >= a1):
            pts.append(a); a += step
        sa, ea = math.radians(a0), math.radians(pts[-1])
        x0, y0 = C + R * math.cos(sa), C + R * math.sin(sa)
        x1, y1 = C + R * math.cos(ea), C + R * math.sin(ea)
        sweep = 1 if side < 0 else 0
        stems += (f"<path d='M{f(x0)} {f(y0)} A{R} {R} 0 1 {sweep} {f(x1)} {f(y1)}' fill='none' stroke='{STEM}' stroke-width='2'/>")
        for i, a in enumerate(pts[1:]):
            r = math.radians(a)
            x, y = C + R * math.cos(r), C + R * math.sin(r)
            g = a + 90 if side < 0 else a - 90
            out = g - 45 if side < 0 else g + 45
            deco += leaf(x, y, out, 15, 7, SAGE[k % 4], '#5d7a4a', .8); k += 1
            if i % 3 == 1:
                ox, oy = C + (R + 4) * math.cos(r), C + (R + 4) * math.sin(r)
                deco += berry(ox, oy, .8, a - 90 + (0 if side < 0 else 0))
            elif i % 3 == 2:
                deco += blossom(x, y, 4.6)
    deco += blossom(120, 222, 6.5) + blossom(120, 17, 5.5)
    return svg(240, 240, stems + deco)

A = {
    'FRAME': uri(frame()), 'BERRY': uri(strawberry()), 'SHIELD': uri(shield()), 'VINE': uri(underline()),
    'ICON': uri(icon()), 'PETALS': uri(petals()), 'WREATH': uri(wreath()),
    'BLOSSOM': uri(svg(20, 20, blossom(10, 10, 4.8))),
}
