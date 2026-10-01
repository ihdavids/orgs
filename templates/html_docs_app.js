/* ------------------------------------------------------------------ *
 * The docs page.
 *
 * Three things it does, in the order they were asked for:
 *
 *  1. A contents rail built from ORG_NODES, where clicking a row
 *     *goes to the heading* - unfolding whatever it is inside, scrolling
 *     it under the masthead and marking it - rather than hiding the rest
 *     of the document, which is what the old tree did and which read as
 *     the link being broken.
 *  2. Fuzzy search. The matcher is `internal/common/dnd/fuzzy.go` said
 *     again, the same way `worg/src/fuzzy.ts` says it: a term matches a
 *     heading's own name loosely ("runsrc" finds "Running a source
 *     block") and the body text only on a whole word, because fuzzy
 *     matching over a page of prose matches very nearly everything.
 *     Change one and change all three. Arrowing through the hits takes
 *     the page to each one as a *look*: Escape puts back where you were,
 *     Enter keeps where you have got to.
 *  3. Folding, anchors and the odds and ends of reading - a copy button
 *     on each code block, tables that scroll inside their own box, a
 *     light/dark switch.
 *
 * It is plain DOM on purpose. The page used to carry jQuery for this and
 * nothing else.
 * ------------------------------------------------------------------ */
(function () {
  'use strict';

  var REDUCED = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  var entries = [];          // flat, in document order
  var byId = Object.create(null);
  var tops = null;           // cached scroll offsets for the spy
  var current = null;
  var marked = [];

  /* ---------------------------------------------------------------- *
   * Fuzzy matching - fuzzy.go, letter for letter.
   * ---------------------------------------------------------------- */

  function isBoundary(c) { return ' -_/(),.\':'.indexOf(c) >= 0; }

  // How good a match `pattern` is for `text`, as a subsequence: adjacent
  // letters, letters that start a word and a match right at the front all
  // count for more, a big gap between letters counts for less. null when
  // the pattern is not in the text at all.
  function fuzzyScore(pattern, text) {
    if (pattern === '') return 0;
    var pat = pattern.toLowerCase(), txt = text.toLowerCase();
    var score = 0, pi = 0, last = -1;
    for (var ti = 0; ti < txt.length && pi < pat.length; ti++) {
      if (txt.charAt(ti) !== pat.charAt(pi)) continue;
      score += 10;
      if (ti === 0) score += 20;
      else if (last === ti - 1) score += 12;
      else if (isBoundary(txt.charAt(ti - 1))) score += 10;
      if (last >= 0 && ti - last > 1) score -= Math.min(ti - last - 1, 6);
      last = ti;
      pi++;
    }
    if (pi < pat.length) return null;
    score -= Math.floor(txt.length / 12);
    return score;
  }

  // Which letters of `text` the pattern matched, for drawing the match.
  // The same walk as fuzzyScore, so what is emboldened is what scored.
  function fuzzyPositions(pattern, text) {
    var pat = pattern.toLowerCase(), txt = text.toLowerCase();
    var out = [], pi = 0;
    for (var ti = 0; ti < txt.length && pi < pat.length; ti++) {
      if (txt.charAt(ti) !== pat.charAt(pi)) continue;
      out.push(ti);
      pi++;
    }
    return pi < pat.length ? [] : out;
  }

  // Every term has to match somewhere, so "source run" and "run source"
  // both find the same heading.
  function fuzzyScoreAll(terms, name, label) {
    var rest = label.toLowerCase();
    var at = rest.indexOf(name.toLowerCase());
    if (at >= 0) rest = rest.slice(at + name.length);
    var total = 0;
    for (var i = 0; i < terms.length; i++) {
      var n = fuzzyScore(terms[i], name);
      if (n !== null) { total += n * 2; continue; }
      var j = rest.indexOf(terms[i]);
      if (j >= 0) { total += 20 - Math.floor(j / 8); continue; }
      return null;
    }
    return total;
  }

  function fuzzyTerms(filter) {
    return filter.trim().toLowerCase().split(/\s+/).filter(function (t) { return t !== ''; });
  }

  /* ---------------------------------------------------------------- *
   * Small helpers
   * ---------------------------------------------------------------- */

  function esc(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;')
                    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  // A heading's name arrives as html, because a headline may hold code,
  // a link or an emphasis. The index wants the words out of it.
  function textOf(html) {
    var d = document.createElement('div');
    d.innerHTML = html;
    return (d.textContent || '').replace(/\s+/g, ' ').trim();
  }

  var slugs = Object.create(null);
  function slugify(name) {
    var s = name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64);
    if (!s) s = 'section';
    if (slugs[s] === undefined) { slugs[s] = 1; return s; }
    slugs[s] += 1;
    return s + '-' + slugs[s];
  }

  // How much of the top of the window something else is standing in. Asked
  // of whatever the theme marked rather than of a class name, because the
  // two themes put very different things up there: the docs page has a
  // masthead across every width, and the read-the-docs one has a bar that
  // exists only on a narrow screen. A bar that is not displayed measures
  // zero, which is the right answer rather than a special case.
  function headerHeight() {
    var h = document.querySelector('[data-sticky-header]');
    return h ? h.offsetHeight : 0;
  }

  function svg(paths, extra) {
    return '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" ' +
           'stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"' +
           (extra ? ' ' + extra : '') + '>' + paths + '</svg>';
  }

  var ICON_SEARCH = svg('<circle cx="11" cy="11" r="7"></circle><path d="M20 20l-3.6-3.6"></path>');
  var ICON_LINK   = svg('<path d="M10 13a5 5 0 0 0 7.5.5l3-3a5 5 0 0 0-7-7l-1.7 1.7"></path>' +
                        '<path d="M14 11a5 5 0 0 0-7.5-.5l-3 3a5 5 0 0 0 7 7l1.7-1.7"></path>');
  var ICON_SUN    = svg('<circle cx="12" cy="12" r="4"></circle><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4' +
                        'M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"></path>', 'class="when-light"');
  var ICON_MOON   = svg('<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"></path>', 'class="when-dark"');
  var ICON_MENU   = svg('<path d="M4 7h16M4 12h16M4 17h16"></path>');

  /* ---------------------------------------------------------------- *
   * The index: one entry per heading, in document order.
   * ---------------------------------------------------------------- */

  function buildIndex(nodes, depth, path, parent) {
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i];
      var el = document.getElementById(n.Id);
      if (!el) continue;
      var name = textOf(n.Name);
      var e = {
        id: n.Id,
        nameHtml: n.Name,
        name: name,
        path: path,
        depth: depth,
        parent: parent,
        children: [],
        el: el,
        titleEl: document.getElementById(n.Id + '-title'),
        contentEl: document.getElementById(n.Id + '-content'),
        textEl: document.getElementById(n.Id + '-text'),
        slug: slugify(name),
        row: null,
        item: null
      };
      // Only this heading's own prose: a parent would otherwise claim
      // every word written under it and win every search.
      e.body = e.textEl ? (e.textEl.textContent || '').replace(/\s+/g, ' ').trim() : '';
      e.bodyLow = e.body.toLowerCase();
      if (e.titleEl) e.titleEl.id = e.slug;
      if (parent) parent.children.push(e);
      entries.push(e);
      byId[n.Id] = e;
      if (n.Children && n.Children.length) {
        buildIndex(n.Children, depth + 1, path ? path + ' / ' + name : name, e);
      }
    }
  }

  /* ---------------------------------------------------------------- *
   * The contents rail
   * ---------------------------------------------------------------- */

  function buildTree(list, parentUl) {
    for (var i = 0; i < list.length; i++) {
      var e = list[i];
      var li = document.createElement('li');
      li.className = 'tv-item depth-' + Math.min(e.depth, 5);
      li.setAttribute('data-open', '0');

      var row = document.createElement('div');
      row.className = 'tv-row';

      var twist = document.createElement('button');
      twist.type = 'button';
      twist.className = 'tv-twist' + (e.children.length ? '' : ' is-leaf');
      if (e.children.length) {
        twist.setAttribute('aria-label', 'Show what is under ' + e.name);
        twist.setAttribute('aria-expanded', 'false');
        twist.addEventListener('click', (function (item) {
          return function (ev) { ev.preventDefault(); ev.stopPropagation(); toggleItem(item); };
        })(li));
      } else {
        twist.tabIndex = -1;
        twist.setAttribute('aria-hidden', 'true');
      }

      var a = document.createElement('a');
      a.className = 'tv-label';
      a.href = '#' + e.slug;
      a.innerHTML = e.nameHtml;
      a.title = e.name;
      a.addEventListener('click', (function (entry) {
        return function (ev) { ev.preventDefault(); goTo(entry, null, true); closeRail(); };
      })(e));

      row.appendChild(twist);
      row.appendChild(a);
      li.appendChild(row);
      e.row = row;
      e.item = li;

      if (e.children.length) {
        var ul = document.createElement('ul');
        buildTree(e.children, ul);
        li.appendChild(ul);
      }
      parentUl.appendChild(li);
    }
  }

  function toggleItem(li, force) {
    var open = force === undefined ? li.getAttribute('data-open') !== '1' : !!force;
    li.setAttribute('data-open', open ? '1' : '0');
    var t = li.querySelector('.tv-twist');
    if (t && !t.classList.contains('is-leaf')) t.setAttribute('aria-expanded', open ? 'true' : 'false');
  }

  function openTreeTo(e) {
    for (var p = e.parent; p; p = p.parent) if (p.item) toggleItem(p.item, true);
  }

  function setAll(open) {
    var items = document.querySelectorAll('#navbar .tv-item');
    for (var i = 0; i < items.length; i++) {
      // Its own row's twist, not any twist under it - asked loosely, a
      // branch with a single leaf in it counts as a leaf and expand-all
      // opens nothing but the top.
      var twist = items[i].firstChild ? items[i].firstChild.firstChild : null;
      if (!twist || twist.classList.contains('is-leaf')) continue;
      toggleItem(items[i], open);
    }
  }

  // Keep the current row in view inside the rail only. scrollIntoView
  // would scroll the page as well, which fights the scroll that put us
  // here in the first place.
  function revealInRail(e) {
    var rail = document.getElementById('sidebar');
    if (!rail || !e.row) return;
    var r = e.row.getBoundingClientRect(), b = rail.getBoundingClientRect();
    if (r.top < b.top + 8) rail.scrollTop -= (b.top + 8 - r.top);
    else if (r.bottom > b.bottom - 8) rail.scrollTop += (r.bottom - b.bottom + 8);
  }

  // The chain from the root down to the current heading, marked on the rail
  // rows themselves. The read-the-docs theme draws that whole branch rather
  // than only the row you are standing on, which is what "current" means on
  // a page of sections; a theme with no use for it simply styles nothing.
  // Only the few rows that carry the class are asked for, so this costs
  // nothing on a document with hundreds of headings.
  function markBranch(e) {
    var was = document.querySelectorAll('#navbar .tv-item.is-branch');
    for (var i = 0; i < was.length; i++) was[i].classList.remove('is-branch');
    for (var p = e; p; p = p.parent) if (p.item) p.item.classList.add('is-branch');
  }

  function setCurrent(e) {
    if (current === e) return;
    if (current && current.row) current.row.classList.remove('is-current');
    current = e;
    markBranch(e);
    // Said out loud so a theme can put the heading somewhere of its own - a
    // breadcrumb, a title bar - without this file having to know it is there.
    try { document.dispatchEvent(new CustomEvent('docs:current', { detail: e })); } catch (err) { /* old browser */ }
    if (!e) return;
    if (e.row) e.row.classList.add('is-current');
    openTreeTo(e);
    revealInRail(e);
  }

  /* ---------------------------------------------------------------- *
   * Going to a heading
   * ---------------------------------------------------------------- */

  // Answers with the sections it actually opened, so a preview can fold
  // them back up again on the way out: unfolding to show somebody a hit is
  // part of the look, not part of what they asked for.
  function unfold(e) {
    var changed = [];
    for (var p = e; p; p = p.parent) {
      if (p.el.classList.contains('is-folded')) { p.el.classList.remove('is-folded'); changed.push(p); }
    }
    tops = null;
    return changed;
  }

  function flash(e) {
    e.el.classList.remove('just-landed');
    void e.el.offsetWidth;
    e.el.classList.add('just-landed');
    window.setTimeout(function () { e.el.classList.remove('just-landed'); }, 1800);
  }

  // `now` cuts rather than animates whatever the distance. Arrowing down a
  // list of hits repeats, and a queued smooth scroll per keypress arrives
  // after the key that asked for it - the page ends up chasing the cursor.
  function goTo(e, terms, push, now) {
    var opened = unfold(e);
    openTreeTo(e);
    clearMarks();
    if (terms && terms.length) markTerms(e, terms);
    var target = e.titleEl || e.el;
    var y = Math.max(target.getBoundingClientRect().top + window.pageYOffset - headerHeight() - 14, 0);
    // Smooth over a short hop, which is what makes it read as a move rather
    // than a cut; instant over a long one. This document is a hundred
    // thousand pixels tall, and animating across all of it is a second of
    // blur that says nothing about where you came from or where you landed.
    var far = Math.abs(y - window.pageYOffset) > window.innerHeight * 3;
    window.scrollTo({ top: y, behavior: (REDUCED || far || now) ? 'auto' : 'smooth' });
    flash(e);
    setCurrent(e);
    if (push && history.replaceState) history.replaceState(null, '', '#' + e.slug);
    return opened;
  }

  /* ---------------------------------------------------------------- *
   * Marking what was searched for.
   *
   * Over text nodes rather than over innerHTML: a regular expression run
   * across markup lands inside a tag as readily as inside a sentence, and
   * the old search did exactly that - which is why a hit could take the
   * rest of the page's formatting with it.
   * ---------------------------------------------------------------- */

  function clearMarks() {
    for (var i = 0; i < marked.length; i++) {
      var m = marked[i];
      if (!m.parentNode) continue;
      var p = m.parentNode;
      p.replaceChild(document.createTextNode(m.textContent), m);
      p.normalize();
    }
    marked = [];
  }

  function markTerms(e, terms) {
    var root = e.textEl;
    if (!root) return;
    var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, null, false);
    var texts = [], n;
    while ((n = walker.nextNode())) if (n.nodeValue && n.nodeValue.trim()) texts.push(n);
    var budget = 300;
    for (var i = 0; i < texts.length && budget > 0; i++) {
      var tn = texts[i], low = tn.nodeValue.toLowerCase(), hits = [];
      for (var t = 0; t < terms.length; t++) {
        var j = low.indexOf(terms[t]);
        while (j >= 0) { hits.push([j, j + terms[t].length]); j = low.indexOf(terms[t], j + terms[t].length); }
      }
      if (!hits.length) continue;
      hits.sort(function (a, b) { return a[0] - b[0]; });
      var merged = [];
      for (var h = 0; h < hits.length; h++) {
        var lastHit = merged[merged.length - 1];
        if (lastHit && hits[h][0] <= lastHit[1]) lastHit[1] = Math.max(lastHit[1], hits[h][1]);
        else merged.push([hits[h][0], hits[h][1]]);
      }
      var frag = document.createDocumentFragment(), at = 0, src = tn.nodeValue;
      for (var k = 0; k < merged.length && budget > 0; k++) {
        if (merged[k][0] > at) frag.appendChild(document.createTextNode(src.slice(at, merged[k][0])));
        var mk = document.createElement('mark');
        mk.className = 'doc-hit';
        mk.textContent = src.slice(merged[k][0], merged[k][1]);
        frag.appendChild(mk);
        marked.push(mk);
        at = merged[k][1];
        budget--;
      }
      if (at < src.length) frag.appendChild(document.createTextNode(src.slice(at)));
      tn.parentNode.replaceChild(frag, tn);
    }
  }

  /* ---------------------------------------------------------------- *
   * Search
   * ---------------------------------------------------------------- */

  var results = [], selected = -1, lastTerms = [];

  function search(q) {
    var terms = fuzzyTerms(q);
    lastTerms = terms;
    if (!terms.length) return [];
    var out = [];
    for (var i = 0; i < entries.length; i++) {
      var e = entries[i];
      var label = e.name + ' ' + e.path;
      var s = fuzzyScoreAll(terms, e.name, label);
      if (s !== null) { out.push({ e: e, score: s + 1000, kind: 'head' }); continue; }
      // Not in any heading, so look in this heading's own prose - whole
      // words only, and always below anything the title answered.
      var ok = true, first = -1;
      for (var t = 0; t < terms.length; t++) {
        var at = e.bodyLow.indexOf(terms[t]);
        if (at < 0) { ok = false; break; }
        if (first < 0 || at < first) first = at;
      }
      if (ok && e.body) out.push({ e: e, score: 100 - Math.min(Math.floor(first / 40), 60), kind: 'body', at: first });
    }
    out.sort(function (a, b) { return b.score - a.score; });
    return out.slice(0, 40);
  }

  function boldSubsequence(text, term) {
    var pos = fuzzyPositions(term, text);
    if (!pos.length) return esc(text);
    var set = Object.create(null);
    for (var i = 0; i < pos.length; i++) set[pos[i]] = true;
    var out = '', open = false;
    for (var c = 0; c < text.length; c++) {
      if (set[c] && !open) { out += '<b>'; open = true; }
      else if (!set[c] && open) { out += '</b>'; open = false; }
      out += esc(text.charAt(c));
    }
    return out + (open ? '</b>' : '');
  }

  function snippet(e, at, terms) {
    var from = Math.max(0, at - 50), to = Math.min(e.body.length, at + 110);
    var s = (from > 0 ? '…' : '') + e.body.slice(from, to) + (to < e.body.length ? '…' : '');
    var low = s.toLowerCase(), out = '', i = 0;
    while (i < s.length) {
      var best = -1, len = 0;
      for (var t = 0; t < terms.length; t++) {
        var j = low.indexOf(terms[t], i);
        if (j >= 0 && (best < 0 || j < best)) { best = j; len = terms[t].length; }
      }
      if (best < 0) { out += esc(s.slice(i)); break; }
      out += esc(s.slice(i, best)) + '<b>' + esc(s.substr(best, len)) + '</b>';
      i = best + len;
    }
    return out;
  }

  function renderResults(q) {
    var box = document.getElementById('searchresults');
    // A preview is of a result that is about to stop existing, so the look
    // ends with the query that asked for it rather than leaving somebody
    // halfway down a document they did not choose to be in.
    cancelPreview();
    results = search(q);
    selected = results.length ? 0 : -1;
    if (!q.trim()) { box.setAttribute('data-open', '0'); box.innerHTML = ''; return; }
    if (!results.length) {
      box.innerHTML = '<div class="sr-empty">Nothing matched &ldquo;' + esc(q) + '&rdquo;</div>';
      box.setAttribute('data-open', '1');
      return;
    }
    var html = '<div class="sr-count"><span>' + results.length + (results.length === 40 ? '+' : '') + ' result' +
               (results.length === 1 ? '' : 's') + '</span>' +
               '<span class="sr-keys"><kbd>&uarr;</kbd><kbd>&darr;</kbd> look &middot; ' +
               '<kbd>&crarr;</kbd> keep &middot; <kbd>esc</kbd> back</span></div>';
    for (var i = 0; i < results.length; i++) {
      var r = results[i], e = r.e;
      html += '<a class="sr-item" href="#' + esc(e.slug) + '" data-i="' + i + '"' +
              (i === selected ? ' aria-selected="true"' : '') + '>';
      html += '<span class="sr-name">' + boldSubsequence(e.name, lastTerms[0] || '') + '</span>';
      if (e.path) html += '<span class="sr-path">' + esc(e.path) + '</span>';
      if (r.kind === 'body') html += '<span class="sr-snip">' + snippet(e, r.at, lastTerms) + '</span>';
      html += '</a>';
    }
    box.innerHTML = html;
    box.setAttribute('data-open', '1');
  }

  // Keep the selected row in view inside the results box only, the way
  // revealInRail does for the rail. scrollIntoView scrolls every scrollable
  // ancestor, the document included - which is the one thing that must not
  // move here, since the document is now showing the preview.
  function revealInList(item, box) {
    var r = item.getBoundingClientRect(), b = box.getBoundingClientRect();
    if (r.top < b.top + 4) box.scrollTop -= (b.top + 4 - r.top);
    else if (r.bottom > b.bottom - 4) box.scrollTop += (r.bottom - b.bottom + 4);
  }

  function markSelection() {
    var box = document.getElementById('searchresults');
    var items = box.querySelectorAll('.sr-item');
    for (var i = 0; i < items.length; i++) {
      if (i === selected) { items[i].setAttribute('aria-selected', 'true'); revealInList(items[i], box); }
      else items[i].removeAttribute('aria-selected');
    }
  }

  /* ---------------------------------------------------------------- *
   * Previewing a result
   *
   * Arrowing through the hits takes the page to each one, so a result is
   * read rather than guessed at from its snippet. That is a *look*, not a
   * move: Escape puts back everything the look changed - where the page
   * was, which sections were folded, which row the rail called current -
   * and Enter keeps it. A preview that could not be taken back would make
   * the arrow keys something you think twice about pressing, which is the
   * opposite of what they are for.
   * ---------------------------------------------------------------- */

  var preview = null;

  function startPreview() {
    if (preview) return;
    preview = { y: window.pageYOffset, current: current, folded: [] };
  }

  function previewTo(i) {
    if (i < 0 || i >= results.length) return;
    startPreview();
    var e = results[i].e;
    var opened = goTo(e, lastTerms, false, true);
    for (var k = 0; k < opened.length; k++) preview.folded.push(opened[k]);

    // The results panel is over the page, and on a narrow screen it is over
    // all of it - so the heading just previewed would land underneath the
    // list that asked for it. Drop it below the panel when the two actually
    // overlap, which on a wide screen they never do. Less scroll, not more:
    // the page moves *down* to put the heading further down the screen.
    var box = document.getElementById('searchresults');
    var t = (e.titleEl || e.el).getBoundingClientRect(), b = box.getBoundingClientRect();
    if (t.left < b.right && t.right > b.left && t.top < b.bottom) {
      window.scrollTo({ top: Math.max(window.pageYOffset - (b.bottom - t.top) - 12, 0), behavior: 'auto' });
    }
  }

  // Put the page back exactly as it was found. The folds go back first, so
  // the scroll lands against the same document it was taken from.
  function cancelPreview() {
    if (!preview) return;
    var p = preview;
    preview = null;
    clearMarks();
    for (var i = 0; i < p.folded.length; i++) p.folded[i].el.classList.add('is-folded');
    tops = null;
    var all = document.querySelectorAll('.just-landed');
    for (var j = 0; j < all.length; j++) all[j].classList.remove('just-landed');
    setCurrent(p.current);
    window.scrollTo({ top: p.y, behavior: 'auto' });
  }

  function keepPreview() { preview = null; }

  function moveSelection(d) {
    var box = document.getElementById('searchresults');
    if (!results.length || box.getAttribute('data-open') !== '1') return;
    // The first arrow press shows what is already selected rather than
    // stepping past it - otherwise the top hit, which is the one the search
    // thinks you meant, is the one result you can never look at first.
    if (preview) selected = (selected + d + results.length) % results.length;
    markSelection();
    previewTo(selected);
  }

  function chooseResult(i) {
    if (i < 0 || i >= results.length) return;
    // Already standing there if this was arrowed to, so there is nothing to
    // animate; a result picked with the mouse still gets the short hop.
    var shown = !!preview;
    keepPreview();
    closeSearch();
    // The choice is made, so the page gets the keyboard back - an arrow key
    // after Enter is somebody reading what they landed on, not still
    // walking a list that is no longer on screen.
    var field = document.getElementById('searchfield');
    if (field) field.blur();
    goTo(results[i].e, lastTerms, true, shown);
  }

  function closeSearch() {
    var box = document.getElementById('searchresults');
    box.setAttribute('data-open', '0');
  }

  /* ---------------------------------------------------------------- *
   * Folding, anchors, code and tables
   * ---------------------------------------------------------------- */

  function decorateHeadings() {
    for (var i = 0; i < entries.length; i++) {
      var e = entries[i];
      if (!e.titleEl) continue;

      var fold = document.createElement('button');
      fold.type = 'button';
      fold.className = 'fold-btn';
      fold.title = 'Fold this section';
      fold.setAttribute('aria-label', 'Fold ' + e.name);
      fold.addEventListener('click', (function (entry) {
        return function (ev) {
          ev.preventDefault();
          entry.el.classList.toggle('is-folded');
          tops = null;
        };
      })(e));

      var link = document.createElement('button');
      link.type = 'button';
      link.className = 'anchor-btn';
      link.title = 'Copy a link to this section';
      link.setAttribute('aria-label', 'Copy a link to ' + e.name);
      link.innerHTML = ICON_LINK;
      link.addEventListener('click', (function (entry) {
        return function (ev) {
          ev.preventDefault();
          var url = location.href.split('#')[0] + '#' + entry.slug;
          if (navigator.clipboard) navigator.clipboard.writeText(url);
          if (history.replaceState) history.replaceState(null, '', '#' + entry.slug);
        };
      })(e));

      e.titleEl.insertBefore(fold, e.titleEl.firstChild);
      e.titleEl.appendChild(link);
    }
  }

  // A table is given a box of its own to scroll inside. The exporter
  // writes a bare <table>, and a wide one either stretches the page
  // sideways or is cut off at the edge of it.
  function wrapTables() {
    var tables = document.querySelectorAll('.doc-body table');
    for (var i = 0; i < tables.length; i++) {
      var t = tables[i];
      if (t.parentNode && t.parentNode.classList && t.parentNode.classList.contains('table-scroll')) continue;
      var w = document.createElement('div');
      w.className = 'table-scroll';
      t.parentNode.insertBefore(w, t);
      w.appendChild(t);
    }
  }

  function decorateCode() {
    var blocks = document.querySelectorAll('.doc-body pre > code');
    for (var i = 0; i < blocks.length; i++) {
      var pre = blocks[i].parentNode;
      var wrap = document.createElement('div');
      wrap.className = 'code-wrap';
      pre.parentNode.insertBefore(wrap, pre);
      wrap.appendChild(pre);

      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'copy-btn';
      btn.textContent = 'copy';
      btn.addEventListener('click', (function (code, b) {
        return function () {
          if (!navigator.clipboard) return;
          navigator.clipboard.writeText(code.textContent).then(function () {
            b.textContent = 'copied';
            window.setTimeout(function () { b.textContent = 'copy'; }, 1200);
          });
        };
      })(blocks[i], btn));
      wrap.appendChild(btn);
    }
  }

  /* ---------------------------------------------------------------- *
   * Syntax colouring
   * ---------------------------------------------------------------- */

  function registerOrg() {
    if (!window.hljs || hljs.getLanguage('org')) return;
    hljs.registerLanguage('org', function () {
      return {
        name: 'Org',
        case_insensitive: false,
        contains: [
          { className: 'section', begin: /^\*+ .*$/ },
          { className: 'meta', begin: /^\s*#\+[A-Za-z_]+:?/, end: /$/ },
          { className: 'comment', begin: /^\s*#(?!\+)[^\n]*$/ },
          { className: 'attr', begin: /^\s*:[A-Za-z_@#%]+:/ },
          { className: 'number', begin: /[<\[]\d{4}-\d{2}-\d{2}[^>\]\n]*[>\]]/ },
          { className: 'link', begin: /\[\[/, end: /\]\]/ },
          { className: 'bullet', begin: /^\s*(?:[-+]|\d+[.)])\s(?:\[[ X-]\]\s)?/ },
          { className: 'code', begin: /[=~][^=~\n]+[=~]/ }
        ]
      };
    });
  }

  // hljs is handed the language the org block declared. A name it has
  // never heard of is called plaintext rather than left to be guessed at:
  // detection on a short block is wrong often enough to be worse than no
  // colour at all - a page of json was coming out coloured as css.
  function normalizeLanguages() {
    var alias = { sh: 'bash', shell: 'bash', zsh: 'bash', elisp: 'lisp', 'emacs-lisp': 'lisp',
                  jsonc: 'json', html: 'xml', conf: 'ini', django: 'plaintext', text: 'plaintext' };
    var codes = document.querySelectorAll('pre > code[class*="language-"]');
    for (var i = 0; i < codes.length; i++) {
      var c = codes[i], m = /language-([\w+-]+)/.exec(c.className);
      if (!m) continue;
      var lang = m[1].toLowerCase();
      if (alias[lang]) lang = alias[lang];
      if (!window.hljs || !hljs.getLanguage(lang)) lang = 'plaintext';
      c.className = c.className.replace(/language-[\w+-]+/, 'language-' + lang);
    }
    // A block with no language at all stays plain: a guess is worse.
    var bare = document.querySelectorAll('pre > code:not([class*="language-"])');
    for (var j = 0; j < bare.length; j++) bare[j].className += ' language-plaintext';
  }

  /* ---------------------------------------------------------------- *
   * The scroll spy
   * ---------------------------------------------------------------- */

  function measure() {
    tops = [];
    for (var i = 0; i < entries.length; i++) {
      var e = entries[i];
      if (!e.titleEl || !e.titleEl.offsetParent) continue;   // inside a folded parent
      tops.push({ y: e.titleEl.getBoundingClientRect().top + window.pageYOffset, e: e });
    }
    tops.sort(function (a, b) { return a.y - b.y; });
  }

  function spy() {
    if (!tops) measure();
    if (!tops.length) return;
    var line = window.pageYOffset + headerHeight() + 30;
    var lo = 0, hi = tops.length - 1, best = 0;
    while (lo <= hi) {
      var mid = (lo + hi) >> 1;
      if (tops[mid].y <= line) { best = mid; lo = mid + 1; } else hi = mid - 1;
    }
    setCurrent(tops[best].e);
  }

  /* ---------------------------------------------------------------- *
   * Light and dark
   * ---------------------------------------------------------------- */

  function applyTheme(t) {
    if (t) document.documentElement.setAttribute('data-theme', t);
    else document.documentElement.removeAttribute('data-theme');
    syncHighlightTheme();
  }

  // The highlight.js stylesheet is picked the same way the palette is, and
  // has to be switched rather than overridden: both sheets define the same
  // class names, so whichever loads second would otherwise win whatever the
  // page is set to.
  function syncHighlightTheme() {
    var light = document.getElementById('hljs-light');
    var dark = document.getElementById('hljs-dark');
    if (!light || !dark) return;   // the file asked for a style of its own
    var isDark = currentTheme() === 'dark';
    light.disabled = isDark;
    dark.disabled = !isDark;
  }

  function currentTheme() {
    var set = document.documentElement.getAttribute('data-theme');
    if (set) return set;
    return (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light';
  }

  /* ---------------------------------------------------------------- *
   * The rail, on a narrow screen
   * ---------------------------------------------------------------- */

  // Narrow enough that the rail is over the page rather than beside it -
  // which is exactly when its own toggle is on screen. Asking the toggle
  // rather than naming a width keeps the one breakpoint in the stylesheet
  // that draws it, so two themes may disagree about where narrow starts.
  function closeRail() {
    var t = document.getElementById('rail-toggle');
    if (t && t.offsetParent !== null) document.body.removeAttribute('data-rail');
  }

  /* ---------------------------------------------------------------- *
   * Boot
   * ---------------------------------------------------------------- */

  function boot() {
    var stored = null;
    try { stored = localStorage.getItem('docsTheme'); } catch (err) { /* private mode */ }
    applyTheme(stored);
    if (!stored && window.matchMedia) {
      var mq = window.matchMedia('(prefers-color-scheme: dark)');
      var follow = function () { syncHighlightTheme(); };
      if (mq.addEventListener) mq.addEventListener('change', follow);
      else if (mq.addListener) mq.addListener(follow);
    }

    buildIndex(ORG_NODES || [], 0, '', null);
    buildTree(entries.filter(function (e) { return !e.parent; }), document.getElementById('navbar'));
    decorateHeadings();
    wrapTables();
    registerOrg();
    normalizeLanguages();
    decorateCode();
    if (window.hljs) hljs.highlightAll();

    // Search
    var field = document.getElementById('searchfield');
    var box = document.getElementById('searchresults');
    var timer = null;
    field.addEventListener('input', function () {
      window.clearTimeout(timer);
      var v = field.value;
      timer = window.setTimeout(function () { renderResults(v); }, 70);
    });
    field.addEventListener('keydown', function (ev) {
      if (ev.key === 'ArrowDown') { ev.preventDefault(); moveSelection(1); }
      else if (ev.key === 'ArrowUp') { ev.preventDefault(); moveSelection(-1); }
      else if (ev.key === 'Enter') { ev.preventDefault(); chooseResult(selected); }
      else if (ev.key === 'Escape') { cancelPreview(); field.value = ''; closeSearch(); field.blur(); }
    });
    field.addEventListener('focus', function () { if (field.value.trim()) renderResults(field.value); });
    box.addEventListener('mousedown', function (ev) {
      var a = ev.target.closest ? ev.target.closest('.sr-item') : null;
      if (!a) return;
      ev.preventDefault();
      chooseResult(parseInt(a.getAttribute('data-i'), 10));
    });
    document.addEventListener('click', function (ev) {
      // Clicking into the page is reading it, so a preview standing when
      // that happens is kept rather than snatched back out from under the
      // click. Escape is the way back, and it is still the only one.
      if (!ev.target.closest || !ev.target.closest('.search')) { keepPreview(); closeSearch(); }
    });
    var form = document.getElementById('searchform');
    if (form) form.addEventListener('submit', function (ev) { ev.preventDefault(); chooseResult(selected); });

    // Keys: / or the platform's palette chord opens search, Escape leaves it.
    document.addEventListener('keydown', function (ev) {
      var typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName) ||
                   document.activeElement.isContentEditable;
      if ((ev.key === 'k' || ev.key === 'K') && (ev.metaKey || ev.ctrlKey)) {
        ev.preventDefault(); field.focus(); field.select(); return;
      }
      if (ev.key === '/' && !typing) { ev.preventDefault(); field.focus(); field.select(); return; }
      if (ev.key === 'Escape') { cancelPreview(); clearMarks(); closeSearch(); closeRail(); }
    });

    // Rail controls
    document.getElementById('rail-expand').addEventListener('click', function () { setAll(true); });
    document.getElementById('rail-collapse').addEventListener('click', function () {
      setAll(false);
      if (current) openTreeTo(current);
    });
    document.getElementById('rail-toggle').addEventListener('click', function () {
      if (document.body.getAttribute('data-rail') === 'open') document.body.removeAttribute('data-rail');
      else document.body.setAttribute('data-rail', 'open');
    });

    // Theme
    document.getElementById('theme-toggle').addEventListener('click', function () {
      var next = currentTheme() === 'dark' ? 'light' : 'dark';
      applyTheme(next);
      try { localStorage.setItem('docsTheme', next); } catch (err) { /* private mode */ }
    });

    // The spy
    var ticking = false;
    window.addEventListener('scroll', function () {
      if (ticking) return;
      ticking = true;
      window.requestAnimationFrame(function () { spy(); ticking = false; });
    }, { passive: true });
    window.addEventListener('resize', function () { tops = null; });
    window.addEventListener('load', function () { tops = null; });

    // A link somebody was sent, or a reload: the slug is stable across
    // exports where the uuid the exporter writes is not.
    var hash = decodeURIComponent((location.hash || '').slice(1));
    var landed = null;
    if (hash) for (var i = 0; i < entries.length; i++) if (entries[i].slug === hash) { landed = entries[i]; break; }
    if (landed) window.setTimeout(function () { goTo(landed, null, false); }, 60);
    else { spy(); if (entries.length) openTreeTo(entries[0]); }
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', boot);
  else boot();

  window.orgDocs = { goTo: goTo, entries: entries, search: search };

  // Written here rather than in the markup so the page has no inline
  // handlers to maintain in two places.
  window.ORG_ICONS = { search: ICON_SEARCH, sun: ICON_SUN, moon: ICON_MOON, menu: ICON_MENU };
})();
