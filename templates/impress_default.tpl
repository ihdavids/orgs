<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width={{width}}">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <title>{{title}}</title>
  {%if fontfamily%}<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family={{fontfamily}}&display=swap">{%endif%}
  <link rel="stylesheet" href="{{impress_cdn}}/css/impress-common.css">
  <link rel="stylesheet" href="{{hljs_cdn}}/styles/{{hljs_style}}.min.css">
  {%if extra_css%}<link rel="stylesheet" href="{{extra_css}}">{%endif%}
  <style>
  {%autoescape off%}
  {{stylesheet}}
  {%endautoescape%}
  </style>
  {%if autofit%}
  <!-- A step sizes itself to its content, so there is nothing for the fitter
       to fit *into* until one is given a height. The deck's own data-height is
       that height: it is what impress.js scales the canvas by, so a step the
       size of it is a step the size of the screen. -->
  <style>
  #impress .step { height: {{height}}px; overflow: hidden; }
  </style>
  {%endif%}
  <!-- The theme: one of the shared four-framework ones (the mapping and then
       the theme), or impress's own impress_theme_<name>.css. Either way it
       comes after the house stylesheet, because a theme is allowed to
       overrule it. -->
  <style>
  {%autoescape off%}
  {{themedata}}
  {%endautoescape%}
  </style>
  {%autoescape off%}
  {{head}}
  {%endautoescape%}
</head>
<body class="impress-not-supported"
    data-transition-duration="{{transition_duration}}"
    data-width="{{width}}"
    data-height="{{height}}"
    data-max-scale="{{max_scale}}"
    data-min-scale="{{min_scale}}"
    data-perspective="{{perspective}}"
    {%if autoplay%}data-autoplay="{{autoplay}}"{%endif%}
>
<div class="fallback-message">
  <p>Your browser <b>doesn't support the features required</b> by impress.js, so you are presented with a simplified version of this presentation.</p>
  <p>For the best experience please use the latest <b>Chrome</b>, <b>Safari</b> or <b>Firefox</b> browser.</p>
</div>

<div id="impress">
  {%autoescape off%}
  {{slide_data}}
  {%endautoescape%}
</div>

{%if toolbar%}<div id="impress-toolbar"></div>{%endif%}
{%if progress%}
<div class="impress-progressbar"><div></div></div>
<div class="impress-progress"></div>
{%endif%}
{%if hint%}
<div class="hint"><p>Use a spacebar or arrow keys to navigate. Press <b>N</b> for notes.</p></div>
{%endif%}
{%if notes%}
<div id="impress-notes-panel" aria-live="polite"></div>
{%endif%}

<script>
if ("ontouchstart" in document.documentElement) {
  var hint = document.querySelector(".hint");
  if (hint) { hint.innerHTML = "<p>Swipe left or right to navigate</p>"; }
}
</script>
<script src="{{hljs_cdn}}/highlight.min.js"></script>
<script src="{{impress_cdn}}/js/impress.js"></script>
<script>
  impress().init();
  if (window.hljs) { document.querySelectorAll('pre code').forEach(function (b) { hljs.highlightElement(b); }); }
</script>
{%if notes%}
<script>
(function () {
  var panel = document.getElementById('impress-notes-panel');
  if (!panel) { return; }
  var show = function () {
    var cur = document.querySelector('#impress .step.active');
    var notes = cur ? cur.querySelector('.notes') : null;
    panel.innerHTML = notes ? notes.innerHTML : '<p class="impress-notes-empty">No notes on this step.</p>';
  };
  document.addEventListener('impress:stepenter', show);
  document.addEventListener('keyup', function (e) {
    var t = e.target || {};
    if (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable) { return; }
    if (e.key === 'n' || e.key === 'N') { panel.classList.toggle('open'); show(); }
    if (e.key === 'Escape') { panel.classList.remove('open'); }
  });
  show();
})();
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
