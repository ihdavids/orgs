<!-- Read-the-docs template for the html exporter (HTML_THEME: rtd) -->
<!--
     The same application as the docs theme - the contents rail, the fuzzy
     search with its preview, the folding, the anchors, the code copy - in
     the dress sphinx_rtd_theme wears: a dark rail the full height of the
     window with a blue search block at the top of it, breadcrumbs over the
     page, and the document in a column on white.

     The script is html_docs_app.js, included by both themes rather than
     said twice. It asks the page for ids rather than for a layout, so a
     theme is a stylesheet and a shell: #sidebar, #navbar, #searchfield,
     #searchresults, #searchform, #rail-expand, #rail-collapse,
     #rail-toggle, #theme-toggle, a .doc-body around the document, and
     [data-sticky-header] on whatever is standing over the top of the page.
-->
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{title}}</title>
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <!-- Lato for the prose and Roboto Slab for the headings is what this theme
       is read in, and swapping them for a system stack loses most of what
       somebody means when they ask for it. A file naming its own
       #+HTML_FONTFAMILY still wins: it is first in the display stack. -->
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Lato:ital,wght@0,400;0,700;1,400&family=Roboto+Slab:wght@400;700&family={{fontfamily}}&display=swap">
{%if hljs_style_default%}
  <link id="hljs-light" rel="stylesheet" href="{{hljs_cdn}}/styles/github.min.css">
  <link id="hljs-dark" rel="stylesheet" href="{{hljs_cdn}}/styles/github-dark.min.css">
{%else%}
  <link rel="stylesheet" href="{{hljs_cdn}}/styles/{{hljs_style}}.min.css">
{%endif%}

  <script>
    // Before anything paints: the page the reader chose last time, and the
    // matching highlight sheet. Left to the main script this is a flash of
    // the wrong theme on every load. The key is the docs theme's, on
    // purpose - the two are the same documentation read two ways, and
    // choosing dark in one and finding light in the other reads as a bug.
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
// scroll spy - is built from it plus the document itself.
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

  <!-- The bar across the top exists only on a narrow screen, where the rail
       has slid off the side and there would otherwise be nothing to open it
       with. It carries [data-sticky-header] so the script knows how much of
       the window something else is standing in; on a wide screen it is not
       displayed and measures zero, which is the right answer. -->
  <div class="rtd-top" data-sticky-header>
    <button id="rail-toggle" class="rtd-top-menu" type="button" aria-label="Show the contents">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 7h16M4 12h16M4 17h16"></path></svg>
    </button>
    <span class="rtd-top-title">{{title}}</span>
  </div>

  <nav id="sidebar" class="rtd-side" aria-label="Contents">
    <div class="rtd-side-search">
      <a class="rtd-brand" href="#">{{title}}</a>
{%if subtitle%}
      <div class="rtd-brand-sub">{{subtitle}}</div>
{%endif%}
      <div class="search">
        <form id="searchform" class="search-field" role="search" autocomplete="off">
          <input id="searchfield" name="searchfield" type="search" placeholder="Search docs" aria-label="Search the documentation" autocomplete="off" spellcheck="false">
        </form>
        <div id="searchresults" class="search-results" data-open="0" role="listbox" aria-label="Search results"></div>
      </div>
    </div>

    <div class="rtd-side-nav">
      <p class="rtd-caption">
        <span class="rtd-caption-text">Contents</span>
        <span class="rtd-caption-tools">
          <button id="rail-expand" type="button">expand</button>
          <button id="rail-collapse" type="button">collapse</button>
        </span>
      </p>
      <ul id="navbar" class="treeviewul"></ul>
    </div>
  </nav>

  <section class="rtd-main">
    <div class="rtd-breadcrumbs">
      <ul class="rtd-crumbs">
        <li><a href="#">Docs</a></li>
        <li class="rtd-crumb-sep" aria-hidden="true">&raquo;</li>
        <li id="rtd-crumb-here" class="rtd-crumb-here">{{title}}</li>
      </ul>
      <button id="theme-toggle" class="icon-btn" type="button" aria-label="Light or dark">
        <svg class="when-light" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="4"></circle><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"></path></svg>
        <svg class="when-dark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"></path></svg>
      </button>
    </div>

    <div class="rtd-content">
      <div class="doc-body">
    {%autoescape off%}
    {{html_data}}
    {%endautoescape%}
      </div>

      <footer class="rtd-footer">
        <hr>
        <p class="rtd-footer-line">
          <span>{{title}}</span>
          <a class="rtd-top-link" href="#">Back to top</a>
        </p>
      </footer>
    </div>
  </section>

  <script>
    // The breadcrumb's last crumb is where the reader is, which the shared
    // script already works out for the rail. It says so on a docs:current
    // event rather than this page watching the scroll a second time - two
    // answers to "which heading am I in" would disagree at the edges, and
    // the edges are exactly where a breadcrumb is read.
    (function () {
      var here = document.getElementById('rtd-crumb-here');
      var title = here ? here.textContent : '';
      document.addEventListener('docs:current', function (ev) {
        if (!here) return;
        var e = ev.detail;
        here.innerHTML = e ? e.nameHtml : title;
      });
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
