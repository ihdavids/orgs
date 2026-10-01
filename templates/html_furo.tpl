<!-- Furo/diataxis template for the html exporter (HTML_THEME: furo) -->
<!--
     The third dress on the same application (html_docs_app.js): the three
     column layout the Furo sphinx theme wears, which is what diataxis.fr
     and a good deal of modern python documentation is read in. Contents
     down the left, the document in the middle, and "On this page" down
     the right.

     The right column is the one thing here that is not in the other two
     themes, and it is built from what the shared script already knows: it
     is the current top level section's own headings, redrawn on the
     docs:current event. On a site of many pages that column is the page
     you are on; a single org file's equivalent of a page is its top level
     section, which is also the unit the left rail is a list of.
-->
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{title}}</title>
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{fontfamily}}&display=swap">
{%if hljs_style_default%}
  <link id="hljs-light" rel="stylesheet" href="{{hljs_cdn}}/styles/github.min.css">
  <link id="hljs-dark" rel="stylesheet" href="{{hljs_cdn}}/styles/github-dark.min.css">
{%else%}
  <link rel="stylesheet" href="{{hljs_cdn}}/styles/{{hljs_style}}.min.css">
{%endif%}

  <script>
    // Before anything paints: the page the reader chose last time, and the
    // matching highlight sheet. The key is shared with the other two
    // documentation themes on purpose - the same documentation read three
    // ways, and choosing dark in one and finding light in another reads as
    // a bug rather than as a separate setting.
    (function () {
      var t = null;
      try { t = localStorage.getItem('docsTheme'); } catch (e) { /* private mode */ }
      if (t) document.documentElement.setAttribute('data-theme', t);
      var dark = t ? t === 'dark'
                   : (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches);
      var l = document.getElementById('hljs-light'), d = document.getElementById('hljs-dark');
      if (l && d) { l.disabled = dark; d.disabled = !dark; }
    })();
  </script>

  <script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js"></script>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/languages/go.min.js"></script>

<script>
// The heading tree the exporter walked out of the document: id, name (which
// is html, since a headline may hold code or a link), children and the org
// level. Everything on this page - the contents rail, the search index, the
// scroll spy, the column on the right - is built from it plus the document.
var ORG_NODES =
    {%autoescape off%}
    {{nodes_json}}
    {%endautoescape%}
    ;
</script>

<script>
{% include "html_docs_app.js" %}
</script>

<style>
{%autoescape off%}
{{stylesheet}}
{%endautoescape%}
</style>
</head>
<body{%if havebodyattr%} class="{{bodyattr}}"{%endif%}>

  <!-- Narrow screens only: the one thing that can open the rail once it has
       slid off the side. [data-sticky-header] is how the shared script asks
       how much of the top of the window is taken; not displayed on a wide
       screen, so it measures zero there. -->
  <header class="fu-mobile" data-sticky-header>
    <button id="rail-toggle" class="fu-icon" type="button" aria-label="Show the contents">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 7h16M4 12h16M4 17h16"></path></svg>
    </button>
    <span class="fu-mobile-title">{{title}}</span>
    <button id="theme-toggle-m" class="fu-icon fu-theme" type="button" aria-label="Light or dark">
      <svg class="when-light" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="4"></circle><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"></path></svg>
      <svg class="when-dark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"></path></svg>
    </button>
  </header>

  <div class="fu-page">

    <nav id="sidebar" class="fu-side" aria-label="Contents">
      <div class="fu-brand">
        <a class="fu-brand-name" href="#">{{title}}</a>
{%if subtitle%}
        <div class="fu-brand-sub">{{subtitle}}</div>
{%endif%}
      </div>

      <div class="search">
        <form id="searchform" class="search-field" role="search" autocomplete="off">
          <span class="icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7"></circle><path d="M20 20l-3.6-3.6"></path></svg></span>
          <input id="searchfield" name="searchfield" type="search" placeholder="Search" aria-label="Search the documentation" autocomplete="off" spellcheck="false">
          <span class="search-hint">/</span>
        </form>
        <div id="searchresults" class="search-results" data-open="0" role="listbox" aria-label="Search results"></div>
      </div>

      <div class="fu-side-nav">
        <p class="fu-caption">
          <span>Contents</span>
          <span class="fu-caption-tools">
            <button id="rail-expand" type="button">expand</button>
            <button id="rail-collapse" type="button">collapse</button>
          </span>
        </p>
        <ul id="navbar" class="treeviewul"></ul>
      </div>
    </nav>

    <main class="fu-main">
      <div class="fu-tools">
        <button id="theme-toggle" class="fu-icon fu-theme" type="button" aria-label="Light or dark">
          <svg class="when-light" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="4"></circle><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"></path></svg>
          <svg class="when-dark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"></path></svg>
        </button>
      </div>

      <article class="fu-article">
        <div class="doc-body">
      {%autoescape off%}
      {{html_data}}
      {%endautoescape%}
        </div>
      </article>

      <footer class="fu-footer">
        <span>{{title}}</span>
        <a class="fu-back" href="#">Back to top</a>
      </footer>
    </main>

    <aside class="fu-toc" aria-label="On this page">
      <div class="fu-toc-inner">
        <p class="fu-toc-label">On this page</p>
        <ul id="fu-toc-list" class="fu-toc-list"></ul>
      </div>
    </aside>

  </div>

  <script>
    // The right hand column.
    //
    // Furo's is the headings of the page you are on. A single org file has
    // no pages, and its nearest equivalent is the top level section - which
    // is also what the left rail is a list of, so the two columns answer
    // "which section" and "where in it" rather than both answering the same
    // question. It is redrawn only when the section changes, never on the
    // scroll itself: rebuilding a list under somebody's pointer on every
    // frame is both the expensive way and the one that makes a link
    // impossible to click.
    (function () {
      var list = document.getElementById('fu-toc-list');
      var panel = document.querySelector('.fu-toc');
      var shownRoot = undefined, rows = [], marked = null;

      function rootOf(e) { var r = e; while (r && r.parent) r = r.parent; return r; }

      function draw(root) {
        list.innerHTML = '';
        rows = [];
        if (!root) { panel.setAttribute('data-empty', '1'); return; }
        // The section's own heading first, then everything under it. A
        // column that began at the subheadings would have no line for the
        // thing you are actually reading when you are at the top of it.
        add(root, 0);
        walk(root.children, 1);
        panel.removeAttribute('data-empty');
      }

      function walk(kids, depth) {
        for (var i = 0; i < kids.length; i++) { add(kids[i], depth); walk(kids[i].children, depth + 1); }
      }

      function add(e, depth) {
        var li = document.createElement('li');
        li.className = 'fu-toc-item depth-' + Math.min(depth, 4);
        var a = document.createElement('a');
        a.href = '#' + e.slug;
        a.innerHTML = e.nameHtml;
        a.addEventListener('click', (function (entry) {
          return function (ev) { ev.preventDefault(); window.orgDocs.goTo(entry, null, true); };
        })(e));
        li.appendChild(a);
        list.appendChild(li);
        rows.push({ e: e, li: li });
      }

      function mark(e) {
        if (marked) marked.classList.remove('is-current');
        marked = null;
        for (var i = 0; i < rows.length; i++) {
          if (rows[i].e === e) { rows[i].li.classList.add('is-current'); marked = rows[i].li; break; }
        }
        if (marked) reveal(marked);
      }

      // Inside the column only. scrollIntoView scrolls every scrollable
      // ancestor, the document included - and the document moving because
      // a list beside it marked a row would be the page chasing its own
      // scroll spy, which never settles.
      function reveal(li) {
        var r = li.getBoundingClientRect(), b = panel.getBoundingClientRect();
        if (r.top < b.top + 8) panel.scrollTop -= (b.top + 8 - r.top);
        else if (r.bottom > b.bottom - 8) panel.scrollTop += (r.bottom - b.bottom + 8);
      }

      document.addEventListener('docs:current', function (ev) {
        var e = ev.detail;
        var root = e ? rootOf(e) : null;
        if (root !== shownRoot) { shownRoot = root; draw(root); }
        mark(e);
      });

      // Nothing has happened yet on a page nobody has scrolled, and the
      // column would be empty until the first scroll event. The shared
      // script settles on a heading during its own boot, so one tick later
      // there is an answer to ask for.
      window.setTimeout(function () {
        if (shownRoot === undefined && window.orgDocs && window.orgDocs.entries.length) {
          shownRoot = window.orgDocs.entries[0];
          draw(shownRoot);
        }
      }, 200);
    })();

    // The narrow screen's own theme button is the same button. It is a
    // second element rather than one moved about, because moving it would
    // mean the shared script's listener following it; this just forwards.
    (function () {
      var m = document.getElementById('theme-toggle-m'), d = document.getElementById('theme-toggle');
      if (m && d) m.addEventListener('click', function () { d.click(); });
    })();
  </script>

  <script type="module">
    import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
    const dark = document.documentElement.getAttribute('data-theme') === 'dark' ||
      (!document.documentElement.getAttribute('data-theme') &&
       window.matchMedia('(prefers-color-scheme: dark)').matches);
    mermaid.initialize({ startOnLoad: true, maxTextSize: 2000000, theme: dark ? 'dark' : 'default' });
  </script>
  {%autoescape off%}
  {{post_scripts}}
  {%endautoescape%}
</body>
</html>
