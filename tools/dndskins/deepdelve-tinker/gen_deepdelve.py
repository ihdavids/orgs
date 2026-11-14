import sys, os, re
sys.path.insert(0, os.path.dirname(__file__))
from svgkit import uri, cluster
from clusters import C

X = "<svg xmlns='http://www.w3.org/2000/svg' "

T = {}
# rock: lit noise, tiled
T['ROCK'] = uri(X + "width='320' height='320'><filter id='r' x='0' y='0' width='100%' height='100%'>"
    "<feTurbulence type='fractalNoise' baseFrequency='.018' numOctaves='5' seed='11' stitchTiles='stitch'/>"
    "<feDiffuseLighting lighting-color='#8a7461' surfaceScale='4.5' diffuseConstant='1.1'><feDistantLight azimuth='235' elevation='38'/></feDiffuseLighting>"
    "</filter><rect width='320' height='320' filter='url(#r)'/></svg>")
# wood grain: streaks laid over a brown gradient (alpha only)
T['GRAIN'] = uri(X + "width='400' height='120'><filter id='w' x='0' y='0' width='100%' height='100%'>"
    "<feTurbulence type='fractalNoise' baseFrequency='.004 .16' numOctaves='3' seed='5' stitchTiles='stitch'/>"
    "<feColorMatrix values='0 0 0 0 .12  0 0 0 0 .07  0 0 0 0 .03  0 0 0 -1.6 1.1'/></filter>"
    "<rect width='400' height='120' filter='url(#w)'/></svg>")
T['GRAINV'] = uri(X + "width='120' height='400'><filter id='w' x='0' y='0' width='100%' height='100%'>"
    "<feTurbulence type='fractalNoise' baseFrequency='.16 .004' numOctaves='3' seed='9' stitchTiles='stitch'/>"
    "<feColorMatrix values='0 0 0 0 .12  0 0 0 0 .07  0 0 0 0 .03  0 0 0 -1.6 1.1'/></filter>"
    "<rect width='120' height='400' filter='url(#w)'/></svg>")
# paper fibre
T['PAPER'] = uri(X + "width='220' height='220'><filter id='p' x='0' y='0' width='100%' height='100%'>"
    "<feTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='2' seed='2' stitchTiles='stitch'/>"
    "<feColorMatrix values='0 0 0 0 .45  0 0 0 0 .36  0 0 0 0 .22  0 0 0 -.9 .55'/></filter>"
    "<rect width='220' height='220' filter='url(#p)'/></svg>")


def bracket(flip):
    # an L of blackened iron with three rivets, top left; flip mirrors it
    tr = {'tl': '', 'tr': "transform='translate(40 0) scale(-1 1)'",
          'bl': "transform='translate(0 40) scale(1 -1)'", 'br': "transform='translate(40 40) scale(-1 -1)'"}[flip]
    return uri(X + "width='40' height='40' viewBox='0 0 40 40'><g %s>"
        "<path d='M2 3 Q2 2 3 2 L37 2 Q38 4 37 6 L36 11 L12 11 Q11 11 11 12 L11 36 Q9 38 6 37 L3 37 Q2 37 2 36 Z' "
        "fill='#2d2622' stroke='#0e0a08' stroke-width='1.4' stroke-linejoin='round'/>"
        "<path d='M4 4 L35 4 M4 4 L4 34' stroke='#6e5f52' stroke-width='1.2' stroke-linecap='round' opacity='.8'/>"
        "<g fill='#57493f' stroke='#0e0a08' stroke-width='.9'><circle cx='6.5' cy='6.5' r='2.4'/>"
        "<circle cx='28' cy='6.5' r='2.1'/><circle cx='6.5' cy='28' r='2.1'/></g>"
        "<g fill='#c9b49a' opacity='.7'><circle cx='5.8' cy='5.8' r='.8'/><circle cx='27.4' cy='5.8' r='.7'/><circle cx='5.8' cy='27.4' r='.7'/></g>"
        "</g></svg>" % tr)

for k in ('tl', 'tr', 'bl', 'br'):
    T['BR_' + k.upper()] = bracket(k)

# a pencil underline for box titles
T['RULE'] = uri(X + "width='240' height='8' viewBox='0 0 240 8'><path d='M1 5 C40 2 80 6 120 4 S200 2 239 5' "
    "fill='none' stroke='%237d2a1a' stroke-width='1.6' stroke-linecap='round' opacity='.75'/></svg>".replace('%23', '#'))
# a small cut gem, for the box titles
T['GEM'] = uri(X + "width='16' height='16' viewBox='0 0 16 16'><path d='M4 2 H12 L15 6 L8 15 L1 6 Z' fill='#5fd3e6' stroke='#1b1418' stroke-width='1.1' stroke-linejoin='round'/>"
    "<path d='M1 6 H15 M4 2 L6 6 L8 15 L10 6 L12 2' fill='none' stroke='#1d6f8c' stroke-width='.8'/><path d='M5 4 L6 6 L4 6 Z' fill='#e4fdff'/></svg>")
# a rope coil line between the two halves of the header? -- a timber cross beam end
for k, v in C.items():
    T['CR_' + k.upper()] = uri(v)


def flipped(svg):
    # hang a cluster from the ceiling
    m = re.match(r"(<svg[^>]*viewBox='0 0 (\d+) (\d+)'[^>]*>)(.*)</svg>", svg, re.S)
    head, w, h, body = m.group(1), m.group(2), m.group(3), m.group(4)
    return head + "<g transform='translate(0 %s) scale(1 -1)'>" % h + body + "</g></svg>"


T['CR_HANG'] = uri(flipped(C['violet']))
T['CR_HANGC'] = uri(flipped(C['cyan']))

src = open(os.path.join(os.path.dirname(__file__), 'deepdelve.src.css')).read()
decl = ''
for k, v in T.items():
    if src.count('@@' + k + '@@') > 1:
        name = '--dd-i-' + k.lower().replace('_', '-')
        decl += '  %s: %s;\n' % (name, v)
        src = src.replace('@@' + k + '@@', 'var(%s)' % name)
    else:
        src = src.replace('@@' + k + '@@', v)
src = src.replace('/*@@IMAGES@@*/', decl)
left = re.findall(r'@@\w+@@', src)
if left:
    raise SystemExit('unfilled: %s' % left)
out = os.path.expanduser('~/dev/orgs/templates/dnd_skins/deepdelve.css')
open(out, 'w').write(src)
svg = open(os.path.join(os.path.dirname(__file__), 'deepdelve.svg')).read()
open(os.path.expanduser('~/dev/orgs/templates/dnd_skins/deepdelve.svg'), 'w').write(svg)
print('css', len(src), 'svg', len(svg), 'total', len(src) + len(svg))
