"""Ornaments for the Gilded Tome skin, as svg strings."""

NS = "xmlns='http://www.w3.org/2000/svg'"

GOLD = """<linearGradient id='g' x1='0' y1='0' x2='1' y2='1'>
<stop offset='0' stop-color='#fff4c2'/><stop offset='.22' stop-color='#e9c25a'/>
<stop offset='.45' stop-color='#94641a'/><stop offset='.62' stop-color='#f6d983'/>
<stop offset='.8' stop-color='#b3821f'/><stop offset='1' stop-color='#6e4a10'/></linearGradient>
<linearGradient id='gv' x1='0' y1='0' x2='0' y2='1'>
<stop offset='0' stop-color='#fff1b8'/><stop offset='.45' stop-color='#d9ac45'/>
<stop offset='.55' stop-color='#9a6b18'/><stop offset='1' stop-color='#e7c264'/></linearGradient>
<radialGradient id='ruby' cx='.35' cy='.3' r='.8'><stop offset='0' stop-color='#ffb3b3'/>
<stop offset='.3' stop-color='#c42634'/><stop offset='1' stop-color='#4a060c'/></radialGradient>
<radialGradient id='sap' cx='.35' cy='.3' r='.8'><stop offset='0' stop-color='#cfe0ff'/>
<stop offset='.3' stop-color='#2f57b8'/><stop offset='1' stop-color='#0b1840'/></radialGradient>"""

DK = "#3d2606"

# one arm of the corner scrollwork, running along the top edge; the corner
# mirrors it across the diagonal for the arm down the side
ARM = f"""
<path d='M20 11 C 34 9, 42 19, 56 18 C 70 17, 74 7, 90 7 C 103 7, 109 18, 101 23 C 95 27, 88 21, 93 16'
 fill='none' stroke='{DK}' stroke-width='5.2' stroke-linecap='round'/>
<path d='M20 11 C 34 9, 42 19, 56 18 C 70 17, 74 7, 90 7 C 103 7, 109 18, 101 23 C 95 27, 88 21, 93 16'
 fill='none' stroke='url(#gv)' stroke-width='3.2' stroke-linecap='round'/>
<path d='M70 15 C 74 22, 82 22, 84 17' fill='none' stroke='{DK}' stroke-width='2.6' stroke-linecap='round'/>
<path d='M70 15 C 74 22, 82 22, 84 17' fill='none' stroke='url(#gv)' stroke-width='1.3' stroke-linecap='round'/>
<circle cx='62' cy='12' r='2.6' fill='url(#g)' stroke='{DK}' stroke-width='.8'/>
<circle cx='112' cy='9' r='2' fill='url(#g)' stroke='{DK}' stroke-width='.7'/>
"""

LEAF = f"""
<path d='M12 12 C 36 15, 50 32, 54 54 C 32 50, 15 36, 12 12 Z' fill='url(#g)' stroke='{DK}' stroke-width='1.1'/>
<path d='M15 15 C 28 24, 40 36, 50 50' fill='none' stroke='{DK}' stroke-width='.9' opacity='.8'/>
<path d='M24 18 C 26 26, 30 30, 38 31 M18 24 C 26 26, 30 30, 31 38' fill='none' stroke='{DK}' stroke-width='.7' opacity='.6'/>
<path d='M54 54 C 60 60, 58 68, 52 66 C 48 64, 50 59, 53 60' fill='none' stroke='{DK}' stroke-width='3' stroke-linecap='round'/>
<path d='M54 54 C 60 60, 58 68, 52 66 C 48 64, 50 59, 53 60' fill='none' stroke='url(#gv)' stroke-width='1.6' stroke-linecap='round'/>
"""

BOSS = f"""
<path d='M11 0 L22 11 L11 22 L0 11 Z' fill='url(#g)' stroke='{DK}' stroke-width='1.1'/>
<path d='M11 4 L18 11 L11 18 L4 11 Z' fill='none' stroke='#fff3c0' stroke-width='.6' opacity='.7'/>
<circle cx='11' cy='11' r='4' fill='url(#ruby)' stroke='{DK}' stroke-width='.8'/>
<circle cx='9.6' cy='9.6' r='1' fill='#fff' opacity='.8'/>
"""


def corner(flip=''):
    """The page's corner, top left; flip is h, v or hv."""
    t = {'': '', 'h': "transform='translate(120 0) scale(-1 1)'",
         'v': "transform='translate(0 120) scale(1 -1)'",
         'hv': "transform='translate(120 120) scale(-1 -1)'"}[flip]
    return f"""<svg {NS} viewBox='0 0 120 120' width='120' height='120'><defs>{GOLD}</defs>
<g {t}>{LEAF}<g>{ARM}</g><g transform='matrix(0 1 1 0 0 0)'>{ARM}</g>
<g transform='translate(1 1)'>{BOSS}</g></g></svg>"""


def boxcorner(flip=''):
    t = {'': '', 'h': "transform='translate(32 0) scale(-1 1)'",
         'v': "transform='translate(0 32) scale(1 -1)'",
         'hv': "transform='translate(32 32) scale(-1 -1)'"}[flip]
    arm = f"""<path d='M9 4 C 15 3, 18 7, 24 6 C 28 5.5, 29 3, 27.5 2' fill='none' stroke='{DK}' stroke-width='3' stroke-linecap='round'/>
<path d='M9 4 C 15 3, 18 7, 24 6 C 28 5.5, 29 3, 27.5 2' fill='none' stroke='url(#gv)' stroke-width='1.6' stroke-linecap='round'/>"""
    return f"""<svg {NS} viewBox='0 0 32 32' width='32' height='32'><defs>{GOLD}</defs>
<g {t}>{arm}<g transform='matrix(0 1 1 0 0 0)'>{arm}</g>
<path d='M3 3 C 10 4, 14 8, 15 15 C 8 14, 4 10, 3 3 Z' fill='url(#g)' stroke='{DK}' stroke-width='.8'/>
<circle cx='4.5' cy='4.5' r='3.4' fill='url(#g)' stroke='{DK}' stroke-width='.8'/>
<circle cx='4.5' cy='4.5' r='1.6' fill='url(#ruby)'/></g></svg>"""


def divider():
    """The centre of the rule under the header."""
    half = f"""<path d='M96 14 C 88 5, 76 4, 70 10 C 65 15, 70 21, 75 17' fill='none' stroke='{DK}' stroke-width='3.4' stroke-linecap='round'/>
<path d='M96 14 C 88 5, 76 4, 70 10 C 65 15, 70 21, 75 17' fill='none' stroke='url(#gv)' stroke-width='1.9' stroke-linecap='round'/>
<path d='M96 14 C 88 23, 74 24, 62 18 C 50 13, 36 13, 22 14' fill='none' stroke='{DK}' stroke-width='3' stroke-linecap='round'/>
<path d='M96 14 C 88 23, 74 24, 62 18 C 50 13, 36 13, 22 14' fill='none' stroke='url(#gv)' stroke-width='1.5' stroke-linecap='round'/>
<path d='M52 14 C 56 8, 62 8, 63 12' fill='none' stroke='{DK}' stroke-width='2.4' stroke-linecap='round'/>
<path d='M52 14 C 56 8, 62 8, 63 12' fill='none' stroke='url(#gv)' stroke-width='1.1' stroke-linecap='round'/>
<circle cx='18' cy='14' r='2.6' fill='url(#g)' stroke='{DK}' stroke-width='.8'/>"""
    return f"""<svg {NS} viewBox='0 0 240 28' width='240' height='28'><defs>{GOLD}</defs>
{half}<g transform='translate(240 0) scale(-1 1)'>{half}</g>
<path d='M120 1 L133 14 L120 27 L107 14 Z' fill='url(#g)' stroke='{DK}' stroke-width='1.1'/>
<circle cx='120' cy='14' r='6' fill='url(#ruby)' stroke='{DK}' stroke-width='1'/>
<circle cx='118' cy='12' r='1.6' fill='#fff' opacity='.85'/>
<circle cx='103' cy='14' r='2.4' fill='url(#sap)' stroke='{DK}' stroke-width='.6'/>
<circle cx='137' cy='14' r='2.4' fill='url(#sap)' stroke='{DK}' stroke-width='.6'/></svg>"""


def grain():
    return f"""<svg {NS} width='260' height='260'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='.85' numOctaves='3' stitchTiles='stitch'/>
<feColorMatrix values='0 0 0 0 .32 0 0 0 0 .2 0 0 0 0 .07 0 0 0 -1.1 .62'/></filter>
<rect width='100%' height='100%' filter='url(#n)'/></svg>"""


def mottle():
    return f"""<svg {NS} width='900' height='900'><filter id='m'><feTurbulence type='fractalNoise' baseFrequency='.006' numOctaves='4' seed='7' stitchTiles='stitch'/>
<feColorMatrix values='0 0 0 0 .45 0 0 0 0 .27 0 0 0 0 .08 1.3 0 0 0 -.62'/></filter>
<rect width='100%' height='100%' filter='url(#m)'/></svg>"""


def damask():
    """The tooled leather the page lies on."""
    return f"""<svg {NS} width='72' height='72' viewBox='0 0 72 72'><g fill='none' stroke='#c9963a' stroke-opacity='.13' stroke-width='1.2'>
<path d='M36 6 C 46 18, 46 26, 36 36 C 26 26, 26 18, 36 6 Z M36 66 C 26 54, 26 46, 36 36 C 46 46, 46 54, 36 66 Z'/>
<path d='M6 36 C 18 26, 26 26, 36 36 C 26 46, 18 46, 6 36 Z M66 36 C 54 46, 46 46, 36 36 C 46 26, 54 26, 66 36 Z'/>
<circle cx='0' cy='0' r='8'/><circle cx='72' cy='0' r='8'/><circle cx='0' cy='72' r='8'/><circle cx='72' cy='72' r='8'/></g></svg>"""


def bookcap(flip=''):
    """A brass corner protector for the leather book."""
    t = {'': '', 'h': "transform='translate(26 0) scale(-1 1)'",
         'v': "transform='translate(0 26) scale(1 -1)'",
         'hv': "transform='translate(26 26) scale(-1 -1)'"}[flip]
    return f"""<svg {NS} viewBox='0 0 26 26' width='26' height='26'><defs>{GOLD}</defs><g {t}>
<path d='M0 0 H26 C 18 4, 10 8, 8 12 C 6 16, 4 20, 0 26 Z' fill='url(#g)' stroke='{DK}' stroke-width='1'/>
<path d='M3 3 H18 C 12 6, 8 9, 6 12 C 5 14, 4 16, 3 18 Z' fill='none' stroke='#fff1b8' stroke-width='.6' opacity='.7'/>
<circle cx='6' cy='6' r='1.8' fill='#5a3a0c'/><circle cx='5.5' cy='5.5' r='.7' fill='#fff1b8'/></g></svg>"""


def medal_rim():
    """Beading round an ability medallion."""
    dots = ''.join(
        f"<circle cx='{50 + 46.5 * __import__('math').cos(i * 0.2618):.2f}' cy='{50 + 46.5 * __import__('math').sin(i * 0.2618):.2f}' r='2.3'/>"
        for i in range(24))
    return f"""<svg {NS} viewBox='0 0 100 100'><g fill='#f3d27a' stroke='#5a3a0c' stroke-width='.6'>{dots}</g></svg>"""


ART = {
    'corner_tl': corner(''), 'corner_tr': corner('h'), 'corner_bl': corner('v'), 'corner_br': corner('hv'),
    'bc_tl': boxcorner(''), 'bc_tr': boxcorner('h'), 'bc_bl': boxcorner('v'), 'bc_br': boxcorner('hv'),
    'cap_tl': bookcap(''), 'cap_tr': bookcap('h'), 'cap_bl': bookcap('v'), 'cap_br': bookcap('hv'),
    'divider': divider(), 'grain': grain(), 'mottle': mottle(), 'damask': damask(), 'rim': medal_rim(),
}
