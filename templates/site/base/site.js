/* orgs site: light and dark, the search box, code copy, the contents spy and
 * the menu on a phone. No framework, nothing fetched but the search index,
 * which is a script (assets/search.js) rather than json because a page opened
 * from disk (file://) may load a script but may not fetch.
 */
(function () {
  var doc = document.documentElement;
  var root = document.body.getAttribute('data-root') || '';

  function isDark() {
    var t = doc.getAttribute('data-theme');
    if (t) return t === 'dark';
    return window.matchMedia && matchMedia('(prefers-color-scheme: dark)').matches;
  }
  var toggle = document.querySelector('.theme-toggle');
  if (toggle) toggle.addEventListener('click', function () {
    var next = isDark() ? 'light' : 'dark';
    doc.setAttribute('data-theme', next);
    try { localStorage.setItem('orgsSiteTheme', next); } catch (e) {}
  });

  var menu = document.querySelector('.nav-toggle');
  if (menu) menu.addEventListener('click', function () {
    var open = document.body.classList.toggle('nav-open');
    menu.setAttribute('aria-expanded', open ? 'true' : 'false');
  });

  // Copy buttons on code.
  document.querySelectorAll('.code').forEach(function (block) {
    var pre = block.querySelector('pre');
    if (!pre || !navigator.clipboard) return;
    var b = document.createElement('button');
    b.className = 'copy-code';
    b.type = 'button';
    b.textContent = 'Copy';
    b.addEventListener('click', function () {
      navigator.clipboard.writeText(pre.innerText.replace(/\n$/, '')).then(function () {
        b.textContent = 'Copied';
        setTimeout(function () { b.textContent = 'Copy'; }, 1400);
      });
    });
    block.appendChild(b);
  });

  // The contents list follows the reading.
  var links = Array.prototype.slice.call(document.querySelectorAll('.page-aside .toc a'));
  if (links.length && 'IntersectionObserver' in window) {
    var byId = {};
    links.forEach(function (a) { byId[decodeURIComponent(a.getAttribute('href').slice(1))] = a; });
    var visible = {};
    var spy = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) { visible[e.target.id] = e.isIntersecting; });
      var heads = document.querySelectorAll('.page-body h2[id], .page-body h3[id], .page-body h4[id]');
      var current = null;
      for (var i = 0; i < heads.length; i++) {
        if (visible[heads[i].id]) { current = heads[i].id; break; }
        if (heads[i].getBoundingClientRect().top < 120) current = heads[i].id;
      }
      links.forEach(function (a) { a.classList.remove('active'); });
      if (current && byId[current]) byId[current].classList.add('active');
    }, { rootMargin: '-60px 0px -55% 0px' });
    document.querySelectorAll('.page-body h2[id], .page-body h3[id], .page-body h4[id]').forEach(function (h) { spy.observe(h); });
  }

  // Search.
  var input = document.querySelector('[data-search]');
  var panel = document.querySelector('.search-results');
  if (!input || !panel) return;
  var index = null, loading = false, hits = [], at = -1;

  function load(then) {
    if (index) return then();
    if (window.SITE_SEARCH) { index = window.SITE_SEARCH; return then(); }
    if (loading) return;
    loading = true;
    var s = document.createElement('script');
    s.src = root + 'assets/search.js';
    s.onload = function () { index = window.SITE_SEARCH || []; then(); };
    document.head.appendChild(s);
  }
  function esc(s) {
    return String(s).replace(/[&<>"]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]; });
  }
  function mark(text, words) {
    var out = esc(text);
    words.forEach(function (w) {
      if (!w) return;
      out = out.replace(new RegExp('(' + w.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + ')', 'ig'), '<mark>$1</mark>');
    });
    return out;
  }
  function snippet(text, words) {
    if (!text) return '';
    var low = text.toLowerCase(), i = -1;
    for (var k = 0; k < words.length && i < 0; k++) i = low.indexOf(words[k]);
    if (i < 0) return text.slice(0, 140) + (text.length > 140 ? '…' : '');
    var from = Math.max(0, i - 50);
    return (from > 0 ? '…' : '') + text.slice(from, from + 160) + (from + 160 < text.length ? '…' : '');
  }
  // Every word has to be there; where it is decides the order - a title
  // beats a tag beats the text, and a match at the start of a word counts
  // for more than one inside it.
  function score(e, words) {
    var total = 0;
    var title = e.t.toLowerCase(), tags = (e.g || []).join(' ').toLowerCase(), text = (e.x || '').toLowerCase();
    for (var i = 0; i < words.length; i++) {
      var w = words[i], s = 0, at;
      if ((at = title.indexOf(w)) >= 0) s = (at === 0 || /\W/.test(title[at - 1])) ? 12 : 8;
      else if (tags.indexOf(w) >= 0) s = 6;
      else if ((at = text.indexOf(w)) >= 0) s = (at === 0 || /\W/.test(text[at - 1])) ? 3 : 1;
      if (!s) return 0;
      total += s;
    }
    return total + (e.p ? 0 : 1);
  }
  function show() {
    var q = input.value.trim().toLowerCase();
    if (!q) { panel.hidden = true; return; }
    var words = q.split(/\s+/);
    hits = index.map(function (e) { return { e: e, s: score(e, words) }; })
      .filter(function (h) { return h.s > 0; })
      .sort(function (a, b) { return b.s - a.s; })
      .slice(0, 10);
    at = hits.length ? 0 : -1;
    if (!hits.length) {
      panel.innerHTML = '<div class="r-none">Nothing matches “' + esc(input.value.trim()) + '”.</div>';
    } else {
      panel.innerHTML = hits.map(function (h, i) {
        var e = h.e;
        var where = e.p ? e.p : (e.d || '');
        return '<a href="' + esc(root + e.u) + '"' + (i === 0 ? ' class="active"' : '') + '>' +
          '<span class="r-title">' + mark(e.t, words) + '</span>' +
          (where ? '<span class="r-where">' + esc(where) + '</span>' : '') +
          (e.x ? '<span class="r-text">' + mark(snippet(e.x, words), words) + '</span>' : '') + '</a>';
      }).join('');
    }
    panel.hidden = false;
  }
  function move(d) {
    var items = panel.querySelectorAll('a');
    if (!items.length) return;
    at = (at + d + items.length) % items.length;
    items.forEach(function (a, i) { a.classList.toggle('active', i === at); });
    items[at].scrollIntoView({ block: 'nearest' });
  }
  input.addEventListener('focus', function () { load(function () { if (input.value) show(); }); });
  input.addEventListener('input', function () { load(show); });
  input.addEventListener('keydown', function (e) {
    if (e.key === 'ArrowDown') { e.preventDefault(); move(1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); move(-1); }
    else if (e.key === 'Enter') {
      var a = panel.querySelectorAll('a')[at];
      if (a) { e.preventDefault(); location.href = a.getAttribute('href'); }
    } else if (e.key === 'Escape') { input.value = ''; panel.hidden = true; input.blur(); }
  });
  document.addEventListener('click', function (e) {
    if (!panel.contains(e.target) && e.target !== input) panel.hidden = true;
  });
  document.addEventListener('keydown', function (e) {
    var t = e.target.tagName;
    if (e.key === '/' && t !== 'INPUT' && t !== 'TEXTAREA' && !e.target.isContentEditable) {
      e.preventDefault();
      input.focus();
    }
  });
})();
