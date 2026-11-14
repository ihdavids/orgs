import sys, os, re, math
sys.path.insert(0, os.path.dirname(__file__))
from svgkit import uri

X = "<svg xmlns='http://www.w3.org/2000/svg' "


def r1(v):
    s = '%.1f' % v
    s = s[:-2] if s.endswith('.0') else s
    return '0' if s == '-0' else s


def gear(cx, cy, ro, ri, teeth, hole=0, tw=.5):
    """Outline of a gear: teeth flat-topped, slightly tapered."""
    d = []
    step = 2 * math.pi / teeth
    for i in range(teeth):
        a = i * step
        a0, a1 = a - step * tw / 2, a + step * tw / 2
        t0, t1 = a - step * tw * .36, a + step * tw * .36
        for ang, r in ((a0, ri), (t0, ro), (t1, ro), (a1, ri)):
            d.append((cx + math.cos(ang) * r, cy + math.sin(ang) * r))
    p = 'M' + ' L'.join(r1(x) + ' ' + r1(y) for x, y in d) + 'Z'
    if hole:
        p += 'M%s %sa%s %s 0 1 0 %s 0a%s %s 0 1 0 %s 0Z' % (r1(cx - hole), r1(cy), r1(hole), r1(hole), r1(2 * hole), r1(hole), r1(hole), r1(-2 * hole))
    return p


def ring(cx, cy, r):
    return 'M%s %sa%s %s 0 1 0 %s 0a%s %s 0 1 0 %s 0Z' % (r1(cx - r), r1(cy), r1(r), r1(r), r1(2 * r), r1(r), r1(r), r1(-2 * r))


T = {}

# ---- blueprint doodles behind everything: faint line icons
ST = "fill='none' stroke='%23000' stroke-width='1.6' stroke-linecap='round' stroke-linejoin='round'"
g1 = gear(70, 70, 34, 27, 12, 0) + ring(70, 70, 11)
g2 = gear(300, 250, 24, 18, 9, 0) + ring(300, 250, 7)
# wrench, drawn upright then turned
wr = ("M-4.5 -22 L-4.5 26 Q-4.5 31 0 31 Q4.5 31 4.5 26 L4.5 -22 "
      "A12 12 0 1 0 -4.5 -22Z M-4 -40 L-4 -30 L4 -30 L4 -40")
fl = ("M-6 -30 L-6 -10 L-20 14 Q-23 22 -14 22 L14 22 Q23 22 20 14 L6 -10 L6 -30 Z M-9 -30 L9 -30 M-15 8 Q-6 4 0 8 T15 8")
bolt = "M-8 -14 L8 -14 L8 -6 L-8 -6Z M-5 -6 L-5 22 L5 22 L5 -6 M-5 0 L5 3 M-5 6 L5 9 M-5 12 L5 15 M-5 18 L5 21"
icons = ("<g stroke='%s' fill='none' stroke-width='1.6' stroke-linecap='round' stroke-linejoin='round'>"
         "<path d='%s'/><path d='%s'/>"
         "<path transform='translate(250 80) rotate(40)' d='%s'/>"
         "<path transform='translate(90 270) rotate(-14)' d='%s'/>"
         "<path transform='translate(190 170) rotate(28)' d='%s'/>"
         "<path d='M150 330 h40 M170 310 v40' stroke-dasharray='3 4'/><path d='M20 180 l26 0 l0 -26' stroke-dasharray='2 4'/>"
         "</g>")
T['DOODLES'] = uri(X + "width='360' height='360' viewBox='0 0 360 360' opacity='.11'>" +
                   icons % ('#4a2c14', g1, g2, wr, fl, bolt) + "</svg>")

# parchment mottling
T['MOTTLE'] = uri(X + "width='300' height='300'><filter id='m' x='0' y='0' width='100%' height='100%'>"
    "<feTurbulence type='fractalNoise' baseFrequency='.012' numOctaves='4' seed='4' stitchTiles='stitch'/>"
    "<feColorMatrix values='0 0 0 0 .55  0 0 0 0 .36  0 0 0 0 .14  0 0 0 -1.4 .75'/></filter>"
    "<rect width='300' height='300' filter='url(#m)'/></svg>")
T['FIBRE'] = uri(X + "width='200' height='200'><filter id='p' x='0' y='0' width='100%' height='100%'>"
    "<feTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='2' seed='8' stitchTiles='stitch'/>"
    "<feColorMatrix values='0 0 0 0 .4  0 0 0 0 .3  0 0 0 0 .2  0 0 0 -.9 .5'/></filter>"
    "<rect width='200' height='200' filter='url(#p)'/></svg>")

# signboard wood grain: wavy lines and a knot
grain = ''
for i, y in enumerate((7, 15, 24, 33, 41)):
    grain += "<path d='M0 %d C30 %d 60 %d 90 %d S150 %d 200 %d'/>" % (y, y - 3, y + 3, y, y - 3, y + 1)
T['SIGNGRAIN'] = uri(X + "width='200' height='48' viewBox='0 0 200 48' preserveAspectRatio='none'>"
    "<g fill='none' stroke='%2321140f' stroke-opacity='.28' stroke-width='1.1'>".replace('%23', '#') + grain +
    "<ellipse cx='140' cy='24' rx='9' ry='3.5'/><ellipse cx='140' cy='24' rx='4' ry='1.4'/></g></svg>")

# a chain hanging down: links alternate face-on and edge-on
T['CHAIN'] = uri(X + "width='8' height='14' viewBox='0 0 8 14'>"
    "<rect x='1.2' y='.8' width='5.6' height='8' rx='2.8' fill='none' stroke='#2b1d18' stroke-width='1.3'/>"
    "<rect x='3.1' y='7.4' width='1.8' height='6.4' rx='.9' fill='#6d6466' stroke='#2b1d18' stroke-width='.8'/></svg>")


def nut(size=28, col='#4d3f44', hi='#8c7c80'):
    # a hex joint with a bolt in the middle
    c = size / 2
    r = c - 1.5
    hx = ' '.join('%s,%s' % (r1(c + r * math.cos(math.radians(a))), r1(c + r * math.sin(math.radians(a)))) for a in range(0, 360, 60))
    r2 = r * .74
    hx2 = ' '.join('%s,%s' % (r1(c + r2 * math.cos(math.radians(a))), r1(c + r2 * math.sin(math.radians(a)))) for a in range(30, 390, 60))
    return (X + "width='%d' height='%d' viewBox='0 0 %d %d'>"
            "<polygon points='%s' fill='%s' stroke='#21151a' stroke-width='1.5' stroke-linejoin='round'/>"
            "<polygon points='%s' fill='%s' stroke='#21151a' stroke-width='1'/>"
            "<circle cx='%s' cy='%s' r='%s' fill='#2e2328' />"
            "<path d='M%s %s a%s %s 0 0 1 %s %s' fill='none' stroke='%s' stroke-width='1.3' stroke-linecap='round'/>"
            "<circle cx='%s' cy='%s' r='%s' fill='#b8a9ad'/></svg>") % (
        size, size, size, size, hx, col, hx2, '#5f5055', r1(c), r1(c), r1(r * .3),
        r1(c - r * .62), r1(c - r * .3), r1(r * .7), r1(r * .7), r1(r * .7), r1(-r * .25), hi,
        r1(c - r * .1), r1(c - r * .12), r1(r * .1))


T['NUT'] = uri(nut())

# big decorative gears
BG = gear(100, 100, 96, 82, 18, 0, .55) + ring(100, 100, 64) + ring(100, 100, 22)
spokes = ''.join("M%s %s L%s %s" % (r1(100 + 22 * math.cos(a)), r1(100 + 22 * math.sin(a)), r1(100 + 64 * math.cos(a)), r1(100 + 64 * math.sin(a)))
                 for a in [i * math.pi / 3 for i in range(6)])
T['GEAR_BIG'] = uri(X + "width='200' height='200' viewBox='0 0 200 200'>"
    "<path d='%s' fill='#b07a2c' fill-opacity='.22' fill-rule='evenodd' stroke='#5a3a1c' stroke-opacity='.55' stroke-width='2.5'/>"
    "<path d='%s' stroke='#5a3a1c' stroke-opacity='.55' stroke-width='9' stroke-linecap='round'/>"
    "<circle cx='100' cy='100' r='9' fill='#5a3a1c' fill-opacity='.5'/></svg>" % (BG, spokes))
SG = gear(50, 50, 47, 38, 11, 0, .55) + ring(50, 50, 14)
T['GEAR_SM'] = uri(X + "width='100' height='100' viewBox='0 0 100 100'>"
    "<path d='%s' fill='#8a3a33' fill-opacity='.2' fill-rule='evenodd' stroke='#5a2a24' stroke-opacity='.55' stroke-width='2.2'/></svg>" % SG)
# the portrait's toothed ring
PG = gear(105, 105, 104, 92, 28, 0, .5)
T['GEAR_RING'] = uri(X + "width='210' height='210' viewBox='0 0 210 210'>"
    "<path d='%s' fill='#9a6a2a' stroke='#2b1d18' stroke-width='2'/>"
    "<circle cx='105' cy='105' r='92' fill='none' stroke='#e2b866' stroke-opacity='.5' stroke-width='1.2'/></svg>" % PG)

def hexcell(fill_a, fill_b, inner):
    P = "26,1.5 74,1.5 98.5,50 74,98.5 26,98.5 1.5,50"
    Q = "29.5,8 70.5,8 91,50 70.5,92 29.5,92 9,50"
    return (X + "viewBox='0 0 100 100' preserveAspectRatio='none'>"
            "<defs><linearGradient id='c' x1='0' y1='0' x2='1' y2='1'><stop offset='0' stop-color='%s'/><stop offset='.5' stop-color='%s'/><stop offset='1' stop-color='#5a1c1e'/></linearGradient></defs>"
            "<polygon points='%s' fill='url(#c)' stroke='#2b1d18' stroke-width='2.2' vector-effect='non-scaling-stroke' stroke-linejoin='round'/>"
            "<polygon points='%s' fill='%s' stroke='#2b1d18' stroke-width='1.6' vector-effect='non-scaling-stroke' stroke-linejoin='round'/>"
            "<polygon points='31,11 69,11 73,19 27,19' fill='#fff' fill-opacity='.35'/>"
            "</svg>") % (fill_a, fill_b, P, Q, inner)


T['HEX'] = uri(hexcell('#e3a094', '#a9484a', '#f8f0dc'))
T['HEXON'] = uri(hexcell('#f6dc9a', '#c9963a', '#fff8e6'))

# ----- build
here = os.path.dirname(__file__)
src = open(os.path.join(here, 'tinker.src.css')).read()
decl = ''
for k, v in T.items():
    n = src.count('@@' + k + '@@')
    if n > 1:
        name = '--tw-i-' + k.lower().replace('_', '-')
        decl += '  %s: %s;\n' % (name, v)
        src = src.replace('@@' + k + '@@', 'var(%s)' % name)
    else:
        src = src.replace('@@' + k + '@@', v)
src = src.replace('/*@@IMAGES@@*/', decl)
left = re.findall(r'@@\w+@@', src)
if left:
    raise SystemExit('unfilled: %s' % left)
open(os.path.expanduser('~/dev/orgs/templates/dnd_skins/tinkers-workshop.css'), 'w').write(src)
svg = open(os.path.join(here, 'tinker.svg')).read()
open(os.path.expanduser('~/dev/orgs/templates/dnd_skins/tinkers-workshop.svg'), 'w').write(svg)
print('css', len(src), 'svg', len(svg), 'total', len(src) + len(svg))
