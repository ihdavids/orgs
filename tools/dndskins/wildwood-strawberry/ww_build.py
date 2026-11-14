import os, re
from ww_assets import A

OUT = os.path.expanduser('~/dev/orgs/templates/dnd_skins/')
css = open(os.path.join(os.path.dirname(__file__), 'wildwood.src.css')).read()
for k, v in A.items():
    css = css.replace('@@' + k + '@@', v)
left = re.findall(r'@@\w+@@', css)
assert not left, left
P = 'html[data-skin="wildwood"]'
css = css.replace('$P', P)
open(OUT + 'wildwood.css', 'w').write(css)
svg = open(os.path.join(os.path.dirname(__file__), 'wildwood.src.svg')).read()
open(OUT + 'wildwood.svg', 'w').write(svg)
print('css', len(css), 'svg', len(svg))
