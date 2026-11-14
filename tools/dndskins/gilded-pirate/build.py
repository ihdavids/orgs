import os, re, sys, importlib
from common import expand
def build(sid, src, artmod, svg=None, pfx=None):
    pfx = pfx or sid[:2]
    art = importlib.import_module(artmod).ART
    css = open(src).read()
    css = re.sub(r'@S(?=[\s.{,:\[#>])', 'html[data-skin="%s"]' % sid, css)
    css = re.sub(r'\{\{(\w+)\}\}', lambda m: 'var(--%s-%s)' % (pfx, m.group(1).replace('_','-')), css)
    used = [k for k in art if ('--%s-%s)' % (pfx, k.replace('_','-'))) in css]
    from common import uri
    pre = 'html[data-skin="%s"] {\n' % sid + ''.join('  --%s-%s: %s;\n' % (pfx, k.replace('_','-'), uri(art[k])) for k in used) + '}\n'
    head_end = css.index('*/') + 2
    css = css[:head_end] + '\n\n/* the ornaments, drawn once and used by name below */\n' + pre + css[head_end:]
    out = os.path.join(os.path.dirname(os.path.abspath(__file__)), '../../../templates/dnd_skins/%s.css' % sid)
    open(out, 'w').write(css)
    if svg: open(out[:-4] + '.svg', 'w').write(open(svg).read())
    print(sid, len(css), 'bytes')
if __name__ == '__main__':
    which = sys.argv[1]
    if which == 'g': build('gilded-tome', 'gilded.css.in', 'art_gilded', 'gilded.svg')
    else: build('pirates-ledger', 'pirate.css.in', 'art_pirate', 'pirate.svg', 'pl')
