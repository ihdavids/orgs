"""Small helpers to draw svg bits and turn them into css data uris."""
import math, random, urllib.parse


def uri(svg):
    svg = ' '.join(svg.split())
    return "url(\"data:image/svg+xml," + urllib.parse.quote(svg, safe=" =:/'(),;.-_!*~") + "\")"


def f(v):
    s = '%d' % round(v)
    return '0' if s == '-0' else s


def pts(ps):
    return ' '.join(f(x) + ',' + f(y) for x, y in ps)


PAL = {
    'cyan':    ('#e4fdff', '#8fe6f2', '#3fbfd6', '#1d6f8c'),
    'violet':  ('#f6e6ff', '#cfa2f0', '#9a62d6', '#4f2b86'),
    'amber':   ('#fff3c4', '#ffc95c', '#e8901f', '#93470f'),
    'emerald': ('#e2fbd9', '#86e08a', '#36ad5c', '#165c34'),
    'ruby':    ('#ffe0e4', '#f58ba0', '#d5395f', '#7c1734'),
    'rose':    ('#fff0f5', '#f6b8d0', '#df7aa6', '#8a3460'),
}
INK = '#1b1418'


def crystal(x, y, ang, h, w, pal, skew=0.0):
    """A pointed prism standing on (x, y), leaning ang degrees."""
    hi, light, mid, dark = PAL[pal]
    a = math.radians(ang)
    ca, sa = math.cos(a), math.sin(a)

    def T(px, py):
        return (x + px * ca - py * sa, y + px * sa + py * ca)
    tip = (w * skew, -h)
    L0, L1 = (-w / 2, 0), (-w / 2, -h * .72)
    R0, R1 = (w / 2, 0), (w / 2, -h * .76)
    ridge0, ridge1 = (w * .1, 0), (w * .1 + w * skew * .2, -h * .74)
    left = [L0, L1, ridge1, ridge0]
    right = [ridge0, ridge1, R1, R0]
    capL = [L1, tip, ridge1]
    capR = [ridge1, tip, R1]
    out = [L0, L1, tip, R1, R0]
    s = ''
    s += "<polygon points='%s' fill='%s'/>" % (pts(map(lambda p: T(*p), left)), light)
    s += "<polygon points='%s' fill='%s'/>" % (pts(map(lambda p: T(*p), right)), mid)
    s += "<polygon points='%s' fill='%s'/>" % (pts(map(lambda p: T(*p), capL)), hi)
    s += "<polygon points='%s' fill='%s'/>" % (pts(map(lambda p: T(*p), capR)), light)
    # a streak of light down the bright face
    g0, g1 = T(-w * .28, -h * .08), T(-w * .28, -h * .6)
    s += "<path d='M%s,%sL%s,%s' stroke='#fff' stroke-opacity='.8' stroke-width='%s' stroke-linecap='round'/>" % (
        f(g0[0]), f(g0[1]), f(g1[0]), f(g1[1]), f(max(.8, w * .07)))
    s += "<polygon points='%s' fill='none' stroke='%s' stroke-width='1.3' stroke-linejoin='round'/>" % (
        pts(map(lambda p: T(*p), out)), INK)
    r0, r1 = T(*ridge0), T(*ridge1)
    s += "<path d='M%s,%sL%s,%sM%s,%sL%s,%sL%s,%s' fill='none' stroke='%s' stroke-width='.8' stroke-opacity='.75'/>" % (
        f(r0[0]), f(r0[1]), f(r1[0]), f(r1[1]),
        f(T(*L1)[0]), f(T(*L1)[1]), f(r1[0]), f(r1[1]), f(T(*R1)[0]), f(T(*R1)[1]), dark)
    return s


def cluster(w, h, specs, glow=None, rocks=True, seed=1):
    """specs: list of (x, y, ang, len, width, pal[, skew]) drawn in order."""
    rnd = random.Random(seed)
    s = "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 %d %d' width='%d' height='%d'>" % (w, h, w, h)
    if glow:
        cx, cy, rx, ry, col = glow
        s += ("<defs><radialGradient id='g'><stop offset='0' stop-color='%s' stop-opacity='.55'/>"
              "<stop offset='1' stop-color='%s' stop-opacity='0'/></radialGradient></defs>"
              "<ellipse cx='%s' cy='%s' rx='%s' ry='%s' fill='url(#g)'/>") % (col, col, f(cx), f(cy), f(rx), f(ry))
    for sp in specs:
        s += crystal(*sp)
    if rocks:
        base = [sp for sp in specs]
        xs = [sp[0] for sp in base]
        ys = [sp[1] for sp in base]
        x0, x1 = min(xs) - 8, max(xs) + 8
        yb = max(ys)
        # a couple of rough rocks over the roots
        n = max(2, int((x1 - x0) / 16))
        for i in range(n):
            cx = x0 + (x1 - x0) * (i + .5) / n + rnd.uniform(-3, 3)
            r = rnd.uniform(6, 10)
            poly = []
            for k in range(7):
                t = math.pi * (1 + k / 6)
                rr = r * rnd.uniform(.75, 1.1)
                poly.append((cx + math.cos(t) * rr * 1.3, yb + 4 + math.sin(t) * rr * .8))
            poly.append((cx + r * 1.3, yb + 6))
            poly.append((cx - r * 1.3, yb + 6))
            s += "<polygon points='%s' fill='%s' stroke='%s' stroke-width='1.1' stroke-linejoin='round'/>" % (
                pts(poly), rnd.choice(['#4a3d34', '#56473b', '#3e332c']), INK)
    s += "</svg>"
    return s
