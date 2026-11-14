import math, re

def uri(svg):
    """svg markup -> css url() with a data uri (quotes, #, <, > encoded)."""
    s = re.sub(r'\s+', ' ', svg.strip())
    s = s.replace('> <', '><')
    s = s.replace('"', "'")
    s = s.replace('%', '%25').replace('#', '%23').replace('<', '%3C').replace('>', '%3E')
    return 'url("data:image/svg+xml,' + s + '")'

def f(x):
    return ('%.1f' % x).rstrip('0').rstrip('.')

def leaf(x, y, ang, ln, wd, fill, ink, sw=1.0, rib=True, extra=''):
    """a leaf with its stalk at (x,y) pointing at ang degrees"""
    h = wd / 2
    p = (f"M0 0 C{f(ln*.3)} {f(-h*1.3)} {f(ln*.75)} {f(-h*1.1)} {f(ln)} 0 "
         f"C{f(ln*.75)} {f(h*1.1)} {f(ln*.3)} {f(h*1.3)} 0 0Z")
    r = f"<path d='M{f(ln*.08)} 0 Q{f(ln*.5)} {f(-h*.15)} {f(ln*.86)} 0' fill='none' stroke='{ink}' stroke-width='{f(sw*.6)}' opacity='.75'/>" if rib else ''
    return (f"<g transform='translate({f(x)} {f(y)}) rotate({f(ang)})' {extra}>"
            f"<path d='{p}' fill='{fill}' stroke='{ink}' stroke-width='{f(sw)}' stroke-linejoin='round'/>{r}</g>")
