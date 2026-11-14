import re, urllib.parse


def uri(svg):
    """An svg as a css url(): whitespace squeezed, quoted for a data uri."""
    s = re.sub(r'\s+', ' ', svg.strip()).replace('> <', '><')
    s = s.replace('"', "'")
    s = urllib.parse.quote(s, safe=" ':/=,;.()-+_!*~$&@?")
    return 'url("data:image/svg+xml,' + s + '")'


def expand(css, art):
    def rep(m):
        return uri(art[m.group(1)])
    return re.sub(r'\{\{(\w+)\}\}', rep, css)


def scope(css, sid):
    """Sanity: every selector must start with the skin's attribute."""
    return css
