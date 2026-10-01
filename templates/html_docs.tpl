<!-- Documentation template for the html exporter (HTML_THEME: docs) -->
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{title}}</title>
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{fontfamily}}&display=swap">
{%if hljs_style_default%}
  <!-- Two highlight themes, one of which is switched off: a dark palette on
       a light page is the one thing that gives away a page that has grown a
       light mode as an afterthought. A file naming its own
       #+HTML_HIGHLIGHT_STYLE gets that one in both. -->
  <link id="hljs-light" rel="stylesheet" href="{{hljs_cdn}}/styles/github.min.css">
  <link id="hljs-dark" rel="stylesheet" href="{{hljs_cdn}}/styles/github-dark.min.css">
{%else%}
  <link rel="stylesheet" href="{{hljs_cdn}}/styles/{{hljs_style}}.min.css">
{%endif%}

  <script>
    // Before anything paints: the page the reader chose last time, and the
    // matching highlight sheet. Left to the main script this is a flash of
    // the wrong theme on every load.
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

{%if wordcloud%}
<script src="https://cdnjs.cloudflare.com/ajax/libs/d3/7.8.5/d3.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/d3-cloud/1.2.7/d3.layout.cloud.min.js"></script>
<script>
function wordcloud(name, words) {
    function draw(words) {
        d3.select(name)
        .attr("width", layout.size()[0])
        .attr("height", layout.size()[1])
      .append("g")
        .attr("transform", "translate(" + layout.size()[0] / 2 + "," + layout.size()[1] / 2 + ")")
      .selectAll("text")
        .data(words)
      .enter().append("text")
        .style("font-size", function(d) { return d.size + "px"; })
        .style("fill", function(d){return "hsl(" + Math.random() * 360 + ",72%,70%)"; })
        .style("font-family", "Impact")
        .attr("text-anchor", "middle")
        .attr("transform", function(d) {
          return "translate(" + [d.x, d.y] + ")rotate(" + d.rotate + ")";
        })
        .text(function(d) { return d.text; });
    }

    var layout = d3.layout.cloud()
        .size([800, 600])
        .words( words.map(function(d) {
            return {text: d, size: 20 + Math.random() * 70};
    }))
    .padding(5)
    .rotate(function() { return ~~(Math.random() * 1.5) * 90; })
    .font("Impact")
    .fontSize(function(d) { return d.size; })
    .on("end", draw);

    layout.start();
}
</script>
{%endif%}

<script>
// The heading tree the exporter walked out of the document: id, name (which
// may carry markup, since a headline can hold code and links), parent and
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
    <div class="doc-wrapper">
      <div class="header-wrapper">
        <div id="header" class="header">
          <div class="header-container" data-sticky-header>
            <div class="header-left"><h1 class="brand">Docs</h1></div>
            <button id="rail-toggle" class="icon-btn rail-toggle" type="button" aria-label="Show the contents">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 7h16M4 12h16M4 17h16"></path></svg>
            </button>
            <div class="header-title">{{title}}</div>
            <div class="header-right">
              <div class="search">
                <form id="searchform" class="search-field" role="search" autocomplete="off">
                  <span class="icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7"></circle><path d="M20 20l-3.6-3.6"></path></svg></span>
                  <input id="searchfield" name="searchfield" type="search" placeholder="Search the documentation" aria-label="Search the documentation" autocomplete="off" spellcheck="false">
                  <span class="search-hint">/</span>
                </form>
                <div id="searchresults" class="search-results" data-open="0" role="listbox" aria-label="Search results"></div>
              </div>
              <button id="theme-toggle" class="icon-btn" type="button" aria-label="Light or dark">
                <svg class="when-light" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="4"></circle><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"></path></svg>
                <svg class="when-dark" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"></path></svg>
              </button>
            </div>
          </div>
        </div>
      </div>

      <div id="master-wrapper" class="master-wrapper clear">
        <nav id="sidebar" class="sidebar" aria-label="Contents">
          <div class="rail-head">
            <h2 class="rail-label">Contents</h2>
            <span>
              <button id="rail-expand" type="button">expand</button>
              &middot;
              <button id="rail-collapse" type="button">collapse</button>
            </span>
          </div>
          <ul id="navbar" class="treeviewul"></ul>
        </nav>

        <main class="display-box">
          <div class="doc-body">
        {%autoescape off%}
        {{html_data}}
        {%endautoescape%}
          </div>
        </main>
      </div>
    </div>

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
