import os, re, sys, importlib
skin, mod, src = sys.argv[1], sys.argv[2], sys.argv[3]
A = importlib.import_module(mod).A
OUT = os.path.expanduser('~/dev/orgs/templates/dnd_skins/')
css = open(src).read()
for k, v in A.items():
    css = css.replace('@@' + k + '@@', v)
left = re.findall(r'@@\w+@@', css); assert not left, left
css = css.replace('$P', 'html[data-skin="%s"]' % skin)
open(OUT + skin + '.css', 'w').write(css)
print('css', len(css), {k: len(v) for k, v in A.items()})
