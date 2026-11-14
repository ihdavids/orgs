"""Ink ornaments for the Pirate's Ledger skin."""
import math

NS = "xmlns='http://www.w3.org/2000/svg'"
INK = '#3a2412'
WASH = '#b48a58'


def f(x):
    return ('%.1f' % x).rstrip('0').rstrip('.')


def rope(d, w=6, light='#cfae74', dark='#5a3e1f', extra=''):
    """A rope along path d: a light core, twist marks, an inked edge."""
    return (f"<path d='{d}' fill='none' stroke='{INK}' stroke-width='{w + 2}' stroke-linecap='round' {extra}/>"
            f"<path d='{d}' fill='none' stroke='{light}' stroke-width='{w}' stroke-linecap='round' {extra}/>"
            f"<path d='{d}' fill='none' stroke='{dark}' stroke-width='{w}' stroke-dasharray='1.3 {w * .55:.1f}' {extra}/>")


def knot(flip=''):
    t = {'': '', 'h': "transform='translate(64 0) scale(-1 1)'",
         'v': "transform='translate(0 64) scale(1 -1)'",
         'hv': "transform='translate(64 64) scale(-1 -1)'"}[flip]
    body = (rope('M8 64 V30 C 8 14, 14 8, 30 8 H64', 6)
            + rope('M30 8 C 44 8, 46 26, 32 30 C 18 34, 10 22, 18 14 C 24 8, 36 12, 36 22', 5)
            + rope('M8 30 C 8 44, 26 46, 30 32 C 33 22, 24 14, 16 20', 5)
            + f"<circle cx='26' cy='26' r='3' fill='{INK}' opacity='.25'/>")
    return f"<svg {NS} viewBox='0 0 64 64' width='64' height='64'><g {t}>{body}</g></svg>"


def compass():
    c = 200
    out = [f"<svg {NS} viewBox='0 0 400 400' width='400' height='400'><g opacity='.13'><g fill='none' stroke='{INK}'>"]
    for r, w in ((188, 1.6), (182, .7), (150, 1), (146, .5), (60, .8)):
        out.append(f"<circle cx='{c}' cy='{c}' r='{r}' stroke-width='{w}'/>")
    # degree ticks
    for i in range(72):
        a = i * math.pi / 36
        r1 = 150 if i % 2 else 140
        out.append(f"<path d='M{f(c + r1 * math.cos(a))} {f(c + r1 * math.sin(a))} L{f(c + 182 * math.cos(a))} {f(c + 182 * math.sin(a))}' stroke-width='{.9 if i % 9 == 0 else .4}'/>")
    # rhumb lines
    for i in range(16):
        a = i * math.pi / 8
        out.append(f"<path d='M{c} {c} L{f(c + 200 * math.cos(a))} {f(c + 200 * math.sin(a))}' stroke-width='.35' stroke-dasharray='3 4'/>")
    out.append("</g>")
    # the star: 4 long points, 4 middle, 8 short
    def point(a, r, half, fill, shade):
        tip = (c + r * math.cos(a), c + r * math.sin(a))
        l = (c + half * math.cos(a - math.pi / 4), c + half * math.sin(a - math.pi / 4))
        rr = (c + half * math.cos(a + math.pi / 4), c + half * math.sin(a + math.pi / 4))
        return (f"<path d='M{c} {c} L{f(l[0])} {f(l[1])} L{f(tip[0])} {f(tip[1])} Z' fill='{fill}' stroke='{INK}' stroke-width='1' stroke-linejoin='round'/>"
                f"<path d='M{c} {c} L{f(rr[0])} {f(rr[1])} L{f(tip[0])} {f(tip[1])} Z' fill='{shade}' stroke='{INK}' stroke-width='1' stroke-linejoin='round'/>")
    for i in range(8):
        a = i * math.pi / 4 + math.pi / 8
        out.append(point(a, 92, 14, '#e8d5aa', '#8a6a44'))
    for i in range(4):
        a = i * math.pi / 2 + math.pi / 4
        out.append(point(a, 125, 22, '#ead9b0', '#6e5032'))
    for i in range(4):
        a = i * math.pi / 2 - math.pi / 2
        out.append(point(a, 178, 28, '#efe0bb', '#3a2412'))
    out.append(f"<circle cx='{c}' cy='{c}' r='9' fill='#e8d5aa' stroke='{INK}' stroke-width='1.2'/><circle cx='{c}' cy='{c}' r='3' fill='{INK}'/>")
    # fleur-ish north mark
    out.append("</g></svg>")
    return ''.join(out)


def tentacle():
    """A kraken arm reaching in from the right and curling."""
    pts = []
    # centre line: from off the right edge, sweeping left and down, then a spiral
    n = 90
    for i in range(n + 1):
        t = i / n
        if t < .55:
            u = t / .55
            x = 250 - 150 * u
            y = 30 + 70 * u * u + 20 * math.sin(u * 3)
            w = 30 - 14 * u
        else:
            u = (t - .55) / .45
            ang = -math.pi / 2 + u * 2.3 * math.pi
            rad = 34 * (1 - .75 * u)
            cx, cy = 100 - 0, 100 + 34 + 0
            # start the spiral where the sweep ended (at angle -pi/2 => top)
            x = cx + rad * math.cos(ang + math.pi)
            y = cy + rad * math.sin(ang + math.pi) * -1 - 0
            w = 16 * (1 - .85 * u)
        pts.append((x, y, w))
    # make it continuous: shift spiral so its first point meets the sweep's last
    k = int(.55 * n) + 1
    sx, sy = pts[k - 1][0] - pts[k][0], pts[k - 1][1] - pts[k][1]
    pts = pts[:k] + [(x + sx, y + sy, w) for x, y, w in pts[k:]]
    left, right, normals = [], [], []
    for i, (x, y, w) in enumerate(pts):
        a = pts[max(i - 1, 0)]
        b = pts[min(i + 1, len(pts) - 1)]
        dx, dy = b[0] - a[0], b[1] - a[1]
        l = math.hypot(dx, dy) or 1
        nx, ny = -dy / l, dx / l
        normals.append((nx, ny))
        left.append((x + nx * w / 2, y + ny * w / 2))
        right.append((x - nx * w / 2, y - ny * w / 2))
    poly = left + right[::-1]
    d = 'M' + ' L'.join(f'{f(x)} {f(y)}' for x, y in poly) + ' Z'
    out = [f"<svg {NS} viewBox='0 0 250 200' width='250' height='200'>"]
    out.append(f"<path d='{d}' fill='{WASH}' stroke='{INK}' stroke-width='1.6' stroke-linejoin='round'/>")
    # shading along the top side
    sd = 'M' + ' L'.join(f'{f(x + (rx - x) * .35)} {f(y + (ry - y) * .35)}' for (x, y), (rx, ry) in zip(left[:k], right[:k]))
    out.append(f"<path d='{sd}' fill='none' stroke='{INK}' stroke-width='.8' opacity='.45'/>")
    # hatching across the arm
    for i in range(2, len(pts) - 4, 3):
        (x1, y1), (x2, y2) = left[i], right[i]
        out.append(f"<path d='M{f(x1)} {f(y1)} L{f(x1 + (x2 - x1) * .3)} {f(y1 + (y2 - y1) * .3)}' stroke='{INK}' stroke-width='.6' opacity='.6'/>")
    # suckers along the underside
    for i in range(3, len(pts) - 6, 4):
        x, y, w = pts[i]
        nx, ny = normals[i]
        sx, sy = x - nx * w * .28, y - ny * w * .28
        r = max(1.4, w * .17)
        out.append(f"<ellipse cx='{f(sx)}' cy='{f(sy)}' rx='{f(r)}' ry='{f(r * .8)}' fill='#e7cfa0' stroke='{INK}' stroke-width='.8'/>"
                   f"<circle cx='{f(sx)}' cy='{f(sy)}' r='{f(r * .35)}' fill='{INK}' opacity='.7'/>")
    out.append("</svg>")
    return ''.join(out)


def anchor():
    return f"""<svg {NS} viewBox='0 0 80 96' width='80' height='96'><g fill='none' stroke='{INK}' stroke-linecap='round' stroke-linejoin='round'>
<circle cx='40' cy='10' r='6' stroke-width='3'/>
<path d='M24 22 H56' stroke-width='5'/><path d='M24 22 H56' stroke='#b48a58' stroke-width='2.4'/>
<circle cx='22' cy='22' r='2.5' fill='{INK}'/><circle cx='58' cy='22' r='2.5' fill='{INK}'/>
<path d='M40 16 V86' stroke-width='6'/><path d='M40 18 V84' stroke='#b48a58' stroke-width='3'/>
<path d='M10 60 C 12 78, 26 88, 40 88 C 54 88, 68 78, 70 60' stroke-width='5'/>
<path d='M10 60 C 12 78, 26 88, 40 88 C 54 88, 68 78, 70 60' stroke='#b48a58' stroke-width='2.2'/>
<path d='M4 66 L10 56 L17 64 Z M76 66 L70 56 L63 64 Z' fill='{INK}' stroke-width='1.5'/>
<path d='M46 14 C 60 22, 24 30, 40 38 C 56 46, 22 54, 38 62 C 52 69, 30 76, 34 82' stroke='#cfae74' stroke-width='2.4'/>
<path d='M46 14 C 60 22, 24 30, 40 38 C 56 46, 22 54, 38 62 C 52 69, 30 76, 34 82' stroke-width='.8' stroke-dasharray='1 2'/>
</g></svg>"""


def spyglass():
    return f"""<svg {NS} viewBox='0 0 200 44' width='200' height='44'><g stroke='{INK}' stroke-width='1.3' stroke-linejoin='round'>
<g transform='rotate(-8 100 22)'>
<path d='M8 16 L58 14 L58 30 L8 28 Z' fill='#8a6a44'/>
<path d='M58 12 L110 10 L110 34 L58 32 Z' fill='#c9a46a'/>
<path d='M110 8 L170 5 L170 39 L110 36 Z' fill='#8a6a44'/>
<path d='M170 3 L190 2 L190 42 L170 41 Z' fill='#d8b984'/>
<path d='M4 17 L8 16 L8 28 L4 27 Z' fill='{INK}'/>
<path d='M14 15.7 V28.3 M30 15 V29 M64 12 V32 M104 10.2 V33.8 M116 8 V36 M164 5.2 V38.8' fill='none' stroke-width='.8'/>
<path d='M120 14 L160 12 M62 17 L104 15 M12 19 L54 18' stroke='#f3e3bd' stroke-width='1.4' opacity='.8' fill='none'/>
<ellipse cx='190' cy='22' rx='3' ry='20' fill='#e8d5aa'/>
<path d='M64 22 C 64 36, 100 40, 112 30' fill='none' stroke='#cfae74' stroke-width='2.4'/>
</g></g></svg>"""


def cobweb():
    cx, cy = 0, 0
    out = [f"<svg {NS} viewBox='0 0 130 130' width='130' height='130'><g fill='none' stroke='#4a3a2a' stroke-linecap='round'>"]
    angs = [4, 18, 33, 49, 63, 78, 87]
    lens = [128, 112, 120, 104, 122, 110, 126]
    for a, l in zip(angs, lens):
        r = math.radians(a)
        out.append(f"<path d='M0 0 L{f(l * math.cos(r))} {f(l * math.sin(r))}' stroke-width='.7' opacity='.7'/>")
    for ring in (16, 30, 46, 63, 82, 100):
        segs = []
        for i in range(len(angs) - 1):
            r1, r2 = math.radians(angs[i]), math.radians(angs[i + 1])
            k = ring * (0.95 + .1 * ((i * 7 + ring) % 3) / 2)
            x1, y1 = k * math.cos(r1), k * math.sin(r1)
            x2, y2 = k * math.cos(r2), k * math.sin(r2)
            mx, my = (x1 + x2) / 2 * .86, (y1 + y2) / 2 * .86
            segs.append(f"M{f(x1)} {f(y1)} Q{f(mx)} {f(my)} {f(x2)} {f(y2)}")
        out.append(f"<path d='{' '.join(segs)}' stroke-width='.55' opacity='.6'/>")
    # a broken strand hanging down
    out.append("<path d='M70 40 C 72 60, 66 72, 70 92' stroke-width='.5' opacity='.5'/>")
    out.append("</g></svg>")
    return ''.join(out)


def ring_stain(seed=3):
    return f"""<svg {NS} viewBox='0 0 220 220' width='220' height='220'><filter id='r' x='-10%' y='-10%' width='120%' height='120%'>
<feTurbulence type='fractalNoise' baseFrequency='.03' numOctaves='3' seed='{seed}'/>
<feDisplacementMap in='SourceGraphic' scale='14'/><feGaussianBlur stdDeviation='.8'/></filter>
<g filter='url(#r)' fill='none'>
<circle cx='110' cy='110' r='86' stroke='#7a4a1c' stroke-opacity='.32' stroke-width='3.5'/>
<circle cx='110' cy='110' r='82' stroke='#8a5a26' stroke-opacity='.12' stroke-width='9'/>
<path d='M40 150 A 86 86 0 0 0 170 168' stroke='#6a3c14' stroke-opacity='.22' stroke-width='5'/>
</g></svg>"""


def tea():
    """Tea and coffee blotches, foxing and a tide line, for the whole page."""
    return f"""<svg {NS} width='700' height='700'>
<filter id='t'><feTurbulence type='fractalNoise' baseFrequency='.0045' numOctaves='4' seed='11' stitchTiles='stitch'/>
<feColorMatrix values='0 0 0 0 .42 0 0 0 0 .24 0 0 0 0 .07 1.9 0 0 0 -.95'/></filter>
<filter id='w'><feTurbulence type='fractalNoise' baseFrequency='.012' numOctaves='2' seed='4' stitchTiles='stitch'/>
<feColorMatrix values='0 0 0 0 .35 0 0 0 0 .2 0 0 0 0 .06 0 0 0 9 -4.6'/><feComponentTransfer><feFuncA type='table' tableValues='0 .5 0 0'/></feComponentTransfer></filter>
<rect width='100%' height='100%' filter='url(#t)'/>
<rect width='100%' height='100%' filter='url(#w)' opacity='.35'/>
<g fill='#7a4a1c'>
<circle cx='83' cy='120' r='2.2' opacity='.35'/><circle cx='90' cy='126' r='1.2' opacity='.3'/>
<circle cx='410' cy='60' r='3' opacity='.25'/><circle cx='600' cy='330' r='1.8' opacity='.35'/>
<circle cx='240' cy='470' r='2.6' opacity='.3'/><circle cx='246' cy='478' r='1.1' opacity='.3'/>
<circle cx='520' cy='610' r='3.4' opacity='.22'/><circle cx='130' cy='640' r='1.6' opacity='.35'/>
<circle cx='330' cy='250' r='1.3' opacity='.3'/><circle cx='660' cy='80' r='2' opacity='.3'/>
</g></svg>"""


def grain():
    return f"""<svg {NS} width='240' height='240'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='3' stitchTiles='stitch'/>
<feColorMatrix values='0 0 0 0 .3 0 0 0 0 .18 0 0 0 0 .06 0 0 0 -1.2 .66'/></filter>
<rect width='100%' height='100%' filter='url(#n)'/></svg>"""


def scroll_end(side='l'):
    """The rolled end of a scroll banner."""
    g = "" if side == 'l' else "transform='translate(26 0) scale(-1 1)'"
    return f"""<svg {NS} viewBox='0 0 26 38' width='26' height='38' preserveAspectRatio='none'><g {g} stroke='{INK}' stroke-width='1.3' stroke-linejoin='round'>
<path d='M26 3 H10 C 4 3, 2 8, 6 11 L 2 30 C 1 34, 4 36, 8 35 H26 Z' fill='#e9d6a8'/>
<path d='M10 3 C 16 3, 17 11, 11 12 C 7 12.5, 6 9, 9 8' fill='#cdb27c'/>
<path d='M8 35 C 3 35, 3 28, 8 27 C 12 26.5, 13 31, 10 32' fill='#bfa06a'/>
<path d='M11 12 L 8 27' fill='none' stroke-width='.7' stroke-dasharray='2 2'/>
</g></svg>"""


def wave_rule():
    """The ink line the header stands on: a rope with an anchor."""
    return anchor()


ART = {
    'knot_tl': knot(''), 'knot_tr': knot('h'), 'knot_bl': knot('v'), 'knot_br': knot('hv'),
    'compass': compass(), 'tentacle': tentacle(), 'anchor': anchor(), 'spyglass': spyglass(),
    'cobweb': cobweb(), 'ring1': ring_stain(3), 'tea': tea(), 'grain': grain(),
    'scroll_l': scroll_end('l'), 'scroll_r': scroll_end('r'),
}
