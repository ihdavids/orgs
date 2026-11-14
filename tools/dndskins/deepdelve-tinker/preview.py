import sys; sys.path.insert(0,'.')
from clusters import C
from svgkit import uri
css = "body{background:#2a221d;display:flex;flex-wrap:wrap;gap:30px;padding:30px} div{height:220px;width:260px;border:1px dashed #555;background:no-repeat center/contain}"
h = ""
for i,(k,v) in enumerate(C.items()):
    css += ".c%d{background-image:%s}" % (i, uri(v)); h += "<div class='c%d'></div>" % i
open('prev.html','w').write("<html><head><style>"+css+"</style></head><body>"+h)
for k,v in C.items(): print(k, len(uri(v)))
