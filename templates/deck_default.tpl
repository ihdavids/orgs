<!DOCTYPE html>
<html lang="en" class="no-js">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
  <title>{{title}}</title>

  <!-- deck.js core, then the extensions, then the themes: the order is the
       order deck.js' own introduction uses, and the themes have to come last
       or the core's defaults win. -->
  <link rel="stylesheet" href="{{deck_cdn}}/core/deck.core.css">
  {%if goto%}<link rel="stylesheet" href="{{deck_cdn}}/extensions/goto/deck.goto.css">{%endif%}
  {%if menu%}<link rel="stylesheet" href="{{deck_cdn}}/extensions/menu/deck.menu.css">{%endif%}
  {%if navigation%}<link rel="stylesheet" href="{{deck_cdn}}/extensions/navigation/deck.navigation.css">{%endif%}
  {%if status%}<link rel="stylesheet" href="{{deck_cdn}}/extensions/status/deck.status.css">{%endif%}
  {%if scale%}<link rel="stylesheet" href="{{deck_cdn}}/extensions/scale/deck.scale.css">{%endif%}
{%if not local_theme%}  <link rel="stylesheet" href="{{deck_cdn}}/themes/style/{{theme}}.css">{%endif%}
  <link rel="stylesheet" href="{{deck_cdn}}/themes/transition/{{transition}}.css">
  <link rel="stylesheet" media="print" href="{{deck_cdn}}/core/print.css">
  <link rel="stylesheet" href="{{hljs_cdn}}/styles/{{hljs_style}}.min.css">
  {%if fontfamily%}<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{fontfamily}}&display=swap">{%endif%}
  {%if extra_css%}<link rel="stylesheet" href="{{extra_css}}">{%endif%}
  <style>
  {%autoescape off%}
  {{stylesheet}}
  {%endautoescape%}
  {%if fontfamily%}
  .deck-container { font-family: "{{fontfamily}}", "Helvetica Neue", Helvetica, Arial, sans-serif; }
  {%endif%}
  </style>
  {%if local_theme%}
  <!-- The theme: the four-framework mapping, then the theme itself. The
       transition theme above is left alone - how a slide moves is not what it
       looks like. -->
  <style>
  {%autoescape off%}
  {{themedata}}
  {%endautoescape%}
  </style>
  {%endif%}
  {%autoescape off%}
  {{head}}
  {%endautoescape%}

  <!-- deck.core.css asks Modernizr whether it may animate. Without it every
       .csstransforms rule is dead and the transition themes do nothing. -->
  <script src="{{modernizr}}"></script>
</head>
<body class="deck-body">
<div class="deck-container" data-fit-box>

  {%autoescape off%}
  {{slide_data}}
  {%endautoescape%}

  {%if navigation%}
  <!-- deck.navigation -->
  <a href="#" class="deck-prev-link" title="Previous">&#8592;</a>
  <a href="#" class="deck-next-link" title="Next">&#8594;</a>
  {%endif%}

  {%if status%}
  <!-- deck.status -->
  <p class="deck-status" aria-role="status">
    <span class="deck-status-current"></span> / <span class="deck-status-total"></span>
  </p>
  {%endif%}

  {%if goto%}
  <!-- deck.goto -->
  <form action="." method="get" class="goto-form">
    <label for="goto-slide">Go to slide:</label>
    <input type="text" name="slidenum" id="goto-slide" list="goto-datalist">
    <datalist id="goto-datalist"></datalist>
    <input type="submit" value="Go">
  </form>
  {%endif%}

  {%if permalink%}
  <a href="." title="Permalink to this slide" class="deck-permalink">#</a>
  {%endif%}

</div>

{%if notes%}
<!-- The speaker's own crib sheet: a :NOTES: drawer on a slide, shown on `n`.
     deck.js has no notes view, so this is ours, and it is the same key
     reveal.js uses for the same thing. -->
<div id="deck-notes-panel" aria-live="polite"></div>
{%endif%}

<script src="{{jquery}}"></script>
<script src="{{deck_cdn}}/core/deck.core.js"></script>
{%if menu%}<script src="{{deck_cdn}}/extensions/menu/deck.menu.js"></script>{%endif%}
{%if goto%}<script src="{{deck_cdn}}/extensions/goto/deck.goto.js"></script>{%endif%}
{%if status%}<script src="{{deck_cdn}}/extensions/status/deck.status.js"></script>{%endif%}
{%if navigation%}<script src="{{deck_cdn}}/extensions/navigation/deck.navigation.js"></script>{%endif%}
{%if scale%}<script src="{{deck_cdn}}/extensions/scale/deck.scale.js"></script>{%endif%}
<script src="{{hljs_cdn}}/highlight.min.js"></script>
<script>
jQuery(function ($) {
  $.deck('.slide', {%autoescape off%}{{options}}{%endautoescape%});
  if (window.hljs) { document.querySelectorAll('pre code').forEach(function (b) { hljs.highlightElement(b); }); }
});
</script>
{%if notes%}
<script>
jQuery(function ($) {
  var panel = document.getElementById('deck-notes-panel');
  if (!panel) { return; }
  var show = function () {
    var cur = document.querySelector('.deck-container .deck-current');
    var notes = cur ? cur.querySelector(':scope > .deck-notes') : null;
    panel.innerHTML = notes ? notes.innerHTML : '<p class="deck-notes-empty">No notes on this slide.</p>';
  };
  $(document).on('deck.change', show);
  document.addEventListener('keyup', function (e) {
    // Not while the go-to box is being typed into.
    var t = e.target || {};
    if (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable) { return; }
    if (e.key === 'n' || e.key === 'N') { panel.classList.toggle('open'); show(); }
    if (e.key === 'Escape') { panel.classList.remove('open'); }
  });
  show();
});
</script>
{%endif%}
{%if mermaid%}
<script type="module">
  import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.esm.min.mjs';
  mermaid.initialize({ startOnLoad: true, maxTextSize: 2000000{%if mermaid_theme%}, theme: '{{mermaid_theme}}'{%endif%} });
</script>
{%endif%}
{%if wordcloud%}
<script src="https://cdnjs.cloudflare.com/ajax/libs/d3/7.8.5/d3.min.js"></script>
<script src="https://cdnjs.cloudflare.com/ajax/libs/d3-cloud/1.2.7/d3.layout.cloud.min.js"></script>
<script>
function wordcloud(name, words) {
  var layout = d3.layout.cloud()
    .size([800, 600])
    .words(words.map(function (d) { return { text: d, size: 20 + Math.random() * 70 }; }))
    .padding(5)
    .rotate(function () { return ~~(Math.random() * 1.5) * 90; })
    .font("Impact")
    .fontSize(function (d) { return d.size; })
    .on("end", function (words) {
      d3.select(name)
        .attr("width", layout.size()[0]).attr("height", layout.size()[1])
        .append("g")
        .attr("transform", "translate(" + layout.size()[0] / 2 + "," + layout.size()[1] / 2 + ")")
        .selectAll("text").data(words).enter().append("text")
        .style("font-size", function (d) { return d.size + "px"; })
        .style("fill", function () { return "hsl(" + Math.random() * 360 + ",72%,70%)"; })
        .style("font-family", "Impact")
        .attr("text-anchor", "middle")
        .attr("transform", function (d) { return "translate(" + [d.x, d.y] + ")rotate(" + d.rotate + ")"; })
        .text(function (d) { return d.text; });
    });
  layout.start();
}
</script>
{%endif%}
{%if backdrop%}
<style>
{%autoescape off%}
{{backdrop_css}}
{%endautoescape%}
</style>
<script>
{%autoescape off%}
window.ORG_BACKDROP = {{backdrop_config}};
{{backdrop_js}}
{%endautoescape%}
</script>
{%endif%}
{%if autofit%}
<style>
{%autoescape off%}
{{autofit_css}}
{%endautoescape%}
</style>
<script>
{%autoescape off%}
{{autofit_js}}
{%endautoescape%}
</script>
{%endif%}
</body>
</html>
