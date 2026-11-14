import math
from common import uri, leaf, f

INK = '#4a3218'
BARK1, BARK2 = '#ecdcb0', '#c4a167'
GREENS = ['#7fa05a', '#5f8a47', '#a5b96c', '#6f9e86', '#8aa860']
ROWAN = '#b8452a'
NS = "vector-effect='non-scaling-stroke'"

def svg(w, h, body, extra=''):
    return (f"<svg xmlns='http://www.w3.org/2000/svg' width='{w}' height='{h}' "
            f"viewBox='0 0 {w} {h}' {extra}>{body}</svg>")

# ---- the bark frame, a 9-slice for border-image (slice 32 of 224) ----
def frame():
    defs = f"""<defs>
<linearGradient id='a' x1='0' y1='0' x2='0' y2='1'><stop offset='0' stop-color='{BARK1}'/><stop offset='1' stop-color='{BARK2}'/></linearGradient>
<linearGradient id='b' x1='0' y1='0' x2='1' y2='0'><stop offset='0' stop-color='{BARK1}'/><stop offset='1' stop-color='{BARK2}'/></linearGradient>
<g id='c'>
<path d='M9.6 32 L9.4 6 Q16 1.4 22.6 5.6 L22.4 32Z' fill='url(%23b)'/>
<path d='M9.6 32 L9.4 6 Q16 1.4 22.6 5.6 L22.4 32' fill='none' stroke='{INK}' stroke-width='1.3'/>
<path d='M13.4 31 Q12.6 22 14 12 M18.8 31 Q19.6 20 18.4 13' fill='none' stroke='{INK}' stroke-width='.6' opacity='.5'/>
<ellipse cx='16' cy='5.3' rx='6.4' ry='2.3' fill='#f1e3bd' stroke='{INK}' stroke-width='1'/>
<ellipse cx='16' cy='5.3' rx='3' ry='1' fill='none' stroke='{INK}' stroke-width='.6'/>
<path d='M32 9.6 L6 9.4 Q1.4 16 5.6 22.6 L32 22.4Z' fill='url(%23a)'/>
<path d='M32 9.6 L6 9.4 Q1.4 16 5.6 22.6 L32 22.4' fill='none' stroke='{INK}' stroke-width='1.3'/>
<path d='M31 13.4 Q22 12.6 10 14 M31 18.8 Q20 19.6 11 18.4' fill='none' stroke='{INK}' stroke-width='.6' opacity='.5'/>
<ellipse cx='5.3' cy='16' rx='2.3' ry='6.4' fill='#f1e3bd' stroke='{INK}' stroke-width='1'/>
<ellipse cx='5.3' cy='16' rx='1' ry='3' fill='none' stroke='{INK}' stroke-width='.6'/>
<path d='M8.5 12.5 L19.5 23.5 M12.5 8.5 L23.5 19.5 M8.5 19.5 L19.5 8.5 M12.5 23.5 L23.5 12.5' stroke='{INK}' stroke-width='3' stroke-linecap='round'/>
<path d='M8.5 12.5 L19.5 23.5 M12.5 8.5 L23.5 19.5 M8.5 19.5 L19.5 8.5 M12.5 23.5 L23.5 12.5' stroke='#d2a456' stroke-width='1.6' stroke-linecap='round'/>
</g>
<g id='e'>
<path d='M32 9.6 C70 8.2 100 10.9 130 9.3 S180 9 192 9.6 L192 22.4 C160 23.4 120 21.5 90 22.8 S50 22.1 32 22.4Z' fill='url(%23a)'/>
<path d='M144.5 9.8 C147 5.5 151 3 157 1.8 C152 4.2 149.5 6.5 148.5 9.6Z' fill='url(%23a)' stroke='{INK}' stroke-width='.9'/>
{leaf(156.5, 2.2, 8, 10, 4.4, GREENS[1], INK, .7)}
<path d='M32 9.6 C70 8.2 100 10.9 130 9.3 S180 9 192 9.6 M192 22.4 C160 23.4 120 21.5 90 22.8 S50 22.1 32 22.4' fill='none' stroke='{INK}' stroke-width='1.3'/>
<path d='M34 13.4 C70 12.6 104 14.6 140 13.1 S176 13.6 190 13.3 M33 18.7 C78 19.7 112 17.5 152 18.9 S180 18.4 191 18.6' fill='none' stroke='{INK}' stroke-width='.6' opacity='.5'/>
<path d='M96 16.2 q9 -.8 20 0 M160 15.8 q6 .5 14 -.2 M40 16 q5 .4 11 0' fill='none' stroke='{INK}' stroke-width='.5' opacity='.45'/>
<ellipse cx='70' cy='15.9' rx='4.2' ry='2.1' fill='#b8935a' stroke='{INK}' stroke-width='.8'/>
<ellipse cx='70' cy='15.9' rx='1.6' ry='.7' fill='{INK}'/>
</g></defs>"""
    body = defs
    for r in (0, 90, 180, 270):
        t = f" transform='rotate({r} 112 112)'" if r else ''
        body += f"<use href='%23c'{t}/><use href='%23e'{t}/>"
    return svg(224, 224, body.replace('%23', '#'))

# ---- a sprig of leaves for a corner, and a tendril with rowan berries ----
def sprig():
    b = f"<path d='M6 46 C20 40 30 26 40 18 S54 8 60 4' fill='none' stroke='{INK}' stroke-width='1.6' stroke-linecap='round'/>"
    for (x, y, a, l, w, c) in [(14, 42, -115, 15, 7, 0), (21, 37, 15, 15, 7, 1), (30, 28, -105, 16, 7.5, 2),
                               (37, 21, 10, 16, 7, 3), (46, 13, -85, 14, 6.5, 4), (59, 5, -25, 12, 6, 1)]:
        b += leaf(x, y, a, l, w, GREENS[c], INK, .9)
    return svg(66, 50, b)

def tendril():
    b = (f"<path d='M68 8 C52 10 44 22 30 24 C16 26 8 18 12 10 C15 4 23 6 21 12 C19.6 15.6 15 14 15.8 11' "
         f"fill='none' stroke='{INK}' stroke-width='1.3' stroke-linecap='round'/>")
    b += leaf(54, 10, 125, 14, 6.5, GREENS[0], INK, .9)
    b += leaf(42, 20, -60, 13, 6, GREENS[3], INK, .9)
    for (x, y) in [(36, 30), (40.5, 32.5), (35, 35)]:
        b += f"<circle cx='{x}' cy='{y}' r='2.6' fill='{ROWAN}' stroke='{INK}' stroke-width='.7'/>"
    b += f"<path d='M37 26 L36 28 M38 26 L40 30' stroke='{INK}' stroke-width='.6'/>"
    return svg(70, 42, b)

# ---- the seed shield an ability score sits in ----
def seed():
    p = 'M50 13 C75 13 94 23 94 49 C94 78 73 102 50 120 C27 102 6 78 6 49 C6 23 25 13 50 13Z'
    b = f"""<defs><radialGradient id='g' cx='.5' cy='.35' r='.7'><stop offset='0' stop-color='%23fffcf0'/><stop offset='.75' stop-color='%23f3eed2'/><stop offset='1' stop-color='%23dfe3bb'/></radialGradient></defs>
<path d='{p}' fill='url(%23g)' stroke='{INK}' stroke-width='2.2'/>
<path d='{p}' fill='none' stroke='{INK}' stroke-width='.8' stroke-dasharray='2 2.6' opacity='.5' transform='translate(50 62) scale(.86) translate(-50 -62)'/>
<path d='M50 13 C50 8 52 5 55 2' fill='none' stroke='{INK}' stroke-width='1.6' stroke-linecap='round'/>
{leaf(51, 8, -155, 12, 5.5, GREENS[1], INK, .8)}{leaf(53.5, 4.5, -25, 12, 5.5, GREENS[0], INK, .8)}"""
    return svg(100, 124, b.replace('%23', '#'))

# ---- a sawn log end for the round stat tiles ----
def log():
    b = f"""<ellipse cx='50' cy='30' rx='48.5' ry='28.5' fill='#b48c55' stroke='{INK}' stroke-width='1.6' {NS}/>
<ellipse cx='50' cy='30' rx='45' ry='25.5' fill='#f8f0d8' stroke='{INK}' stroke-width='1' {NS}/>
<g fill='none' stroke='#dcc79c' stroke-width='1' {NS}>
<ellipse cx='50.6' cy='30.5' rx='38' ry='21' {NS}/><ellipse cx='49.4' cy='29.8' rx='30' ry='16.5' {NS}/>
<ellipse cx='50.4' cy='30.2' rx='21' ry='11.5' {NS}/><ellipse cx='50' cy='30' rx='11' ry='6' {NS}/>
<path d='M52 29 L80 20' {NS}/></g>"""
    return svg(100, 60, b, "preserveAspectRatio='none'")

def shield():
    b = f"""<path d='M50 3 C64 9 80 9 96 6 L96 44 C96 74 76 96 50 107 C24 96 4 74 4 44 L4 6 C20 9 36 9 50 3Z' fill='#f2f0d6' stroke='{INK}' stroke-width='1.8' {NS}/>
<path d='M50 9 C63 14 78 14 90 12 L90 44 C90 70 72 89 50 99 C28 89 10 70 10 44 L10 12 C22 14 37 14 50 9Z' fill='none' stroke='#6c8a44' stroke-width='1.2' {NS}/>"""
    return svg(100, 110, b, "preserveAspectRatio='none'")

# ---- a vine for underlining headings (tiles along x) ----
def vine():
    b = f"<path d='M0 7 C10 4 22 4 32 7 S54 10 64 7' fill='none' stroke='{INK}' stroke-width='1' opacity='.8'/>"
    b += leaf(30, 6.6, -38, 8, 3.8, GREENS[0], INK, .6, rib=False)
    b += leaf(52, 8.6, 32, 7, 3.4, GREENS[3], INK, .6, rib=False)
    return svg(64, 14, b)

def leaf_icon():
    return svg(18, 14, leaf(2, 12, -32, 17, 8, GREENS[1], INK, .9) )

# ---- the wreath round the portrait ----
def wreath():
    C, R = 120, 103
    b = ''
    stems = ''
    leaves = ''
    k = 0
    for side in (-1, 1):
        a0, a1 = (100, 263) if side < 0 else (80, -83)
        step = 9.5 if side < 0 else -9.5
        pts = []
        a = a0
        while (a <= a1) if side < 0 else (a >= a1):
            pts.append(a); a += step
        # the stem
        sa, ea = math.radians(a0), math.radians(pts[-1])
        x0, y0 = C + R * math.cos(sa), C + R * math.sin(sa)
        x1, y1 = C + R * math.cos(ea), C + R * math.sin(ea)
        sweep = 1 if side < 0 else 0
        stems += (f"<path d='M{f(x0)} {f(y0)} A{R} {R} 0 1 {sweep} {f(x1)} {f(y1)}' fill='none' "
                  f"stroke='{INK}' stroke-width='1.7'/>")
        for i, a in enumerate(pts[1:]):
            r = math.radians(a)
            x, y = C + R * math.cos(r), C + R * math.sin(r)
            g = a + 90 if side < 0 else a - 90
            out = g - 42 if side < 0 else g + 42
            inn = g + 22 if side < 0 else g - 22
            col = GREENS[k % 5]; k += 1
            leaves += leaf(x, y, out, 17, 7.5, col, INK, .9)
            leaves += leaf(x, y, inn, 14, 6.5, GREENS[(k + 2) % 5], INK, .9)
            if i % 5 == 3:
                bx, by = C + (R + 9) * math.cos(r + .05), C + (R + 9) * math.sin(r + .05)
                for dx, dy in ((0, 0), (3.6, 2), (.4, 4)):
                    leaves += f"<circle cx='{f(bx+dx)}' cy='{f(by+dy)}' r='2.5' fill='{ROWAN}' stroke='{INK}' stroke-width='.7'/>"
    # twine where the two stems cross at the foot
    tw = (f"<path d='M112 220 C104 230 98 226 106 221 M128 220 C136 230 142 226 134 221' fill='none' stroke='{INK}' stroke-width='3'/>"
          f"<path d='M112 220 C104 230 98 226 106 221 M128 220 C136 230 142 226 134 221' fill='none' stroke='#d2a456' stroke-width='1.6'/>"
          f"<ellipse cx='120' cy='222' rx='6' ry='4' fill='#d2a456' stroke='{INK}' stroke-width='1'/>")
    return svg(240, 240, stems + leaves + tw)

# ---- paper grain ----
def grain():
    b = """<filter id='n'><feTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='2' stitchTiles='stitch'/>
<feColorMatrix values='0 0 0 0 .29 0 0 0 0 .2 0 0 0 0 .09 0 0 0 -1.6 1.1'/></filter>
<rect width='100%' height='100%' filter='url(#n)' opacity='.33'/>"""
    return svg(220, 220, b)

# ---- pigment blooms: blotches of noise, for the wash ----
def blooms():
    b = """<filter id='n' x='0' y='0' width='100%' height='100%'><feTurbulence type='fractalNoise' baseFrequency='.006' numOctaves='4' seed='7' stitchTiles='stitch'/>
<feColorMatrix values='0 0 0 0 .36 0 0 0 0 .52 0 0 0 0 .3 0 0 0 -2.6 1.45'/></filter>
<filter id='m' x='0' y='0' width='100%' height='100%'><feTurbulence type='fractalNoise' baseFrequency='.008' numOctaves='3' seed='23' stitchTiles='stitch'/>
<feColorMatrix values='0 0 0 0 .72 0 0 0 0 .58 0 0 0 0 .25 0 0 0 -3 1.55'/></filter>
<filter id='t' x='0' y='0' width='100%' height='100%'><feTurbulence type='fractalNoise' baseFrequency='.005' numOctaves='3' seed='51' stitchTiles='stitch'/>
<feColorMatrix values='0 0 0 0 .38 0 0 0 0 .62 0 0 0 0 .6 0 0 0 -3 1.6'/></filter>
<rect width='100%' height='100%' filter='url(#n)' opacity='.55'/>
<rect width='100%' height='100%' filter='url(#t)' opacity='.6'/>
<rect width='100%' height='100%' filter='url(#m)' opacity='.5'/>"""
    return svg(700, 700, b)

A = {
    'FRAME': uri(frame()), 'SPRIG': uri(sprig()), 'TENDRIL': uri(tendril()), 'SEED': uri(seed()),
    'LOG': uri(log()), 'SHIELD': uri(shield()), 'VINE': uri(vine()), 'LEAF': uri(leaf_icon()),
    'WREATH': uri(wreath()), 'GRAIN': uri(grain()), 'BLOOMS': uri(blooms()),
}
