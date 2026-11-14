cd "$(dirname "$0")" && python3 -c "
s=open('eldritch.tpl.css').read()
for k in ['fissures','star','star2','eye','sigil']:
    s=s.replace('@@'+k.upper()+'@@',open(k+'.uri').read().strip())
open('../../../templates/dnd_skins/eldritch-pact.css','w').write(s)
"
