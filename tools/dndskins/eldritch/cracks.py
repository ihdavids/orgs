import random, math
random.seed(13)
W,H=1000,1400
paths=[];thin=[]
def walk(x,y,ang,n,step,depth,out):
    pts=[(x,y)]
    for i in range(n):
        ang+=random.uniform(-.6,.6)
        s=step*random.uniform(.6,1.3)
        x+=math.cos(ang)*s; y+=math.sin(ang)*s
        pts.append((x,y))
        if depth<2 and random.random()<.22:
            walk(x,y,ang+random.choice([-1,1])*random.uniform(.6,1.2),int(n*random.uniform(.25,.5)),step*.75,depth+1,thin if depth>=0 else out)
        if x<-50 or x>W+50 or y<-50 or y>H+50: break
    out.append(pts)
starts=[(0,180,0.2),(1000,420,math.pi-0.3),(0,820,-0.15),(1000,1100,math.pi+0.2),(380,0,1.4),(640,1400,-1.6),(0,1320,-0.6),(1000,60,math.pi+0.5)]
for x,y,a in starts:
    walk(x,y,a,random.randint(14,22),34,0,paths)
def d(pl): return ' '.join('M'+' L'.join(f'{x:.0f} {y:.0f}' for x,y in p) for p in pl)
print(len(d(paths)),len(d(thin)))
open('main.txt','w').write(d(paths)); open('thin.txt','w').write(d(thin))
