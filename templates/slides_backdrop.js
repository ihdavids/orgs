/* The animated backdrop: a scene drawn behind the deck.
 *
 * A theme is one file of colours and a type scale, which is what makes one
 * theme work in all four frameworks. This is the one thing a palette cannot
 * do: a field of lit cubes turning slowly through space, a dotted world with
 * light running between its cities, crystal, wireframes, a low-poly field.
 *
 * The same split as the theme mapping, for the same reason. **A theme asks for
 * a scene by name and tunes it with custom properties** (`@backdrop: cubes` in
 * its comment header, then `--backdrop-colors` and friends); the scenes live
 * here, once, and work in reveal.js, impress.js, WebSlides and deck.js without
 * knowing which one they are in - because all any of them needs is a canvas
 * behind the slide, and `slides_backdrop.css` is where the four frameworks are
 * told to get out of its way.
 *
 * Six rules, each of which is the difference between a backdrop and a problem:
 *
 *  1. **The slide is what is being read.** Every scene is drawn to be looked
 *     *past*: nothing crosses the middle of the screen at full strength, the
 *     distance fades into the deck's own ground (`--backdrop-fog`), and a
 *     theme can wash the lot down with `--backdrop-veil`. A backdrop that
 *     competes with the text is a backdrop that has to be turned off.
 *  2. **It stops when nobody is looking.** A background tab gets no animation
 *     frame at all, so the loop is also hung off `visibilitychange` rather
 *     than left to be restarted by a frame that will never arrive - and the
 *     clock is not advanced across the gap, or a deck left for an hour would
 *     lurch when it came back.
 *  3. **`prefers-reduced-motion` means one frame and stop.** Not a blank
 *     screen: the scene is composed to be worth looking at standing still, so
 *     the reader who cannot take the motion still gets the deck's design.
 *  4. **The field is the same every time** (a seeded generator rather than
 *     Math.random). A deck is rehearsed, and a backdrop that reshuffled itself
 *     on every export or every reload could not be.
 *  5. **It is told what it may cost.** The frame rate is capped well below the
 *     display's, the pixel count is capped whatever the screen's density, and
 *     the whole thing bows out rather than drawing badly if there is no canvas.
 *     It is sharing a machine with a projector and someone's live demo.
 *  6. **Additive light only on a dark ground.** Over a pale theme `lighter`
 *     washes out to nothing, so the glow passes are measured against the
 *     deck's own background and skipped when there is no dark to add light to.
 *     (The same thing the character sheet's flourishes have to do over
 *     parchment.)
 */
(function () {
  'use strict';

  // The page's own object, not a copy of it: `state` is written back onto it
  // so that `ORG_BACKDROP.state` in a console answers what this backdrop is
  // doing and why.
  var CFG = window.ORG_BACKDROP = (window.ORG_BACKDROP || {});
  var SCENES = {};

  // ── What the theme asked for ───────────────────────────────────────────────
  //
  // Everything tunable is a custom property, so a theme tunes its own scene in
  // the one file it already has and nothing here needs to know its name. A
  // document keyword (`#+BACKDROP_SPEED:`) arrives on CFG and wins, because
  // that is somebody deciding about this deck rather than about the theme.

  function cssVar(name, dflt) {
    var v = '';
    try { v = getComputedStyle(document.documentElement).getPropertyValue(name); } catch (e) { }
    v = (v || '').trim();
    return v === '' ? dflt : v;
  }
  function cssNum(name, dflt) {
    var n = parseFloat(cssVar(name, ''));
    return isNaN(n) ? dflt : n;
  }
  // Colours are whitespace separated, never comma separated: `rgb(1, 2, 3)`
  // has commas of its own and a list that could not hold one would be a list
  // with a trap in it.
  function cssList(name, dflt) {
    var v = cssVar(name, '');
    if (!v) { return dflt; }
    var parts = v.split(/\s+/);
    var out = [];
    for (var i = 0; i < parts.length; i++) { if (parts[i]) { out.push(parts[i]); } }
    return out.length ? out : dflt;
  }

  // ── Colour ────────────────────────────────────────────────────────────────

  function rgb(c) {
    c = (c || '').trim();
    if (c.charAt(0) === '#') {
      if (c.length === 4) {
        return [parseInt(c.charAt(1) + c.charAt(1), 16),
                parseInt(c.charAt(2) + c.charAt(2), 16),
                parseInt(c.charAt(3) + c.charAt(3), 16)];
      }
      return [parseInt(c.substr(1, 2), 16), parseInt(c.substr(3, 2), 16), parseInt(c.substr(5, 2), 16)];
    }
    var m = c.match(/[-+]?[0-9]*\.?[0-9]+/g);
    if (m && m.length >= 3) { return [+m[0], +m[1], +m[2]]; }
    return [120, 120, 130];
  }
  function mix(a, b, t) {
    return [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t];
  }
  function scale(c, k) { return [c[0] * k, c[1] * k, c[2] * k]; }
  function paint(c, a) {
    return 'rgba(' + (c[0] | 0) + ',' + (c[1] | 0) + ',' + (c[2] | 0) + ',' + (a === undefined ? 1 : (a < 0 ? 0 : a > 1 ? 1 : a).toFixed(3)) + ')';
  }
  // Luminance rather than brightness, for the reason ReadInk gives on the
  // server: the eye is far more sensitive to green than to blue, so an average
  // calls #0000ff light and #00ff00 dark, which is backwards for exactly the
  // saturated colours people reach for.
  function lum(c) { return (0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]) / 255; }

  // ── A field that is the same every time ───────────────────────────────────

  var seed = 0x9e3779b9;
  function rnd() {
    seed = seed + 0x6D2B79F5 | 0;
    var t = Math.imul(seed ^ seed >>> 15, 1 | seed);
    t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
    return ((t ^ t >>> 14) >>> 0) / 4294967296;
  }
  function reseed(s) { seed = s | 0; }
  function between(a, b) { return a + rnd() * (b - a); }
  function pick(list) { return list[(rnd() * list.length) | 0]; }

  // ── The camera ────────────────────────────────────────────────────────────
  //
  // One perspective divide and nothing else: the camera sits at the origin
  // looking along +z, so a point's depth *is* its z and the painter's sort is
  // a sort on one number. Everything is measured in units of the canvas
  // height, which is what keeps a scene looking the same on a laptop and on a
  // projector instead of getting sparse as the window grows.

  var ctx = null, W = 0, H = 0, DPR = 1, cx = 0, cy = 0, FOCAL = 1.05;

  function px(x, y, z) {
    var s = (FOCAL * H) / z;
    return { x: cx + x * s, y: cy + y * s, s: s };
  }

  // R = Rz · Ry · Rx, as a flat nine so a scene can rotate a mesh without
  // allocating anything per vertex.
  function matOf(ax, ay, az) {
    var sx = Math.sin(ax), kx = Math.cos(ax);
    var sy = Math.sin(ay), ky = Math.cos(ay);
    var sz = Math.sin(az), kz = Math.cos(az);
    return [kz * ky, kz * sy * sx - sz * kx, kz * sy * kx + sz * sx,
            sz * ky, sz * sy * sx + kz * kx, sz * sy * kx - kz * sx,
            -sy, ky * sx, ky * kx];
  }
  // R = a · b. matOf composes as Rz·Ry·Rx, which is the right order for
  // tumbling one object; looking *down* at something that is turning needs the
  // tilt applied after the spin, and that is a different product.
  function mul(a, b) {
    var o = [0, 0, 0, 0, 0, 0, 0, 0, 0];
    for (var i = 0; i < 3; i++) {
      for (var j = 0; j < 3; j++) {
        o[i * 3 + j] = a[i * 3] * b[j] + a[i * 3 + 1] * b[3 + j] + a[i * 3 + 2] * b[6 + j];
      }
    }
    return o;
  }

  function xf(m, x, y, z, out) {
    out[0] = m[0] * x + m[1] * y + m[2] * z;
    out[1] = m[3] * x + m[4] * y + m[5] * z;
    out[2] = m[6] * x + m[7] * y + m[8] * z;
    return out;
  }
  function norm3(v) {
    var l = Math.sqrt(v[0] * v[0] + v[1] * v[1] + v[2] * v[2]) || 1;
    v[0] /= l; v[1] /= l; v[2] /= l;
    return v;
  }

  // The light, in the same space as everything else: over the viewer's left
  // shoulder, which is where the eye expects light to come from and the only
  // reason a flat-shaded cube reads as a cube at all.
  var LX = -0.42, LY = -0.62, LZ = -0.66;

  // ── The state a scene is handed ───────────────────────────────────────────

  var PAL = [], FOG = [12, 14, 32], GLOW = [160, 200, 255], VEIL = '';
  var DENSITY = 1, SPEED = 1, DARK = true;

  function readTheme() {
    PAL = cssList('--backdrop-colors', ['#6b5cff', '#3f6fe0', '#8b6ad8']).map(rgb);
    FOG = rgb(cssVar('--backdrop-fog', cssVar('--slide-bg', '#0b0d1f')));
    GLOW = rgb(cssVar('--backdrop-glow', cssVar('--slide-accent', '#9fd8ff')));
    VEIL = cssVar('--backdrop-veil', '');
    DENSITY = CFG.density !== undefined ? +CFG.density : cssNum('--backdrop-density', 1);
    SPEED = CFG.speed !== undefined ? +CFG.speed : cssNum('--backdrop-speed', 1);
    if (!(DENSITY > 0)) { DENSITY = 1; }
    if (!(SPEED >= 0)) { SPEED = 1; }
    DARK = lum(rgb(cssVar('--slide-bg', '#0b0d1f'))) < 0.5;
  }
  function hue(i) { return PAL[((i % PAL.length) + PAL.length) % PAL.length]; }
  // How much of a thing at this depth survives the distance. Everything fades
  // into the deck's own ground rather than into black, so a scene never puts a
  // hole in the slide behind it.
  function fog(z, near, far) {
    var t = (far - z) / (far - near);
    return t < 0 ? 0 : t > 1 ? 1 : t;
  }

  // ── Meshes ────────────────────────────────────────────────────────────────
  //
  // A mesh is vertices and faces of indices, and the faces carry no normals:
  // they are worked out per frame from the rotated vertices, because a normal
  // transformed badly is a facet lit as though it were somewhere else and that
  // is the one error in flat shading nobody can see the cause of.

  function cubeMesh() {
    return {
      v: [[-1, -1, -1], [1, -1, -1], [1, 1, -1], [-1, 1, -1],
          [-1, -1, 1], [1, -1, 1], [1, 1, 1], [-1, 1, 1]],
      f: [[0, 1, 2, 3], [5, 4, 7, 6], [4, 0, 3, 7], [1, 5, 6, 2], [4, 5, 1, 0], [3, 2, 6, 7]],
      e: [[0, 1], [1, 2], [2, 3], [3, 0], [4, 5], [5, 6], [6, 7], [7, 4], [0, 4], [1, 5], [2, 6], [3, 7]]
    };
  }

  // An icosahedron: twenty triangles, which is the fewest a lump of rock or a
  // shard of crystal can be made of and still read as one.
  function icoMesh() {
    var g = (1 + Math.sqrt(5)) / 2;
    var v = [[-1, g, 0], [1, g, 0], [-1, -g, 0], [1, -g, 0],
             [0, -1, g], [0, 1, g], [0, -1, -g], [0, 1, -g],
             [g, 0, -1], [g, 0, 1], [-g, 0, -1], [-g, 0, 1]];
    for (var i = 0; i < v.length; i++) { norm3(v[i]); }
    var f = [[0, 11, 5], [0, 5, 1], [0, 1, 7], [0, 7, 10], [0, 10, 11],
             [1, 5, 9], [5, 11, 4], [11, 10, 2], [10, 7, 6], [7, 1, 8],
             [3, 9, 4], [3, 4, 2], [3, 2, 6], [3, 6, 8], [3, 8, 9],
             [4, 9, 5], [2, 4, 11], [6, 2, 10], [8, 6, 7], [9, 8, 1]];
    return { v: v, f: f, e: edgesOf(f) };
  }

  // Every face's edges, each counted once - a wireframe that drew the shared
  // ones twice would be a wireframe with some lines twice as bright as others.
  function edgesOf(faces) {
    var seen = {}, out = [];
    for (var i = 0; i < faces.length; i++) {
      var f = faces[i];
      for (var j = 0; j < f.length; j++) {
        var a = f[j], b = f[(j + 1) % f.length];
        var k = a < b ? a + ':' + b : b + ':' + a;
        if (!seen[k]) { seen[k] = 1; out.push([a, b]); }
      }
    }
    return out;
  }

  function octMesh() {
    var f = [[0, 2, 4], [2, 1, 4], [1, 3, 4], [3, 0, 4], [2, 0, 5], [1, 2, 5], [3, 1, 5], [0, 3, 5]];
    return { v: [[1, 0, 0], [-1, 0, 0], [0, 0, 1], [0, 0, -1], [0, -1, 0], [0, 1, 0]], f: f, e: edgesOf(f) };
  }

  // A sphere by subdividing an icosahedron and pushing every new vertex back
  // out to the surface. Twenty triangles become eighty become three hundred and
  // twenty, all of them very nearly the same size - which is the property that
  // matters and the one a latitude/longitude sphere does not have: that one
  // crowds its poles, so a globe built from it is finely detailed at the top
  // and coarse round the middle, and reads as a grid rather than as a ball.
  function icoSphere(level) {
    var base = icoMesh(), v = [], i;
    for (i = 0; i < base.v.length; i++) { v.push([base.v[i][0], base.v[i][1], base.v[i][2]]); }
    var f = base.f;
    for (var step = 0; step < level; step++) {
      var nf = [], cache = {};
      for (i = 0; i < f.length; i++) {
        var t = f[i];
        var a = mid(t[0], t[1]), b = mid(t[1], t[2]), c = mid(t[2], t[0]);
        nf.push([t[0], a, c], [a, t[1], b], [c, b, t[2]], [a, b, c]);
      }
      f = nf;
    }
    return { v: v, f: f, e: [] };
    function mid(a, b) {
      var k = a < b ? a + ':' + b : b + ':' + a;
      if (cache[k] !== undefined) { return cache[k]; }
      v.push(norm3([(v[a][0] + v[b][0]) / 2, (v[a][1] + v[b][1]) / 2, (v[a][2] + v[b][2]) / 2]));
      return (cache[k] = v.length - 1);
    }
  }

  // A drum: a regular prism with both ends capped. The cap is one polygon
  // rather than a fan of triangles, because a fan is a ring of slivers and a
  // flat-shaded sliver picks up a different normal from its neighbour and
  // flickers.
  // `flute` pulls every other facet in, which is what a fluted column is and
  // is also the cheapest carving there is: it doubles the number of edges that
  // catch the light without adding a single vertex to the silhouette.
  function prismMesh(sides, r0, r1, h, flute) {
    var v = [], f = [], wall = [], top = [], bot = [], i, a, fl;
    flute = flute || 0;
    for (i = 0; i < sides; i++) {
      a = i / sides * 6.2832;
      fl = 1 - flute * (i % 2);
      v.push([Math.cos(a) * r1 * fl, -h / 2, Math.sin(a) * r1 * fl]);
      v.push([Math.cos(a) * r0 * fl, h / 2, Math.sin(a) * r0 * fl]);
      wall.push(i * 2);
    }
    for (i = 0; i < sides; i++) {
      var j = (i + 1) % sides;
      f.push([i * 2, j * 2, j * 2 + 1, i * 2 + 1]);
    }
    // The caps get a ring of their own, *unfluted*. Taking the cap polygon
    // round the fluted vertices turns the top of a fluted drum into a
    // thirty-point star - which is not a thing that happens to stone, and was
    // the spikes sticking out of the first one of these.
    //
    // Set a hair inside the wall, so the join reads as a chamfer rather than
    // as z-fighting between two coplanar faces.
    for (i = 0; i < sides; i++) {
      a = i / sides * 6.2832;
      top.push(v.push([Math.cos(a) * r1 * 0.985, -h / 2, Math.sin(a) * r1 * 0.985]) - 1);
      bot.push(v.push([Math.cos(a) * r0 * 0.985, h / 2, Math.sin(a) * r0 * 0.985]) - 1);
    }
    f.push(top.slice().reverse());
    f.push(bot);
    return { v: v, f: f, e: edgesOf(f) };
  }

  // Weathering. Every vertex pushed off its place - out or in from the axis, up
  // or down - so that no two facets of a drum are the same size and the
  // silhouette has lumps in it.
  //
  // This is the difference between a turned object and a cut one. A prism with
  // identical facets reads as machined whatever you paint on it, because the
  // eye reads the *regularity* long before it reads the colour: evenly spaced
  // highlights down a cylinder are what brushed metal looks like, and no amount
  // of grey makes them stone.
  function weather(mesh, radial, vertical) {
    for (var i = 0; i < mesh.v.length; i++) {
      var p = mesh.v[i];
      var r = Math.sqrt(p[0] * p[0] + p[2] * p[2]);
      if (r > 0.0001) {
        var k = 1 + between(-radial, radial * 0.55);
        p[0] *= k;
        p[2] *= k;
      }
      p[1] += between(-vertical, vertical);
    }
    return mesh;
  }

  // A torus as edges only. The two rings are built separately rather than as a
  // quad mesh because a wireframe wants its warp and weft, not its faces.
  function torusEdges(R, r, nu, nv) {
    var v = [], e = [], i, j;
    for (i = 0; i < nu; i++) {
      for (j = 0; j < nv; j++) {
        var a = i / nu * Math.PI * 2, b = j / nv * Math.PI * 2;
        v.push([(R + r * Math.cos(b)) * Math.cos(a), r * Math.sin(b), (R + r * Math.cos(b)) * Math.sin(a)]);
        e.push([i * nv + j, i * nv + (j + 1) % nv]);
        e.push([i * nv + j, ((i + 1) % nu) * nv + j]);
      }
    }
    return { v: v, f: [], e: e };
  }

  // A globe of lines: parallels and meridians, which is what a sphere has to be
  // drawn as when it is drawn in wire - a triangulated one is a ball of noise.
  function wireSphere(rings, segs) {
    var v = [], e = [], i, j, idx = 0, prev = null;
    for (i = 1; i < rings; i++) {
      var la = Math.PI * i / rings, row = [];
      for (j = 0; j < segs; j++) {
        var lo = Math.PI * 2 * j / segs;
        v.push([Math.sin(la) * Math.cos(lo), -Math.cos(la), Math.sin(la) * Math.sin(lo)]);
        row.push(idx++);
      }
      for (j = 0; j < segs; j++) { e.push([row[j], row[(j + 1) % segs]]); }
      if (prev) { for (j = 0; j < segs; j++) { e.push([prev[j], row[j]]); } }
      prev = row;
    }
    return { v: v, f: [], e: e };
  }

  // ── Drawing a mesh ────────────────────────────────────────────────────────
  //
  // Faces from every object in the scene go into one list and are sorted
  // together, far to near. Sorting per object instead is the mistake that looks
  // right until two of them overlap, and then one is inexplicably in front.

  var faceBuf = [];

  // Fixed barycentric weights for the grain marks. Eleven of them, which is
  // prime against the four taken per facet, so consecutive facets never get the
  // same set and the marks do not line up into rows.
  var SPECK = [
    [0.62, 0.25, 0.13], [0.2, 0.56, 0.24], [0.27, 0.19, 0.54], [0.44, 0.38, 0.18],
    [0.15, 0.33, 0.52], [0.51, 0.14, 0.35], [0.33, 0.47, 0.2], [0.24, 0.28, 0.48],
    [0.46, 0.2, 0.34], [0.18, 0.5, 0.32], [0.36, 0.3, 0.34]
  ];

  // Rotate, scale, place and project a mesh, pushing its faces onto the shared
  // list. `shade` is handed the lambert term and the depth and answers with a
  // colour, which is the only thing that differs between a stone and a shard of
  // glass.
  function pushFaces(mesh, m, pos, size, opts) {
    var vp = [], tmp = [0, 0, 0], i, j;
    for (i = 0; i < mesh.v.length; i++) {
      var s = mesh.v[i];
      xf(m, s[0] * size, s[1] * size, s[2] * size, tmp);
      vp.push([tmp[0] + pos[0], tmp[1] + pos[1], tmp[2] + pos[2]]);
    }
    for (i = 0; i < mesh.f.length; i++) {
      var f = mesh.f[i];
      var a = vp[f[0]], b = vp[f[1]], c = vp[f[2]];
      var n = norm3([(b[1] - a[1]) * (c[2] - a[2]) - (b[2] - a[2]) * (c[1] - a[1]),
                     (b[2] - a[2]) * (c[0] - a[0]) - (b[0] - a[0]) * (c[2] - a[2]),
                     (b[0] - a[0]) * (c[1] - a[1]) - (b[1] - a[1]) * (c[0] - a[0])]);
      var zc = 0, pts = [], ok = true;
      for (j = 0; j < f.length; j++) {
        var p = vp[f[j]];
        if (p[2] < 0.6) { ok = false; break; }   // behind the camera: there is no drawing that
        zc += p[2];
        pts.push(px(p[0], p[1], p[2]));
      }
      if (!ok) { continue; }
      zc /= f.length;
      // Facing away from the camera. The test is against the direction *to* the
      // face rather than against +z, or a face out at the edge of a wide screen
      // is culled while it is still visible.
      var front = (n[0] * a[0] + n[1] * a[1] + n[2] * a[2]) < 0;
      if (!front && !opts.twoSided) { continue; }
      var lam = front ? Math.max(0, n[0] * LX + n[1] * LY + n[2] * LZ)
                      : Math.max(0, -(n[0] * LX + n[1] * LY + n[2] * LZ));
      // Which way the facet faces, as a number between nought and one. A
      // crystal's colour has to differ facet to facet or the whole shard is one
      // flat lump, and the light will not do it - neighbouring facets of a
      // small solid are lit within a few percent of each other.
      var tint = (n[0] * 0.5 + n[1] * 0.3 + n[2] * 0.2 + 1) / 2;
      // `fi` is which face of the mesh this is, so a scene that worked
      // something out per face when it built the mesh - whether a facet of a
      // globe is land, which course a block of stone was cut along - can look
      // it up here rather than recomputing it sixty times a second.
      faceBuf.push({ z: zc, pts: pts, lam: lam, tint: tint, front: front, fi: i,
                     ny: n[1], obj: opts.obj });
    }
  }

  function drawFaces(shade) {
    faceBuf.sort(function (a, b) { return b.z - a.z; });
    for (var i = 0; i < faceBuf.length; i++) {
      var f = faceBuf[i], st = shade(f);
      if (!st) { continue; }
      ctx.beginPath();
      ctx.moveTo(f.pts[0].x, f.pts[0].y);
      for (var j = 1; j < f.pts.length; j++) { ctx.lineTo(f.pts[j].x, f.pts[j].y); }
      ctx.closePath();
      ctx.fillStyle = st.fill;
      ctx.fill();
      if (st.stroke) {
        ctx.strokeStyle = st.stroke;
        ctx.lineWidth = st.width || 1;
        ctx.stroke();
      }
      // Grain: a few marks inside the facet, at fixed weights between its own
      // corners, so they sit where the facet sits and are the same marks every
      // frame. A random scatter in screen space would crawl, which is the one
      // thing texture must not do.
      if (st.speck) {
        ctx.fillStyle = st.speck;
        var a0 = f.pts[0], b0 = f.pts[1], c0 = f.pts[2 % f.pts.length];
        for (var g = 0; g < 4; g++) {
          var w = SPECK[(f.fi * 4 + g) % SPECK.length];
          var sx = a0.x * w[0] + b0.x * w[1] + c0.x * w[2];
          var sy = a0.y * w[0] + b0.y * w[1] + c0.y * w[2];
          var sz = st.speckSize || 1.4;
          ctx.fillRect(sx, sy, sz, sz);
        }
      }
    }
    faceBuf.length = 0;
  }

  // Edges, for the scenes that are nothing but. Drawn after the faces and
  // sorted the same way - a line is a thing at a depth like anything else.
  function drawEdges(mesh, m, pos, size, colour, width, near, far) {
    var tmp = [0, 0, 0], vp = [], i;
    for (i = 0; i < mesh.v.length; i++) {
      var s = mesh.v[i];
      xf(m, s[0] * size, s[1] * size, s[2] * size, tmp);
      vp.push([tmp[0] + pos[0], tmp[1] + pos[1], tmp[2] + pos[2]]);
    }
    ctx.lineWidth = width;
    ctx.lineCap = 'round';
    for (i = 0; i < mesh.e.length; i++) {
      var a = vp[mesh.e[i][0]], b = vp[mesh.e[i][1]];
      if (a[2] < 0.8 || b[2] < 0.8) { continue; }
      var pa = px(a[0], a[1], a[2]), pb = px(b[0], b[1], b[2]);
      var k = fog((a[2] + b[2]) / 2, near, far);
      ctx.strokeStyle = paint(mix(FOG, colour, 0.25 + 0.75 * k), 0.12 + 0.5 * k);
      ctx.beginPath();
      ctx.moveTo(pa.x, pa.y);
      ctx.lineTo(pb.x, pb.y);
      ctx.stroke();
    }
    return vp;
  }

  // ── cubes ─────────────────────────────────────────────────────────────────
  //
  // A field of lit blocks drifting past the camera, each turning on an axis of
  // its own. Three things make it read as space rather than as confetti: the
  // far ones fade into the deck's ground, the near ones fade *out* again before
  // they can fill the screen with one enormous face, and nothing is ever lit
  // from more than one direction - a cube with three equally bright faces is a
  // hexagon.

  SCENES.cubes = function () {
    // The field is deep and the camera is a long way back from it. The first
    // version of this had the near plane at three units and cubes a unit
    // across, which is a cube the height of the slide arriving every few
    // seconds - a scene that is *in front of* the deck rather than behind it.
    var mesh = cubeMesh(), NEAR = 10, FAR = 74, items = [];
    var n = Math.max(8, Math.round(46 * DENSITY));
    reseed(0x5C1DE5);
    for (var i = 0; i < n; i++) {
      items.push(born({}, between(NEAR + 1, FAR), i));
    }
    function born(c, z, i) {
      c.x = between(-34, 34);
      c.y = between(-22, 22);
      c.z = z;
      c.size = between(0.5, 2.1);
      c.ax = rnd() * 6.283; c.ay = rnd() * 6.283; c.az = rnd() * 6.283;
      c.rx = between(-0.22, 0.22); c.ry = between(-0.26, 0.26); c.rz = between(-0.14, 0.14);
      c.vx = between(-0.12, 0.12); c.vy = between(-0.1, 0.1);
      c.vz = -between(0.8, 2.2);
      c.col = hue(i);
      return c;
    }
    return function (t, dt) {
      var i, c;
      for (i = 0; i < items.length; i++) {
        c = items[i];
        c.ax += c.rx * dt; c.ay += c.ry * dt; c.az += c.rz * dt;
        c.x += c.vx * dt; c.y += c.vy * dt; c.z += c.vz * dt;
        if (c.z < NEAR) { born(c, FAR, i); }
        pushFaces(mesh, matOf(c.ax, c.ay, c.az), [c.x, c.y, c.z], c.size, { obj: c });
      }
      drawFaces(function (f) {
        var c = f.obj, k = fog(f.z, NEAR, FAR);
        // Out of the near distance as well as into the far one. A cube that
        // sails through the camera is a slide nobody can read for two seconds.
        var a = 0.92 * k * Math.min(1, (f.z - NEAR) / 9);
        if (a < 0.012) { return null; }
        var col = mix(scale(c.col, 0.34), c.col, f.lam);
        col = mix(col, mix(c.col, [255, 255, 255], 0.55), f.lam * f.lam * 0.42);
        col = mix(FOG, col, 0.45 + 0.55 * k);
        return {
          fill: paint(col, a),
          stroke: paint(mix(col, GLOW, 0.35), a * 0.5),
          width: 1
        };
      });
    };
  };

  // ── crystal ───────────────────────────────────────────────────────────────
  //
  // Shards: an icosahedron with every vertex pulled in or out, which is the
  // cheapest thing that looks grown rather than modelled. They are drawn
  // *two sided* and translucent, so the far facets show through the near ones -
  // that, and nothing else, is what makes a solid read as glass.

  SCENES.crystal = function () {
    var NEAR = 5, FAR = 34, items = [];
    var n = Math.max(3, Math.round(9 * DENSITY));
    reseed(0xC27574);
    for (var i = 0; i < n; i++) {
      var m = icoMesh();
      // Pulled along one axis as well as per vertex: a shard is longer than it
      // is wide, and a lump that is not is a pebble.
      var sx = between(0.55, 1), sy = between(1, 1.9), sz = between(0.55, 1);
      for (var j = 0; j < m.v.length; j++) {
        var k = between(0.72, 1.3);
        m.v[j] = [m.v[j][0] * sx * k, m.v[j][1] * sy * k, m.v[j][2] * sz * k];
      }
      items.push({
        mesh: m, size: between(1.6, 4.6),
        x: between(-16, 16), y: between(-9, 9), z: between(NEAR + 2, FAR),
        ax: rnd() * 6.283, ay: rnd() * 6.283, az: rnd() * 6.283,
        rx: between(-0.1, 0.1), ry: between(-0.12, 0.12), rz: between(-0.07, 0.07),
        vy: between(-0.09, 0.09), vz: -between(0.1, 0.4),
        col: hue(i), col2: hue(i + 1)
      });
    }
    return function (t, dt) {
      var i;
      for (i = 0; i < items.length; i++) {
        var s = items[i];
        s.ax += s.rx * dt; s.ay += s.ry * dt; s.az += s.rz * dt;
        s.y += s.vy * dt; s.z += s.vz * dt;
        if (s.z < NEAR) { s.z = FAR; s.x = between(-16, 16); s.y = between(-9, 9); }
        pushFaces(s.mesh, matOf(s.ax, s.ay, s.az), [s.x, s.y, s.z], s.size, { obj: s, twoSided: true });
      }
      drawFaces(function (f) {
        var s = f.obj, k = fog(f.z, NEAR, FAR);
        // The two colours mixed by how the facet is lit stands in for
        // refraction: a crystal is never one colour, and which colour a facet
        // happens to be is the whole of what the eye reads as depth in glass.
        var col = mix(s.col, s.col2, f.tint);
        col = mix(col, [255, 255, 255], f.lam * f.lam * 0.5);
        // Gentler with the distance than the solid scenes are. A shard is
        // already translucent, so fading it into the ground as well takes the
        // colour out twice and leaves a field of grey glass.
        col = mix(FOG, col, 0.68 + 0.32 * k);
        var a = (f.front ? 0.3 : 0.15) * (0.55 + 0.45 * k);
        return {
          fill: paint(col, a),
          // The lit edge is where a facet is: a translucent fill on its own is
          // a cloud. Brightest where the light is, so an edge turning away
          // dims rather than blinking out.
          stroke: paint(mix(col, GLOW, 0.5 + 0.4 * f.lam), (f.front ? 0.42 : 0.14) * (0.45 + 0.55 * k)),
          width: f.front ? 1.1 : 0.8
        };
      });
    };
  };

  // ── wireframe ─────────────────────────────────────────────────────────────
  //
  // Solids drawn as nothing but their edges, tumbling slowly. No faces at all,
  // which means no sorting and no hidden-line problem - and that is honest: a
  // wireframe you can see the back of is what a wireframe is for.

  SCENES.wireframe = function () {
    var NEAR = 4, FAR = 38, items = [];
    var kinds = [icoMesh(), octMesh(), cubeMesh(), torusEdges(1, 0.38, 16, 9), wireSphere(7, 14)];
    var n = Math.max(3, Math.round(10 * DENSITY));
    reseed(0x721BE5);
    for (var i = 0; i < n; i++) {
      items.push({
        mesh: kinds[i % kinds.length], size: between(1.4, 4.2),
        x: between(-17, 17), y: between(-10, 10), z: between(NEAR + 2, FAR),
        ax: rnd() * 6.283, ay: rnd() * 6.283, az: rnd() * 6.283,
        rx: between(-0.15, 0.15), ry: between(-0.18, 0.18), rz: between(-0.1, 0.1),
        vy: between(-0.1, 0.1), vz: -between(0.12, 0.5),
        col: hue(i), dot: (i % 3) === 0
      });
    }
    return function (t, dt) {
      for (var i = 0; i < items.length; i++) {
        var s = items[i];
        s.ax += s.rx * dt; s.ay += s.ry * dt; s.az += s.rz * dt;
        s.y += s.vy * dt; s.z += s.vz * dt;
        if (s.z < NEAR) { s.z = FAR; s.x = between(-17, 17); s.y = between(-10, 10); }
        var vp = drawEdges(s.mesh, matOf(s.ax, s.ay, s.az), [s.x, s.y, s.z], s.size,
                           s.col, s.size > 3 ? 1.4 : 1, NEAR, FAR);
        if (!s.dot) { continue; }
        // A node at each vertex on some of them. All of them and it is a mesh
        // of beads; none of them and the shapes have no joints.
        for (var j = 0; j < vp.length; j++) {
          var p = vp[j];
          if (p[2] < 1) { continue; }
          var q = px(p[0], p[1], p[2]), k = fog(p[2], NEAR, FAR);
          ctx.fillStyle = paint(mix(FOG, GLOW, 0.4 + 0.6 * k), 0.25 + 0.45 * k);
          var r = Math.max(0.8, q.s * 0.012);
          ctx.fillRect(q.x - r, q.y - r, r * 2, r * 2);
        }
      }
    };
  };

  // ── A heightfield, flat shaded ────────────────────────────────────────────
  //
  // The machinery behind both `lowpoly` and `waves`, which are the same scene
  // told twice: a sheet of triangles running away to the horizon with the whole
  // thing moving towards the viewer. What differs between them is the shape of
  // the surface and how it takes the light, which is what a scene passes in -
  // everything else here was written once and had no business being written
  // again.
  //
  // Three decisions are the field's rather than either scene's:
  //
  //  1. **The height is sampled at a point that moves with time**, rather than
  //     the grid being moved. A grid that moved would have to be wrapped, and
  //     the seam where it wrapped would travel up the screen once a minute for
  //     the whole talk.
  //  2. **The rows bunch up towards the horizon.** Evenly spaced in z, a field
  //     this deep spends most of its triangles on three inches of horizon.
  //  3. **Each triangle is stroked in its own colour.** Adjacent antialiased
  //     fills leave a hairline of the page showing through every shared edge,
  //     and a field with a bright crack round each face is not a low-poly
  //     field, it is a net.

  function heightField(cfg) {
    var COLS = Math.max(12, Math.round(cfg.cols * DENSITY));
    var ROWS = Math.max(10, Math.round(cfg.rows * DENSITY));
    return function (t) {
      var drift = t * cfg.drift, r, c, cols = COLS + 1;
      var xs = [], zs = [], hs = [];
      for (c = 0; c <= COLS; c++) { xs.push(cfg.x0 + (cfg.x1 - cfg.x0) * c / COLS); }
      for (r = 0; r <= ROWS; r++) {
        var u = r / ROWS;
        zs.push(cfg.z0 + (cfg.z1 - cfg.z0) * u * u);
      }
      for (r = 0; r <= ROWS; r++) {
        for (c = 0; c <= COLS; c++) { hs.push(cfg.height(xs[c], zs[r] + drift, t)); }
      }
      ctx.lineJoin = 'round';
      // Far rows first. A heightfield seen from above is not sortable by cell
      // centre - a tall near ridge and a far one can share a depth - but row
      // order is exactly right for a camera looking along it.
      for (r = ROWS - 1; r >= 0; r--) {
        for (c = 0; c < COLS; c++) {
          var z0 = zs[r], z1 = zs[r + 1];
          var h00 = hs[r * cols + c], h10 = hs[r * cols + c + 1];
          var h01 = hs[(r + 1) * cols + c], h11 = hs[(r + 1) * cols + c + 1];
          tri(xs[c], h00, z0, xs[c + 1], h10, z0, xs[c], h01, z1);
          tri(xs[c + 1], h10, z0, xs[c + 1], h11, z1, xs[c], h01, z1);
        }
      }
      if (cfg.after) { cfg.after(t, xs, zs, hs, cols, COLS, ROWS, drift); }
    };
    function tri(ax, ah, az, bx, bh, bz, cx3, ch, cz3) {
      if (az < 1 || bz < 1 || cz3 < 1) { return; }
      var ay = cfg.base - ah, by = cfg.base - bh, cy3 = cfg.base - ch;
      var ux = bx - ax, uy = by - ay, uz = bz - az;
      var vx = cx3 - ax, vy = cy3 - ay, vz = cz3 - az;
      var n = norm3([uy * vz - uz * vy, uz * vx - ux * vz, ux * vy - uy * vx]);
      var lam = Math.abs(n[0] * LX + n[1] * LY + n[2] * LZ);
      var k = fog(az, cfg.z0, cfg.z1);
      var col = cfg.shade((ah + bh + ch) / 3, lam, k, n);
      var pa = px(ax, ay, az), pb = px(bx, by, bz), pc = px(cx3, cy3, cz3);
      ctx.beginPath();
      ctx.moveTo(pa.x, pa.y);
      ctx.lineTo(pb.x, pb.y);
      ctx.lineTo(pc.x, pc.y);
      ctx.closePath();
      var st = paint(col, cfg.alpha === undefined ? 0.9 : cfg.alpha);
      ctx.fillStyle = st;
      ctx.fill();
      ctx.strokeStyle = st;
      ctx.lineWidth = 1;
      ctx.stroke();
    }
  }

  // ── lowpoly ───────────────────────────────────────────────────────────────
  //
  // Ridges and hollows: the sharp one, with a different material in the
  // hollows from on the ridges. That colour-by-height is what a low-poly field
  // does that a grey one cannot.

  SCENES.lowpoly = function () {
    var hi = 2.8;
    return heightField({
      cols: 26, rows: 18, x0: -26, x1: 26, z0: 5.5, z1: 42, base: 5.4, drift: 0.55,
      height: function (x, z) {
        return 1.55 * Math.sin(x * 0.21 + z * 0.09) * Math.cos(z * 0.16)
             + 0.85 * Math.sin(x * 0.44 - z * 0.3)
             + 0.4 * Math.sin(x * 0.9 + z * 0.7);
      },
      shade: function (h, lam, k) {
        var col = mix(hue(0), hue(1), Math.min(1, Math.max(0, (h + hi) / (2 * hi))));
        col = mix(scale(col, 0.3), mix(col, [255, 255, 255], 0.12), 0.2 + 0.8 * lam);
        return mix(FOG, col, 0.1 + 0.9 * k * k);
      }
    });
  }

  // ── waves ─────────────────────────────────────────────────────────────────
  //
  // The same sheet as an open swell: longer wavelengths, a shallower rise, and
  // the light handled completely differently. A hillside is lit; water is lit
  // *and* reflects, so the shading carries a sharp highlight term on top of the
  // soft one - and the whole difference between a field of triangles and water
  // is that the highlight falls on the faces turned up towards the light rather
  // than being spread evenly over the swell.
  //
  // The crests then get a line of foam, drawn after the surface so it is never
  // half covered by the row in front. Only where the water is actually rising
  // does one appear, which is why the sea looks like it is going somewhere.

  SCENES.waves = function () {
    var amp = 2.4;
    return heightField({
      cols: 32, rows: 21, x0: -36, x1: 36, z0: 7, z1: 64, base: 5.2, drift: 0.9,
      alpha: 0.95,
      height: function (x, z, t) {
        return 1.25 * Math.sin(x * 0.13 + z * 0.07 + t * 0.35)
             + 0.8 * Math.sin(z * 0.2 - t * 0.5)
             + 0.35 * Math.sin((x + z) * 0.34 + t * 0.9);
      },
      shade: function (h, lam, k) {
        // The sheen is the lambert term taken to a high power, which is the
        // cheapest thing that behaves like a specular highlight: it is nothing
        // on a facet lying flat and everything on one turned to throw the light
        // at the viewer, so the glitter lands on the faces of the swell rather
        // than on the swell.
        //
        // It was written as "how far the facet is turned towards the sky",
        // which on open water is very nearly all of them - so every triangle
        // carried a full-strength highlight and the sea came out as a milky
        // slab with no shape in it at all.
        var col = mix(hue(0), hue(1), Math.min(1, Math.max(0, (h + amp) / (2 * amp))));
        col = mix(scale(col, 0.38), col, 0.2 + 0.8 * lam);
        col = mix(col, mix(GLOW, [255, 255, 255], 0.35), Math.pow(lam, 10) * 0.85);
        return mix(FOG, col, 0.06 + 0.94 * k * k);
      },
      after: function (t, xs, zs, hs, cols, COLS, ROWS, drift) {
        ctx.lineWidth = 1.4;
        ctx.lineCap = 'round';
        for (var r = ROWS - 1; r >= 0; r--) {
          var z = zs[r];
          if (z < 1) { continue; }
          var k = fog(z, 7, 64);
          for (var c = 0; c < COLS; c++) {
            var h0 = hs[r * cols + c], h1 = hs[r * cols + c + 1];
            // A crest is the top of a wave in this row *and* a wave that is
            // still climbing in z - the second half is what keeps foam off the
            // back of every swell, where water does not break.
            if (h0 < amp * 0.52 || h1 < amp * 0.52) { continue; }
            if (hs[(r + 1) * cols + c] > h0) { continue; }
            var pa = px(xs[c], 5.2 - h0, z), pb = px(xs[c + 1], 5.2 - h1, z);
            ctx.strokeStyle = paint(mix(FOG, GLOW, 0.5 + 0.5 * k), 0.08 + 0.4 * k);
            ctx.beginPath();
            ctx.moveTo(pa.x, pa.y);
            ctx.lineTo(pb.x, pb.y);
            ctx.stroke();
          }
        }
      }
    });
  }

  // ── fog ───────────────────────────────────────────────────────────────────
  //
  // Soft drifting haze, and the only scene here with no geometry in it at all -
  // it is a dozen radial gradients moving slowly past each other. Which is the
  // honest way to draw fog on a canvas: the alternative is a particle system
  // blurred, and a blur over the whole viewport every frame costs more than
  // every other scene in this file put together.
  //
  // It is also the one that most needs the ground to be dark. Haze is *added*
  // light, and over a pale theme adding light to white does nothing at all -
  // hence the normal-blending fallback, which paints the same shapes as a wash
  // instead.

  SCENES.fog = function () {
    var n = Math.max(5, Math.round(14 * DENSITY)), blobs = [], i;
    reseed(0xF06);
    for (i = 0; i < n; i++) {
      blobs.push({
        x: between(-0.2, 1.2), y: between(-0.1, 1.1),
        r: between(0.28, 0.82),
        vx: between(-0.012, 0.012), vy: between(-0.008, 0.008),
        ph: rnd() * 6.283, br: between(0.5, 1),
        col: hue(i)
      });
    }
    return function (t, dt) {
      ctx.globalCompositeOperation = DARK ? 'lighter' : 'source-over';
      for (var i = 0; i < blobs.length; i++) {
        var b = blobs[i];
        b.x += b.vx * dt;
        b.y += b.vy * dt;
        // Round the sides rather than bouncing: a bank of fog that turned round
        // at the edge of the screen would have a rhythm, and fog has none.
        if (b.x < -0.4) { b.x = 1.4; } else if (b.x > 1.4) { b.x = -0.4; }
        if (b.y < -0.4) { b.y = 1.4; } else if (b.y > 1.4) { b.y = -0.4; }
        var breathe = 0.78 + 0.22 * Math.sin(t * 0.13 + b.ph);
        var rr = b.r * H * breathe, X = b.x * W, Y = b.y * H;
        var a = (DARK ? 0.22 : 0.15) * b.br * breathe;
        var g = ctx.createRadialGradient(X, Y, 0, X, Y, rr);
        g.addColorStop(0, paint(b.col, a));
        g.addColorStop(0.55, paint(mix(b.col, FOG, 0.4), a * 0.42));
        g.addColorStop(1, paint(FOG, 0));
        ctx.fillStyle = g;
        ctx.fillRect(X - rr, Y - rr, rr * 2, rr * 2);
      }
      ctx.globalCompositeOperation = 'source-over';
    };
  };

  // ── topology ──────────────────────────────────────────────────────────────
  //
  // Contour lines over a field that is slowly eroding: a map of somewhere that
  // will not hold still. Marching squares, which is the whole of it - a scalar
  // sampled on a grid, and for each cell and each level the one or two segments
  // where that level crosses it.
  //
  // Two things make it read as a map rather than as interference. The levels
  // are evenly spaced, so where the ground is steep the lines crowd together
  // exactly as they do on a real contour map; and every few levels is drawn
  // heavier, which is the index contour an Ordnance Survey sheet has for the
  // same reason - a wall of identical hairlines cannot be counted.

  SCENES.topology = function () {
    var GX = Math.max(20, Math.round(46 * DENSITY)), GY = Math.max(14, Math.round(27 * DENSITY));
    var LEVELS = 11, SPAN = 2.9;
    var f = new Float64Array((GX + 1) * (GY + 1));
    return function (t) {
      var i, j, w = GX + 1;
      for (j = 0; j <= GY; j++) {
        for (i = 0; i <= GX; i++) {
          var x = (i / GX - 0.5) * 7.2, y = (j / GY - 0.5) * 4.4;
          f[j * w + i] = Math.sin(x * 1.05 + t * 0.21)
                       + Math.sin(y * 1.27 - t * 0.17)
                       + 0.74 * Math.sin((x + y) * 0.86 + t * 0.12)
                       + 0.52 * Math.sin(Math.sqrt(x * x + y * y) * 1.9 - t * 0.33);
        }
      }
      var cw = W / GX, ch = H / GY;
      ctx.lineCap = 'round';
      for (var l = 0; l < LEVELS; l++) {
        var lev = -SPAN + (2 * SPAN) * (l + 0.5) / LEVELS;
        var index = (l % 4) === 0;
        var col = mix(hue(0), hue(1), l / (LEVELS - 1));
        ctx.strokeStyle = paint(mix(FOG, index ? mix(col, GLOW, 0.45) : col, 0.95),
                                index ? 0.75 : 0.4);
        ctx.lineWidth = index ? 2 : 1.2;
        ctx.beginPath();
        for (j = 0; j < GY; j++) {
          for (i = 0; i < GX; i++) {
            cell(i, j, lev, w, cw, ch);
          }
        }
        ctx.stroke();
      }
      function cell(i, j, lev, w, cw, ch) {
        var a = f[j * w + i], b = f[j * w + i + 1];
        var c = f[(j + 1) * w + i + 1], d = f[(j + 1) * w + i];
        var k = (a > lev ? 1 : 0) | (b > lev ? 2 : 0) | (c > lev ? 4 : 0) | (d > lev ? 8 : 0);
        if (k === 0 || k === 15) { return; }
        var x0 = i * cw, y0 = j * ch, x1 = x0 + cw, y1 = y0 + ch;
        // The crossing point on each side, by linear interpolation - the one
        // thing marching squares cannot be cheap about, because rounding each
        // crossing to the middle of its edge is what turns contours into a
        // staircase.
        var T = { x: x0 + cw * (lev - a) / (b - a), y: y0 };
        var R = { x: x1, y: y0 + ch * (lev - b) / (c - b) };
        var B = { x: x0 + cw * (lev - d) / (c - d), y: y1 };
        var L = { x: x0, y: y0 + ch * (lev - a) / (d - a) };
        switch (k) {
          case 1: case 14: seg(L, T); break;
          case 2: case 13: seg(T, R); break;
          case 3: case 12: seg(L, R); break;
          case 4: case 11: seg(R, B); break;
          case 6: case 9: seg(T, B); break;
          case 7: case 8: seg(L, B); break;
          // The two saddles. Drawn as both pairs rather than resolved by
          // sampling the middle: at this grid size the ambiguity is a pixel
          // wide and the extra sample is a quarter of the field again.
          case 5: seg(L, T); seg(R, B); break;
          case 10: seg(T, R); seg(L, B); break;
        }
      }
      function seg(p, q) {
        ctx.moveTo(p.x, p.y);
        ctx.lineTo(q.x, q.y);
      }
    };
  };

  // ── globe ─────────────────────────────────────────────────────────────────
  //
  // The world as dots, turning, with light running between its cities.
  //
  // The land is a 5° grid held as runs rather than as a bitmap: a row of a
  // bitmap is seventy-two characters that have to be counted to be read, and a
  // continent that is one column out is a continent nobody can find the bug in.
  // A run is `[firstColumn, lastColumn]` at 5° a column, counted from the
  // antimeridian, in a row 5° of latitude deep counted from the pole - so
  // `[36, 43]` on the row at 27.5°N is the Sahara and can be read as such.
  //
  // It is deliberately coarse. What is wanted is the *suggestion* of the world
  // behind a slide, and at this scale the shapes read while no detail is
  // claimed that is not there.
  var LAND = {
    1: [[27, 32]],
    2: [[14, 22], [24, 32], [55, 57]],
    3: [[11, 22], [25, 32], [39, 42], [48, 71]],
    4: [[3, 7], [8, 23], [26, 31], [38, 71]],
    5: [[3, 8], [9, 23], [26, 27], [32, 33], [37, 71]],
    6: [[4, 8], [9, 24], [35, 35], [37, 70]],
    7: [[10, 25], [34, 42], [43, 68]],
    8: [[11, 25], [36, 44], [45, 64]],
    9: [[11, 22], [34, 36], [37, 39], [40, 62], [64, 64]],
    10: [[11, 21], [35, 36], [38, 39], [40, 48], [51, 60], [61, 64]],
    11: [[12, 20], [34, 35], [36, 45], [49, 60], [62, 64]],
    12: [[13, 19], [33, 47], [49, 60]],
    13: [[14, 18], [19, 21], [32, 47], [49, 59]],
    14: [[15, 18], [32, 44], [44, 47], [50, 53], [55, 57], [60, 60]],
    15: [[17, 19], [32, 44], [51, 52], [55, 57], [60, 61]],
    16: [[20, 23], [33, 45], [52, 52], [55, 57], [60, 61]],
    17: [[20, 26], [37, 44], [56, 59]],
    18: [[20, 27], [38, 44], [56, 62]],
    19: [[20, 29], [38, 44], [57, 66]],
    20: [[20, 28], [38, 44], [45, 46], [61, 64]],
    21: [[21, 28], [38, 44], [44, 45], [59, 65]],
    22: [[22, 28], [38, 43], [44, 45], [58, 66]],
    23: [[21, 26], [39, 42], [58, 66]],
    24: [[21, 25], [39, 42], [59, 66]],
    25: [[21, 24], [64, 66], [70, 71]],
    26: [[21, 22], [65, 65], [70, 71]],
    27: [[21, 22]],
    28: [[21, 22]]
  };

  // The cities are hubs because they are what somebody looking at a map of
  // network traffic expects to be lit up, and because a line between two of
  // them reads as a route while a line between two random dots reads as a
  // mistake. Longitude, latitude.
  var HUBS = [
    [-0.1, 51.5], [-74, 40.7], [139.7, 35.7], [151.2, -33.9], [-46.6, -23.5],
    [3.4, 6.5], [72.9, 19.1], [37.6, 55.8], [-118.2, 34.1], [103.8, 1.3],
    [55.3, 25.2], [31.2, 30.0], [28.0, -26.2], [-99.1, 19.4], [-79.4, 43.7],
    [121.5, 31.2], [127.0, 37.6], [174.8, -36.9], [-58.4, -34.6], [2.3, 48.9],
    [77.2, 28.6], [116.4, 39.9], [-43.2, -22.9], [13.4, 52.5], [100.5, 13.8]
  ];
  var ROUTES = [
    [0, 1], [1, 8], [0, 7], [2, 15], [15, 9], [9, 3], [3, 6], [6, 10], [10, 0],
    [1, 4], [4, 18], [5, 11], [11, 0], [12, 5], [13, 8], [14, 1], [16, 2],
    [17, 3], [19, 0], [20, 6], [21, 2], [22, 4], [23, 7], [24, 9], [2, 17],
    [19, 12], [20, 10], [21, 16], [14, 23], [18, 12]
  ];

  function onSphere(lon, lat) {
    var la = lat * Math.PI / 180, lo = lon * Math.PI / 180;
    return [Math.cos(la) * Math.sin(lo), -Math.sin(la), Math.cos(la) * Math.cos(lo)];
  }

  // Is this point on land? The runs are 5° cells counted from the pole and from
  // the antimeridian, so this is two divisions and a walk along one row.
  function isLand(v) {
    var lat = -Math.asin(Math.max(-1, Math.min(1, v[1]))) * 180 / Math.PI;
    var lon = Math.atan2(v[0], v[2]) * 180 / Math.PI;
    var r = Math.floor((90 - lat) / 5), c = Math.floor((lon + 180) / 5);
    var runs = LAND[r];
    if (!runs) { return false; }
    for (var i = 0; i < runs.length; i++) {
      if (c >= runs[i][0] && c <= runs[i][1]) { return true; }
    }
    return false;
  }

  SCENES.globe = function () {
    // Faceted, not dotted. A grid of dots is a grid however finely it is
    // spaced - it has rows, and the eye finds them - so the world here is a
    // subdivided icosahedron flat shaded, with the facets that happen to be
    // over land picked out. The coastlines come out ragged and approximate,
    // which is what "ephemeral" was asking for in the first place and is also
    // the truth about a 5° land mask.
    var R = 7.6, CXX = 5.2, CYY = 1.1, CZ = 30, TILT = -0.32;
    var level = DENSITY >= 1.3 ? 4 : (DENSITY >= 0.7 ? 3 : 2);
    var mesh = icoSphere(level), i, j;
    // Land, and a little noise, worked out once: both are fixed to the sphere
    // and neither has any business being recomputed per frame.
    var meta = [];
    reseed(0x610BE5);
    for (i = 0; i < mesh.f.length; i++) {
      var f = mesh.f[i], cen = [0, 0, 0];
      for (j = 0; j < f.length; j++) {
        cen[0] += mesh.v[f[j]][0]; cen[1] += mesh.v[f[j]][1]; cen[2] += mesh.v[f[j]][2];
      }
      norm3(cen);
      meta.push({ land: isLand(cen), tint: rnd(), lat: cen[1] });
    }

    var hubs = HUBS.map(function (h) { return onSphere(h[0], h[1]); });
    // More routes than there are hubs, and shorter waits between pulses: the
    // lines are the only thing on this that is *happening*, and a globe with
    // three lights a minute on it reads as a still picture somebody forgot to
    // animate.
    var routes = [];
    for (i = 0; i < ROUTES.length; i++) {
      routes.push(route(ROUTES[i][0], ROUTES[i][1], i));
    }
    for (i = 0; i < HUBS.length; i++) {
      routes.push(route(i, (i * 7 + 3) % HUBS.length, i + 100));
      routes.push(route(i, (i * 11 + 5) % HUBS.length, i + 200));
    }
    function route(a, b, k) {
      return { a: a, b: b, period: between(3.5, 9), at: rnd() * 9, lift: between(0.09, 0.3) };
    }
    var tmp = [0, 0, 0];

    return function (t, dt) {
      var m = matOf(TILT, Math.PI + t * 0.045, 0), i;

      // The air round it. On a faceted globe this is all that keeps the limb
      // from being a hard polygon against the page.
      var mid = px(CXX, CYY, CZ), rad = R * (FOCAL * H) / CZ;
      var g = ctx.createRadialGradient(mid.x, mid.y, rad * 0.9, mid.x, mid.y, rad * 1.22);
      g.addColorStop(0, paint(mix(FOG, GLOW, 0.3), 0.3));
      g.addColorStop(1, paint(FOG, 0));
      ctx.fillStyle = g;
      ctx.beginPath();
      ctx.arc(mid.x, mid.y, rad * 1.22, 0, 6.2832);
      ctx.fill();

      pushFaces(mesh, m, [CXX, CYY, CZ], R, { obj: meta });
      drawFaces(function (fc) {
        var d = fc.obj[fc.fi];
        // Ocean is the ground with a little colour in it; land is the colour.
        // The per-facet jitter is what stops a continent being one flat shape -
        // low poly is a lot of nearly-equal tones, not a posterised photograph.
        var col = d.land ? mix(hue(0), hue(1), 0.35 + 0.5 * d.tint)
                         : mix(FOG, hue(0), 0.16 + 0.12 * d.tint);
        col = mix(scale(col, d.land ? 0.42 : 0.6), col, 0.25 + 0.75 * fc.lam);
        return {
          fill: paint(col, d.land ? 0.96 : 0.9),
          // Stroked in its own colour: the facets share edges, and an
          // antialiased seam between two fills is a hairline of the page.
          stroke: paint(col, d.land ? 0.96 : 0.9),
          width: 1
        };
      });

      var add = DARK ? 'lighter' : 'source-over';
      for (i = 0; i < routes.length; i++) {
        var rt = routes[i];
        rt.at += dt;
        arc(hubs[rt.a], hubs[rt.b], rt.lift, m, (rt.at % rt.period) / rt.period, add);
      }

      for (i = 0; i < hubs.length; i++) {
        var p = xf(m, hubs[i][0], hubs[i][1], hubs[i][2], tmp);
        if (p[2] > -0.06) { continue; }
        var f2 = -p[2];
        var qq = px(p[0] * R + CXX, p[1] * R + CYY, p[2] * R + CZ);
        var rr = Math.max(1.4, rad * 0.009);
        ctx.fillStyle = paint(GLOW, 0.35 + 0.5 * f2);
        ctx.beginPath();
        ctx.arc(qq.x, qq.y, rr, 0, 6.2832);
        ctx.fill();
        var ring = ((t * 0.35 + i * 0.37) % 1);
        if (ring < 0.75) {
          ctx.strokeStyle = paint(GLOW, (0.3 - ring * 0.4) * (0.3 + 0.7 * f2));
          ctx.lineWidth = 1;
          ctx.beginPath();
          ctx.arc(qq.x, qq.y, rr + ring * rad * 0.07, 0, 6.2832);
          ctx.stroke();
        }
      }

      function arc(A, B, lift, m, pulse, add) {
        var N = 26, pts = [], vis = [], k;
        for (k = 0; k <= N; k++) {
          var u = k / N;
          var v = norm3([A[0] + (B[0] - A[0]) * u, A[1] + (B[1] - A[1]) * u, A[2] + (B[2] - A[2]) * u]);
          var h = 1 + lift * Math.sin(Math.PI * u);
          var w = xf(m, v[0] * h, v[1] * h, v[2] * h, [0, 0, 0]);
          pts.push(px(w[0] * R + CXX, w[1] * R + CYY, w[2] * R + CZ));
          vis.push(-w[2]);
        }
        ctx.lineWidth = 1;
        ctx.lineCap = 'round';
        for (k = 0; k < N; k++) {
          if (vis[k] < 0.02 || vis[k + 1] < 0.02) { continue; }
          var f = Math.min(vis[k], vis[k + 1]);
          ctx.strokeStyle = paint(mix(FOG, GLOW, 0.55), 0.07 + 0.26 * f);
          ctx.beginPath();
          ctx.moveTo(pts[k].x, pts[k].y);
          ctx.lineTo(pts[k + 1].x, pts[k + 1].y);
          ctx.stroke();
        }
        if (pulse > 1) { return; }
        var head = pulse * N, tail = head - 4.5;
        ctx.globalCompositeOperation = add;
        ctx.lineWidth = 2;
        for (k = Math.max(0, Math.floor(tail)); k < Math.min(N, Math.ceil(head)); k++) {
          if (vis[k] < 0.02 || vis[k + 1] < 0.02) { continue; }
          var along = 1 - (head - k) / 4.5;
          if (along < 0) { continue; }
          ctx.strokeStyle = paint(GLOW, along * along * 0.75 * Math.min(vis[k], vis[k + 1]));
          ctx.beginPath();
          ctx.moveTo(pts[k].x, pts[k].y);
          ctx.lineTo(pts[k + 1].x, pts[k + 1].y);
          ctx.stroke();
        }
        var hi2 = Math.min(N, Math.round(head));
        if (vis[hi2] > 0.02) {
          ctx.fillStyle = paint(GLOW, 0.8 * vis[hi2]);
          ctx.beginPath();
          ctx.arc(pts[hi2].x, pts[hi2].y, 1.8, 0, 6.2832);
          ctx.fill();
        }
        ctx.globalCompositeOperation = 'source-over';
      }
    };
  };

  // ── matrix ────────────────────────────────────────────────────────────────
  //
  // Columns of glyphs falling away into the distance. The one everybody knows,
  // and the thing that makes it a *field* rather than a screensaver is that the
  // columns are at depths: a column eight units back is smaller, dimmer and
  // falling slower in screen terms, all three of which come out of the one
  // perspective divide rather than being faked per column.
  //
  // The glyphs are drawn, not animated. A column mutates one character every so
  // often and leaves the rest alone - rewriting the whole column every frame is
  // both more work and less convincing, because what the eye follows is the
  // bright head and a trail that is *stable* behind it.

  var GLYPHS = 'ｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉﾊﾋﾌﾍﾎﾏﾐﾑﾒﾓﾔﾕﾖﾗﾘﾙﾚﾛﾜ0123456789+-*/<>=';

  SCENES.matrix = function () {
    var NEAR = 7, FAR = 40, TOP = -16, BOT = 16, STEP = 0.95;
    var n = Math.max(14, Math.round(84 * DENSITY));
    var cols = [], i;
    reseed(0x4A1E);
    for (i = 0; i < n; i++) {
      cols.push(spawn({}, between(TOP - 14, BOT)));
    }
    function spawn(c, y) {
      c.x = between(-22, 22);
      c.z = between(NEAR, FAR);
      c.y = y;
      c.len = (between(7, 20)) | 0;
      c.speed = between(3.2, 8.5);
      c.hue = hue((rnd() * 3) | 0);
      c.ch = [];
      for (var k = 0; k < c.len; k++) { c.ch.push(GLYPHS.charAt((rnd() * GLYPHS.length) | 0)); }
      return c;
    }
    return function (t, dt) {
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      var add = DARK ? 'lighter' : 'source-over';
      for (var i = 0; i < cols.length; i++) {
        var c = cols[i];
        c.y += c.speed * dt;
        if (c.y - c.len * STEP > BOT) { spawn(c, TOP - between(0, 12)); continue; }
        // One character in the column changes at a time, which is what a column
        // of falling glyphs does - the whole column rewriting itself every frame
        // is a column of noise with no head to follow.
        if (rnd() < dt * 7) { c.ch[(rnd() * c.len) | 0] = GLYPHS.charAt((rnd() * GLYPHS.length) | 0); }
        var k = fog(c.z, NEAR, FAR);
        var q0 = px(c.x, c.y, c.z);
        // Sized in *world* units, not in pixels-at-this-depth: a glyph is three
        // quarters of the gap between glyphs, so a column looks like a column
        // at every depth. Written as a fraction of the projected scale the
        // first time, which made a column eight units back two pixels tall.
        var size = Math.max(5, q0.s * STEP * 0.78);
        ctx.font = size.toFixed(1) + 'px ' + 'ui-monospace, Menlo, monospace';
        for (var j = 0; j < c.len; j++) {
          var y = c.y - j * STEP;
          if (y < TOP - 2 || y > BOT + 2) { continue; }
          var q = px(c.x, y, c.z);
          var down = 1 - j / c.len;
          if (j === 0) {
            // The head is the only thing in the scene lit rather than coloured,
            // which is the whole reason a column reads as falling.
            ctx.globalCompositeOperation = add;
            ctx.fillStyle = paint(mix(GLOW, [255, 255, 255], 0.5), 0.55 + 0.4 * k);
            ctx.fillText(c.ch[j], q.x, q.y);
            ctx.globalCompositeOperation = 'source-over';
            continue;
          }
          ctx.fillStyle = paint(mix(FOG, c.hue, 0.5 + 0.5 * k), down * down * (0.14 + 0.58 * k));
          ctx.fillText(c.ch[j], q.x, q.y);
        }
      }
    };
  };

  // ── rings ─────────────────────────────────────────────────────────────────
  //
  // Luminous hoops round a lit core.
  //
  // Built first as a set of concentric circles in one plane, which is a flat
  // picture whatever you do to it: nested ellipses all of the same shape read
  // as a target, and the only thing that varies between them is their size.
  // What makes a set of rings look like it is in a space is that **no two share
  // a plane** - each one tilted and turned differently, so some are nearly
  // edge-on and some nearly face-on, and they cross each other.
  //
  // Then: they precess, so the arrangement is never twice the same; half of
  // them are made of particles rather than drawn as a line, which gives a ring
  // body and keeps the set from being nine of one thing; and every one of them
  // passes behind the core, which is the cheapest statement there is that the
  // middle of this picture is in front of some of it.

  SCENES.rings = function () {
    var CZ = 26, CYY = 0.4, i, j;
    var n = Math.max(4, Math.round(9 * DENSITY)), hoops = [];
    reseed(0x8171);
    for (i = 0; i < n; i++) {
      var dusty = (i % 2) === 1;
      hoops.push({
        r: 3.1 + i * between(1.2, 1.9),
        tilt: between(-1.42, -0.12), yaw: between(-0.85, 0.85),
        prec: between(-0.05, 0.05), spin: between(0.35, 0.9) * (rnd() < 0.3 ? -1 : 1),
        at: rnd() * 6.283, col: hue(i), weight: between(0.9, 2.3),
        dusty: dusty,
        grains: dusty ? grainsFor(between(70, 150) | 0) : null
      });
    }
    function grainsFor(k) {
      var g = [];
      for (var q = 0; q < k; q++) {
        g.push({ a: rnd() * 6.283, off: between(-0.22, 0.22), b: between(0.25, 1), sz: 0.6 + Math.pow(rnd(), 2) * 2 });
      }
      return g;
    }
    var motes = [], mn = Math.max(40, Math.round(200 * DENSITY));
    for (i = 0; i < mn; i++) {
      motes.push({
        r: between(2.6, 3.1 + n * 1.6), a: rnd() * 6.283,
        tilt: between(-1.5, -0.05), yaw: between(-1.2, 1.2),
        sp: between(0.25, 0.7), b: between(0.25, 1), sz: between(0.7, 2)
      });
    }
    var tmp = [0, 0, 0];

    return function (t, dt) {
      var mid = px(0, CYY, CZ), unit = (FOCAL * H) / CZ, i;
      var add = DARK ? 'lighter' : 'source-over';

      ctx.globalCompositeOperation = add;
      var g = ctx.createRadialGradient(mid.x, mid.y, 0, mid.x, mid.y, unit * 12);
      g.addColorStop(0, paint(mix(GLOW, [255, 255, 255], 0.5), DARK ? 0.5 : 0.25));
      g.addColorStop(0.08, paint(hue(0), 0.22));
      g.addColorStop(1, paint(FOG, 0));
      ctx.fillStyle = g;
      ctx.fillRect(mid.x - unit * 12, mid.y - unit * 12, unit * 24, unit * 24);
      ctx.globalCompositeOperation = 'source-over';

      for (i = 0; i < hoops.length; i++) { hoop(hoops[i], dt, true); }
      for (i = 0; i < motes.length; i++) { mote(motes[i], dt, true); }

      // The core, between the two halves of every ring.
      ctx.globalCompositeOperation = add;
      var cg = ctx.createRadialGradient(mid.x, mid.y, 0, mid.x, mid.y, unit * 1.5);
      cg.addColorStop(0, paint([255, 255, 255], 0.95));
      cg.addColorStop(0.4, paint(GLOW, 0.65));
      cg.addColorStop(1, paint(GLOW, 0));
      ctx.fillStyle = cg;
      ctx.beginPath();
      ctx.arc(mid.x, mid.y, unit * 1.5, 0, 6.2832);
      ctx.fill();
      ctx.globalCompositeOperation = 'source-over';

      for (i = 0; i < hoops.length; i++) { hoop(hoops[i], 0, false); }
      for (i = 0; i < motes.length; i++) { mote(motes[i], 0, false); }

      function hoop(h, step, behind) {
        if (step) {
          h.at += step * h.spin / Math.pow(h.r, 0.6);
          h.yaw += step * h.prec;
        }
        var m = matOf(h.tilt, h.yaw, 0), k;
        ctx.globalCompositeOperation = DARK ? 'lighter' : 'source-over';
        if (h.dusty) {
          // A ring with body: the grains sit a little off the true circle, so
          // it has a thickness and a grittiness a stroked line cannot have.
          for (k = 0; k < h.grains.length; k++) {
            var gr = h.grains[k];
            var a = gr.a + h.at;
            xf(m, Math.cos(a) * (h.r + gr.off), 0, Math.sin(a) * (h.r + gr.off), tmp);
            var gz = tmp[2] + CZ;
            if ((gz > CZ) !== behind || gz < 1.5) { continue; }
            var gq = px(tmp[0], tmp[1] + CYY, gz);
            var gk = fog(gz, CZ - h.r * 1.2, CZ + h.r * 1.2);
            ctx.fillStyle = paint(mix(h.col, GLOW, 0.35 + 0.4 * gk),
                                  (0.12 + 0.55 * gk) * gr.b * (behind ? 0.4 : 1));
            ctx.fillRect(gq.x - gr.sz / 2, gq.y - gr.sz / 2, gr.sz, gr.sz);
          }
        } else {
          var N = 90, pts = [], zs = [];
          for (k = 0; k <= N; k++) {
            var aa = k / N * 6.2832;
            xf(m, Math.cos(aa) * h.r, 0, Math.sin(aa) * h.r, tmp);
            zs.push(tmp[2] + CZ);
            pts.push(px(tmp[0], tmp[1] + CYY, tmp[2] + CZ));
          }
          ctx.lineCap = 'round';
          for (k = 0; k < N; k++) {
            if ((zs[k] > CZ) !== behind || zs[k] < 1.5 || zs[k + 1] < 1.5) { continue; }
            var depth = fog((zs[k] + zs[k + 1]) / 2, CZ - h.r * 1.2, CZ + h.r * 1.2);
            var col = mix(h.col, GLOW, 0.3 + 0.4 * depth);
            // Wide and faint, then thin and bright: a bloom filter in three
            // lines of canvas rather than a second render target.
            ctx.lineWidth = h.weight * 5;
            ctx.strokeStyle = paint(col, (0.03 + 0.055 * depth) * (behind ? 0.5 : 1));
            seg(pts[k], pts[k + 1]);
            ctx.lineWidth = h.weight;
            ctx.strokeStyle = paint(col, (0.2 + 0.55 * depth) * (behind ? 0.45 : 1));
            seg(pts[k], pts[k + 1]);
          }
        }
        for (k = 0; k < 11; k++) {
          var ba = h.at - k * 0.05;
          xf(m, Math.cos(ba) * h.r, 0, Math.sin(ba) * h.r, tmp);
          var bz = tmp[2] + CZ;
          if ((bz > CZ) !== behind || bz < 1.5) { continue; }
          var q = px(tmp[0], tmp[1] + CYY, bz);
          var fade = 1 - k / 11, dk = fog(bz, CZ - h.r * 1.2, CZ + h.r * 1.2);
          ctx.fillStyle = paint(mix(GLOW, [255, 255, 255], 0.4),
                                fade * fade * (0.3 + 0.6 * dk) * (behind ? 0.4 : 1));
          ctx.beginPath();
          ctx.arc(q.x, q.y, Math.max(1, h.weight * 2 * fade), 0, 6.2832);
          ctx.fill();
        }
        ctx.globalCompositeOperation = 'source-over';
        function seg(a, b) {
          ctx.beginPath();
          ctx.moveTo(a.x, a.y);
          ctx.lineTo(b.x, b.y);
          ctx.stroke();
        }
      }

      function mote(d, step, behind) {
        if (step) { d.a += step * d.sp / Math.pow(d.r, 0.7); }
        xf(matOf(d.tilt, d.yaw, 0), Math.cos(d.a) * d.r, 0, Math.sin(d.a) * d.r, tmp);
        var z = tmp[2] + CZ;
        if ((z > CZ) !== behind || z < 1.5) { return; }
        var q = px(tmp[0], tmp[1] + CYY, z), k = fog(z, CZ - 16, CZ + 16);
        ctx.fillStyle = paint(mix(FOG, mix(hue(1), GLOW, 0.4), 0.5 + 0.5 * k),
                              (0.14 + 0.5 * k) * d.b * (behind ? 0.5 : 1));
        var r2 = Math.max(0.7, d.sz * (0.4 + 0.6 * k));
        ctx.fillRect(q.x - r2 / 2, q.y - r2 / 2, r2, r2);
      }
    };
  };

  // ── orrery ────────────────────────────────────────────────────────────────
  //
  // A faceted body rippling in the middle of a set of luminous hoops, with
  // debris tumbling round the whole thing.
  //
  // The body is an icosphere whose every vertex is pushed along its own
  // direction by a sum of four waves - each one twice the frequency and rather
  // less than half the amplitude of the one before. That is the whole of what
  // "fractal" means here and it is what keeps the shape from reading as a
  // balloon: one wave gives a lumpy sphere, two gives a bean, and by the fourth
  // there is detail at a scale small enough that the eye stops looking for the
  // sphere underneath. The waves run at different rates, so the form never
  // repeats and never quite settles.
  //
  // Three things carry it:
  //
  //  1. **Flat shading, recomputed.** The normals are worked out per frame from
  //     the moved vertices, so the facets catch the light as the surface rolls
  //     under them. Shading it once and animating the vertices gives a shape
  //     that changes with the lighting painted on, which reads as a bug.
  //  2. **The body is lit and the hoops are not** - the body is Lambert with no
  //     highlight, the hoops are additive with no shading. That is the whole
  //     difference between something solid and something glowing.
  //  3. **Every hoop passes behind the body.** Each is drawn in two passes, the
  //     far segments then the near ones, because a ring that is always in front
  //     is a circle painted on the picture.

  SCENES.orrery = function () {
    // Off to one side and below the title. The first version of this filled the
    // middle of the slide, which is a backdrop competing with the words rather
    // than sitting behind them.
    var CZ = 21, CXX = 1.8, CYY = 2.4, R = 2.5, i;
    reseed(0x02234);
    // Finer facets than the stone had. A fractal surface needs enough of them
    // that the smallest wave has somewhere to show: at eighty faces the top
    // octave does nothing but move the big ones about.
    var level = DENSITY >= 0.8 ? 3 : 2;
    var body = icoSphere(level);
    var rest = [];
    for (i = 0; i < body.v.length; i++) {
      rest.push([body.v[i][0], body.v[i][1], body.v[i][2]]);
    }
    var disp = new Float64Array(body.v.length);
    var meta = { face: new Float64Array(body.f.length), grain: [] };
    for (i = 0; i < body.f.length; i++) { meta.grain.push(rnd()); }

    var hoops = [];
    var hn = Math.max(3, Math.round(5 * DENSITY));
    for (i = 0; i < hn; i++) {
      hoops.push({
        r: 5.0 + i * between(1.3, 2.1),
        tilt: between(-1.35, -0.25), yaw: between(-0.6, 0.6),
        prec: between(-0.035, 0.035),
        at: rnd() * 6.283, spin: between(0.3, 0.75) * (rnd() < 0.3 ? -1 : 1),
        col: hue(2 + i), weight: between(1, 2.4)
      });
    }

    var motes = [], mn = Math.max(90, Math.round(420 * DENSITY));
    for (i = 0; i < mn; i++) {
      motes.push({
        r: between(4.2, 15), a: rnd() * 6.283,
        tilt: between(-1.5, -0.1), yaw: between(-1.2, 1.2),
        sp: between(0.25, 0.7), b: between(0.25, 1), sz: between(0.7, 2.2)
      });
    }
    var tmp = [0, 0, 0];

    return function (t, dt) {
      var i, j, spin = t * 0.08;
      var mid = px(CXX, CYY, CZ), unit = (FOCAL * H) / CZ;

      ctx.globalCompositeOperation = DARK ? 'lighter' : 'source-over';
      var g = ctx.createRadialGradient(mid.x, mid.y, unit * 1.5, mid.x, mid.y, unit * 15);
      g.addColorStop(0, paint(mix(hue(0), GLOW, 0.5), DARK ? 0.14 : 0.07));
      g.addColorStop(1, paint(FOG, 0));
      ctx.fillStyle = g;
      ctx.fillRect(mid.x - unit * 15, mid.y - unit * 15, unit * 30, unit * 30);
      ctx.globalCompositeOperation = 'source-over';

      for (i = 0; i < hoops.length; i++) { hoop(hoops[i], dt, true); }
      for (i = 0; i < motes.length; i++) { mote(motes[i], dt, true); }

      // Four waves over the rest shape, each finer and weaker than the last.
      for (i = 0; i < rest.length; i++) {
        var p = rest[i];
        var d = 1
              + 0.3 * Math.sin(p[0] * 1.9 + p[1] * 0.7 + t * 0.33)
              + 0.17 * Math.sin(p[1] * 3.4 - p[2] * 1.1 - t * 0.27)
              + 0.095 * Math.sin(p[2] * 6.1 + p[0] * 2.2 + t * 0.44)
              + 0.05 * Math.sin((p[0] + p[1] + p[2]) * 11.3 - t * 0.6);
        disp[i] = d;
        body.v[i][0] = p[0] * d;
        body.v[i][1] = p[1] * d;
        body.v[i][2] = p[2] * d;
      }
      for (i = 0; i < body.f.length; i++) {
        var f = body.f[i], sum = 0;
        for (j = 0; j < f.length; j++) { sum += disp[f[j]]; }
        meta.face[i] = sum / f.length;
      }
      pushFaces(body, matOf(t * 0.05, spin, 0), [CXX, CYY, CZ], R, { obj: meta });
      drawFaces(function (fc) {
        // Crests are one material and hollows the other, so the ripple is
        // visible in the colour as well as in the silhouette - on a body this
        // round, shading alone leaves the middle of it nearly flat.
        var d = fc.obj.face[fc.fi];
        var k = Math.min(1, Math.max(0, (d - 0.78) / 0.44));
        var col = mix(hue(0), hue(1), k * k);
        col = mix(col, scale(col, 0.88 + fc.obj.grain[fc.fi] * 0.24), 0.45);
        var sky = 0.5 - fc.ny * 0.5;
        // Capped: a lit facet of a pale body can come out brighter than the
        // heading over it, and a backdrop that is the brightest thing on the
        // slide has stopped being a backdrop.
        col = scale(col, Math.min(0.92, 0.32 + 0.44 * fc.lam + 0.28 * sky));
        // A breath of the hoops' own light where the body turns away. Not a rim
        // light - that is what made the last one look like brass - just enough
        // that the glow around it is visibly falling on something.
        col = mix(col, GLOW, Math.pow(1 - fc.lam, 4) * 0.14);
        return { fill: paint(col, 1), stroke: paint(col, 1), width: 1 };
      });

      for (i = 0; i < hoops.length; i++) { hoop(hoops[i], 0, false); }
      for (i = 0; i < motes.length; i++) { mote(motes[i], 0, false); }

      function hoop(h, step, behind) {
        if (step) {
          h.at += step * h.spin / Math.pow(h.r, 0.6);
          h.yaw += step * h.prec;
        }
        var m = matOf(h.tilt, h.yaw, 0), N = 90, k;
        var pts = [], zs = [];
        for (k = 0; k <= N; k++) {
          var a = k / N * 6.2832;
          xf(m, Math.cos(a) * h.r, 0, Math.sin(a) * h.r, tmp);
          zs.push(tmp[2] + CZ);
          pts.push(px(tmp[0] + CXX, tmp[1] + CYY, tmp[2] + CZ));
        }
        ctx.globalCompositeOperation = DARK ? 'lighter' : 'source-over';
        ctx.lineCap = 'round';
        for (k = 0; k < N; k++) {
          if ((zs[k] > CZ) !== behind) { continue; }
          if (zs[k] < 1.5 || zs[k + 1] < 1.5) { continue; }
          var depth = fog((zs[k] + zs[k + 1]) / 2, CZ - h.r * 1.2, CZ + h.r * 1.2);
          var col = mix(h.col, GLOW, 0.3 + 0.4 * depth);
          ctx.lineWidth = h.weight * 4.5;
          ctx.strokeStyle = paint(col, (0.03 + 0.05 * depth) * (behind ? 0.5 : 1));
          seg(pts[k], pts[k + 1]);
          ctx.lineWidth = h.weight;
          ctx.strokeStyle = paint(col, (0.18 + 0.5 * depth) * (behind ? 0.45 : 1));
          seg(pts[k], pts[k + 1]);
        }
        for (k = 0; k < 11; k++) {
          var aa = h.at - k * 0.05;
          xf(m, Math.cos(aa) * h.r, 0, Math.sin(aa) * h.r, tmp);
          var bz = tmp[2] + CZ;
          if ((bz > CZ) !== behind || bz < 1.5) { continue; }
          var q = px(tmp[0] + CXX, tmp[1] + CYY, bz);
          var fade = 1 - k / 11, dk = fog(bz, CZ - h.r * 1.2, CZ + h.r * 1.2);
          ctx.fillStyle = paint(mix(GLOW, [255, 255, 255], 0.4),
                                fade * fade * (0.3 + 0.6 * dk) * (behind ? 0.4 : 1));
          ctx.beginPath();
          ctx.arc(q.x, q.y, Math.max(1, h.weight * 2 * fade), 0, 6.2832);
          ctx.fill();
        }
        ctx.globalCompositeOperation = 'source-over';
        function seg(a, b) {
          ctx.beginPath();
          ctx.moveTo(a.x, a.y);
          ctx.lineTo(b.x, b.y);
          ctx.stroke();
        }
      }

      function mote(d, step, behind) {
        if (step) { d.a += step * d.sp / Math.pow(d.r, 0.7); }
        xf(matOf(d.tilt, d.yaw, 0), Math.cos(d.a) * d.r, 0, Math.sin(d.a) * d.r, tmp);
        var z = tmp[2] + CZ;
        if ((z > CZ) !== behind || z < 1.5) { return; }
        var q = px(tmp[0] + CXX, tmp[1] + CYY, z), k = fog(z, CZ - 16, CZ + 16);
        ctx.fillStyle = paint(mix(FOG, mix(hue(3), GLOW, 0.4), 0.45 + 0.55 * k),
                              (0.12 + 0.55 * k) * d.b * (behind ? 0.5 : 1));
        var r2 = Math.max(0.7, d.sz * (0.4 + 0.6 * k));
        ctx.fillRect(q.x - r2 / 2, q.y - r2 / 2, r2, r2);
      }
    };
  };

  // ── vortex ────────────────────────────────────────────────────────────────
  //
  // Hairline filaments twisted around an axis, pinched at the waist and flaring
  // into a crown above and roots below, over a drifting star field.
  //
  // The waist is a catenary - `cosh` - rather than a cone or a parabola, which
  // is not fussiness: a cone flares in a straight line and reads as a funnel,
  // and the shape the eye recognises here is the one a hanging chain makes,
  // tight through the middle and opening fast at both ends.

  SCENES.vortex = function () {
    var CZ = 27.5, Y0 = -9.6, Y1 = 9.6, WAIST = 2.1, A = 5.4, TWIST = 0.26;
    var n = Math.max(12, Math.round(46 * DENSITY)), fils = [], i;
    reseed(0xE7B);
    for (i = 0; i < n; i++) {
      fils.push({
        th: rnd() * 6.283, col: hue(i), lean: between(-0.1, 0.1), sway: rnd() * 6.283,
        at: rnd() * 12, period: between(5, 13), up: rnd() < 0.45
      });
    }
    var stars = [], sn = Math.max(40, Math.round(170 * DENSITY));
    for (i = 0; i < sn; i++) {
      stars.push({ x: between(-26, 26), y: between(-16, 16), z: between(26, 54), b: between(0.3, 1) });
    }
    return function (t, dt) {
      var i, j;
      for (i = 0; i < stars.length; i++) {
        var s = stars[i];
        s.x -= dt * 0.22;
        if (s.x < -26) { s.x = 26; }
        var sq = px(s.x, s.y, s.z), sk = fog(s.z, 26, 54);
        ctx.fillStyle = paint(mix(FOG, GLOW, 0.3 + 0.7 * sk), 0.1 + 0.4 * sk * s.b);
        var sr = Math.max(0.6, 1.3 * sk * s.b);
        ctx.fillRect(sq.x - sr, sq.y - sr, sr * 2, sr * 2);
      }
      ctx.globalCompositeOperation = DARK ? 'lighter' : 'source-over';
      ctx.lineWidth = 1;
      ctx.lineCap = 'round';
      var spin = t * 0.17;
      for (i = 0; i < fils.length; i++) {
        var f = fils[i], N = 30, prev = null, pk = 0;
        // A pulse runs the length of each filament and then waits. The waiting
        // is most of the cycle on purpose: twenty threads all lit at once is a
        // flicker, and what reads as something *travelling* is one thread
        // lighting up while its neighbours are dark.
        f.at += dt;
        var cyc = (f.at % f.period) / f.period;
        var pulse = cyc < 0.45 ? (f.up ? 1 - cyc / 0.45 : cyc / 0.45) : -1;
        for (j = 0; j <= N; j++) {
          var u = j / N, y = Y0 + (Y1 - Y0) * u;
          var rad = WAIST * Math.cosh(y / A);
          // The whole column breathes, and each filament a little out of step
          // with its neighbours - a vortex of threads turning in lockstep is a
          // wire sculpture.
          rad *= 1 + 0.07 * Math.sin(t * 0.5 + f.sway + u * 3);
          var th = f.th + TWIST * y + spin + f.lean * y;
          var z = Math.sin(th) * rad + CZ;
          if (z < 1.2) { prev = null; continue; }
          var q = px(Math.cos(th) * rad, y, z);
          var k = fog(z, CZ - 16, CZ + 16);
          if (prev) {
            // Brighter where the filament is tightest, which is where they all
            // cross - the waist is the point of the shape and has to be the
            // brightest thing in it.
            var tight = 1 - Math.min(1, (rad - WAIST) / 10);
            var lit = pulse < 0 ? 0 : Math.max(0, 1 - Math.abs(u - pulse) / 0.16);
            lit = lit * lit;
            ctx.strokeStyle = paint(mix(mix(f.col, GLOW, 0.35 + 0.4 * tight), [255, 255, 255], lit * 0.7),
                                    (0.05 + 0.3 * tight + 0.55 * lit) * (0.3 + 0.7 * ((k + pk) / 2)));
            ctx.lineWidth = 1 + lit * 1.6;
            ctx.beginPath();
            ctx.moveTo(prev.x, prev.y);
            ctx.lineTo(q.x, q.y);
            ctx.stroke();
          }
          prev = q;
          pk = k;
        }
      }
      ctx.globalCompositeOperation = 'source-over';
    };
  };

  // ── bloom ─────────────────────────────────────────────────────────────────
  //
  // A flower unfurling from a closed bud into an open head and shutting again.
  //
  // Phyllotaxis - a golden angle between neighbours, radius as the square root
  // of the index - is how a real seed head is arranged, and on its own it is
  // *too* regular to look alive: the parastichy spirals line up into a visible
  // lattice, which is lovely in a sunflower photographed up close and reads as
  // graph paper at this size. Four things break it without losing the growth:
  //
  //  1. **Petals.** The radius is modulated by a lobe count, so the head has an
  //     edge that goes in and out rather than a circular rim.
  //  2. **A wave travelling outward**, so the surface is never still even at
  //     the moment the opening pauses. This is most of what makes it *flow*.
  //  3. **Jitter per point**, in angle and radius both, enough to stop the
  //     spirals and not enough to stop the arrangement.
  //  4. **Pollen.** A few hundred motes drifting off the head, which is the one
  //     thing in the picture not attached to the geometry.

  SCENES.bloom = function () {
    var GOLD = 2.39996323, CZ = 21.5, CYY = 1.2, TILT = -0.62;
    var n = Math.max(500, Math.round(2600 * DENSITY)), R = 9.4, LOBES = 7;
    var seeds = [], i;
    reseed(0xB100);
    for (i = 0; i < n; i++) {
      var u = i / n;
      seeds.push({
        u: u,
        a: i * GOLD + between(-0.1, 0.1),
        j: between(0.9, 1.1),
        b: between(0.55, 1),
        sz: between(0.7, 1.5)
      });
    }
    var motes = [], mn = Math.max(40, Math.round(220 * DENSITY));
    for (i = 0; i < mn; i++) {
      motes.push({
        r: between(2, 15), a: rnd() * 6.283, y: between(-7, 5),
        sp: between(0.05, 0.22), bob: rnd() * 6.283, b: between(0.3, 1)
      });
    }
    var stars = [], sn = Math.max(30, Math.round(120 * DENSITY));
    for (i = 0; i < sn; i++) {
      stars.push({ x: between(-26, 26), y: between(-16, 16), z: between(28, 56), b: between(0.3, 1) });
    }
    var tmp = [0, 0, 0];
    return function (t, dt) {
      var i, p;
      for (i = 0; i < stars.length; i++) {
        var st = stars[i];
        var sq = px(st.x, st.y, st.z), sk = fog(st.z, 28, 56);
        ctx.fillStyle = paint(mix(FOG, GLOW, 0.3 + 0.7 * sk), 0.08 + 0.3 * sk * st.b);
        ctx.fillRect(sq.x - 1, sq.y - 1, 2, 2);
      }
      // Slow, and never quite shut: a bud that closes completely leaves the
      // slide empty for ten seconds at a time.
      var open = 0.3 + 0.7 * (0.5 + 0.5 * Math.sin(t * 0.085));
      var m = matOf(TILT, t * 0.06, 0);
      ctx.globalCompositeOperation = DARK ? 'lighter' : 'source-over';
      for (i = 0; i < seeds.length; i++) {
        var s = seeds[i], u = s.u;
        // The lobes, and a ripple running out from the middle. Both are small -
        // a fifth of the radius between them - and together they are the whole
        // difference between a flower and a dartboard.
        var lobe = 1 + 0.17 * Math.cos(LOBES * s.a + t * 0.11);
        var wave = 1 + 0.06 * Math.sin(u * 13 - t * 0.9);
        var rad = R * Math.sqrt(u) * (0.22 + 0.78 * open) * lobe * wave * s.j;
        var a = s.a + t * 0.05 + (1 - open) * u * 1.6;    // the head twists as it shuts
        // Closed, the rim lifts into a bud; open, it lies back, and the heart
        // still stands proud of it. The ripple reaches the height too, or the
        // surface would flex in plan and stay flat in section.
        var y = -(1 - open) * 7.5 * u * u - 1.1 * (1 - u) * (1 - u)
              - 0.55 * Math.sin(u * 11 - t * 0.9) * u;
        xf(m, Math.cos(a) * rad, y, Math.sin(a) * rad, tmp);
        var z = tmp[2] + CZ;
        if (z < 1.5) { continue; }
        var q = px(tmp[0], tmp[1] + CYY, z);
        var col = u < 0.3 ? mix(mix(hue(1), [255, 255, 255], 0.75), hue(1), u / 0.3)
                          : mix(hue(1), hue(0), (u - 0.3) / 0.7);
        var k = fog(z, CZ - 10, CZ + 10);
        ctx.fillStyle = paint(col, (0.2 + 0.5 * k) * (1 - 0.3 * u) * s.b);
        var rr = Math.max(0.9, (0.055 - 0.03 * u) * q.s * s.sz);
        ctx.beginPath();
        ctx.arc(q.x, q.y, rr, 0, 6.2832);
        ctx.fill();
      }
      // Pollen, drifting off it.
      for (i = 0; i < motes.length; i++) {
        var d = motes[i];
        d.a += dt * d.sp;
        d.y -= dt * 0.25;
        if (d.y < -9) { d.y = 6; }
        var yy = d.y + Math.sin(t * 0.5 + d.bob) * 0.4;
        xf(m, Math.cos(d.a) * d.r, yy, Math.sin(d.a) * d.r, tmp);
        var dz = tmp[2] + CZ;
        if (dz < 1.5) { continue; }
        var dq = px(tmp[0], tmp[1] + CYY, dz), dk = fog(dz, CZ - 12, CZ + 12);
        ctx.fillStyle = paint(mix(hue(1), GLOW, 0.5), (0.1 + 0.4 * dk) * d.b);
        ctx.fillRect(dq.x - 1, dq.y - 1, 2, 2);
      }
      ctx.globalCompositeOperation = 'source-over';
    };
  };

  // ── plume ─────────────────────────────────────────────────────────────────
  //
  // A landscape of ridges running away to a horizon, seen from just above it,
  // with a column of smoke rising behind and a band of light to cut them all
  // against.
  //
  // Drawn **through the camera** rather than in screen space, which is the
  // whole difference between this and the layered-bands version it replaces.
  // Each ridge is a real line of terrain at its own distance, so perspective
  // does the work: the near ones are wide and far apart, the far ones compress
  // towards the horizon, and the ridges sample the *same* height field at
  // different depths, which is what makes them read as one piece of country
  // rather than as six cut-outs that happen to be stacked.
  //
  // Enough of them, and that is a contour map: a set of lines across one
  // surface, crowding where the ground is steep. The camera is lifted a little
  // so the horizon sits below the middle of the slide and you are looking down
  // into the valleys instead of along them - which is also what keeps the
  // whole picture in the lower half, where a title is not.

  SCENES.plume = function () {
    var N = Math.max(8, Math.round(17 * DENSITY));   // ridges
    var Z0 = 7, Z1 = 70, BASE = 3.4, LIFT = 0.1;
    var ridges = [], i;
    reseed(0x9100);
    for (i = 0; i < N; i++) { ridges.push({ u: i / (N - 1 || 1), drift: between(0.9, 1.1) }); }
    var puffs = [], pn = Math.max(14, Math.round(34 * DENSITY));
    for (i = 0; i < pn; i++) {
      puffs.push({ at: rnd(), lean: between(-0.5, 0.5), wob: rnd() * 6.283, br: between(0.5, 1) });
    }
    // One height field, sampled by every ridge at its own z. Three octaves,
    // which is as few as will give a range a peak, a shoulder and a crag.
    function height(x, z) {
      return 2.3 * Math.sin(x * 0.085 + z * 0.021)
           + 1.15 * Math.sin(x * 0.19 - z * 0.05 + 1.7)
           + 0.5 * Math.sin(x * 0.42 + z * 0.11 + 0.6)
           + 0.22 * Math.sin(x * 0.95 - z * 0.3);
    }
    return function (t, dt) {
      var i, j;
      // The camera is tilted up by shifting the horizon down the page: the
      // vanishing point of this projection is the middle of the canvas, and
      // moving the whole scene down is the same picture as pitching the camera.
      var SHIFT = H * LIFT;

      // And it wanders. Two sines per axis at frequencies with no common
      // multiple, which never repeats inside any talk and is still the same
      // wander every time the deck is opened - a random walk would be neither:
      // it drifts away from where it started and it is different at every
      // rehearsal.
      //
      // The pan is applied to *where the terrain is sampled*, not to the
      // projection, which is what makes it a camera move rather than a slide:
      // shifting the sample point by a fixed number of world units moves the
      // near ridges further across the screen than the far ones, because world
      // units are bigger on screen close up. That difference is the parallax,
      // and it is the whole reason this reads as movement through a landscape
      // instead of a picture being dragged sideways.
      var panX = 16 * Math.sin(t * 0.021) + 9 * Math.sin(t * 0.0133 + 1.7);
      var panY = 0.42 * Math.sin(t * 0.017 + 0.6) + 0.26 * Math.sin(t * 0.0091 + 2.4);
      var eye = BASE + panY;
      var HX = W * 0.66, HY = H * 0.5 + SHIFT;

      var sky = ctx.createRadialGradient(HX, HY, 0, HX, HY, H * 0.8);
      sky.addColorStop(0, paint(mix(FOG, GLOW, 0.5), DARK ? 0.22 : 0.12));
      sky.addColorStop(0.45, paint(mix(FOG, hue(1), 0.5), 0.1));
      sky.addColorStop(1, paint(FOG, 0));
      ctx.globalCompositeOperation = DARK ? 'lighter' : 'source-over';
      ctx.fillStyle = sky;
      ctx.fillRect(0, 0, W, H);

      for (i = 0; i < puffs.length; i++) {
        var p = puffs[i];
        p.at += dt * 0.035;
        if (p.at > 1) { p.at -= 1; }
        var rise = p.at;
        var PX = HX + W * (p.lean * rise * 0.26 + 0.02 * Math.sin(t * 0.3 + p.wob));
        var PY = HY - H * rise * 0.52;
        var rr = H * (0.035 + rise * 0.3);
        var a = (DARK ? 0.2 : 0.1) * p.br * Math.min(1, rise * 6) * (1 - rise) * (1 - rise);
        var pg = ctx.createRadialGradient(PX, PY, 0, PX, PY, rr);
        pg.addColorStop(0, paint(mix(hue(1), GLOW, 0.45), a));
        pg.addColorStop(1, paint(FOG, 0));
        ctx.fillStyle = pg;
        ctx.fillRect(PX - rr, PY - rr, rr * 2, rr * 2);
      }
      ctx.globalCompositeOperation = 'source-over';

      // Far ridge first, so each one is cut out against the one behind it.
      for (i = N - 1; i >= 0; i--) {
        var r = ridges[i], u = r.u;
        // Quadratic in u, so the ridges crowd towards the horizon the way the
        // contours of a real range do when you are standing in it.
        var z = Z0 + (Z1 - Z0) * u * u;
        var drift = t * 0.55 * r.drift;
        var col = mix(FOG, mix(hue(1), hue(0), 1 - u), 0.74 - 0.62 * (1 - u));
        // How wide this ridge has to be *in world units* to reach both edges of
        // the screen at its own depth. A fixed width works for the near ones and
        // leaves the far ones short of the frame - and a silhouette that stops
        // short closes itself with a diagonal to the corner, which is the notch
        // the first version of this had cut out of its horizon.
        var half = (W / 2) / ((FOCAL * H) / z) + 1.5;
        var step = half / 90;
        ctx.beginPath();
        var first = null;
        for (j = -half; j <= half; j += step) {
          var h = height(j + panX, z + drift);
          var q = px(j, eye - h, z);
          q.y += SHIFT;
          if (!first) { first = q; ctx.moveTo(q.x, q.y); } else { ctx.lineTo(q.x, q.y); }
        }
        // Closed down to the bottom of the page so the ridge is a silhouette
        // and not a wire: everything in front of the horizon is solid.
        ctx.lineTo(W + 10, H + 10);
        ctx.lineTo(-10, H + 10);
        ctx.closePath();
        ctx.fillStyle = paint(col, 1);
        ctx.fill();
        // The lit crest. Brightest on the far ridges, which are the ones
        // standing against the light.
        ctx.strokeStyle = paint(mix(col, GLOW, 0.25 + 0.4 * u), 0.3 + 0.45 * u);
        ctx.lineWidth = 1.2;
        ctx.beginPath();
        var started = false;
        for (j = -half; j <= half; j += step) {
          var hh = height(j + panX, z + drift);
          var qq = px(j, eye - hh, z);
          qq.y += SHIFT;
          if (!started) { started = true; ctx.moveTo(qq.x, qq.y); } else { ctx.lineTo(qq.x, qq.y); }
        }
        ctx.stroke();
      }
    };
  };

  // ── dust ──────────────────────────────────────────────────────────────────
  //
  // A planet made of nothing but dust, turning, with a ring of debris round it.
  //
  // The grains start on a Fibonacci sphere - one golden angle round and a step
  // down in height per grain - because that is the only arrangement that is
  // even all over: a latitude/longitude lattice crowds its poles and random
  // points clump. But *even* is not the same as *natural*, and a perfectly
  // even shell reads as a mesh, with rows in it the eye finds immediately. So
  // every grain is then pushed off its place - round, up, and in or out of the
  // shell - by enough to destroy the lattice and not enough to lose the sphere.
  //
  // Sizes are drawn from a steep curve rather than a range, so almost every
  // grain is a single pixel and a few are three. That ratio is what dust looks
  // like; a uniform range looks like gravel.

  SCENES.dust = function () {
    var R = 8.4, CXX = 4.4, CYY = 0.8, CZ = 27, TILT = -0.42;
    var n = Math.max(500, Math.round(3400 * DENSITY)), pts = [], i;
    reseed(0xD057);
    for (i = 0; i < n; i++) {
      var y = 1 - (i / (n - 1)) * 2;
      var rr = Math.sqrt(Math.max(0, 1 - y * y));
      var a = i * 2.39996323 + between(-0.42, 0.42);
      y += between(-0.012, 0.012);
      rr = Math.sqrt(Math.max(0, 1 - y * y));
      var v = [Math.cos(a) * rr, y, Math.sin(a) * rr];
      // A shell with thickness. Flat on the surface, the grains on the limb
      // make a hard circle; given a little depth they fade into one.
      var shell = 1 + between(-0.045, 0.055);
      // Sizes from a steep curve, but with a floor of one device pixel.
      // Below that a fillRect is not a smaller mark, it is the same mark at a
      // lower alpha - so a distribution running down to a third of a pixel does
      // not give fine dust, it gives dust that is not there.
      pts.push([v[0] * shell, v[1] * shell, v[2] * shell,
                1 + Math.pow(rnd(), 3) * 2.8, 0.45 + rnd() * 0.55]);
    }
    var ring = [], rn = Math.max(120, Math.round(700 * DENSITY));
    for (i = 0; i < rn; i++) {
      ring.push({
        r: between(1.22, 1.82), a: rnd() * 6.283,
        y: between(-0.05, 0.05) * between(0.2, 1),
        b: between(0.3, 1), sz: 1 + Math.pow(rnd(), 2.6) * 2
      });
    }
    var tmp = [0, 0, 0];
    return function (t, dt) {
      var m = matOf(TILT, t * 0.07, 0), i, p;
      // The body it is made of. Thousands of separate grains never add up to a
      // mass on their own - each one is a single pixel and the eye reads the
      // gaps - so the planet is a glow with the grains on top of it.
      var mid = px(CXX, CYY, CZ), rad = R * (FOCAL * H) / CZ;
      ctx.globalCompositeOperation = DARK ? 'lighter' : 'source-over';
      var bg = ctx.createRadialGradient(mid.x, mid.y, 0, mid.x, mid.y, rad * 1.1);
      bg.addColorStop(0, paint(mix(hue(0), hue(1), 0.4), DARK ? 0.14 : 0.08));
      bg.addColorStop(0.7, paint(hue(0), 0.07));
      bg.addColorStop(1, paint(FOG, 0));
      ctx.fillStyle = bg;
      ctx.beginPath();
      ctx.arc(mid.x, mid.y, rad * 1.1, 0, 6.2832);
      ctx.fill();
      // Additive, so where the grains crowd - the limb, and the near face - they
      // build into something bright rather than painting over each other.
      for (i = 0; i < pts.length; i++) {
        var g = pts[i];
        p = xf(m, g[0], g[1], g[2], tmp);
        var face = -p[2];
        var front = face > 0;
        var q = px(p[0] * R + CXX, p[1] * R + CYY, p[2] * R + CZ);
        var col = mix(hue(0), hue(1), (p[1] + 1) / 2);
        ctx.fillStyle = paint(mix(FOG, col, 0.65 + 0.35 * Math.abs(face)),
                              (front ? 0.3 + 0.6 * face : 0.14 * (1 + face)) * g[4]);
        var sz = Math.max(1, g[3] * (front ? 0.55 + 0.45 * face : 0.5));
        ctx.fillRect(q.x - sz / 2, q.y - sz / 2, sz, sz);
      }
      for (i = 0; i < ring.length; i++) {
        var d = ring[i];
        d.a += dt * 0.26 / Math.pow(d.r, 1.2);
        p = xf(m, Math.cos(d.a) * d.r, d.y, Math.sin(d.a) * d.r, tmp);
        var rq = px(p[0] * R + CXX, p[1] * R + CYY, p[2] * R + CZ);
        var rk = 0.5 - p[2] * 0.5;
        ctx.fillStyle = paint(GLOW, (0.22 + 0.55 * rk) * d.b);
        var rs = Math.max(1, d.sz);
        ctx.fillRect(rq.x - rs / 2, rq.y - rs / 2, rs, rs);
      }
      ctx.globalCompositeOperation = 'source-over';
    };
  };

  // Names people will actually write. A scene has one spelling in the theme
  // files that ship and every other name somebody would guess, because a
  // backdrop that silently does nothing is indistinguishable from one that is
  // broken.
  SCENES.network = SCENES.globe;
  SCENES.world = SCENES.globe;
  SCENES.prism = SCENES.crystal;
  SCENES.crystalline = SCENES.crystal;
  SCENES.wire = SCENES.wireframe;
  SCENES.lattice = SCENES.wireframe;
  SCENES.terrain = SCENES.lowpoly;
  SCENES.field = SCENES.lowpoly;
  SCENES.blocks = SCENES.cubes;

  // ── net ───────────────────────────────────────────────────────────────────
  //
  // A lattice of points with a line between any two that are near enough, and
  // light running along some of those lines. The shape of it is Vanta's NET,
  // which is the one of these that everybody recognises - and the reason it
  // reads as a network rather than as a scatter is the *threshold*: a line only
  // exists while its ends are close, so the mesh makes and breaks itself as the
  // points drift and the eye reads that as connection rather than as decoration.
  //
  // The pairs are found once, from a grid the points never leave by more than a
  // cell. Testing every pair every frame is 400 points squared, sixty times a
  // second, for a picture nobody is looking at directly.

  SCENES.net = function () {
    var NEAR = 7, FAR = 44, REACH = 6.4;
    var n = Math.max(40, Math.round(190 * DENSITY));
    var pts = [], links = [], i, j;
    reseed(0x4E7);
    for (i = 0; i < n; i++) {
      pts.push({
        hx: between(-19, 19), hy: between(-13, 13), hz: between(NEAR + 2, FAR),
        ph: rnd() * 6.283, sp: between(0.25, 0.75), amp: between(0.35, 1.3),
        x: 0, y: 0, z: 0, q: null
      });
    }
    for (i = 0; i < n; i++) {
      for (j = i + 1; j < n; j++) {
        var dx = pts[i].hx - pts[j].hx, dy = pts[i].hy - pts[j].hy, dz = pts[i].hz - pts[j].hz;
        var d = Math.sqrt(dx * dx + dy * dy + dz * dz);
        if (d < REACH) { links.push({ a: i, b: j, d: d }); }
      }
    }
    // A run of light every so often, on a link chosen once rather than at
    // random each time: a pulse that jumped about would be a string of
    // unrelated flashes, and what is wanted is traffic going somewhere.
    var runs = [];
    var howMany = Math.min(links.length, Math.max(4, Math.round(14 * DENSITY)));
    for (i = 0; i < howMany; i++) {
      runs.push({ link: (rnd() * links.length) | 0, at: rnd() * 8, period: between(3.5, 9), dir: rnd() < 0.5 });
    }
    return function (t, dt) {
      var i, p;
      for (i = 0; i < pts.length; i++) {
        p = pts[i];
        // Each point breathes around where it belongs. Drifting freely, the
        // lattice would have come apart within a minute of the talk starting.
        p.x = p.hx + Math.sin(t * p.sp + p.ph) * p.amp;
        p.y = p.hy + Math.cos(t * p.sp * 0.8 + p.ph * 1.7) * p.amp * 0.8;
        p.z = p.hz + Math.sin(t * p.sp * 0.6 + p.ph * 0.5) * p.amp;
        p.q = p.z > 1 ? px(p.x, p.y, p.z) : null;
      }
      ctx.lineWidth = 1;
      ctx.lineCap = 'round';
      for (i = 0; i < links.length; i++) {
        var a = pts[links[i].a], b = pts[links[i].b];
        if (!a.q || !b.q) { continue; }
        var z = (a.z + b.z) / 2, k = fog(z, NEAR, FAR);
        // Fading with length as well as with depth. A line at exactly the
        // threshold would otherwise blink on at full strength as two points
        // drifted together, which is the one thing in this that would catch the
        // eye of somebody trying to read the slide.
        var reach = 1 - links[i].d / REACH;
        ctx.strokeStyle = paint(mix(FOG, hue(0), 0.55 + 0.45 * k), 0.12 + 0.7 * k * reach);
        ctx.beginPath();
        ctx.moveTo(a.q.x, a.q.y);
        ctx.lineTo(b.q.x, b.q.y);
        ctx.stroke();
      }
      for (i = 0; i < pts.length; i++) {
        p = pts[i];
        if (!p.q) { continue; }
        var kk = fog(p.z, NEAR, FAR);
        var r = Math.max(1, p.q.s * 0.009);
        ctx.fillStyle = paint(mix(FOG, hue(1), 0.55 + 0.45 * kk), 0.35 + 0.6 * kk);
        ctx.beginPath();
        ctx.arc(p.q.x, p.q.y, r, 0, 6.2832);
        ctx.fill();
      }
      var add = DARK ? 'lighter' : 'source-over';
      ctx.globalCompositeOperation = add;
      for (i = 0; i < runs.length; i++) {
        var rr = runs[i];
        rr.at += dt;
        var u = (rr.at % rr.period) / rr.period;
        if (u > 0.55) { continue; }           // most of the cycle is the gap between runs
        u = u / 0.55;
        if (rr.dir) { u = 1 - u; }
        var l = links[rr.link], pa = pts[l.a], pb = pts[l.b];
        if (!pa.q || !pb.q) { continue; }
        var hx = pa.x + (pb.x - pa.x) * u, hy = pa.y + (pb.y - pa.y) * u, hz = pa.z + (pb.z - pa.z) * u;
        if (hz < 1) { continue; }
        var hq = px(hx, hy, hz), f = fog(hz, NEAR, FAR);
        // The tail trails *behind* the head, so which side of u it is on is
        // which way the run is going. Three stops, written in ascending order
        // because a gradient read out of order is a gradient to debug.
        var grad = ctx.createLinearGradient(pa.q.x, pa.q.y, pb.q.x, pb.q.y);
        var lit = paint(GLOW, 0.6 * f), out = paint(GLOW, 0);
        if (rr.dir) {
          grad.addColorStop(Math.max(0, u - 0.004), out);
          grad.addColorStop(u, lit);
          grad.addColorStop(Math.min(1, u + 0.28), out);
        } else {
          grad.addColorStop(Math.max(0, u - 0.28), out);
          grad.addColorStop(u, lit);
          grad.addColorStop(Math.min(1, u + 0.004), out);
        }
        ctx.strokeStyle = grad;
        ctx.lineWidth = 1.8;
        ctx.beginPath();
        ctx.moveTo(pa.q.x, pa.q.y);
        ctx.lineTo(pb.q.x, pb.q.y);
        ctx.stroke();
        ctx.fillStyle = paint(GLOW, 0.75 * f);
        ctx.beginPath();
        ctx.arc(hq.x, hq.y, 1.7, 0, 6.2832);
        ctx.fill();
      }
      ctx.globalCompositeOperation = 'source-over';
    };
  };
  SCENES.nodes = SCENES.net;
  SCENES.rain = SCENES.matrix;
  SCENES.glyphs = SCENES.matrix;
  SCENES.swell = SCENES.waves;
  SCENES.ocean = SCENES.waves;
  SCENES.haze = SCENES.fog;
  SCENES.mist = SCENES.fog;
  SCENES.contours = SCENES.topology;
  SCENES.erosion = SCENES.topology;
  SCENES.orbits = SCENES.rings;
  SCENES.system = SCENES.orrery;
  SCENES.filaments = SCENES.vortex;
  SCENES.flower = SCENES.bloom;
  SCENES.ridges = SCENES.plume;
  SCENES.grains = SCENES.dust;

  // ── The canvas, and the clock ─────────────────────────────────────────────

  var canvas = null, scene = null, draw = null;
  var STILL = false, FPS = 34, last = 0, lastDraw = 0, clock = 0, raf = 0;

  function sizeTo() {
    var w = window.innerWidth || 1280, h = window.innerHeight || 720;
    // Capped twice: by density, because a backdrop on a 4k panel is four times
    // the work for nothing anybody can see, and by total pixels, because the
    // machine this is on is also running the talk.
    DPR = Math.min(window.devicePixelRatio || 1, 1.5);
    var total = w * h * DPR * DPR;
    if (total > 2.8e6) { DPR = Math.max(0.75, DPR * Math.sqrt(2.8e6 / total)); }
    W = Math.round(w * DPR);
    H = Math.round(h * DPR);
    canvas.width = W;
    canvas.height = H;
    canvas.style.width = w + 'px';
    canvas.style.height = h + 'px';
    cx = W / 2;
    cy = H / 2;
  }

  function paintFrame(dt) {
    // Cleared to the fog rather than to nothing: a transparent canvas lets the
    // deck's own ground through, which is right, but then every scene would
    // have to paint its own distance and three of them would get it wrong. The
    // fog *is* the ground by default, so this is the same picture either way
    // and the near-to-far fade has something to fade into.
    ctx.globalCompositeOperation = 'source-over';
    ctx.clearRect(0, 0, W, H);
    draw(clock, dt);
    // The veil last: a theme that wants its scene quieter says so once here
    // rather than in every colour it chose.
    if (VEIL && VEIL !== 'none') {
      ctx.fillStyle = VEIL;
      ctx.fillRect(0, 0, W, H);
    }
  }

  function frame(stamp) {
    raf = 0;
    if (!last) { last = stamp; }
    var dt = (stamp - last) / 1000;
    last = stamp;
    // A frame after a long gap - a laptop lid, a tab in the background - would
    // otherwise be one enormous step and the whole field would jump.
    if (dt > 0.25) { dt = 0.25; }
    if (stamp - lastDraw >= 1000 / FPS - 1) {
      lastDraw = stamp;
      clock += dt * SPEED;
      paintFrame(dt * SPEED);
    }
    schedule();
  }

  function now() {
    return (window.performance && window.performance.now) ? window.performance.now() : Date.now();
  }

  function schedule() {
    if (raf || STILL || document.hidden) { return; }
    raf = requestAnimationFrame(frame);
  }

  // The second clock, and the reason there are two.
  //
  // requestAnimationFrame is the right one to draw on and is also the one the
  // browser will quietly take away. It stops for a background tab, which is
  // intended and is most of why this loop is cheap - but it *also* stops when
  // the window manager decides the window is occluded, when the page is in an
  // iframe the browser is not painting, on a second display that has gone to
  // sleep, and under a handful of remote-desktop and screen-sharing setups. All
  // of those are states a deck is genuinely being looked at in, and in every
  // one of them the backdrop would paint its first frame and then sit there -
  // which does not look like a throttled animation, it looks like a still
  // image, and there is nothing on screen to say otherwise.
  //
  // So a plain timer runs alongside, and whichever of the two gets there first
  // paints. The frame-rate gate in `frame` is what stops them drawing twice: if
  // rAF is running, every beat finds the last frame too recent and does
  // nothing, which costs a comparison thirty times a second. If rAF is not
  // running, the timer is the clock and the scene carries on at the same speed.
  // The one state both agree to skip is a genuinely hidden document, where
  // nobody can see either of them.
  function beat() {
    if (STILL || document.hidden) { return; }
    var t = now();
    if (t - lastDraw >= 1000 / FPS - 1) { frame(t); }
  }

  function start() {
    if (!document.body) { return; }
    canvas = document.createElement('canvas');
    canvas.className = 'org-backdrop';
    // It is scenery. A reader on a screen reader has no use for it and a mouse
    // has no business reaching it - the slide is underneath.
    canvas.setAttribute('aria-hidden', 'true');
    ctx = canvas.getContext && canvas.getContext('2d');
    if (!ctx) { return; }
    readTheme();
    document.body.insertBefore(canvas, document.body.firstChild);
    document.documentElement.classList.add('org-has-backdrop');
    sizeTo();
    scene = SCENES[String(CFG.scene || '').toLowerCase()];
    if (!scene) {
      // Said out loud: a name this build does not have is a typo in somebody's
      // theme, and a backdrop that silently does nothing looks like a bug in
      // the exporter rather than one character in a css comment.
      if (window.console) { console.warn('orgs backdrop: no scene called "' + CFG.scene + '"'); }
      canvas.parentNode.removeChild(canvas);
      document.documentElement.classList.remove('org-has-backdrop');
      return;
    }
    draw = scene();

    STILL = false;
    try { STILL = window.matchMedia('(prefers-reduced-motion: reduce)').matches; } catch (e) { }
    // "Reduce motion" is a system-wide setting, and somebody who turned it on
    // for their desktop has not necessarily decided anything about the deck
    // they are presenting - so a deck can say otherwise, in as many words.
    // `#+SLIDE_BACKDROP_MOTION: always`, or `--backdrop-motion: always` from a
    // theme that is built around its scene moving.
    var motion = String(CFG.motion || cssVar('--backdrop-motion', '')).toLowerCase();
    if (motion === 'always') { STILL = false; }
    if (motion === 'never') { STILL = true; }
    if (STILL || SPEED === 0) {
      // One frame, some way into the scene so it is not the arrangement
      // everything started in, and then nothing. Still a designed picture
      // behind the deck rather than a blank.
      clock = 9;
      paintFrame(0);
      // Worth being able to ask. "It is not animating" has three answers -
      // reduced motion, a speed of zero, and a scene name this build does not
      // have - and from the outside they look identical.
      CFG.state = SPEED === 0 ? 'still: speed is 0' : 'still: prefers-reduced-motion';
      if (window.console) {
        console.info('orgs backdrop: ' + CFG.state +
          ' (set #+SLIDE_BACKDROP_MOTION: always to overrule it)');
      }
      window.addEventListener('resize', function () { sizeTo(); paintFrame(0); });
      return;
    }

    // One frame now, rather than at the first animation frame. A tab that is
    // not in front is handed no animation frame at all, so a deck opened in a
    // background tab - which is how every deck is opened, from a link - would
    // otherwise be shown its ground with nothing on it until it came forward.
    paintFrame(0);

    // Repaint immediately, not at the next animation frame. Setting
    // `canvas.width` *clears* the canvas, so a resize leaves it blank until
    // something draws - and the next frame may be a long way off: a tab that is
    // not in front gets none at all, and the reader comes back to an empty
    // backdrop with no way to tell it from a broken one. Reveal.js lays itself
    // out on load and fires a resize while doing it, so this is the common case
    // rather than the edge.
    window.addEventListener('resize', function () { sizeTo(); paintFrame(0); });
    // A hidden tab is handed no animation frame at all, so coming back has to
    // be hung off the event. Resetting `last` is the other half of it: without
    // that, the first frame back carries the whole gap.
    document.addEventListener('visibilitychange', function () {
      if (!document.hidden) { last = 0; lastDraw = 0; schedule(); }
    });
    // A theme can be swapped under a running deck in the presentations tab, and
    // a scene still drawing last theme's colours reads as the swap half failing.
    if (window.MutationObserver) {
      new MutationObserver(function () { readTheme(); }).observe(document.documentElement, {
        attributes: true, attributeFilter: ['style', 'class', 'data-theme']
      });
    }
    CFG.state = 'running';
    schedule();
    setInterval(beat, Math.round(1000 / FPS));
  }

  if (!CFG.scene) { return; }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start);
  } else {
    start();
  }
}());
